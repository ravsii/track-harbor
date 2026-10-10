package youtube

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"meshvault/internal/download"
)

func fakeTools(t *testing.T, script string) string {
	t.Helper()
	dir:=t.TempDir()
	for name,body:=range map[string]string{"yt-dlp":script,"ffmpeg":"exit 0"} {
		if err:=os.WriteFile(filepath.Join(dir,name),[]byte("#!/bin/sh\n"+body+"\n"),0700);err!=nil { t.Fatal(err) }
	}
	t.Setenv("PATH",dir)
	return dir
}

func TestSearchViaYtDlp(t *testing.T) {
	dir:=fakeTools(t,`printf '%s\n' "$@" > "$ARGS_FILE"
printf '%s' '{"entries":[{"id":"abcdefghijk","title":"Song title","uploader":"Artist"}]}'`)
	argsFile:=filepath.Join(dir,"args")
	t.Setenv("ARGS_FILE",argsFile)
	p,err:=NewProvider()
	if err!=nil { t.Fatal(err) }
	track,err:=p.Search(`Artist song; $(anything)`)
	if err!=nil || track.ID!="abcdefghijk" || track.Title!="Song title" || track.Artist!="Artist" { t.Fatalf("track=%+v err=%v",track,err) }
	args,err:=os.ReadFile(argsFile)
	if err!=nil { t.Fatal(err) }
	for _,want:=range []string{"--ignore-config","--flat-playlist","--dump-single-json","ytsearch1:Artist song; $(anything)"} {
		if !strings.Contains(string(args),want) { t.Fatalf("missing %s in %s",want,args) }
	}
}

func TestEmptySearchIsNotTechnicalFailure(t *testing.T) {
	for _,tc:=range []struct{script string; notFound bool}{
		{`printf '%s' '{"entries":[]}'`,true},
		{`printf '%s' 'invalid JSON'`,false},
		{`printf '%s' '{"entries":[{"id":"../../bad","title":"Song"}]}'`,false},
		{`printf '%s' 'ERROR: HTTP Error 500: server error' >&2; exit 1`,false},
	} {
		t.Run(tc.script,func(t *testing.T){
			fakeTools(t,tc.script)
			p,err:=NewProvider();if err!=nil { t.Fatal(err) }
			_,err=p.Search("Song")
			if err==nil || errors.Is(err,download.ErrNotFound)!=tc.notFound { t.Fatalf("wrong classification: %v",err) }
			if strings.Contains(tc.script,"HTTP Error") { var httpErr *download.HTTPError; if !errors.As(err,&httpErr) || httpErr.StatusCode!=500 { t.Fatalf("HTTP status lost: %v",err) } }
		})
	}
}

func TestDownloadConvertsMP3AndCleansTemporaryFiles(t *testing.T) {
	dir:=fakeTools(t,`printf '%s\n' "$@" > "$ARGS_FILE"
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output" ]; then shift; output="$1"; fi
  shift
done
printf '%s' 'mp3 bytes' > "${output%.*}.mp3"`)
	argsFile:=filepath.Join(dir,"args")
	t.Setenv("ARGS_FILE",argsFile)
	p,err:=NewProvider();if err!=nil { t.Fatal(err) }
	response,err:=p.Download(download.Track{ID:"abcdefghijk"})
	if err!=nil { t.Fatal(err) }
	file:=response.Body.(*temporaryAudio).File.Name()
	data,err:=io.ReadAll(response.Body)
	if err!=nil || string(data)!="mp3 bytes" || response.ContentLength!=9 { t.Fatalf("body=%s size=%d err=%v",data,response.ContentLength,err) }
	if err:=response.Body.Close();err!=nil { t.Fatal(err) }
	if _,err:=os.Stat(filepath.Dir(file));!os.IsNotExist(err) { t.Fatalf("temporary directory survived Close: %v",err) }
	args,err:=os.ReadFile(argsFile);if err!=nil { t.Fatal(err) }
	for _,want:=range []string{"--extract-audio","--audio-format\nmp3","--audio-quality\n192K","--no-playlist","https://www.youtube.com/watch?v=abcdefghijk"} {
		if !strings.Contains(string(args),want) { t.Fatalf("missing %s in %s",want,args) }
	}
}

func TestMissingToolsFailBeforeDownloading(t *testing.T) {
	t.Setenv("PATH",t.TempDir())
	if _,err:=NewProvider();err==nil || !strings.Contains(err.Error(),"yt-dlp") { t.Fatalf("missing yt-dlp: %v",err) }
	dir:=fakeTools(t,"exit 0")
	if err:=os.Remove(filepath.Join(dir,"ffmpeg"));err!=nil { t.Fatal(err) }
	if _,err:=NewProvider();err==nil || !strings.Contains(err.Error(),"ffmpeg") { t.Fatalf("missing ffmpeg: %v",err) }
}
