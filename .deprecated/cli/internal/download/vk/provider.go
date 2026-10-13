package vk

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ravsii/trackharbor/cli/internal/download"
	"github.com/ravsii/trackharbor/cli/internal/repository"
	sourcevk "github.com/ravsii/trackharbor/vk"
)

// Provider uses the URLs returned by VK sync; it never searches YouTube.
type Provider struct {
	songs map[string]repository.VKSong
	byID  map[string]string
}

func NewProvider(songs map[string]repository.VKSong) *Provider {
	byID := make(map[string]string, len(songs))
	for _, song := range songs {
		byID[strconv.FormatInt(song.OwnerID, 10)+"_"+strconv.FormatInt(song.AudioID, 10)] = song.URL
	}
	return &Provider{songs: songs, byID: byID}
}
func (*Provider) Name() string { return "vk" }

func (p *Provider) Search(query string) (download.Track, error) {
	song, ok := p.songs[query]
	if !ok {
		return download.Track{}, download.ErrNotFound
	}
	if song.OwnerID == 0 || song.AudioID <= 0 || !strings.HasPrefix(song.URL, "https://") {
		return download.Track{}, fmt.Errorf("VK audio URL unavailable; run trackharbor vk sync to refresh the library")
	}
	var artists []string
	if err := json.Unmarshal([]byte(song.Artists), &artists); err != nil {
		return download.Track{}, fmt.Errorf("invalid VK artist list")
	}
	return download.Track{ID: strconv.FormatInt(song.OwnerID, 10) + "_" + strconv.FormatInt(song.AudioID, 10), Artist: strings.Join(artists, ", "), Title: song.Title}, nil
}

func (p *Provider) Download(track download.Track) (download.DownloadResponse, error) {
	return p.DownloadWithProgress(track, nil, nil)
}

func (p *Provider) DownloadWithProgress(track download.Track, onProxy func(string), onProgress func(download.DownloadProgress)) (download.DownloadResponse, error) {
	source := p.byID[track.ID]
	if !strings.HasPrefix(source, "https://") {
		return download.DownloadResponse{}, fmt.Errorf("VK audio URL unavailable; run trackharbor vk sync")
	}
	if onProgress != nil {
		onProgress(download.DownloadProgress{Converting: true})
	}
	f, size, closeFile, err := sourcevk.DownloadMP3(source)
	if err != nil {
		return download.DownloadResponse{}, err
	}
	return download.DownloadResponse{Body: &temporaryFile{File: f, closeFile: closeFile}, ContentLength: size}, nil
}

type temporaryFile struct {
	*os.File
	closeFile func() error
}

func (f *temporaryFile) Close() error { return f.closeFile() }
