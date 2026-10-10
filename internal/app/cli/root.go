package cli

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/pprof"
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
		if allocsEnabled, _ := cmd.Flags().GetBool("allocs"); allocsEnabled {
			allocF, err := os.Create("meshvault_allocs.pprof")
			if err != nil {
				log.Printf("create allocs profile: %v", err)
			} else {
				defer allocF.Close()
				if err := pprof.Lookup("allocs").WriteTo(allocF, 0); err != nil {
					log.Printf("write allocs profile: %v", err)
				} else {
					log.Println("pprof: allocs profile saved to meshvault_allocs.pprof")
				}
			}
		}

		if pprofEnabled, _ := cmd.Flags().GetBool("pprof"); pprofEnabled {
			f, err := os.Create("meshvault_cpu.pprof")
			if err != nil {
				log.Fatalf("create cpu profile: %v", err)
			}
			defer func() {
				pprof.StopCPUProfile()
				f.Close()
				log.Println("pprof: cpu profile saved to meshvault_cpu.pprof")

				memF, err := os.Create("meshvault_mem.pprof")
				if err != nil {
					log.Printf("create mem profile: %v", err)
					return
				}
				defer memF.Close()
				if err := pprof.WriteHeapProfile(memF); err != nil {
					log.Printf("write mem profile: %v", err)
					return
				}
				log.Println("pprof: mem profile saved to meshvault_mem.pprof")
			}()
			pprof.StartCPUProfile(f)
		}

		database, err := db.New()
		if err != nil {
			log.Fatalln(err)
		}
		defer database.Close()

		repo := repository.NewSongRepository(database)

		songList, err := repo.GetAll()
		if err != nil {
			log.Fatalln(err)
		}
		databasePath, err := filepath.Abs("meshtune.db")
		if err != nil {
			log.Fatalln(err)
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "База: %s\nТреков в базе: %d\n", databasePath, len(songList))
		if len(songList) == 0 {
			fmt.Fprintln(out, "В базе нет треков. Сначала выполните meshvault sync.")
			return
		}

		err = os.MkdirAll(testFolder, os.ModePerm)
		if err != nil {
			log.Fatalln(err)
		}

		disp := download.NewDisplay()
		store, err := repository.NewDownloadedTracksRepository(database)
		if err != nil {
			log.Fatalln("инициализация истории скачиваний:", err)
		}
		provider := mp3party.NewProvider()
		downloader := download.New(provider, downloadWorkers, testFolder).WithDownloadStore(store).WithDisplay(disp)

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
	rootCmd.Flags().Bool("pprof", false, "Save cpu+mem profile to meshvault_{cpu,mem}.pprof")
	rootCmd.Flags().Bool("allocs", false, "Save allocs profile to meshvault_allocs.pprof")
}
