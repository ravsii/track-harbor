package download

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"
)

var youtubeVideoID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

type artworkStrategy interface {
	Execute(context.Context, func(string) error) error
}

type socks4Dialer func(context.Context, string, string, time.Duration) (net.Conn, error)

// Artwork is optional: a failed source image or YouTube preview must never
// prevent writing the library's title and artist.
func fetchCover(sourceURL, provider, videoID string, route artworkStrategy, dialSOCKS4 socks4Dialer) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	urls := []string{sourceURL}
	if provider == "youtube" && youtubeVideoID.MatchString(videoID) {
		urls = append(urls, "https://i.ytimg.com/vi/"+videoID+"/hqdefault.jpg")
	}
	var lastErr error
	for _, imageURL := range urls {
		if imageURL == "" {
			continue
		}
		if data, err := loadImage(ctx, imageURL, route, dialSOCKS4); err == nil {
			return data, nil
		} else {
			lastErr = err
		}
	}
	return nil, lastErr
}

func loadImage(ctx context.Context, imageURL string, route artworkStrategy, dialSOCKS4 socks4Dialer) ([]byte, error) {
	u, err := url.Parse(imageURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("invalid artwork URL")
	}
	var data []byte
	fetch := func(proxyURL string) error {
		var fetchErr error
		data, fetchErr = fetchArtwork(ctx, imageURL, proxyURL, dialSOCKS4)
		return fetchErr
	}
	if route != nil {
		err = route.Execute(ctx, fetch)
	} else {
		err = fetch("")
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

func writeMP3Metadata(mp3 string, metadata *MP3Metadata, data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	args := []string{"-v", "error", "-nostdin", "-y", "-i", mp3}
	if len(data) > 0 {
		image, err := os.CreateTemp(filepath.Dir(mp3), ".cover-*.img")
		if err != nil {
			return err
		}
		defer os.Remove(image.Name())
		if _, err = image.Write(data); err != nil {
			image.Close()
			return err
		}
		if err = image.Close(); err != nil {
			return err
		}
		args = append(args, "-i", image.Name())
	}
	output, err := os.CreateTemp(filepath.Dir(mp3), ".covered-*.mp3")
	if err != nil {
		return err
	}
	defer os.Remove(output.Name())
	if err = output.Close(); err != nil {
		return err
	}
	args = append(args, "-map", "0:a:0", "-c:a", "copy")
	if len(data) > 0 {
		args = append(args, "-map", "1:v:0", "-c:v", "mjpeg", "-disposition:v:0", "attached_pic",
			"-metadata:s:v", "title=Album cover", "-metadata:s:v", "comment=Cover (front)")
	}
	if metadata != nil {
		args = append(args, "-metadata", "title="+metadata.Title, "-metadata", "artist="+metadata.Artist)
	}
	args = append(args, "-id3v2_version", "3", output.Name())
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not write MP3 metadata")
	}
	if err := os.Rename(output.Name(), mp3); err != nil {
		return fmt.Errorf("replace staged MP3: %w", err)
	}
	return nil
}

func fetchArtwork(ctx context.Context, imageURL, proxyURL string, dialSOCKS4 socks4Dialer) ([]byte, error) {
	transport := &http.Transport{DisableKeepAlives: true, TLSHandshakeTimeout: 8 * time.Second, ResponseHeaderTimeout: 10 * time.Second}
	if proxyURL != "" {
		proxy, err := url.Parse(proxyURL)
		if err != nil || proxy.Host == "" || proxy.User != nil && proxy.Scheme == "socks4" {
			return nil, fmt.Errorf("invalid artwork proxy")
		}
		switch proxy.Scheme {
		case "http", "https", "socks5":
			transport.Proxy = http.ProxyURL(proxy)
		case "socks4":
			if dialSOCKS4 == nil {
				return nil, fmt.Errorf("SOCKS4 artwork routing unavailable")
			}
			transport.DialContext = func(ctx context.Context, network, target string) (net.Conn, error) {
				return dialSOCKS4(ctx, proxy.Host, target, 8*time.Second)
			}
		default:
			return nil, fmt.Errorf("unsupported artwork proxy")
		}
	}
	defer transport.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid artwork request")
	}
	client := &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return http.ErrUseLastResponse
		}
		return nil
	}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("artwork unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &HTTPError{Stage: "artwork", StatusCode: http.StatusTooManyRequests}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("artwork HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > 5<<20 {
		return nil, fmt.Errorf("artwork too large")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20+1))
	if err != nil || len(data) > 5<<20 {
		return nil, fmt.Errorf("artwork too large or incomplete")
	}
	format := http.DetectContentType(data)
	if format != "image/jpeg" && format != "image/png" && format != "image/webp" {
		return nil, fmt.Errorf("artwork is not a supported image")
	}
	return data, nil
}
