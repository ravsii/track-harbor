package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"meshvault/internal/db"
	"meshvault/internal/repository"
	"meshvault/internal/source/spotify"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "sync",
	Long:  `sync long`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Parse spotify headers:")
		hMap, err := parseHeaders()
		if err != nil {
			panic(err)
		}

		fmt.Println(hMap)

		info, err := spotify.GetSongInfo(
			hMap["authorization"],
			hMap["client-token"],
		)
		if err != nil {
			log.Fatalln(err)
		}

		fmt.Printf("%#v\n", info)

		database, err := db.New()
		if err != nil {
			log.Fatalln(err)
		}

		repo := repository.NewSongRepository(database)

		err = repo.DeleteAll()
		if err != nil {
			log.Fatalln(err)
		}

		var ss []repository.Song

		for _, song := range info.Data.PlaylistV2.Content.Items {
			artists := make([]string, 0, len(song.ItemV3.Data.IdentityTrait.Contributors.Items))

			for _, c := range song.ItemV3.Data.IdentityTrait.Contributors.Items {
				if c.Name != "" {
					artists = append(artists, c.Name)
				}
			}

			var buf bytes.Buffer

			err := json.NewEncoder(&buf).Encode(artists)
			if err != nil {
				log.Fatalln(err)
			}

			ss = append(ss, repository.Song{
				Artists:   buf.String(),
				Title:     song.ItemV3.Data.IdentityTrait.Name,
				CreatedAt: time.Now(),
			})
		}

		err = repo.BulkInsert(ss)
		if err != nil {
			log.Fatalln(err)
		}
	},
}

func parseHeaders() (map[string]string, error) {
	scanner := bufio.NewScanner(os.Stdin)

	headerMap := make(map[string]string)

	for scanner.Scan() {
		text := scanner.Text()
		headers := strings.SplitN(text, ":", 2)
		if len(headers) != 2 {
			fmt.Println(headers)
			continue
		}
		key, value := strings.ToLower(headers[0]), strings.TrimSpace(headers[1])
		headerMap[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return headerMap, nil
}
