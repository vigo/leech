package app

import (
	"io"
	"sync"
	"time"
)

// rateLimiter implements a token bucket for bandwidth limiting.
type rateLimiter struct {
	mu         sync.Mutex
	rate       int64 // bytes per second, 0 = unlimited
	tokens     int64
	lastRefill time.Time
}

func newRateLimiter(bytesPerSecond int64) *rateLimiter {
	return &rateLimiter{
		rate:       bytesPerSecond,
		tokens:     bytesPerSecond,
		lastRefill: time.Now(),
	}
}

func (rl *rateLimiter) wait(n int) {
	if rl.rate == 0 {
		return
	}

	remaining := int64(n)

	for remaining > 0 {
		// request at most rl.rate tokens at a time to avoid deadlock
		request := remaining
		if request > rl.rate {
			request = rl.rate
		}

		rl.waitForTokens(request)
		remaining -= request
	}
}

func (rl *rateLimiter) waitForTokens(n int64) {
	for {
		rl.mu.Lock()

		now := time.Now()
		elapsed := now.Sub(rl.lastRefill)
		newTokens := int64(elapsed.Seconds() * float64(rl.rate))

		if newTokens > 0 {
			rl.tokens += newTokens
			if rl.tokens > rl.rate {
				rl.tokens = rl.rate
			}
			rl.lastRefill = now
		}

		if rl.tokens >= n {
			rl.tokens -= n
			rl.mu.Unlock()

			return
		}

		rl.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
}

type rateLimitedReader struct {
	reader  io.Reader
	limiter *rateLimiter
}

func (r *rateLimitedReader) Read(p []byte) (int, error) {
	// cap read size to rate limit to avoid large bursts
	if r.limiter.rate > 0 && int64(len(p)) > r.limiter.rate {
		p = p[:r.limiter.rate]
	}

	n, err := r.reader.Read(p)
	if n > 0 {
		r.limiter.wait(n)
	}

	return n, err
}
