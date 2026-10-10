package proxy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

const MaxProxies = 2000
const ProbeWorkers = 100
const CacheTTL = time.Hour

type Source interface {
	Fetch(context.Context) ([]string, error)
}
type Checker interface {
	Check(context.Context, string) error
}
type Store interface {
	Valid(context.Context, time.Time) ([]string, error)
	Save(context.Context, string, time.Time) error
}

type Manager struct {
	source  Source
	checker Checker
	store   Store
	logger  *zap.Logger
}

func NewManager(source Source, checker Checker, store Store, logger *zap.Logger) *Manager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Manager{source: source, checker: checker, store: store, logger: logger}
}

func usableAddresses(input []string) []string {
	var addresses []string
	seen := make(map[string]bool)
	for _, raw := range input {
		address, err := publicProxy(raw)
		if err != nil || seen[address] {
			continue
		}
		seen[address] = true
		addresses = append(addresses, address)
		if len(addresses) == MaxProxies {
			break
		}
	}
	return addresses
}

// Prepare uses a fresh cache without probing. Otherwise it obtains one batch
// and verifies it with a fixed 100-worker pool; only successful probes persist.
func (m *Manager) Prepare(ctx context.Context, progress func(checked, total, valid int)) (*Pool, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cached, err := m.store.Valid(ctx, time.Now())
	if err != nil {
		return nil, fmt.Errorf("чтение кеша прокси: %w", err)
	}
	if cached = usableAddresses(cached); len(cached) > 0 {
		m.logger.Info("proxy_cache_loaded", zap.Int("valid", len(cached)))
		if progress != nil {
			progress(0, 0, len(cached))
		}
		return NewPool(cached)
	}
	addresses, err := m.source.Fetch(ctx)
	if err != nil {
		return nil, fmt.Errorf("ProxyScrape: %w", err)
	}
	addresses = usableAddresses(addresses)
	m.logger.Info("proxy_probe_started", zap.Int("total", len(addresses)), zap.Int("workers", ProbeWorkers))
	if progress != nil {
		progress(0, len(addresses), 0)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan string, len(addresses))
	for _, address := range addresses {
		jobs <- address
	}
	close(jobs)
	type result struct {
		address   string
		valid     bool
		checkedAt time.Time
	}
	results := make(chan result, ProbeWorkers)
	var wg sync.WaitGroup
	for i := 0; i < min(ProbeWorkers, len(addresses)); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case address, ok := <-jobs:
					if !ok {
						return
					}
					valid := m.checker.Check(ctx, address) == nil
					select {
					case results <- result{address: address, valid: valid, checkedAt: time.Now()}:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}
	go func() { wg.Wait(); close(results) }()
	defer func() { cancel(); wg.Wait() }()
	var valid []string
	checked := 0
	for result := range results {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		checked++
		if result.valid {
			if err := m.store.Save(ctx, result.address, result.checkedAt.Add(CacheTTL)); err != nil {
				return nil, fmt.Errorf("запись кеша прокси: %w", err)
			}
			valid = append(valid, result.address)
		}
		if progress != nil {
			progress(checked, len(addresses), len(valid))
		}
		if checked%100 == 0 {
			m.logger.Info("proxy_probe_progress", zap.Int("checked", checked), zap.Int("total", len(addresses)), zap.Int("valid", len(valid)))
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.logger.Info("proxy_probe_finished", zap.Int("checked", checked), zap.Int("valid", len(valid)))
	return NewPool(valid)
}
