package cli

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

type syncTransport func(*http.Request) (*http.Response, error)

func (f syncTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const syncFixture = `{"data":{"playlistV2":{"name":"My playlist","content":{"totalCount":2,"items":[{"itemV3":{"data":{"identityTrait":{"name":"First song","contributors":{"items":[{"name":"Artist A"},{"name":"Artist B"},{"name":""}]}}}}},{"itemV3":{"data":{"identityTrait":{"name":"Second song","contributors":{"items":[]}}}}}]}}}}`

// Exercise the real command and SQLite; replace only the external HTTP request.
func runSyncTest(t *testing.T, headers string, transport http.RoundTripper, prepare ...func(*sql.DB)) (string, error) {
	t.Helper()
	t.Chdir(t.TempDir())
	database, err := sql.Open("sqlite3", "meshtune.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	_, err = database.Exec(`CREATE TABLE songs (id INTEGER PRIMARY KEY, artists TEXT NOT NULL, title TEXT NOT NULL, created_at DATETIME NOT NULL, UNIQUE (artists, title))`)
	if err != nil {
		t.Fatal(err)
	}
	for _, setup := range prepare {
		setup(database)
	}
	return executeSyncTest(t, strings.NewReader(headers), transport)
}

func executeSyncTest(t *testing.T, input io.Reader, transport http.RoundTripper) (string, error) {
	t.Helper()
	var err error
	oldClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: transport}
	defer func() { http.DefaultClient = oldClient }()

	var output bytes.Buffer
	syncCmd.SetIn(input)
	syncCmd.SetOut(&output)
	defer func() { syncCmd.SetIn(nil); syncCmd.SetOut(nil) }()
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("panic instead of error: %v", recovered)
			}
		}()
		err = syncCmd.RunE(syncCmd, nil)
	}()
	return output.String(), err
}

func TestParseHeadersCompletesOnBlankLineWithoutEOF(t *testing.T) {
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()
	type result struct {
		headers map[string]string
		err     error
	}
	finished := make(chan result, 1)
	go func() {
		headers, _, err := parseHeaders(r)
		finished <- result{headers, err}
	}()
	if _, err := io.WriteString(w, "authorization: Bearer test\nclient-token: test-client\n\n"); err != nil {
		t.Fatal(err)
	}
	// Keep the writer open: pressing Enter on an empty line must suffice.
	select {
	case got := <-finished:
		if got.err != nil || got.headers["authorization"] != "Bearer test" || got.headers["client-token"] != "test-client" {
			t.Fatalf("unexpected parsed input: %+v", got)
		}
	case <-time.After(time.Second):
		w.Close()
		<-finished
		t.Fatal("parser kept waiting for EOF after a blank line")
	}
}

func TestParseHeadersRejectsCookieStringContainingURL(t *testing.T) {
	// Synthetic cookies: the ':' in https:// must not make this a header.
	headers, skipped, err := parseHeaders(strings.NewReader("sp_dc=fake-session;sp_landing=https%3A%2F%2Fexample.com;sp_landingref=https://example.com/;\n"))
	if err != nil || len(headers) != 0 || skipped != 1 {
		t.Fatalf("cookie string mistaken for a header: headers=%v skipped=%d err=%v", headers, skipped, err)
	}
}

func TestSyncExplainsHeaderFormatForCookieInput(t *testing.T) {
	output, err := runSyncTest(t, "cookie: sp_dc=fake-session\n\n", syncTransport(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("must not request Spotify using cookies")
	}))
	if err == nil || !strings.Contains(err.Error(), "Cookie") || !strings.Contains(err.Error(), "authorization") || !strings.Contains(err.Error(), "client-token") {
		t.Fatalf("expected explanation that cookies are not tokens, got %v", err)
	}
	for _, want := range []string{"blank line", "authorization: Bearer", "client-token:"} {
		if !strings.Contains(output, want) {
			t.Errorf("input instructions missing %q: %s", want, output)
		}
	}
	if strings.Contains(output, "fake-session") || strings.Contains(err.Error(), "fake-session") {
		t.Fatal("cookie value leaked in diagnostics")
	}
}

func TestSyncMissingHeadersReportsInputStage(t *testing.T) {
	output, err := runSyncTest(t, "accept: application/json\n", syncTransport(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("should not request Spotify without tokens")
	}))
	if err == nil || !strings.Contains(err.Error(), "[1/4]") || !strings.Contains(err.Error(), "authorization") {
		t.Fatalf("expected missing-header error at input stage, got %v", err)
	}
	if strings.Contains(output, "[2/4]") || strings.Contains(output, "[4/4]") {
		t.Fatalf("command continued after invalid input: %s", output)
	}
}

func TestSyncNetworkFailureReportsRequestStage(t *testing.T) {
	output, err := runSyncTest(t, "authorization: Bearer secret-auth\nclient-token: secret-client\n", syncTransport(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("offline")
	}))
	if err == nil || !strings.Contains(err.Error(), "[2/4]") || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("expected contextual network error, got %v", err)
	}
	if strings.Contains(output, "[3/4]") || strings.Contains(output, "[4/4]") {
		t.Fatalf("command continued after network failure: %s", output)
	}
}

func TestSyncHTTPFailureReportsStatus(t *testing.T) {
	output, err := runSyncTest(t, "authorization: Bearer secret-auth\nclient-token: secret-client\n", syncTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader(`{"error":"secret-server-body"}`))}, nil
	}))
	if err == nil || !strings.Contains(err.Error(), "[2/4]") || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected HTTP status in request error, got %v", err)
	}
	if strings.Contains(output, "[3/4]") || strings.Contains(output, "secret-server-body") || strings.Contains(err.Error(), "secret-server-body") {
		t.Fatalf("unsafe HTTP failure output: %s; %v", output, err)
	}
}

func TestSyncDatabaseFailureReportsSaveStage(t *testing.T) {
	response := fixtureTransport(t)
	output, err := runSyncTest(t, "authorization: Bearer secret-auth\nclient-token: secret-client\n", syncTransport(func(req *http.Request) (*http.Response, error) {
		database, err := sql.Open("sqlite3", "meshtune.db")
		if err != nil {
			t.Fatal(err)
		}
		defer database.Close()
		if _, err := database.Exec(`INSERT INTO songs (artists, title, created_at) VALUES ('["Old artist"]', 'Old song', CURRENT_TIMESTAMP)`); err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`CREATE TRIGGER reject_song BEFORE INSERT ON songs BEGIN SELECT RAISE(ABORT, 'write failed'); END;`); err != nil {
			t.Fatal(err)
		}
		return response.RoundTrip(req)
	}))
	if err == nil || !strings.Contains(err.Error(), "[3/4]") || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("expected contextual database error, got %v", err)
	}
	if strings.Contains(output, "[4/4]") {
		t.Fatalf("failed save reported success: %s", output)
	}
	database, err := sql.Open("sqlite3", "meshtune.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM songs WHERE title = 'Old song'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("failed sync erased the previous library: count=%d err=%v", count, err)
	}
}

type failingHeaderReader struct{}

func (failingHeaderReader) Read([]byte) (int, error) { return 0, errors.New("input failed") }

func TestSyncInputFailureReportsReadStage(t *testing.T) {
	t.Chdir(t.TempDir())
	var output bytes.Buffer
	syncCmd.SetIn(failingHeaderReader{})
	syncCmd.SetOut(&output)
	defer func() { syncCmd.SetIn(nil); syncCmd.SetOut(nil) }()
	err := syncCmd.RunE(syncCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "[1/4]") || !strings.Contains(err.Error(), "input failed") {
		t.Fatalf("expected input error, got %v", err)
	}
	if strings.Contains(output.String(), "[2/4]") {
		t.Fatal("request started after input failure")
	}
}

func seedStoredHeaders(t *testing.T, database *sql.DB) {
	t.Helper()
	_, err := database.Exec(`CREATE TABLE spotify_headers (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		authorization TEXT NOT NULL,
		client_token TEXT NOT NULL
	);
	INSERT INTO spotify_headers VALUES (1, 'Bearer old-auth', 'old-client');`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSyncReusesStoredHeadersWithoutReadingInput(t *testing.T) {
	_, err := runSyncTest(t, "authorization: Bearer secret-auth\nclient-token: secret-client\n\n", fixtureTransport(t))
	if err != nil {
		t.Fatal(err)
	}
	output, err := executeSyncTest(t, failingHeaderReader{}, fixtureTransport(t))
	if err != nil {
		t.Fatalf("second sync should use stored headers, not read stdin: %v", err)
	}
	if strings.Contains(output, "Paste Spotify request headers") || !strings.Contains(output, "saved headers") || !strings.Contains(output, "[4/4]") {
		t.Fatalf("unexpected cached sync output: %s", output)
	}
	for _, secret := range []string{"secret-auth", "secret-client"} {
		if strings.Contains(output, secret) {
			t.Fatalf("cached credentials leaked in output")
		}
	}
}

func TestSyncRetriesWithNewHeadersAndPersistsThem(t *testing.T) {
	response := fixtureTransport(t)
	requests := 0
	transport := syncTransport(func(req *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			if req.Header.Get("authorization") != "Bearer old-auth" || req.Header.Get("client-token") != "old-client" {
				t.Error("first request did not use stored headers")
			}
			return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("expired"))}, nil
		}
		return response.RoundTrip(req)
	})
	output, err := runSyncTest(t, "authorization: Bearer secret-auth\nclient-token: secret-client\n\n", transport, func(database *sql.DB) { seedStoredHeaders(t, database) })
	if err != nil || requests != 2 {
		t.Fatalf("expected cached request then one retry, got requests=%d err=%v", requests, err)
	}
	if !strings.Contains(output, "Paste Spotify request headers") || !strings.Contains(output, "HTTP 401") {
		t.Fatalf("missing cache failure and input prompt: %s", output)
	}
	// A third run proves the successful replacement survives reopening the DB.
	if _, err := executeSyncTest(t, failingHeaderReader{}, fixtureTransport(t)); err != nil {
		t.Fatalf("new headers were not persisted: %v", err)
	}
}

func TestSyncDoesNotOverwriteStoredHeadersWhenRetryFails(t *testing.T) {
	requests := 0
	output, err := runSyncTest(t, "authorization: Bearer bad-new-auth\nclient-token: bad-new-client\n\n", syncTransport(func(*http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("expired"))}, nil
	}), func(database *sql.DB) { seedStoredHeaders(t, database) })
	if err == nil || requests != 2 || strings.Contains(output, "[3/4]") {
		t.Fatalf("expected two failed attempts before stopping, requests=%d error=%v output=%s", requests, err, output)
	}
	database, err := sql.Open("sqlite3", "meshtune.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var auth, client string
	if err := database.QueryRow("SELECT authorization, client_token FROM spotify_headers WHERE id = 1").Scan(&auth, &client); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer old-auth" || client != "old-client" {
		t.Fatal("unsuccessful credentials replaced the previous pair")
	}
}

func TestSyncRetriesWhenCachedResponseContainsGraphQLError(t *testing.T) {
	response := fixtureTransport(t)
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	data := encoder.EncodeAll([]byte(`{"errors":[{"message":"expired secret-server-token"}],"data":null}`), nil)
	encoder.Close()
	requests := 0
	output, err := runSyncTest(t, "authorization: Bearer secret-auth\nclient-token: secret-client\n\n", syncTransport(func(req *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
		}
		return response.RoundTrip(req)
	}), func(database *sql.DB) { seedStoredHeaders(t, database) })
	if err != nil || requests != 2 {
		t.Fatalf("API error should trigger fresh headers, requests=%d err=%v", requests, err)
	}
	if strings.Contains(output, "secret-server-token") {
		t.Fatal("API error leaked server response contents")
	}
}

func TestSyncDoesNotSaveRejectedNewHeaders(t *testing.T) {
	_, err := runSyncTest(t, "authorization: Bearer bad-auth\nclient-token: bad-client\n\n", syncTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("expired"))}, nil
	}))
	if err == nil {
		t.Fatal("expected rejected credentials")
	}
	database, err := sql.Open("sqlite3", "meshtune.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM spotify_headers").Scan(&count); err != nil || count != 0 {
		t.Fatalf("rejected credentials persisted: count=%d err=%v", count, err)
	}
}

func TestSyncRestrictsDatabasePermissionsForStoredTokens(t *testing.T) {
	if _, err := runSyncTest(t, "authorization: Bearer secret-auth\nclient-token: secret-client\n\n", fixtureTransport(t)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat("meshtune.db")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("database with credentials is accessible to other users: %o", info.Mode().Perm())
	}
}

func TestSyncRetriesWhenCachedResponseHasIncompletePlaylist(t *testing.T) {
	for _, playlist := range []string{
		`{}`,
		`{"content":null}`,
		`{"content":{"totalCount":0}}`,
		`{"content":{"items":null}}`,
		`{"content":{"items":[]}}`,
		`{"content":{"items":[],"totalCount":-1}}`,
		`{"content":{"items":[],"totalCount":"unknown"}}`,
	} {
		t.Run(playlist, func(t *testing.T) {
			response := fixtureTransport(t)
			encoder, err := zstd.NewWriter(nil)
			if err != nil {
				t.Fatal(err)
			}
			data := encoder.EncodeAll([]byte(`{"data":{"playlistV2":`+playlist+`}}`), nil)
			encoder.Close()
			requests := 0
			_, err = runSyncTest(t, "authorization: Bearer secret-auth\nclient-token: secret-client\n\n", syncTransport(func(req *http.Request) (*http.Response, error) {
				requests++
				if requests == 1 {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
				}
				return response.RoundTrip(req)
			}), func(database *sql.DB) { seedStoredHeaders(t, database) })
			if err != nil || requests != 2 {
				t.Fatalf("incomplete playlist must trigger retry, requests=%d err=%v", requests, err)
			}
		})
	}
}

func TestSyncAcceptsLegitimateEmptyPlaylist(t *testing.T) {
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	data := encoder.EncodeAll([]byte(`{"data":{"playlistV2":{"name":"Empty playlist","content":{"items":[],"totalCount":0}}}}`), nil)
	encoder.Close()
	output, err := runSyncTest(t, "", syncTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
	}), func(database *sql.DB) { seedStoredHeaders(t, database) })
	if err != nil || strings.Contains(output, "Paste Spotify request headers") || !strings.Contains(output, "Saved 0 tracks") {
		t.Fatalf("legitimate empty playlist should succeed without input, err=%v output=%s", err, output)
	}
}

func fixtureTransport(t *testing.T) http.RoundTripper {
	t.Helper()
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	data := encoder.EncodeAll([]byte(syncFixture), nil)
	encoder.Close()
	return syncTransport(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("authorization") != "Bearer secret-auth" || req.Header.Get("client-token") != "secret-client" {
			t.Errorf("authentication headers not forwarded")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Encoding": {"zstd"}}, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})
}

func TestSyncReportsStagesAndTracksWithoutSecrets(t *testing.T) {
	output, err := runSyncTest(t, "authorization: Bearer secret-auth\nclient-token: secret-client\nmalformed-secret\n", fixtureTransport(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"[1/4]", "Ctrl+D", "Read 2 headers", "Invalid lines skipped: 1",
		"authorization: present; client-token: present", "[2/4]", "My playlist", "Received 2 of 2 tracks",
		"1. Artist A, Artist B — First song", "2. Unknown artist — Second song",
		"Page 1", "[3/4]", "meshtune.db", "[4/4]", "Saved 2 tracks in",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q: %s", want, output)
		}
	}
	for _, secret := range []string{"secret-auth", "secret-client", "malformed-secret", "PlaylistResponse"} {
		if strings.Contains(output, secret) {
			t.Errorf("output leaks %q", secret)
		}
	}
	database, err := sql.Open("sqlite3", "meshtune.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM songs").Scan(&count); err != nil || count != 2 {
		t.Fatalf("expected 2 stored tracks, got %d, error %v", count, err)
	}
}
