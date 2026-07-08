package config

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("validateSecurityConfig", func() {
	newCfg := func(env, apiKey string) *AppConfig {
		return &AppConfig{ApiServer: ApiServerConfig{AppEnv: env, ApiKey: apiKey}}
	}

	It("errors when APP_ENV=prod and API_KEY is empty", func() {
		Expect(validateSecurityConfig(newCfg("prod", ""))).To(HaveOccurred())
	})

	It("errors when APP_ENV=production and API_KEY is empty", func() {
		Expect(validateSecurityConfig(newCfg("production", ""))).To(HaveOccurred())
	})

	It("passes when APP_ENV=prod and API_KEY is set", func() {
		Expect(validateSecurityConfig(newCfg("prod", "secret"))).ToNot(HaveOccurred())
	})

	It("passes when APP_ENV is non-prod and API_KEY is empty", func() {
		Expect(validateSecurityConfig(newCfg("dev", ""))).ToNot(HaveOccurred())
	})
})
