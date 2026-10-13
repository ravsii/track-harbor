package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/ravsii/trackharbor/cli/internal/config"
	"github.com/ravsii/trackharbor/cli/internal/db"
	"github.com/ravsii/trackharbor/cli/internal/download"
	"github.com/ravsii/trackharbor/cli/internal/proxy"
	"github.com/ravsii/trackharbor/cli/internal/repository"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Manage personal and public proxies",
}

var proxyTestCmd = &cobra.Command{
	Use:          "test",
	Short:        "Check personal proxies and the public proxy cache",
	Long:         "Check HTTPS access to YouTube through all personal_proxy addresses and unexpired cached proxies. If any cached proxies work, source lists are not fetched. Otherwise, the public pool is refreshed.",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		logger, closeLog, err := download.NewFileLogger("logs/proxy-test.jsonl")
		if err != nil {
			return err
		}
		defer func() {
			if err := closeLog(); err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "Log error:", err)
			}
		}()
		logger.Info("proxy_test_started")
		ctx := cmd.Context()
		out := cmd.OutOrStdout()
		fmt.Fprintln(out, "Checking HTTPS access to YouTube. Log: logs/proxy-test.jsonl")
		personal, err := checkProxyGroup(ctx, out, logger, "Personal", cfg.PersonalProxy, proxy.NewPersonalHTTPSChecker())
		if err != nil {
			return err
		}
		database, err := db.New()
		if err != nil {
			return err
		}
		defer database.Close()
		store, err := repository.NewProxyRepository(database)
		if err != nil {
			return err
		}
		cached, err := store.Valid(ctx, time.Now())
		if err != nil {
			return fmt.Errorf("read proxy cache: %w", err)
		}
		public, err := checkProxyGroup(ctx, out, logger, "Cached", cached, proxy.NewHTTPSChecker())
		if err != nil {
			return err
		}
		if public > 0 {
			fmt.Fprintln(out, "Working cached proxies found: lists are not refreshed and cache expiry is unchanged.")
		} else {
			fmt.Fprintln(out, "No working cached proxies. Fetching fresh lists and checking the public pool…")
			pool, err := proxy.NewManager(proxy.NewSources(logger), proxy.NewHTTPSChecker(), store, logger).Refresh(ctx, func(checked, total, valid int) {
				if total > 0 && (checked%100 == 0 || checked == total) {
					fmt.Fprintf(out, "Checking public proxies: %d/%d, available: %d\n", checked, total, valid)
				}
			})
			if err != nil {
				logger.Error("proxy_test_refresh_failed", zap.Error(err))
				fmt.Fprintf(out, "Personal proxies available: %d/%d.\n", personal, len(cfg.PersonalProxy))
				return fmt.Errorf("refresh public proxies: %w", err)
			}
			public = pool.Len()
			for _, address := range pool.List() {
				fmt.Fprintf(out, "  Public: %s — available\n", download.ProxyLabel(address))
			}
		}
		fmt.Fprintf(out, "Total available: personal %d/%d, public %d.\n", personal, len(cfg.PersonalProxy), public)
		logger.Info("proxy_test_finished", zap.Int("personal_available", personal), zap.Int("public_available", public))
		return nil
	},
}

// Output is written by the collecting goroutine only; credentials and raw
// network errors never reach the terminal or structured log.
func checkProxyGroup(ctx context.Context, out io.Writer, logger *zap.Logger, group string, addresses []string, checker proxy.Checker) (int, error) {
	fmt.Fprintf(out, "%s proxies: %d\n", group, len(addresses))
	type result struct {
		address  string
		err      error
		duration time.Duration
	}
	jobs := make(chan string, len(addresses))
	for _, address := range addresses {
		jobs <- address
	}
	close(jobs)
	results := make(chan result, proxy.ProbeWorkers)
	var wg sync.WaitGroup
	for i := 0; i < min(proxy.ProbeWorkers, len(addresses)); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for address := range jobs {
				if ctx.Err() != nil {
					return
				}
				started := time.Now()
				err := checker.Check(ctx, address)
				select {
				case results <- result{address, err, time.Since(started)}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() { wg.Wait(); close(results) }()
	available := 0
	for result := range results {
		if ctx.Err() != nil {
			continue
		}
		label := download.ProxyLabel(result.address)
		status := "available"
		if result.err == nil {
			available++
		} else {
			status = "no HTTPS access to YouTube"
			var networkErr net.Error
			if errors.As(result.err, &networkErr) && networkErr.Timeout() {
				status = "timed out"
			}
		}
		fmt.Fprintf(out, "  %s: %s — %s (%s)\n", group, label, status, result.duration.Round(time.Millisecond))
		logger.Info("proxy_checked", zap.String("group", group), zap.String("proxy", label), zap.Bool("available", result.err == nil), zap.String("status", status), zap.Duration("duration", result.duration))
	}
	if err := ctx.Err(); err != nil {
		return available, err
	}
	return available, nil
}

func init() {
	proxyCmd.AddCommand(proxyTestCmd)
	rootCmd.AddCommand(proxyCmd)
}
