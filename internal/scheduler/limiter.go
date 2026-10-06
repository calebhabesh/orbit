package scheduler

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/network"
)

type bandwidthWaiter struct{ ready chan struct{} }

type BandwidthLimiter struct {
	waiters      []*bandwidthWaiter
	mu           sync.Mutex
	globalRate   int64 // bytes per second, 0 = unlimited
	globalTokens float64
	globalLast   time.Time

	peerRates  map[history.ID]int64
	peerTokens map[history.ID]float64
	peerLast   map[history.ID]time.Time
}

func NewBandwidthLimiter(globalBytesPerSec int64) *BandwidthLimiter {
	now := time.Now()
	var tokens float64
	if globalBytesPerSec > 0 {
		tokens = float64(globalBytesPerSec)
	}
	return &BandwidthLimiter{
		globalRate:   globalBytesPerSec,
		globalTokens: tokens,
		globalLast:   now,
		peerRates:    make(map[history.ID]int64),
		peerTokens:   make(map[history.ID]float64),
		peerLast:     make(map[history.ID]time.Time),
	}
}

func (l *BandwidthLimiter) SetGlobalRate(bytesPerSec int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.globalRate = bytesPerSec
	if bytesPerSec > 0 && l.globalTokens > float64(bytesPerSec) {
		l.globalTokens = float64(bytesPerSec)
	}
}

func (l *BandwidthLimiter) SetPeerRate(peer history.ID, bytesPerSec int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.peerRates[peer] = bytesPerSec
	l.peerTokens[peer] = float64(bytesPerSec)
	l.peerLast[peer] = time.Now()
}

// Acquire serves reservations in arrival order. A stream of tiny reservations
// cannot repeatedly steal refilled tokens from an already waiting chunk.
func (l *BandwidthLimiter) Acquire(ctx context.Context, peer *history.ID, bytes int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if bytes <= 0 {
		return nil
	}
	w := &bandwidthWaiter{ready: make(chan struct{})}
	l.mu.Lock()
	// Bound queued reservations independently of transfer-worker configuration.
	if len(l.waiters) >= 128 {
		l.mu.Unlock()
		return network.ErrBackpressure
	}
	l.waiters = append(l.waiters, w)
	if len(l.waiters) == 1 {
		close(w.ready)
	}
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		for i, pending := range l.waiters {
			if pending == w {
				l.waiters = append(l.waiters[:i], l.waiters[i+1:]...)
				if i == 0 && len(l.waiters) > 0 {
					close(l.waiters[0].ready)
				}
				break
			}
		}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.ready:
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		l.mu.Lock()
		now := time.Now()

		var waitGlobal, waitPeer time.Duration

		// Global bucket check
		if l.globalRate > 0 {
			elapsed := now.Sub(l.globalLast).Seconds()
			l.globalLast = now
			l.globalTokens += elapsed * float64(l.globalRate)
			burstCap := float64(l.globalRate)
			if burstCap < 64*1024 {
				burstCap = 64 * 1024
			}
			if burstCap < float64(bytes) {
				burstCap = float64(bytes)
			}
			if l.globalTokens > burstCap {
				l.globalTokens = burstCap
			}
			if l.globalTokens < float64(bytes) {
				needed := float64(bytes) - l.globalTokens
				waitGlobal = time.Duration(needed / float64(l.globalRate) * float64(time.Second))
			}
		}

		// Peer bucket check
		if peer != nil {
			if peerRate, exists := l.peerRates[*peer]; exists && peerRate > 0 {
				last, hasLast := l.peerLast[*peer]
				if !hasLast {
					last = now
				}
				elapsed := now.Sub(last).Seconds()
				l.peerLast[*peer] = now
				curTokens := l.peerTokens[*peer] + elapsed*float64(peerRate)
				burstCap := float64(peerRate)
				if burstCap < 64*1024 {
					burstCap = 64 * 1024
				}
				if burstCap < float64(bytes) {
					burstCap = float64(bytes)
				}
				if curTokens > burstCap {
					curTokens = burstCap
				}
				l.peerTokens[*peer] = curTokens
				if curTokens < float64(bytes) {
					needed := float64(bytes) - curTokens
					waitPeer = time.Duration(needed / float64(peerRate) * float64(time.Second))
				}
			}
		}

		waitDur := waitGlobal
		if waitPeer > waitDur {
			waitDur = waitPeer
		}

		if waitDur <= 0 {
			// Deduct tokens
			if l.globalRate > 0 {
				l.globalTokens -= float64(bytes)
			}
			if peer != nil {
				if peerRate, exists := l.peerRates[*peer]; exists && peerRate > 0 {
					l.peerTokens[*peer] -= float64(bytes)
				}
			}
			l.mu.Unlock()
			return nil
		}

		l.mu.Unlock()

		timer := time.NewTimer(waitDur)
		select {
		case <-timer.C:
			// Loop again to consume tokens
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	}
}

type throttledReader struct {
	ctx     context.Context
	peer    *history.ID
	r       io.Reader
	limiter *BandwidthLimiter
}

func (tr *throttledReader) Read(p []byte) (int, error) {
	if tr.limiter == nil {
		return tr.r.Read(p)
	}
	// Limit read chunk to 64 KiB for smooth token pacing
	toRead := len(p)
	if toRead > 64*1024 {
		toRead = 64 * 1024
	}
	if err := tr.limiter.Acquire(tr.ctx, tr.peer, toRead); err != nil {
		return 0, err
	}
	return tr.r.Read(p[:toRead])
}

func ThrottledReader(ctx context.Context, peer *history.ID, r io.Reader, limiter *BandwidthLimiter) io.Reader {
	if limiter == nil {
		return r
	}
	return &throttledReader{ctx: ctx, peer: peer, r: r, limiter: limiter}
}

type throttledWriter struct {
	ctx     context.Context
	peer    *history.ID
	w       io.Writer
	limiter *BandwidthLimiter
}

func (tw *throttledWriter) Write(p []byte) (int, error) {
	if tw.limiter == nil {
		return tw.w.Write(p)
	}
	total := 0
	for total < len(p) {
		chunkSize := len(p) - total
		if chunkSize > 64*1024 {
			chunkSize = 64 * 1024
		}
		if err := tw.limiter.Acquire(tw.ctx, tw.peer, chunkSize); err != nil {
			return total, err
		}
		n, err := tw.w.Write(p[total : total+chunkSize])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func ThrottledWriter(ctx context.Context, peer *history.ID, w io.Writer, limiter *BandwidthLimiter) io.Writer {
	if limiter == nil {
		return w
	}
	return &throttledWriter{ctx: ctx, peer: peer, w: w, limiter: limiter}
}
