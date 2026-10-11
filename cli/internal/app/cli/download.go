package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/spf13/cobra"
	"trackharbor/cli/internal/config"
	"trackharbor/cli/internal/db"
	"trackharbor/cli/internal/download"
	"trackharbor/cli/internal/download/youtube"
	"trackharbor/cli/internal/proxy"
	"trackharbor/cli/internal/repository"
)

const (
	defaultDownloadWorkers = 50
)

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download all tracks from the database",
	Long:  "Download Spotify tracks from YouTube using strategy (proxy priority) and VK tracks directly through FFmpeg using vk_strategy. Set both in ~/.trackharbor/config.yaml. Use --workers to set YouTube concurrency (default: 50).",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if _, err := downloadWorkerLimit(cmd); err != nil {
			return err
		}
		_, err := config.Load()
		return err
	},

	Run: func(cmd *cobra.Command, args []string) {
		workerLimit, err := downloadWorkerLimit(cmd)
		if err != nil {
			log.Fatal(err)
		}
		cfg, err := config.Load()
		if err != nil {
			log.Fatal(err)
		}
		logger, closeLog, err := download.NewFileLogger("logs/download.jsonl")
		if err != nil {
			log.Fatalln("create log:", err)
		}
		logger.Info("command_started")
		defer func() {
			logger.Info("command_finished")
			if err := closeLog(); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Download log error: %v\n", err)
			}
		}()
		fatal := func(err error) {
			logger.Error("command_failed", zap.Error(err))
			if logErr := closeLog(); logErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Download log error: %v\n", logErr)
			}
			log.Fatal(err)
		}
		if allocsEnabled, _ := cmd.Flags().GetBool("allocs"); allocsEnabled {
			allocF, err := os.Create("trackharbor_allocs.pprof")
			if err != nil {
				log.Printf("create allocs profile: %v", err)
			} else {
				defer allocF.Close()
				if err := pprof.Lookup("allocs").WriteTo(allocF, 0); err != nil {
					log.Printf("write allocs profile: %v", err)
				} else {
					log.Println("pprof: allocs profile saved to trackharbor_allocs.pprof")
				}
			}
		}

		if pprofEnabled, _ := cmd.Flags().GetBool("pprof"); pprofEnabled {
			f, err := os.Create("trackharbor_cpu.pprof")
			if err != nil {
				fatal(fmt.Errorf("create cpu profile: %w", err))
			}
			defer func() {
				pprof.StopCPUProfile()
				f.Close()
				log.Println("pprof: cpu profile saved to trackharbor_cpu.pprof")

				memF, err := os.Create("trackharbor_mem.pprof")
				if err != nil {
					log.Printf("create mem profile: %v", err)
					return
				}
				defer memF.Close()
				if err := pprof.WriteHeapProfile(memF); err != nil {
					log.Printf("write mem profile: %v", err)
					return
				}
				log.Println("pprof: mem profile saved to trackharbor_mem.pprof")
			}()
			pprof.StartCPUProfile(f)
		}

		database, err := db.New()
		if err != nil {
			fatal(err)
		}
		defer database.Close()

		var songList []repository.Song
		var spotifyTable int
		if err := database.Get(&spotifyTable, "SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'songs'"); err != nil {
			fatal(err)
		}
		if spotifyTable > 0 {
			songList, err = repository.NewSongRepository(database).GetAll()
			if err != nil {
				fatal(err)
			}
		}
		vkRepo, err := repository.NewVKRepository(database)
		if err != nil {
			fatal(err)
		}
		vkSongs, err := vkRepo.GetAll()
		if err != nil {
			fatal(err)
		}
		databasePath, err := config.DatabasePath()
		if err != nil {
			fatal(err)
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Database: %s\nTracks in database: %d (Spotify: %d; VK: %d)\n", databasePath, len(songList)+len(vkSongs), len(songList), len(vkSongs))
		fmt.Fprintln(out, "Log: logs/download.jsonl")
		logger.Info("database_loaded", zap.String("database", databasePath), zap.Int("total", len(songList)))
		if len(songList)+len(vkSongs) == 0 {
			fmt.Fprintln(out, "No tracks in the database. Run trackharbor spotify sync or trackharbor vk sync first.")
			return
		}

		if err := prepareDownloadDirectory(cfg.DownloadDir); err != nil {
			fatal(err)
		}
		fmt.Fprintf(out, "Download directory: %s\n", cfg.DownloadDir)
		logger.Info("download_directory", zap.String("path", cfg.DownloadDir))

		disp := download.NewDisplay()
		store, err := repository.NewDownloadedTracksRepository(database)
		if err != nil {
			fatal(fmt.Errorf("initialize download history: %w", err))
		}
		if err := restoreDownloadQueries(store); err != nil {
			fatal(err)
		}
		if len(vkSongs) > 0 {
			if err := downloadVKTracks(vkRepo, store, cfg.DownloadDir, logger, out); err != nil {
				fatal(err)
			}
		}
		if len(songList) == 0 {
			return // VK-only downloads do not initialize YouTube or its proxies.
		}
		queries := buildQueriesLogged(songList, logger)
		pending := make([]string, 0, len(queries))
		skipped := 0
		for _, query := range queries {
			path, err := download.DownloadedQueryPath(store, "youtube", query, cfg.DownloadDir)
			if err != nil {
				fatal(fmt.Errorf("check download history: %w", err))
			}
			if path == "" {
				pending = append(pending, query)
				continue
			}
			skipped++
			logger.Info("skipped", zap.String("source", "youtube"), zap.String("query", query), zap.String("file_path", path), zap.String("reason", "file_exists_nonempty"))
		}
		fmt.Fprintf(out, "Already downloaded: %d. Remaining: %d.\n", skipped, len(pending))
		logger.Info("queries_prepared", zap.Int("total", len(queries)), zap.Int("skipped", skipped), zap.Int("pending", len(pending)))
		if len(pending) == 0 {
			fmt.Fprintln(out, "All tracks are already downloaded: no searches or connections needed.")
			disp.SetInitialSkipped(skipped)
			disp.SetTotal(len(queries))
			disp.Stop()
			return
		}
		provider, err := youtube.NewProvider()
		if err != nil {
			fatal(err)
		}
		provider.WithSourceDirectory(filepath.Join(cfg.DownloadDir, ".sources"))
		fmt.Fprintf(out, "Source: %s\n", provider.Name())
		strategyName := strings.Join(cfg.Strategy, " -> ")
		fmt.Fprintf(out, "Spotify strategies: %s\n", strategyName)
		var proxyStrategy youtube.ProxyOnly
		workers, proxyCount := workerLimit, 0
		usesProxy, usesDirect, usesPersonalProxy := false, false, false
		for _, name := range cfg.Strategy {
			usesProxy = usesProxy || name == "random_proxy"
			usesDirect = usesDirect || name == "direct"
			usesPersonalProxy = usesPersonalProxy || name == "personal_proxy"
		}
		if usesPersonalProxy {
			fmt.Fprintf(out, "Checking personal proxies: %d (HTTPS to YouTube, 5 s timeout per address)…\n", len(cfg.PersonalProxy))
			checker := proxy.NewPersonalHTTPSChecker()
			available := make([]string, 0, len(cfg.PersonalProxy))
			for _, address := range cfg.PersonalProxy {
				started := time.Now()
				err := checker.Check(cmd.Context(), address)
				if cmd.Context().Err() != nil {
					fatal(cmd.Context().Err())
				}
				label := download.ProxyLabel(address)
				fields := []zap.Field{zap.String("proxy", label), zap.Duration("duration", time.Since(started)), zap.Bool("available", err == nil)}
				if err != nil {
					// Network errors can embed the full authenticated proxy URL.
					// Report only a safe diagnosis, never persist raw errors here.
					reason := "no verified HTTPS access to YouTube"
					var networkErr net.Error
					if errors.As(err, &networkErr) && networkErr.Timeout() {
						reason = "check timed out"
					}
					fmt.Fprintf(out, "  %s: unavailable (%s); excluded for this run.\n", label, reason)
					logger.Warn("personal_proxy_checked", append(fields, zap.String("reason", reason))...)
					continue
				}
				available = append(available, address)
				fmt.Fprintf(out, "  %s: available (%s).\n", label, time.Since(started).Round(time.Millisecond))
				logger.Info("personal_proxy_checked", fields...)
			}
			cfg.PersonalProxy = available // Do not change the user's YAML; recheck on every run.
			proxyCount = len(available)
			fmt.Fprintf(out, "Available personal proxies: %d.\n", proxyCount)
		}
		if usesProxy {
			var pool *proxy.Pool
			proxyStore, poolErr := repository.NewProxyRepository(database)
			if poolErr == nil {
				fmt.Fprintln(out, "Proxies: loading cache or ProxyScrape + Proxifly + TheSpeedX lists (1000 concurrent checks, 1 s timeout)…")
				pool, poolErr = proxy.NewManager(proxy.NewSources(logger), proxy.NewHTTPSChecker(), proxyStore, logger).Prepare(cmd.Context(), func(checked, total, valid int) {
					if total == 0 && valid > 0 {
						fmt.Fprintf(out, "Cached proxies: %d (cache valid for 1 hour)\n", valid)
					}
					if total > 0 && (checked%100 == 0 || checked == total) {
						fmt.Fprintf(out, "Checking proxies: %d/%d, valid: %d\n", checked, total, valid)
					}
				})
			}
			if poolErr != nil {
				if !usesDirect && proxyCount == 0 {
					fatal(fmt.Errorf("prepare proxies: %w", poolErr))
				}
				logger.Warn("proxy_reserve_unavailable", zap.Error(poolErr))
				fmt.Fprintln(out, "Public proxy pool unavailable; other strategies remain active. Error:", poolErr)
			} else {
				proxyCount += pool.Len()
				fmt.Fprintf(out, "Public proxy pool ready: %d. LRU selection respects HTTP 429 cooldowns.\n", pool.Len())
			}
			proxyStrategy = youtube.ProxyOnly{Pool: pool, Unavailable: poolErr}
		}
		if !usesDirect && proxyCount == 0 {
			fatal(fmt.Errorf("no available proxies for configured strategies; direct connections are not allowed"))
		}
		if proxyCount > 0 {
			workers = effectiveDownloadWorkers(proxyCount, workerLimit)
		}
		connection := youtube.Priority{OnFallback: func(from, to string, err error) {
			logger.Warn("strategy_fallback", zap.String("from", from), zap.String("to", to), zap.Error(err))
		}}
		for _, name := range cfg.Strategy {
			switch name {
			case "direct":
				connection.Strategies = append(connection.Strategies, youtube.Direct{})
			case "personal_proxy":
				connection.Strategies = append(connection.Strategies, youtube.PersonalProxy{Addresses: cfg.PersonalProxy})
			case "random_proxy":
				connection.Strategies = append(connection.Strategies, youtube.RandomProxy{ProxyOnly: proxyStrategy})
			}
		}
		provider.WithStrategy(connection)
		fmt.Fprintf(out, "Search and download concurrency: %d (--workers limit: %d).\n", workers, workerLimit)
		logger.Info("workers_configured", zap.String("strategy", strategyName), zap.Int("workers", workers), zap.Int("proxy_count", proxyCount), zap.Int("workers_limit", workerLimit))
		downloader := download.New(provider, workers, cfg.DownloadDir).WithDownloadStore(store).WithLogger(logger).WithDisplay(disp).WithInitialSkipped(skipped)

		downloader.Run(pending)
	},
}

func buildQueries(songs []repository.Song) []string {
	return buildQueriesLogged(songs, nil)
}

func buildQueriesLogged(songs []repository.Song, logger *zap.Logger) []string {
	out := make([]string, 0, len(songs))
	seen := make(map[string]bool)
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
		query := strings.Join(artists, " ") + " " + song.Title
		if !seen[query] {
			seen[query] = true
			out = append(out, query)
		}
	}
	return out
}

func init() {
	downloadCmd.Flags().Int("workers", defaultDownloadWorkers, "Concurrent job limit (>0); capped by the available proxy pool size")
	downloadCmd.Flags().Bool("pprof", false, "Save cpu+mem profile to trackharbor_{cpu,mem}.pprof")
	downloadCmd.Flags().Bool("allocs", false, "Save allocs profile to trackharbor_allocs.pprof")
	rootCmd.AddCommand(downloadCmd)
}

func downloadWorkerLimit(cmd *cobra.Command) (int, error) {
	limit, err := cmd.Flags().GetInt("workers")
	if err != nil {
		return 0, err
	}
	if limit <= 0 {
		return 0, fmt.Errorf("--workers must be a positive number")
	}
	return limit, nil
}

func effectiveDownloadWorkers(proxyCount, limit int) int {
	return min(proxyCount, limit)
}
