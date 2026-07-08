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

	It("rejects an empty configured key with a trailing-space 'ApiKey ' header in prod", func() {
		// Regression for the fail-open bypass: an empty configured ApiKey plus a
		// header of "ApiKey " (trailing space) trims to "" and, under a naive
		// constant-time compare of two empty strings, would authenticate.
		cfg := &config.AppConfig{ApiServer: config.ApiServerConfig{
			AppEnv: "prod",
			ApiKey: "",
		}}
		Expect(do(cfg, "ApiKey ")).To(Equal(http.StatusUnauthorized))
	})

	It("treats an unknown/misspelled APP_ENV as prod (auth still enforced)", func() {
		// Fail-closed: an env not on the recognized non-prod allowlist must NOT
		// bypass the api-key gate.
		cfg := &config.AppConfig{ApiServer: config.ApiServerConfig{
			AppEnv: "prd", // typo, not a recognized non-prod env
			ApiKey: "secret",
		}}
		Expect(do(cfg, "")).To(Equal(http.StatusUnauthorized))
	})

	It("treats an empty APP_ENV as prod (auth still enforced)", func() {
		cfg := &config.AppConfig{ApiServer: config.ApiServerConfig{
			AppEnv: "",
			ApiKey: "secret",
		}}
		Expect(do(cfg, "")).To(Equal(http.StatusUnauthorized))
	})

	It("bypasses auth only for a recognized non-prod env", func() {
		cfg := &config.AppConfig{ApiServer: config.ApiServerConfig{
			AppEnv: "dev",
			ApiKey: "secret",
		}}
		Expect(do(cfg, "")).To(Equal(http.StatusOK))
	})
})
