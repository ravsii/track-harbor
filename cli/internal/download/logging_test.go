package download

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readLog(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var records []map[string]any
	s := bufio.NewScanner(f)
	for s.Scan() {
		var record map[string]any
		if err := json.Unmarshal(s.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	return records
}

func TestZapFileLoggingDistinguishesEmptySearchAndHTTPFailure(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		message string
		code    int
	}{
		{"empty", ErrNotFound, "not_found", 0},
		{"HTTP404", &HTTPError{Stage: "search", StatusCode: 404}, "failed", 404},
		{"HTTP500", &HTTPError{Stage: "search", StatusCode: 500}, "failed", 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "logs", "download.jsonl")
			logger, closeLog, err := NewFileLogger(path)
			if err != nil {
				t.Fatal(err)
			}
			fastDownloader(&testProvider{searchErr: tc.err}, 10, t.TempDir()).WithLogger(logger).Run([]string{"Original query"})
			if err := closeLog(); err != nil {
				t.Fatal(err)
			}
			for _, record := range readLog(t, path) {
				if record["msg"] == tc.message {
					if record["job_id"] != "1" || record["query"] != "Original query" || record["stage"] != "search" || record["ts"] == nil || record["run_id"] == nil {
						t.Fatalf("diagnostic fields missing: %v", record)
					}
					if tc.code > 0 && record["http_status"] != float64(tc.code) {
						t.Fatalf("HTTP code missing: %v", record)
					}
					return
				}
			}
			t.Fatal("terminal result missing from file")
		})
	}
}

func TestZapFileLoggingIsParallelAndAppendsRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "download.jsonl")
	logger, closeLog, err := NewFileLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	var queries []string
	for i := 0; i < 10; i++ {
		queries = append(queries, string(rune('a'+i)))
	}
	fastDownloader(&cacheProvider{body: "abcdef", length: 6, distinct: true}, 10, t.TempDir()).WithLogger(logger).Run(queries)
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}
	first := readLog(t, path)
	completed := 0
	for _, record := range first {
		if record["msg"] == "downloaded" {
			completed++
			if record["bytes"] != float64(6) || len(record["sha256"].(string)) != 64 || record["file_path"] == nil {
				t.Fatalf("file details missing: %v", record)
			}
		}
	}
	if completed != 10 {
		t.Fatalf("lost parallel log entries: %d", completed)
	}
	logger, closeLog, err = NewFileLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	New(&testProvider{searchErr: ErrNotFound}, 1, t.TempDir()).WithLogger(logger).Run([]string{"Next query"})
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}
	all := readLog(t, path)
	if len(all) <= len(first) || all[0]["run_id"] == all[len(all)-1]["run_id"] {
		t.Fatal("log was overwritten or run identity reused")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("log file is not private")
	}
}

func TestZapFileLoggingRedactsMalformedRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://host/%zz?token=secret-location")
		w.WriteHeader(302)
	}))
	defer server.Close()
	_, requestErr := http.Get(server.URL)
	if requestErr == nil {
		t.Fatal("malformed redirect did not fail")
	}
	path := filepath.Join(t.TempDir(), "download.jsonl")
	logger, closeLog, err := NewFileLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	fastDownloader(&testProvider{searchErr: requestErr}, 1, t.TempDir()).WithLogger(logger).Run([]string{"Track"})
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-location") {
		t.Fatal("redirect secret leaked into log")
	}
}

func TestZapFileLoggingRecordsPartialDownload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "download.jsonl")
	logger, closeLog, err := NewFileLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	fastDownloader(&cacheProvider{body: "partial", length: 100}, 1, t.TempDir()).WithLogger(logger).Run([]string{"Track"})
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}
	for _, record := range readLog(t, path) {
		if record["msg"] == "failed" && record["stage"] == "downloading" && record["bytes"] == float64(7) && record["expected_bytes"] == float64(100) {
			return
		}
	}
	t.Fatal("partial download lost byte counts and failure stage")
}

func TestZapLogsFiveRetriesAndOneFinalFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "download.jsonl")
	logger, closeLog, err := NewFileLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	fastDownloader(&testProvider{searchErr: &HTTPError{Stage: "search", StatusCode: 500}}, 1, t.TempDir()).WithLogger(logger).Run([]string{"Track"})
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}
	retries, failures := 0, 0
	for _, record := range readLog(t, path) {
		if record["msg"] == "retry_scheduled" {
			retries++
			if record["retry"] != float64(retries) || record["attempt"] != float64(retries) || record["http_status"] != float64(500) || record["delay"] == nil {
				t.Fatalf("missing retry fields: %v", record)
			}
		}
		if record["msg"] == "failed" {
			failures++
			if record["attempt"] != float64(6) {
				t.Fatalf("final attempt wrong: %v", record)
			}
		}
	}
	if retries != 5 || failures != 1 {
		t.Fatalf("retries=%d final failures=%d", retries, failures)
	}
}
