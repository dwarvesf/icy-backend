package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// The signature endpoint's ApiKey is public (inlined into the browser bundle),
// so this limiter is the only thing bounding signature grinding.
func TestSignatureRateLimitMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.POST("/sig", signatureRateLimitMiddleware(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	call := func(ip string) int {
		req := httptest.NewRequest(http.MethodPost, "/sig", nil)
		req.RemoteAddr = ip + ":12345"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	// The burst must be spendable, a legitimate user retrying should not trip.
	for i := 0; i < signatureBurst; i++ {
		if code := call("203.0.113.10"); code != http.StatusOK {
			t.Fatalf("request %d within burst: got %d, want 200", i+1, code)
		}
	}

	// Immediately past the burst the same caller is throttled.
	if code := call("203.0.113.10"); code != http.StatusTooManyRequests {
		t.Fatalf("past burst: got %d, want 429", code)
	}

	// Throttling must be per caller, not global: one abuser cannot deny
	// service to everyone else.
	if code := call("203.0.113.99"); code != http.StatusOK {
		t.Fatalf("different IP: got %d, want 200", code)
	}
}
