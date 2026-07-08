package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/dwarvesf/icy-backend/internal/utils/config"
)

func TestHTTPMiddlewareAuth(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HTTP apiKeyMiddleware Auth Suite")
}

var _ = Describe("apiKeyMiddleware for /swap/generate-signature", func() {
	const path = "/api/v1/swap/generate-signature"

	newRouter := func(cfg *config.AppConfig) *gin.Engine {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(apiKeyMiddleware(cfg))
		r.POST(path, func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
		return r
	}

	do := func(cfg *config.AppConfig, authHeader string) int {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		w := httptest.NewRecorder()
		newRouter(cfg).ServeHTTP(w, req)
		return w.Code
	}

	It("returns 401 for a no-key request in prod (auth always required, no flag)", func() {
		cfg := &config.AppConfig{ApiServer: config.ApiServerConfig{
			AppEnv: "prod",
			ApiKey: "secret",
		}}
		Expect(do(cfg, "")).To(Equal(http.StatusUnauthorized))
	})

	It("allows a correctly-keyed request in prod", func() {
		cfg := &config.AppConfig{ApiServer: config.ApiServerConfig{
			AppEnv: "prod",
			ApiKey: "secret",
		}}
		Expect(do(cfg, "ApiKey secret")).To(Equal(http.StatusOK))
	})

	It("rejects a wrong-key request in prod", func() {
		cfg := &config.AppConfig{ApiServer: config.ApiServerConfig{
			AppEnv: "prod",
			ApiKey: "secret",
		}}
		Expect(do(cfg, "ApiKey wrong")).To(Equal(http.StatusUnauthorized))
	})
})
