package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"meshvault/internal/config"
	"meshvault/internal/db"
	"meshvault/internal/download"
	"meshvault/internal/proxy"
	"meshvault/internal/repository"
)

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Работа с личными и публичными прокси",
}

var proxyTestCmd = &cobra.Command{
	Use:          "test",
	Short:        "Проверить личные прокси и публичный кеш",
	Long:         "Проверить HTTPS-доступ к YouTube через все my_proxy и актуальные адреса в кеше. Если рабочие кешированные адреса есть, источники не запрашиваются. Иначе публичный пул обновляется.",
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
				fmt.Fprintln(cmd.ErrOrStderr(), "Ошибка журнала:", err)
			}
		}()
		logger.Info("proxy_test_started")
		ctx := cmd.Context()
		out := cmd.OutOrStdout()
		fmt.Fprintln(out, "Проверка HTTPS-доступа к YouTube. Журнал: logs/proxy-test.jsonl")
		personal, err := checkProxyGroup(ctx, out, logger, "Личные", cfg.MyProxy, proxy.NewPersonalHTTPSChecker())
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
			return fmt.Errorf("чтение кеша прокси: %w", err)
		}
		public, err := checkProxyGroup(ctx, out, logger, "Кеш", cached, proxy.NewHTTPSChecker())
		if err != nil {
			return err
		}
		if public > 0 {
			fmt.Fprintln(out, "В кеше есть рабочие прокси: списки не обновляются, срок кеша не продлевается.")
		} else {
			fmt.Fprintln(out, "В кеше нет рабочих прокси. Загружаю свежие списки и проверяю публичный пул…")
			pool, err := proxy.NewManager(proxy.NewSources(logger), proxy.NewHTTPSChecker(), store, logger).Refresh(ctx, func(checked, total, valid int) {
				if total > 0 && (checked%100 == 0 || checked == total) {
					fmt.Fprintf(out, "Проверка публичных: %d/%d, доступных: %d\n", checked, total, valid)
				}
			})
			if err != nil {
				logger.Error("proxy_test_refresh_failed", zap.Error(err))
				fmt.Fprintf(out, "Личных доступно: %d/%d.\n", personal, len(cfg.MyProxy))
				return fmt.Errorf("обновление публичных прокси: %w", err)
			}
			public = pool.Len()
			for _, address := range pool.List() {
				fmt.Fprintf(out, "  Публичный: %s — доступен\n", download.ProxyLabel(address))
			}
		}
		fmt.Fprintf(out, "Итого доступных: личных %d/%d, публичных %d.\n", personal, len(cfg.MyProxy), public)
		logger.Info("proxy_test_finished", zap.Int("personal_available", personal), zap.Int("public_available", public))
		return nil
	},
}

// Output is written by the collecting goroutine only; credentials and raw
// network errors never reach the terminal or structured log.
func checkProxyGroup(ctx context.Context, out io.Writer, logger *zap.Logger, group string, addresses []string, checker proxy.Checker) (int, error) {
	fmt.Fprintf(out, "%s прокси: %d\n", group, len(addresses))
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
		status := "доступен"
		if result.err == nil {
			available++
		} else {
			status = "нет HTTPS-доступа к YouTube"
			var networkErr net.Error
			if errors.As(result.err, &networkErr) && networkErr.Timeout() {
				status = "таймаут"
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
