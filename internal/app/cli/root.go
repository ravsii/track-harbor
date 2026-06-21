package cli

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"meshvault/internal/db"
	"meshvault/internal/download/mp3party"
	"meshvault/internal/repository"
)

const (
	testFolder      = "tmp_music"
	downloadWorkers = 10
)

// rootCmd represents the base command when called without any subcommands
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

		sem := make(chan struct{}, downloadWorkers)
		var wg sync.WaitGroup

		for _, song := range songList {
			wg.Add(1)
			sem <- struct{}{}

			go func(s repository.Song) {
				defer wg.Done()
				defer func() { <-sem }()

				processSong(s)
			}(song)
		}

		wg.Wait()
	},
}

func processSong(song repository.Song) {
	var artists []string

	err := json.NewDecoder(strings.NewReader(song.Artists)).Decode(&artists)
	if err != nil {
		log.Printf("decode artists for song %q: %v", song.Title, err)
		return
	}

	name := strings.Join(artists, " ") + " " + song.Title
	fmt.Println(name)

	items, err := mp3party.FindMusics(name)
	if err != nil {
		fmt.Println("err search:", err)
		return
	}

	if len(items) == 0 {
		fmt.Println("no results for", song.Title)
		return
	}

	for _, item := range items {
		bytes, err := mp3party.Download(item)
		if err != nil {
			fmt.Println("err download:", err)
			continue
		}

		err = os.WriteFile(path.Join(testFolder, item.SongID+".mp3"), bytes, 0o666)
		if err != nil {
			fmt.Println("err write:", err)
			continue
		}
	}
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(syncCmd)
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.meshvault.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
