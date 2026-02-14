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
