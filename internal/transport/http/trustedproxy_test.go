package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/dwarvesf/icy-backend/internal/utils/config"
)

// gin trusts all proxies by default, so ClientIP() returns whatever
// X-Forwarded-For says. That makes every per-IP rate limit decorative: rotate
// the header for unlimited buckets, or pin a victim's IP to drain theirs.
// Measured before the fix: 50 requests over ONE socket with a rotating XFF
// produced 0 throttled, versus 45/50 throttled without the header.
func TestConfigureTrustedProxies_IgnoresSpoofedXFFByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	configureTrustedProxies(r, &config.AppConfig{}, nil) // no TRUSTED_PROXIES
	r.GET("/whoami", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})

	call := func(xff string) string {
		req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
		req.RemoteAddr = "198.51.100.7:44444"
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Body.String()
	}

	if got := call(""); got != "198.51.100.7" {
		t.Fatalf("no XFF: ClientIP() = %q, want the socket peer 198.51.100.7", got)
	}

	// The whole point: a spoofed header must NOT move the client IP.
	for _, spoof := range []string{"1.2.3.4", "203.0.113.9, 198.51.100.7", "::1"} {
		if got := call(spoof); got != "198.51.100.7" {
			t.Fatalf("XFF %q was believed: ClientIP() = %q, want 198.51.100.7", spoof, got)
		}
	}
}

// A malformed list must not silently leave gin trusting everything, which is
// the exact state this function exists to end.
func TestConfigureTrustedProxies_MalformedFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	configureTrustedProxies(r, &config.AppConfig{
		ApiServer: config.ApiServerConfig{TrustedProxies: "not-a-cidr"},
	}, nil)
	r.GET("/whoami", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})

	req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	req.RemoteAddr = "198.51.100.7:44444"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Body.String(); got != "198.51.100.7" {
		t.Fatalf("malformed TRUSTED_PROXIES trusted the header: ClientIP() = %q", got)
	}
}

// A configured proxy SHOULD be believed, otherwise real deployments behind a
// load balancer rate-limit the balancer instead of the caller.
func TestConfigureTrustedProxies_TrustsConfiguredHop(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	configureTrustedProxies(r, &config.AppConfig{
		ApiServer: config.ApiServerConfig{TrustedProxies: "198.51.100.0/24"},
	}, nil)
	r.GET("/whoami", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})

	req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	req.RemoteAddr = "198.51.100.7:44444"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Body.String(); got != "203.0.113.9" {
		t.Fatalf("trusted hop was ignored: ClientIP() = %q, want 203.0.113.9", got)
	}
}
