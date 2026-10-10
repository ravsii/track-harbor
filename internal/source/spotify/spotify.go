package spotify

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/klauspost/compress/zstd"
)

func GetSongInfo(
	bearer string,
	clientToken string,
) (*PlaylistResponse, error) {
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
	req.Header.Add("client-token", clientToken)
	req.Header.Add("origin", "https://open.spotify.com")
	req.Header.Add("sec-gpc", "1")
	req.Header.Add("sec-fetch-dest", "empty")
	req.Header.Add("sec-fetch-mode", "cors")
	req.Header.Add("sec-fetch-site", "same-site")
	req.Header.Add("authorization", bearer)
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
