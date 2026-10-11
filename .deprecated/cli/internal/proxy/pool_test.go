package proxy

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"trackharbor/cli/internal/repository"
)

type sourceFunc func(context.Context) ([]string, error)

func (f sourceFunc) Fetch(ctx context.Context) ([]string, error) { return f(ctx) }

type checkFunc func(context.Context, string) error

func (f checkFunc) Check(ctx context.Context, address string) error { return f(ctx, address) }

func proxyDB(t *testing.T) (*sqlx.DB, *repository.ProxyRepository) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", filepath.Join(t.TempDir(), "proxy.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store, err := repository.NewProxyRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	return db, store
}

func TestPreparationCachesOnlyValidProxiesForOneHour(t *testing.T) {
	_, store := proxyDB(t)
	checked := 0
	source := sourceFunc(func(context.Context) ([]string, error) {
		return []string{"http://203.0.113.1:8080", "http://203.0.113.2:8080"}, nil
	})
	checker := checkFunc(func(_ context.Context, address string) error {
		if address == "http://203.0.113.2:8080" {
			return errors.New("offline")
		}
		return nil
	})
	started := time.Now()
	pool, err := NewManager(source, checker, store, nil).Prepare(context.Background(), func(done, total, valid int) { checked = done })
	if err != nil || pool.Len() != 1 || checked != 2 {
		t.Fatalf("pool=%v checked=%d err=%v", pool, checked, err)
	}
	valid, err := store.Valid(context.Background(), started.Add(59*time.Minute))
	if err != nil || len(valid) != 1 {
		t.Fatalf("cache expired early: %v %v", valid, err)
	}
	// A fresh cache bypasses both the list API and probes.
	noSource := sourceFunc(func(context.Context) ([]string, error) {
		t.Error("fresh cache fetched API")
		return nil, errors.New("unexpected")
	})
	noCheck := checkFunc(func(context.Context, string) error {
		t.Error("fresh cache was probed")
		return errors.New("unexpected")
	})
	if _, err := NewManager(noSource, noCheck, store, nil).Prepare(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	valid, err = store.Valid(context.Background(), time.Now().Add(time.Hour+time.Second))
	if err != nil || len(valid) != 0 {
		t.Fatal("cache persisted beyond one hour")
	}
}

func TestExpiredCacheRefetchesAndProbes(t *testing.T) {
	_, store := proxyDB(t)
	if err := store.Save(context.Background(), "http://203.0.113.1:8080", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	pool, err := NewManager(sourceFunc(func(context.Context) ([]string, error) { calls.Add(1); return []string{"http://203.0.113.2:8080"}, nil }), checkFunc(func(context.Context, string) error { calls.Add(1); return nil }), store, nil).Prepare(context.Background(), nil)
	if err != nil || pool.Len() != 1 || calls.Load() != 2 {
		t.Fatalf("expired cache reused: %v %v", calls.Load(), err)
	}
}

func TestProbePoolChecksEntireBatchWithConcurrencyAt1000(t *testing.T) {
	_, store := proxyDB(t)
	var urls []string
	for i := 0; i < 2100; i++ {
		urls = append(urls, fmt.Sprintf("http://203.0.113.%d:%d", i%200+1, 10000+i))
	}
	var mu sync.Mutex
	active, maximum, checked := 0, 0, 0
	ready := make(chan struct{})
	checker := checkFunc(func(ctx context.Context, address string) error {
		mu.Lock()
		active++
		maximum = max(maximum, active)
		checked++
		if checked == 1000 {
			close(ready)
		}
		mu.Unlock()
		select {
		case <-ready:
		case <-ctx.Done():
			return ctx.Err()
		}
		mu.Lock()
		active--
		mu.Unlock()
		return errors.New("offline")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := NewManager(sourceFunc(func(context.Context) ([]string, error) { return urls, nil }), checker, store, nil).Prepare(ctx, nil)
	if err == nil || checked != 2100 || maximum != 1000 {
		t.Fatalf("checked=%d max=%d err=%v", checked, maximum, err)
	}
}

func TestOldPartialCacheTriggersFullDiscovery(t *testing.T) {
	_, store := proxyDB(t)
	if err := store.Save(context.Background(), "http://203.0.113.1:8080", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var fetched bool
	pool, err := NewManager(sourceFunc(func(context.Context) ([]string, error) {
		fetched = true
		return []string{"socks5://203.0.113.2:8080"}, nil
	}), checkFunc(func(context.Context, string) error { return nil }), store, nil).Prepare(context.Background(), nil)
	if err != nil || !fetched || pool.Len() != 1 {
		t.Fatalf("old cache prevented discovery: fetched=%v err=%v", fetched, err)
	}
	cached, err := NewManager(nil, nil, store, nil).Prepare(context.Background(), nil)
	if err != nil || cached.Len() != 1 || cached.List()[0] != "socks5://203.0.113.2:8080" {
		t.Fatalf("old failed proxy survived refresh: pool=%v err=%v", cached, err)
	}
}

type expiryStore struct {
	*repository.ProxyRepository
	earliest      time.Time
	completeUntil time.Time
}

func (s *expiryStore) Save(ctx context.Context, address string, until time.Time) error {
	if s.earliest.IsZero() || until.Before(s.earliest) {
		s.earliest = until
	}
	return s.ProxyRepository.Save(ctx, address, until)
}
func (s *expiryStore) Complete(ctx context.Context, until time.Time) error {
	s.completeUntil = until
	return s.ProxyRepository.Complete(ctx, until)
}
func TestFullCacheExpiresNoLaterThanFirstVerifiedProxy(t *testing.T) {
	_, repository := proxyDB(t)
	store := &expiryStore{ProxyRepository: repository}
	_, err := NewManager(sourceFunc(func(context.Context) ([]string, error) { return []string{"http://203.0.113.1:80"}, nil }), checkFunc(func(context.Context, string) error { return nil }), store, nil).Prepare(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if store.completeUntil.After(store.earliest) {
		t.Fatal("full cache marker outlives earliest proxy")
	}
}

func TestLRUPoolUsesEveryAvailableProxyBeforeRepeating(t *testing.T) {
	pool, err := NewPool([]string{"http://203.0.113.1:80", "http://203.0.113.2:80", "http://203.0.113.3:80"})
	if err != nil {
		t.Fatal(err)
	}
	previous := ""
	for cycle := 0; cycle < 10; cycle++ {
		seen := make(map[string]bool)
		for i := 0; i < 3; i++ {
			address, err := pool.Next()
			if err != nil || seen[address] || address == previous {
				t.Fatalf("proxy repeated too soon: %s %v", address, err)
			}
			seen[address] = true
			previous = address
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := pool.Next(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

func TestEmptyPoolAndCancellationDoNotFallBackToDirectConnection(t *testing.T) {
	if _, err := NewPool(nil); err == nil {
		t.Fatal("empty proxy pool accepted")
	}
	_, store := proxyDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewManager(sourceFunc(func(context.Context) ([]string, error) { t.Error("canceled preparation fetched API"); return nil, nil }), nil, store, nil).Prepare(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
