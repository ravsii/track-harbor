package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/spf13/cobra"
	"meshvault/internal/db"
	"meshvault/internal/source/spotify"
)

type Song struct {
	ID        int64     `db:"id"`
	Artists   string    `db:"artists"`
	Title     string    `db:"title"`
	CreatedAt time.Time `db:"created_at"`
}

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

		db, err := db.New()
		if err != nil {
			log.Fatalln(err)
		}

		_, err = db.Exec("DELETE FROM songs;")
		if err != nil {
			log.Fatalln(err)
		}

		var ss []Song

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

			ss = append(ss, Song{
				Artists:   buf.String(),
				Title:     song.ItemV3.Data.IdentityTrait.Name,
				CreatedAt: time.Now(),
			})
		}

		tx, err := db.Beginx()
		if err != nil {
			log.Fatalln(err)
		}

		defer tx.Rollback()

		stmt, err := tx.PrepareNamed(`
	INSERT INTO songs (artists, title, created_at)
	VALUES (:artists, :title, :created_at)
`)
		if err != nil {
			log.Fatalln(err)
		}
		defer stmt.Close()

		for _, s := range ss {
			_, err := stmt.Exec(s)
			if err != nil {
				log.Fatalln(err)
			}
		}

		err = tx.Commit()
		if err != nil {
			log.Fatalln(err)
		}
	},
}
