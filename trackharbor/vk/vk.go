package vk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const apiVersion = "5.131"
const pageSize = 50

// ErrUnauthorized means VK rejected the saved access token.
var ErrUnauthorized = errors.New("VK token rejected")

// Match vkpymusic's VkMobileClient request header without changing token auth.
const userAgent = "VKAndroidApp/4.13.1-1206 (Android 4.4.3; SDK 19; armeabi; ; ru)"

type Track struct {
	ID      int64  `json:"id"`
	OwnerID int64  `json:"owner_id"`
	Artist  string `json:"artist"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Date    int64  `json:"date"` // VK upload timestamp; may be absent.
	Album   struct {
		Thumb struct {
			Photo600 string `json:"photo_600"`
			Photo300 string `json:"photo_300"`
		} `json:"thumb"`
	} `json:"album"` // Optional in some VK audio responses.
}

type Client struct {
	token string
	http  *http.Client
}

func NewClient(token string) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Never send VK credentials through download proxies or inherited proxies.
	transport.Proxy = nil
	return &Client{token: token, http: &http.Client{
		Transport:     transport,
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (c *Client) Close() {
	c.http.CloseIdleConnections()
}

func (c *Client) request(ctx context.Context, method string, params url.Values, result any) error {
	params.Set("access_token", c.token)
	params.Set("v", apiVersion)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.vk.com/method/"+method, strings.NewReader(params.Encode()))
	if err != nil {
		return fmt.Errorf("VK %s: cannot create request", method)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)
	response, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Do not expose raw transport errors or response bodies containing credentials.
		return fmt.Errorf("VK %s: connection failed or timed out", method)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("VK %s: HTTP %d", method, response.StatusCode)
	}
	const maxBody = 8 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		return fmt.Errorf("VK %s: response unreadable or too large", method)
	}
	var envelope struct {
		Response json.RawMessage `json:"response"`
		Error    *struct {
			Code int `json:"error_code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("VK %s: invalid JSON response", method)
	}
	if envelope.Error != nil {
		if envelope.Error.Code == 5 || envelope.Error.Code == 1117 {
			return fmt.Errorf("VK %s: %w", method, ErrUnauthorized)
		}
		hint := "request rejected"
		switch envelope.Error.Code {
		case 6:
			hint = "rate limit reached; retry sync later"
		case 7, 15, 20, 23, 27, 28:
			hint = "access denied; this token or application may not have audio access"
		case 14, 17:
			hint = "account verification required; open VK in your browser"
		}
		return fmt.Errorf("VK %s: API error %d (%s)", method, envelope.Error.Code, hint)
	}
	if len(envelope.Response) == 0 || string(envelope.Response) == "null" {
		return fmt.Errorf("VK %s: missing response", method)
	}
	if err := json.Unmarshal(envelope.Response, result); err != nil {
		return fmt.Errorf("VK %s: unexpected response format", method)
	}
	return nil
}

// Ping checks access to the current user's audio library without downloading
// audio or changing stored tracks. An empty library is a valid response.
func (c *Client) Ping(ctx context.Context) error {
	var users []struct {
		ID int64 `json:"id"`
	}
	if err := c.request(ctx, "users.get", url.Values{}, &users); err != nil {
		return err
	}
	if len(users) != 1 || users[0].ID <= 0 {
		return fmt.Errorf("VK users.get returned no current user")
	}
	var page struct {
		Count *int `json:"count"`
		Items *[]struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	params := url.Values{
		"owner_id": {strconv.FormatInt(users[0].ID, 10)},
		"count":    {"1"}, "offset": {"0"},
	}
	if err := c.request(ctx, "audio.get", params, &page); err != nil {
		return err
	}
	if page.Count == nil || page.Items == nil || *page.Count < 0 ||
		len(*page.Items) > 1 || (*page.Count == 0 && len(*page.Items) != 0) ||
		(*page.Count > 0 && len(*page.Items) != 1) {
		return fmt.Errorf("VK audio.get returned an invalid audio page")
	}
	return nil
}

// MyMusic returns only the token owner's library, never another user's playlist.
func (c *Client) MyMusic(ctx context.Context, progress func(int, int)) ([]Track, error) {
	var users []struct {
		ID int64 `json:"id"`
	}
	if err := c.request(ctx, "users.get", url.Values{}, &users); err != nil {
		return nil, err
	}
	if len(users) != 1 || users[0].ID <= 0 {
		return nil, fmt.Errorf("VK users.get: a user access token is required")
	}
	var tracks []Track
	seen := make(map[[2]int64]bool)
	total := -1
	offset := 0
	for {
		if offset > 0 {
			timer := time.NewTimer(400 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		var page struct {
			Count *int     `json:"count"`
			Items *[]Track `json:"items"`
		}
		params := url.Values{
			"owner_id": {strconv.FormatInt(users[0].ID, 10)},
			"count":    {strconv.Itoa(pageSize)}, "offset": {strconv.Itoa(offset)},
		}
		if err := c.request(ctx, "audio.get", params, &page); err != nil {
			return nil, err
		}
		if page.Count == nil || page.Items == nil || *page.Count < 0 {
			return nil, fmt.Errorf("VK audio.get: missing or invalid track count/items")
		}
		if total < 0 {
			total = *page.Count
		}
		if *page.Count != total {
			return nil, fmt.Errorf("VK library changed during import; run sync again")
		}
		if len(*page.Items) > pageSize || len(*page.Items) > total-offset {
			return nil, fmt.Errorf("VK audio.get: incomplete or invalid page; previous library preserved")
		}
		for index, track := range *page.Items {
			key := [2]int64{track.OwnerID, track.ID}
			if track.ID <= 0 || track.OwnerID == 0 {
				return nil, fmt.Errorf("VK audio.get: invalid track ID (offset=%s, page_index=%d, owner_id=%d, id=%d)", params.Get("offset"), index, track.OwnerID, track.ID)
			}
			if seen[key] {
				return nil, fmt.Errorf("VK audio.get: repeated track (offset=%s, requested=%d, returned=%d, page_index=%d, owner_id=%d, id=%d); pages may overlap", params.Get("offset"), pageSize, len(*page.Items), index, track.OwnerID, track.ID)
			}
			seen[key] = true
			tracks = append(tracks, track)
		}
		if progress != nil {
			progress(len(tracks), total)
		}
		offset += pageSize
		if offset >= total {
			return tracks, nil
		}
	}
}
