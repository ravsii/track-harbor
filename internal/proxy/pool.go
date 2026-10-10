// Package proxy discovers, verifies, caches and rotates public HTTP proxies.
package proxy

import (
	"fmt"
	"math/rand"
	"sync"
)

type Pool struct {
	mu        sync.Mutex
	addresses []string
	next      int
	last      string
}

func NewPool(addresses []string) (*Pool, error) {
	addresses = usableAddresses(addresses)
	if len(addresses) == 0 {
		return nil, fmt.Errorf("нет проверенных прокси; прямое подключение не используется")
	}
	p := &Pool{addresses: append([]string(nil), addresses...)}
	p.shuffle()
	return p, nil
}

func (p *Pool) Len() int { return len(p.addresses) }

func (p *Pool) List() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.addresses...)
}

func (p *Pool) shuffle() {
	rand.Shuffle(len(p.addresses), func(i, j int) { p.addresses[i], p.addresses[j] = p.addresses[j], p.addresses[i] })
	if len(p.addresses) > 1 && p.addresses[0] == p.last {
		p.addresses[0], p.addresses[1] = p.addresses[1], p.addresses[0]
	}
	p.next = 0
}

// Every request takes a proxy from a shuffled cycle. A proxy is not reused
// until all others were selected; cycles also avoid immediate repetition.
func (p *Pool) Next() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.next == len(p.addresses) {
		p.shuffle()
	}
	address := p.addresses[p.next]
	p.next++
	p.last = address
	return address, nil
}
