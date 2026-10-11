package proxy

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"trackharbor/internal/download"
	"trackharbor/internal/download/youtube"
)

func TestYouTube429MakesNextRequestUseAnotherLRUProxy(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--proxy" ]; then shift; proxy="$1"; fi
  shift
done
if [ "$proxy" = "http://203.0.113.1:80" ]; then
  echo 'ERROR: HTTP Error 429' >&2
  exit 1
fi
echo '{"entries":[{"id":"dQw4w9WgXcQ","title":"Song","uploader":"Artist"}]}'
`
	for name, contents := range map[string]string{"yt-dlp": script, "ffmpeg": "#!/bin/sh\nexit 0\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	pool, err := NewPool([]string{"http://203.0.113.1:80", "http://203.0.113.2:80"})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := youtube.NewProvider()
	if err != nil {
		t.Fatal(err)
	}
	provider.WithProxies(pool)
	_, err = provider.Search("Song")
	var httpErr *download.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 429 {
		t.Fatalf("expected initial 429: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := provider.Search("Song"); err != nil {
			t.Fatalf("blocked proxy reused: %v", err)
		}
	}
}
