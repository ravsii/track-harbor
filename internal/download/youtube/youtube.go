// Package youtube uses yt-dlp and FFmpeg executables available in PATH.
package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"meshvault/internal/download"
)

var videoID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
var httpStatus = regexp.MustCompile(`HTTP Error ([0-9]{3})`)

type Provider struct { executable string }

func NewProvider() (*Provider,error) {
	path,err:=exec.LookPath("yt-dlp")
	if err!=nil { return nil,fmt.Errorf("YouTube: yt-dlp должен быть установлен и доступен в PATH: %w",err) }
	if _,err:=exec.LookPath("ffmpeg");err!=nil { return nil,fmt.Errorf("YouTube: ffmpeg должен быть установлен и доступен в PATH: %w",err) }
	return &Provider{executable:path},nil
}

func (p *Provider) Name() string { return "youtube" }

func (p *Provider) run(stage string, timeout time.Duration, args ...string) ([]byte,error) {
	ctx,cancel:=context.WithTimeout(context.Background(),timeout)
	defer cancel()
	// Never invoke a shell; query strings are a single argument, not commands.
	cmd:=exec.CommandContext(ctx,p.executable,append([]string{"--ignore-config","--no-progress","--no-cache-dir","--retries","0","--fragment-retries","0"},args...)...)
	var stderr bytes.Buffer
	cmd.Stderr=&stderr
	output,err:=cmd.Output()
	if ctx.Err()!=nil { return nil,fmt.Errorf("yt-dlp %s: %w",stage,ctx.Err()) }
	if err!=nil {
		if match:=httpStatus.FindStringSubmatch(stderr.String());len(match)>0 {
			code,_:=strconv.Atoi(match[1])
			return nil,&download.HTTPError{Stage:stage,StatusCode:code}
		}
		// Do not persist raw stderr: it can contain signed URLs and cookies.
		return nil,fmt.Errorf("yt-dlp %s: %w (проверьте версию yt-dlp, доступ к YouTube и JS runtime)",stage,err)
	}
	return output,nil
}

func (p *Provider) Search(query string) (download.Track,error) {
	output,err:=p.run("search",time.Minute,"--flat-playlist","--dump-single-json","--","ytsearch1:"+query)
	if err!=nil { return download.Track{},err }
	var result struct {
		Entries []struct { ID string `json:"id"`; Title string `json:"title"`; Uploader string `json:"uploader"`; Channel string `json:"channel"` } `json:"entries"`
	}
	if err:=json.Unmarshal(output,&result);err!=nil { return download.Track{},fmt.Errorf("yt-dlp search JSON: %w",err) }
	if result.Entries==nil { return download.Track{},fmt.Errorf("yt-dlp search: missing entries") }
	if len(result.Entries)==0 { return download.Track{},download.ErrNotFound }
	entry:=result.Entries[0]
	if !videoID.MatchString(entry.ID) || entry.Title=="" { return download.Track{},fmt.Errorf("yt-dlp search: invalid video metadata") }
	artist:=entry.Uploader
	if artist=="" { artist=entry.Channel }
	return download.Track{ID:entry.ID,Title:entry.Title,Artist:artist},nil
}

func (p *Provider) Download(track download.Track) (download.DownloadResponse,error) {
	if !videoID.MatchString(track.ID) { return download.DownloadResponse{},fmt.Errorf("YouTube: invalid video ID") }
	dir,err:=os.MkdirTemp("","meshvault-youtube-*")
	if err!=nil { return download.DownloadResponse{},err }
	keep:=false
	defer func(){ if !keep { os.RemoveAll(dir) } }()
	_,err=p.run("download",5*time.Minute,"--no-playlist","--format","bestaudio/best","--extract-audio","--audio-format","mp3","--audio-quality","192K","--output",filepath.Join(dir,"audio.%(ext)s"),"--","https://www.youtube.com/watch?v="+track.ID)
	if err!=nil { return download.DownloadResponse{},err }
	f,err:=os.Open(filepath.Join(dir,"audio.mp3"))
	if err!=nil { return download.DownloadResponse{},fmt.Errorf("yt-dlp MP3 output: %w",err) }
	info,err:=f.Stat()
	if err!=nil || info.Size()==0 {
		f.Close()
		if err==nil { err=fmt.Errorf("empty MP3") }
		return download.DownloadResponse{},err
	}
	keep=true
	return download.DownloadResponse{Body:&temporaryAudio{File:f,dir:dir},ContentLength:info.Size()},nil
}

// The downloader hashes/copies the MP3, then closes it; remove staged media too.
type temporaryAudio struct { *os.File; dir string }

func (f *temporaryAudio) Close() error { return errors.Join(f.File.Close(),os.RemoveAll(f.dir)) }
