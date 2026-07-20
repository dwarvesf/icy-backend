package http

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// signatureRateLimit bounds how often one client may ask for a swap signature.
//
// The endpoint's ApiKey is a NEXT_PUBLIC_ value inlined into the browser
// bundle, so it is readable by anyone and provides authentication in name
// only. Its actual job is abuse control, and it performs none: without this,
// a single caller can grind signatures against the signer and the oracle
// cache without limit. The signed payout is already derived server-side, so
// this is a load and nonce-pressure control, not the last line of defence.
const (
	signatureRatePerMinute = 12
	signatureBurst         = 5
	visitorTTL             = 10 * time.Minute
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type ipRateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	every    rate.Limit
	burst    int
}

func newIPRateLimiter(perMinute int, burst int) *ipRateLimiter {
	l := &ipRateLimiter{
		visitors: make(map[string]*visitor),
		every:    rate.Every(time.Minute / time.Duration(perMinute)),
		burst:    burst,
	}
	go l.reap()
	return l
}

// reap keeps the visitor map from growing without bound under scan traffic.
func (l *ipRateLimiter) reap() {
	for {
		time.Sleep(visitorTTL)
		l.mu.Lock()
		for ip, v := range l.visitors {
			if time.Since(v.lastSeen) > visitorTTL {
				delete(l.visitors, ip)
			}
		}
		l.mu.Unlock()
	}
}

// maxVisitors caps limiter memory. Entries live up to 2x visitorTTL before the
// reaper runs, so without a cap a burst of distinct keys is an amplification
// vector: ~184 bytes each means a million keys is ~175 MB.
const maxVisitors = 50000

func (l *ipRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	v, ok := l.visitors[ip]
	if !ok {
		// Fail CLOSED when full. Admitting unknown keys past the cap would
		// turn memory pressure into a way to disable the limiter.
		if len(l.visitors) >= maxVisitors {
			return false
		}
		v = &visitor{limiter: rate.NewLimiter(l.every, l.burst)}
		l.visitors[ip] = v
	}
	v.lastSeen = time.Now()
	return v.limiter.Allow()
}

// signatureRateLimitMiddleware is the OUTER, cheap gate: it throttles per
// client IP before any body parsing or signature recovery happens, so a flood
// costs us almost nothing.
//
// It cannot key on the wallet, because middleware runs before the handler that
// recovers it. The precise per-wallet limit lives in the swap handler, after
// authentication. Two stages, deliberately: IP bounds the cold path, wallet
// bounds the authenticated one.
//
// gin's ClientIP honours the trusted-proxy configuration, so behind the
// platform's proxy this is the real caller rather than the proxy.
func signatureRateLimitMiddleware() gin.HandlerFunc {
	limiter := newIPRateLimiter(signatureRatePerMinute, signatureBurst)

	return func(c *gin.Context) {
		if !limiter.allow(c.ClientIP()) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "too many signature requests, try again shortly",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
