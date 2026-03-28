package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type rateLimiter struct {
	mu         sync.Mutex
	limiters   map[string]*visitorLimiter
	rate       rate.Limit
	burst      int
	trustProxy bool
}

type visitorLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newRateLimiter(rps float64, burst int, trustProxy bool) *rateLimiter {
	return &rateLimiter{
		limiters:   make(map[string]*visitorLimiter),
		rate:       rate.Limit(rps),
		burst:      burst,
		trustProxy: trustProxy,
	}
}

func (rl *rateLimiter) startCleanup(done <-chan struct{}) {
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			rl.mu.Lock()
			for ip, v := range rl.limiters {
				if time.Since(v.lastSeen) > 5*time.Minute {
					delete(rl.limiters, ip)
				}
			}
			rl.mu.Unlock()
		}
	}
}

func (rl *rateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.limiters[ip]
	if !exists {
		v = &visitorLimiter{
			limiter: rate.NewLimiter(rl.rate, rl.burst),
		}
		rl.limiters[ip] = v
	}
	v.lastSeen = time.Now()
	return v.limiter
}

func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r, rl.trustProxy)
		limiter := rl.getLimiter(ip)
		if !limiter.Allow() {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP extracts the client IP from the request.
// When trustProxy is true and the server is behind a reverse proxy, it reads
// X-Forwarded-For. When false, it uses RemoteAddr directly.
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if i := strings.IndexByte(xff, ','); i != -1 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
