package cli

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadCommandIsRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"download"})
	if err != nil || cmd.Name() != "download" || !cmd.Runnable() {
		t.Fatalf("download subcommand is unavailable: command=%v err=%v", cmd, err)
	}
	for _, flag := range []string{"pprof", "allocs"} {
		if err := cmd.ParseFlags([]string{"--" + flag + "=false"}); err != nil {
			t.Errorf("download does not accept profiling flag %s: %v", flag, err)
		}
	}
}

func TestDownloadCommandHandlesEmptyDatabase(t *testing.T) {
	t.Chdir(t.TempDir())
	database, err := sql.Open("sqlite3", "meshtune.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(`CREATE TABLE songs (id INTEGER PRIMARY KEY, artists TEXT NOT NULL, title TEXT NOT NULL, created_at DATETIME NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	cmd, _, err := rootCmd.Find([]string{"download"})
	if err != nil || cmd.Name() != "download" || cmd.Run == nil {
		t.Fatalf("download handler is unavailable: %v", err)
	}
	var output bytes.Buffer
	cmd.SetOut(&output)
	defer cmd.SetOut(nil)
	cmd.Run(cmd, nil)
	path, err := filepath.Abs("meshtune.db")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{path, "Треков в базе: 0", "В базе нет треков", "meshvault sync"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("empty database output missing %q: %s", want, output.String())
		}
	}
	if _, err := os.Stat("tmp_music"); !os.IsNotExist(err) {
		t.Errorf("empty database should not start downloads or create output directory: %v", err)
	}
	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM songs").Scan(&count); err != nil || count != 0 {
		t.Fatalf("download modified the songs table: count=%d err=%v", count, err)
	}
}
