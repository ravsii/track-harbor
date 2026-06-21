package cli

import (
	"encoding/json"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"meshvault/internal/db"
	"meshvault/internal/download"
	"meshvault/internal/download/mp3party"
	"meshvault/internal/repository"
)

const (
	testFolder      = "tmp_music"
	downloadWorkers = 10
)

var rootCmd = &cobra.Command{
	Use:   "meshvault",
	Short: "A brief description of your application",
	Long: `A longer description that spans multiple lines and likely contains
		examples and usage of using your application. For example:

		Cobra is a CLI library for Go that empowers applications.
		This application is a tool to generate the needed files
		to quickly create a Cobra application.`,

	Run: func(cmd *cobra.Command, args []string) {
		database, err := db.New()
		if err != nil {
			log.Fatalln(err)
		}

		repo := repository.NewSongRepository(database)

		songList, err := repo.GetAll()
		if err != nil {
			log.Fatalln(err)
		}

		err = os.MkdirAll(testFolder, os.ModePerm)
		if err != nil {
			log.Fatalln(err)
		}

		disp := download.NewDisplay()
		provider := mp3party.NewProvider()
		downloader := download.New(provider, downloadWorkers, testFolder).WithDisplay(disp)

		queries := buildQueries(songList)
		downloader.Run(queries)
	},
}

func buildQueries(songs []repository.Song) []string {
	out := make([]string, 0, len(songs))
	for _, song := range songs {
		var artists []string
		err := json.NewDecoder(strings.NewReader(song.Artists)).Decode(&artists)
		if err != nil {
			log.Printf("decode artists for song %q: %v", song.Title, err)
			continue
		}
		out = append(out, strings.Join(artists, " ")+" "+song.Title)
	}
	return out
}

func Execute() {
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(syncCmd)
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
