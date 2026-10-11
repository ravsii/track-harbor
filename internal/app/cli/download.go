package cli

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"

	"go.uber.org/zap"

	"github.com/spf13/cobra"
	"meshvault/internal/db"
	"meshvault/internal/download"
	"meshvault/internal/download/youtube"
	"meshvault/internal/proxy"
	"meshvault/internal/repository"
)

const (
	testFolder             = "tmp_music"
	defaultDownloadWorkers = 50
)

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Скачать все треки из базы",
	Long:  "Скачать все треки из meshtune.db в tmp_music через YouTube. Стратегия: direct (по умолчанию), proxy или fallback. Параллельность задаётся --workers (по умолчанию 50), при доступном пуле прокси ограничивается его размером.",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if _, err := downloadWorkerLimit(cmd); err != nil {
			return err
		}
		_, err := downloadStrategyName(cmd)
		return err
	},

	Run: func(cmd *cobra.Command, args []string) {
		workerLimit, err := downloadWorkerLimit(cmd)
		if err != nil {
			log.Fatal(err)
		}
		strategyName, err := downloadStrategyName(cmd)
		if err != nil {
			log.Fatal(err)
		}
		logger, closeLog, err := download.NewFileLogger("logs/download.jsonl")
		if err != nil {
			log.Fatalln("создание журнала:", err)
		}
		logger.Info("command_started")
		defer func() {
			logger.Info("command_finished")
			if err := closeLog(); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Ошибка журнала скачивания: %v\n", err)
			}
		}()
		fatal := func(err error) {
			logger.Error("command_failed", zap.Error(err))
			if logErr := closeLog(); logErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Ошибка журнала скачивания: %v\n", logErr)
			}
			log.Fatal(err)
		}
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
				fatal(fmt.Errorf("create cpu profile: %w", err))
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
			fatal(err)
		}
		defer database.Close()

		repo := repository.NewSongRepository(database)

		songList, err := repo.GetAll()
		if err != nil {
			fatal(err)
		}
		databasePath, err := filepath.Abs("meshtune.db")
		if err != nil {
			fatal(err)
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "База: %s\nТреков в базе: %d\n", databasePath, len(songList))
		fmt.Fprintln(out, "Журнал: logs/download.jsonl")
		logger.Info("database_loaded", zap.String("database", databasePath), zap.Int("total", len(songList)))
		if len(songList) == 0 {
			fmt.Fprintln(out, "В базе нет треков. Сначала выполните meshvault sync.")
			return
		}

		err = os.MkdirAll(testFolder, os.ModePerm)
		if err != nil {
			fatal(err)
		}

		disp := download.NewDisplay()
		store, err := repository.NewDownloadedTracksRepository(database)
		if err != nil {
			fatal(fmt.Errorf("инициализация истории скачиваний: %w", err))
		}
		provider, err := youtube.NewProvider()
		if err != nil {
			fatal(err)
		}
		fmt.Fprintf(out, "Источник: %s\n", provider.Name())
		fmt.Fprintf(out, "Стратегия: %s\n", strategyName)
		var connection youtube.DownloadStrategy = youtube.Direct{}
		workers, proxyCount := workerLimit, 0
		if strategyName != "direct" {
			var pool *proxy.Pool
			proxyStore, poolErr := repository.NewProxyRepository(database)
			if poolErr == nil {
				fmt.Fprintln(out, "Прокси: загрузка кеша или списков ProxyScrape + Proxifly + TheSpeedX (1000 проверок параллельно, таймаут 1 с)…")
				pool, poolErr = proxy.NewManager(proxy.NewSources(logger), proxy.NewHTTPSChecker(), proxyStore, logger).Prepare(cmd.Context(), func(checked, total, valid int) {
					if total == 0 && valid > 0 {
						fmt.Fprintf(out, "Прокси из кеша: %d (кеш действует 1 час)\n", valid)
					}
					if total > 0 && (checked%100 == 0 || checked == total) {
						fmt.Fprintf(out, "Проверка прокси: %d/%d, валидных: %d\n", checked, total, valid)
					}
				})
			}
			if poolErr != nil {
				if strategyName == "proxy" {
					fatal(fmt.Errorf("подготовка прокси: %w", poolErr))
				}
				logger.Warn("proxy_reserve_unavailable", zap.Error(poolErr))
				fmt.Fprintln(out, "Резерв прокси недоступен; прямые запросы продолжают работать. Ошибка:", poolErr)
			} else {
				proxyCount = pool.Len()
				workers = effectiveDownloadWorkers(proxyCount, workerLimit)
				fmt.Fprintf(out, "Пул прокси готов: %d. Выбор LRU с учётом пауз HTTP 429.\n", proxyCount)
			}
			proxyStrategy := youtube.ProxyOnly{Pool: pool, Unavailable: poolErr}
			connection = proxyStrategy
			if strategyName == "fallback" {
				connection = youtube.DirectWithProxyFallback{Proxy: proxyStrategy, OnFallback: func(err error) { logger.Warn("proxy_fallback", zap.Error(err)) }}
			}
		}
		provider.WithStrategy(connection)
		fmt.Fprintf(out, "Параллельность поиска и скачивания: %d (лимит --workers: %d).\n", workers, workerLimit)
		logger.Info("workers_configured", zap.String("strategy", strategyName), zap.Int("workers", workers), zap.Int("proxy_count", proxyCount), zap.Int("workers_limit", workerLimit))
		downloader := download.New(provider, workers, testFolder).WithDownloadStore(store).WithLogger(logger).WithDisplay(disp)

		queries := buildQueriesLogged(songList, logger)
		logger.Info("queries_prepared", zap.Int("total", len(queries)))
		downloader.Run(queries)
	},
}

func buildQueries(songs []repository.Song) []string {
	return buildQueriesLogged(songs, nil)
}

func buildQueriesLogged(songs []repository.Song, logger *zap.Logger) []string {
	out := make([]string, 0, len(songs))
	for _, song := range songs {
		var artists []string
		err := json.NewDecoder(strings.NewReader(song.Artists)).Decode(&artists)
		if err != nil {
			log.Printf("decode artists for song %q: %v", song.Title, err)
			if logger != nil {
				logger.Warn("query_rejected", zap.String("query", song.Title), zap.Error(err))
			}
			continue
		}
		out = append(out, strings.Join(artists, " ")+" "+song.Title)
	}
	return out
}

func init() {
	downloadCmd.Flags().String("strategy", "direct", "Стратегия соединения: direct, proxy или fallback")
	downloadCmd.Flags().Int("workers", defaultDownloadWorkers, "Лимит параллельных задач (>0); при доступном пуле прокси не больше его размера")
	downloadCmd.Flags().Bool("pprof", false, "Save cpu+mem profile to meshvault_{cpu,mem}.pprof")
	downloadCmd.Flags().Bool("allocs", false, "Save allocs profile to meshvault_allocs.pprof")
	rootCmd.AddCommand(downloadCmd)
}

func downloadWorkerLimit(cmd *cobra.Command) (int, error) {
	limit, err := cmd.Flags().GetInt("workers")
	if err != nil {
		return 0, err
	}
	if limit <= 0 {
		return 0, fmt.Errorf("--workers должен быть положительным числом")
	}
	return limit, nil
}

func effectiveDownloadWorkers(proxyCount, limit int) int {
	return min(proxyCount, limit)
}

func downloadStrategyName(cmd *cobra.Command) (string, error) {
	name, err := cmd.Flags().GetString("strategy")
	if err != nil {
		return "", err
	}
	switch name {
	case "direct", "proxy", "fallback":
		return name, nil
	}
	return "", fmt.Errorf("--strategy: ожидается direct, proxy или fallback, получено %q", name)
}
