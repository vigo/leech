package app

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestRateLimitedReader(t *testing.T) {
	data := strings.Repeat("x", 10240) // 10KB
	r := strings.NewReader(data)

	limiter := newRateLimiter(5120) // 5KB/s
	lr := &rateLimitedReader{reader: r, limiter: limiter}

	start := time.Now()
	buf, err := io.ReadAll(lr)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatal(err)
	}
	if len(buf) != 10240 {
		t.Errorf("read %d bytes, want 10240", len(buf))
	}
	if elapsed < 1*time.Second {
		t.Errorf("expected rate limiting, elapsed: %v", elapsed)
	}
}

func TestRateLimiterZeroMeansUnlimited(t *testing.T) {
	data := strings.Repeat("x", 1024)
	r := strings.NewReader(data)

	limiter := newRateLimiter(0)
	lr := &rateLimitedReader{reader: r, limiter: limiter}

	buf, err := io.ReadAll(lr)
	if err != nil {
		t.Fatal(err)
	}
	if len(buf) != 1024 {
		t.Errorf("read %d bytes, want 1024", len(buf))
	}
}

func TestRateLimitedReaderLargeBuffer(t *testing.T) {
	data := strings.Repeat("x", 200)
	r := strings.NewReader(data)

	limiter := newRateLimiter(50) // 50 bytes/sec
	lr := &rateLimitedReader{reader: r, limiter: limiter}

	// use buffer larger than rate to trigger cap in Read
	buf := make([]byte, 200)
	n, err := lr.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	if n > 50 {
		t.Errorf("read %d bytes, expected at most 50 (rate limit cap)", n)
	}
}

func TestRateLimiterWaitLargeN(t *testing.T) {
	limiter := newRateLimiter(100)
	// wait for 250 bytes — triggers loop splitting into chunks of rl.rate
	limiter.wait(250)
}
