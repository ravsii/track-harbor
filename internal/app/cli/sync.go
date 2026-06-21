package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
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
		info, err := spotify.GetSongInfo()
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
