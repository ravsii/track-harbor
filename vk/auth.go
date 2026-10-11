package vk

import (
	"fmt"
	"net/url"
	"strings"
)

// AuthorizationURL returns the existing VK application flow. The token is
// delivered in blank.html's fragment; callers must ask the user to paste it.
func AuthorizationURL() string {
	return "https://oauth.vk.com/authorize?" + (url.Values{
		"client_id": {"6287487"}, "scope": {"audio,offline"},
		"redirect_uri": {"https://oauth.vk.com/blank.html"},
		"display":      {"page"}, "response_type": {"token"},
	}).Encode()
}

func ParseToken(input string) (string, error) {
	text := strings.TrimSpace(input)
	if strings.Contains(text, "access_token=") {
		fragment := text
		if i := strings.IndexByte(fragment, '#'); i >= 0 {
			fragment = fragment[i+1:]
		}
		if i := strings.IndexByte(fragment, '?'); i >= 0 {
			fragment = fragment[i+1:]
		}
		params, err := url.ParseQuery(strings.TrimPrefix(fragment, "#"))
		if err != nil {
			return "", fmt.Errorf("invalid VK token URL")
		}
		text = params.Get("access_token")
	}
	if text == "" || strings.ContainsAny(text, " \t\r\n:/?#&=") {
		return "", fmt.Errorf("paste a non-empty VK access token or redirect URL containing access_token")
	}
	return text, nil
}
