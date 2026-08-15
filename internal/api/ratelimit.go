package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// rateLimiter is a token bucket that adapts to Apple's X-Rate-Limit headers
// when present (format like "limit:2000;remaining:1998;reset:3600") and
// otherwise paces conservatively.
type rateLimiter struct {
	mu     sync.Mutex
	tokens float64
	rate   float64 // tokens per second
	burst  float64
	last   time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{tokens: 8, rate: 4, burst: 8, last: time.Now()}
}

func (r *rateLimiter) wait(ctx context.Context) {
	for {
		r.mu.Lock()
		now := time.Now()
		r.tokens += now.Sub(r.last).Seconds() * r.rate
		if r.tokens > r.burst {
			r.tokens = r.burst
		}
		r.last = now
		if r.tokens >= 1 {
			r.tokens--
			r.mu.Unlock()
			return
		}
		need := time.Duration((1 - r.tokens) / r.rate * float64(time.Second))
		r.mu.Unlock()
		sleepCtx(ctx, need)
		if ctx.Err() != nil {
			return
		}
	}
}

// observe slows down when the remaining quota reported by the API runs low.
func (r *rateLimiter) observe(resp *http.Response) {
	h := resp.Header.Get("X-Rate-Limit")
	if h == "" {
		return
	}
	var limit, remaining, reset int
	for _, part := range strings.Split(h, ";") {
		kv := strings.SplitN(strings.TrimSpace(part), ":", 2)
		if len(kv) != 2 {
			continue
		}
		n, err := strconv.Atoi(kv[1])
		if err != nil {
			continue
		}
		switch strings.ToLower(kv[0]) {
		case "limit":
			limit = n
		case "remaining":
			remaining = n
		case "reset":
			reset = n
		}
	}
	if limit == 0 || reset == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// Pace so the remaining quota lasts until reset, capped at the default.
	ideal := float64(remaining) / float64(reset)
	if ideal < 0.2 {
		ideal = 0.2
	}
	if ideal < r.rate {
		r.rate = ideal
	}
}

// Headroom returns the limiter's current pacing for `auth doctor`.
func (r *rateLimiter) Headroom() (ratePerSec float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rate
}
