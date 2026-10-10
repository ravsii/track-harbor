package download

import (
	"sync"
	"time"
)

func rateLimitDelay(round int) time.Duration { return time.Duration(min(round+1, 6)) * 5 * time.Second }

// requestGate is shared by all workers and by search and audio requests.
// No future start is reserved: every waiter rechecks the latest global pause.
type requestGate struct {
	mu                      sync.Mutex
	interval                time.Duration
	delay                   func(int) time.Duration
	nextStart, blockedUntil time.Time
	generation              uint64
	round                   int
}

func (g *requestGate) wait(onWait func(paused bool), beforeStart ...func()) uint64 {
	for {
		g.mu.Lock()
		now := time.Now()
		ready := g.nextStart
		if g.blockedUntil.After(ready) {
			ready = g.blockedUntil
		}
		if !now.Before(ready) {
			// Display updates can block. Do them before final admission, then
			// recheck in case another request started or reported 429 meanwhile.
			g.mu.Unlock()
			for _, prepare := range beforeStart {
				prepare()
			}
			g.mu.Lock()
			now = time.Now()
			ready = g.nextStart
			if g.blockedUntil.After(ready) {
				ready = g.blockedUntil
			}
			if now.Before(ready) {
				g.mu.Unlock()
				continue
			}
			g.nextStart = now.Add(g.interval)
			generation := g.generation
			g.mu.Unlock()
			return generation
		}
		paused := now.Before(g.blockedUntil)
		g.mu.Unlock()
		onWait(paused)
		time.Sleep(time.Until(ready))
	}
}

// Requests already in flight belong to a generation. Only its first 429
// advances the cooldown; its other (even late) 429s join the same wave.
func (g *requestGate) block(generation uint64) (time.Duration, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if generation != g.generation {
		return max(0, time.Until(g.blockedUntil)), false
	}
	delay := g.delay(g.round)
	g.round++
	g.generation++
	g.blockedUntil = time.Now().Add(delay)
	return delay, true
}

func (d *Downloader) waitForRequest(jobID, name string, kind StatusKind) uint64 {
	generation := d.gate.wait(func(paused bool) {
		status := StatusQueued
		if paused {
			status = StatusRetryWaiting
		}
		d.sendEvent(status, jobID, name, nil)
	}, func() { d.sendEvent(kind, jobID, name, nil) })
	return generation
}
