package spotify

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/klauspost/compress/zstd"
)

func GetSongInfo() (*PlaylistResponse, error) {
	url := "https://api-partner.spotify.com/pathfinder/v2/query"

	payload := strings.NewReader("{\n  \"variables\": {\n    \"uri\": \"spotify:playlist:37i9dQZF1F5p3rmiWPIYgZ\",\n    \"offset\": 0,\n    \"limit\": 50,\n    \"includeEpisodeContentRatingsV2\": true\n  },\n  \"operationName\": \"fetchPlaylistContents\",\n  \"extensions\": {\n    \"persistedQuery\": {\n      \"version\": 1,\n      \"sha256Hash\": \"a65e12194ed5fc443a1cdebed5fabe33ca5b07b987185d63c72483867ad13cb4\"\n    }\n  }\n}")

	req, _ := http.NewRequest("POST", url, payload)

	req.Header.Add("accept-encoding", "gzip, deflate, br, zstd")
	req.Header.Add("user-agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:150.0) Gecko/20100101 Firefox/150.0")
	req.Header.Add("accept", "application/json")
	req.Header.Add("accept-language", "en")
	req.Header.Add("content-type", "application/json;charset=UTF-8")
	req.Header.Add("referer", "https://open.spotify.com/")
	req.Header.Add("app-platform", "WebPlayer")
	req.Header.Add("spotify-app-version", "1.2.93.434.gba0d48fa")
	req.Header.Add("client-token", "AAAOe2Jg2XFgkdM9mMn1+VDStaG7ge/gTWXInAk6tH1V6NJwq19EKVKT78Y/WG3k0ehiX2k5D1XJHTNbccYy4iuFbgFE5UxU5pvCAvCNz61MbXtQaSWGgc8I6fuXD1+2NgRXLOEJ9Ba2OwsGY0GuTSl47hi8/C7DMQKR45U1kuDdxPsoAJv+3y+uiTi6j4bCDfCvX+oMjEI7rHdzzgfIOacUWXLmc10UhcPK2OuBsXhkZtPR/saqNumhRbkScPVgU9KvjT35gCnfe+Z5QbattN3ecpoFzHzdj4fOH2aiut+k7HPeV1gDiC4QSKhTXCz0V4W+TpLL+1PPTtR3ZSgvdLVOiAqR6hk=")
	req.Header.Add("origin", "https://open.spotify.com")
	req.Header.Add("sec-gpc", "1")
	req.Header.Add("sec-fetch-dest", "empty")
	req.Header.Add("sec-fetch-mode", "cors")
	req.Header.Add("sec-fetch-site", "same-site")
	req.Header.Add("authorization", "Bearer BQBbEz2tF5gO6zCSOnZOJr93FBoaErW6QJsWo6UgoKkeERL6FVdAqKvHGB7_bShScF01w7sAJSQIJQ76KEH_Z39SRJrSB4O5eJjx3s9i32sT8B2S7GlciNCjHWPqsU5ADitPVeJ_lpOdZYHVUquWoOtKLp6OQWVvePzb7i07x70UOW5Z1xB9iI-CG1KzN-WjTP4jdMU-n3U9Whu-bcr2uQgW0fevncCDDMhILZ5Wp3KWGYA9YE9X86V7OOulgjM1sJEaUOtSCeUk6n4_skgLXeN9rO19cvIXod6bWCkytBLavHettrymKOsEZHbP1CKifIRCBXWIARd7j8USVYxW8jT1T_b2jxjNTZAwL9gZcuMb1PU6zENq0pvqfodTjkvDJuGuHNwOPZF4nL0")
	req.Header.Add("connection", "keep-alive")
	req.Header.Add("priority", "u=4")

	resp, _ := http.DefaultClient.Do(req)

	defer resp.Body.Close()

	decoder, err := zstd.NewReader(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	defer decoder.Close()

	var responseFull PlaylistResponse

	err = json.NewDecoder(decoder).Decode(&responseFull)

	return &responseFull, err
}
