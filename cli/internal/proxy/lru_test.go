package proxy

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestPoolSelectsLeastRecentlyUsedAvailableProxy(t *testing.T) {
	addresses := []string{"http://203.0.113.1:80", "http://203.0.113.2:80", "http://203.0.113.3:80"}
	pool, err := NewPool(addresses)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		address, err := pool.Next()
		if err != nil || address != addresses[i%3] {
			t.Fatalf("selection %d: %s, want %s, err=%v", i, address, addresses[i%3], err)
		}
	}
}

func Test429ProxyIsSkippedUntilItsCooldownExpires(t *testing.T) {
	a, b := "http://203.0.113.1:80", "http://203.0.113.2:80"
	pool, err := NewPool([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	pool.now = func() time.Time { return now }
	pool.Next()
	pool.ReportRateLimit(a)
	for i := 0; i < 5; i++ {
		address, err := pool.Next()
		if err != nil || address != b {
			t.Fatalf("blocked proxy reused: %s %v", address, err)
		}
	}
	now = now.Add(5 * time.Second)
	address, err := pool.Next()
	if err != nil || address != a {
		t.Fatalf("oldest expired proxy not selected: %s %v", address, err)
	}
}

func TestPerProxy429CooldownProgressesAndCapsAt30Seconds(t *testing.T) {
	a, b := "http://203.0.113.1:80", "http://203.0.113.2:80"
	pool, err := NewPool([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	pool.now = func() time.Time { return now }
	for _, seconds := range []int{5, 10, 15, 20, 25, 30, 30} {
		if address, _ := pool.Next(); address != a {
			t.Fatalf("expected expired oldest proxy, got %s", address)
		}
		pool.ReportRateLimit(a)
		pool.ReportRateLimit(a) // Concurrent responses in one active pause do not lengthen it.
		now = now.Add(time.Duration(seconds)*time.Second - time.Nanosecond)
		if address, _ := pool.Next(); address != b {
			t.Fatal("cooldown ended early")
		}
		now = now.Add(time.Nanosecond)
	}
}

func TestPoolWaitsWithCancellationWhenEveryProxyIsBlocked(t *testing.T) {
	pool, err := NewPool([]string{"http://203.0.113.1:80"})
	if err != nil {
		t.Fatal(err)
	}
	pool.ReportRateLimit(pool.List()[0])
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	address, err := pool.NextContext(ctx)
	if address != "" || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked pool bypassed: %s %v", address, err)
	}
	canceled, stop := context.WithCancel(context.Background())
	stop()
	fresh, _ := NewPool([]string{"http://203.0.113.2:80"})
	if _, err := fresh.NextContext(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled request selected a proxy")
	}
}

func TestBlockedPoolResumesAfterEarliestPause(t *testing.T) {
	pool, err := NewPool([]string{"http://203.0.113.1:80"})
	if err != nil {
		t.Fatal(err)
	}
	pool.ReportRateLimit(pool.List()[0])
	pool.state[0].blockedUntil = time.Now().Add(20 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	address, err := pool.NextContext(ctx)
	if err != nil || address != pool.List()[0] {
		t.Fatalf("blocked pool did not resume: %s %v", address, err)
	}
	if time.Now().Before(pool.state[0].blockedUntil) {
		t.Fatal("proxy selected before pause expired")
	}
}

func TestConcurrentSelectionsAtomicallyUpdateRecencyAnd429(t *testing.T) {
	var addresses []string
	for i := 0; i < 100; i++ {
		addresses = append(addresses, fmt.Sprintf("http://203.0.113.1:%d", 10000+i))
	}
	pool, err := NewPool(addresses)
	if err != nil {
		t.Fatal(err)
	}
	selected := make(chan string, 100)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			address, err := pool.Next()
			if err != nil {
				t.Error(err)
				return
			}
			selected <- address
			pool.ReportRateLimit(address)
		}()
	}
	wg.Wait()
	close(selected)
	seen := make(map[string]bool)
	for address := range selected {
		if seen[address] {
			t.Fatalf("concurrent LRU reused %s", address)
		}
		seen[address] = true
	}
	if len(seen) != 100 {
		t.Fatal("not all proxies selected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := pool.NextContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("concurrent 429 reports lost: %v", err)
	}
}
