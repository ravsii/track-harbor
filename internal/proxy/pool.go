// Package proxy discovers, verifies, caches and rotates public HTTP proxies.
package proxy

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type proxyState struct {
	lastUsed     uint64
	blockedUntil time.Time
	penalties    int
}

type Pool struct {
	mu        sync.Mutex
	addresses []string
	state     []proxyState
	sequence  uint64
	now       func() time.Time
}

func NewPool(addresses []string) (*Pool, error) {
	addresses = usableAddresses(addresses)
	if len(addresses) == 0 {
		return nil, fmt.Errorf("no verified proxies; direct connections are not used")
	}
	p := &Pool{addresses: append([]string(nil), addresses...), state: make([]proxyState, len(addresses)), now: time.Now}
	return p, nil
}

func (p *Pool) Len() int { return len(p.addresses) }

func (p *Pool) List() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.addresses...)
}

// Next selects the least recently used proxy whose estimated 429 pause expired.
func (p *Pool) Next() (string, error) {
	return p.NextContext(context.Background())
}

// NextContext waits when all proxies are cooling down. Selection and updating
// recency are atomic, including when searches and downloads share this pool.
func (p *Pool) NextContext(ctx context.Context) (string, error) {
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		p.mu.Lock()
		now := p.now()
		selected := -1
		var earliest time.Time
		for i, state := range p.state {
			if now.Before(state.blockedUntil) {
				if earliest.IsZero() || state.blockedUntil.Before(earliest) {
					earliest = state.blockedUntil
				}
				continue
			}
			if selected < 0 || state.lastUsed < p.state[selected].lastUsed {
				selected = i
			}
		}
		if selected >= 0 {
			p.sequence++
			p.state[selected].lastUsed = p.sequence
			address := p.addresses[selected]
			p.mu.Unlock()
			return address, nil
		}
		wait := earliest.Sub(now)
		p.mu.Unlock()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
}

// ReportRateLimit shares each proxy's pause across search and audio requests.
// Multiple responses during the same pause do not amplify the penalty.
func (p *Pool) ReportRateLimit(address string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, current := range p.addresses {
		if current != address {
			continue
		}
		state := &p.state[i]
		now := p.now()
		if now.Before(state.blockedUntil) {
			return
		}
		state.penalties = min(state.penalties+1, 6)
		state.blockedUntil = now.Add(time.Duration(state.penalties) * 5 * time.Second)
		return
	}
}
