package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/ravsii/trackharbor/cli/internal/repository"
)

func TestEffectiveDownloadWorkersNeverExceedsProxyPool(t *testing.T) {
	for _, tc := range []struct{ proxies, limit, want int }{
		{37, 50, 37}, {100, 50, 50}, {1, 50, 1}, {37, 20, 20}, {10, 20, 10}, {100, 100, 100},
	} {
		if got := effectiveDownloadWorkers(tc.proxies, tc.limit); got != tc.want {
			t.Errorf("proxies=%d limit=%d got=%d want=%d", tc.proxies, tc.limit, got, tc.want)
		}
	}
}

func TestDownloadPassesSelectedWorkerCountToDownloader(t *testing.T) {
	for _, tc := range []struct{ proxies, limit, want int }{{3, 50, 3}, {4, 2, 2}} {
		t.Run(fmt.Sprintf("%dproxies_%dlimit", tc.proxies, tc.limit), func(t *testing.T) {
			t.Chdir(t.TempDir())
			database, err := sql.Open("sqlite3", "meshtune.db")
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			if _, err := database.Exec(`CREATE TABLE songs (id INTEGER PRIMARY KEY, artists TEXT NOT NULL, title TEXT NOT NULL, created_at DATETIME NOT NULL); INSERT INTO songs (artists,title,created_at) VALUES ('["Artist"]','Song',CURRENT_TIMESTAMP)`); err != nil {
				t.Fatal(err)
			}
			store, err := repository.NewProxyRepository(sqlx.NewDb(database, "sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			until := time.Now().Add(time.Hour)
			for i := 0; i < tc.proxies; i++ {
				if err := store.Save(t.Context(), fmt.Sprintf("http://203.0.113.1:%d", 10000+i), until); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.Complete(t.Context(), until); err != nil {
				t.Fatal(err)
			}
			if err := database.Close(); err != nil {
				t.Fatal(err)
			}
			tools := t.TempDir()
			script := `#!/bin/sh
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output" ]; then shift; output="$1"; fi
  shift
done
if [ -n "$output" ]; then
  printf 'mp3' > "${output%.*}.mp3"
else
  echo '{"entries":[{"id":"dQw4w9WgXcQ","title":"Song","uploader":"Artist"}]}'
fi
`
			for name, content := range map[string]string{"yt-dlp": script, "ffmpeg": "#!/bin/sh\nexit 0\n"} {
				if err := os.WriteFile(filepath.Join(tools, name), []byte(content), 0700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", tools)
			oldValue := downloadCmd.Flags().Lookup("workers").Value.String()
			oldContext := downloadCmd.Context()
			downloadCmd.SetContext(t.Context())
			t.Cleanup(func() {
				downloadCmd.Flags().Set("workers", oldValue)
				downloadCmd.SetOut(nil)
				downloadCmd.SetContext(oldContext)
			})
			if err := downloadCmd.Flags().Set("workers", fmt.Sprint(tc.limit)); err != nil {
				t.Fatal(err)
			}
			downloadCmd.SetOut(io.Discard)
			downloadCmd.Run(downloadCmd, nil)
			data, err := os.ReadFile("logs/download.jsonl")
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				var fields map[string]any
				if err := json.Unmarshal([]byte(line), &fields); err != nil {
					t.Fatal(err)
				}
				if fields["msg"] == "run_started" {
					found = true
					if fields["workers"] != float64(tc.want) {
						t.Fatalf("actual downloader worker count=%v, want %d", fields["workers"], tc.want)
					}
				}
			}
			if !found {
				t.Fatal("downloader did not run")
			}
		})
	}
}

func TestDownloadWorkersFlagDefaultsTo50AndAcceptsOverride(t *testing.T) {
	flag := downloadCmd.Flags().Lookup("workers")
	if flag == nil || flag.DefValue != "50" {
		t.Fatalf("missing --workers default50: %v", flag)
	}
	t.Cleanup(func() { downloadCmd.Flags().Set("workers", flag.DefValue) })
	if err := downloadCmd.ParseFlags([]string{"--workers", "20"}); err != nil {
		t.Fatal(err)
	}
	limit, err := downloadWorkerLimit(downloadCmd)
	if err != nil || limit != 20 {
		t.Fatalf("limit=%d err=%v", limit, err)
	}
}

func TestInvalidDownloadWorkersHaveNoSideEffects(t *testing.T) {
	if value := os.Getenv("TRACKHARBOR_TEST_INVALID_WORKERS"); value != "" {
		rootCmd.SetArgs([]string{"download", "--workers", value})
		err := rootCmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "workers") {
			t.Fatalf("invalid workers accepted: %v", err)
		}
		return
	}
	for _, value := range []string{"0", "-1", "not-an-int"} {
		t.Run(value, func(t *testing.T) {
			dir := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=^TestInvalidDownloadWorkersHaveNoSideEffects$")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "TRACKHARBOR_TEST_INVALID_WORKERS="+value)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("validation failed: %v\n%s", err, output)
			}
			files, err := os.ReadDir(dir)
			if err != nil || len(files) != 0 {
				t.Fatalf("invalid flags created files: %v %v", files, err)
			}
		})
	}
}
