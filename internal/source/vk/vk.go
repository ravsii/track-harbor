package vk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const apiVersion = "5.131"
const pageSize = 100

type Track struct {
	ID      int64  `json:"id"`
	OwnerID int64  `json:"owner_id"`
	Artist  string `json:"artist"`
	Title   string `json:"title"`
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
		hint := "request rejected"
		switch envelope.Error.Code {
		case 5:
			hint = "authorization failed; obtain a fresh token and run trackharbor vk auth"
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
	for {
		if len(tracks) > 0 {
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
			"count":    {strconv.Itoa(pageSize)}, "offset": {strconv.Itoa(len(tracks))},
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
		if len(*page.Items) > total-len(tracks) || len(*page.Items) == 0 && len(tracks) < total {
			return nil, fmt.Errorf("VK audio.get: incomplete or invalid page; previous library preserved")
		}
		for _, track := range *page.Items {
			key := [2]int64{track.OwnerID, track.ID}
			if track.ID <= 0 || track.OwnerID == 0 || seen[key] {
				return nil, fmt.Errorf("VK audio.get: invalid or repeated track; run sync again")
			}
			seen[key] = true
			tracks = append(tracks, track)
		}
		if progress != nil {
			progress(len(tracks), total)
		}
		if len(tracks) == total {
			return tracks, nil
		}
	}
}
