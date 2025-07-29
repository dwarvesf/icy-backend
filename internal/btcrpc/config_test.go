package btcrpc_test

import (
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/types/environments"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

var _ = Describe("BTC Configuration Multi-Endpoint Support", func() {
	var (
		testLogger *logger.Logger
	)

	BeforeEach(func() {
		testLogger = logger.New(environments.Test)
	})

	Context("Configuration Structure Changes", func() {
		Describe("BitcoinConfig Updates", func() {
			It("should support new BlockstreamAPIURLs field", func() {
				config := &config.BitcoinConfig{
					BlockstreamAPIURLs: []string{
						"https://blockstream.info/api",
						"https://mempool.space/api",
						"https://backup.blockstream.info/api",
					},
					WalletWIF:      "test-wif",
					MaxTxFeeUSD:    10.0,
					ServiceFeeRate: 0.01,
					MinSatshiFee:   1000,
				}

				Expect(config.BlockstreamAPIURLs).To(HaveLen(3))
				Expect(config.BlockstreamAPIURLs[0]).To(Equal("https://blockstream.info/api"))
				Expect(config.BlockstreamAPIURLs[1]).To(Equal("https://mempool.space/api"))
				Expect(config.BlockstreamAPIURLs[2]).To(Equal("https://backup.blockstream.info/api"))
			})

			It("should support additional endpoint configuration options", func() {
				config := &config.BitcoinConfig{
					BlockstreamAPIURLs:             []string{"https://api1.example.com", "https://api2.example.com"},
					WalletWIF:                      "test-wif",
					MaxTxFeeUSD:                    10.0,
					ServiceFeeRate:                 0.01,
					MinSatshiFee:                   1000,
					EndpointTimeout:                30 * time.Second,
					EndpointRetryDelay:             5 * time.Minute,
					EndpointMaxRetries:             3,
					CircuitBreakerFailureThreshold: 5,
					CircuitBreakerTimeout:          10 * time.Minute,
					EndpointLoadBalancing:          "round-robin",
					HealthCheckInterval:            1 * time.Minute,
				}

				Expect(config.EndpointTimeout).To(Equal(30 * time.Second))
				Expect(config.EndpointRetryDelay).To(Equal(5 * time.Minute))
				Expect(config.EndpointMaxRetries).To(Equal(3))
				Expect(config.CircuitBreakerFailureThreshold).To(Equal(5))
				Expect(config.CircuitBreakerTimeout).To(Equal(10 * time.Minute))
				Expect(config.EndpointLoadBalancing).To(Equal("round-robin"))
				Expect(config.HealthCheckInterval).To(Equal(1 * time.Minute))
			})

			It("should maintain backward compatibility with single URL", func() {
				config := &config.BitcoinConfig{
					BlockstreamAPIURL: "https://blockstream.info/api", // Legacy field
					WalletWIF:         "test-wif",
					MaxTxFeeUSD:       10.0,
					ServiceFeeRate:    0.01,
					MinSatshiFee:      1000,
				}

				Expect(config.BlockstreamAPIURL).To(Equal("https://blockstream.info/api"))
				Expect(config.BlockstreamAPIURLs).To(BeNil()) // New field not set
			})
		})

		Describe("Environment Variable Support", func() {
			BeforeEach(func() {
				// Clean up environment before each test
				os.Unsetenv("BLOCKSTREAM_API_URLS")
				os.Unsetenv("BLOCKSTREAM_API_URL")
				os.Unsetenv("BTC_ENDPOINT_TIMEOUT")
				os.Unsetenv("BTC_ENDPOINT_RETRY_DELAY")
				os.Unsetenv("BTC_CIRCUIT_BREAKER_THRESHOLD")
			})

			AfterEach(func() {
				// Clean up environment after each test
				os.Unsetenv("BLOCKSTREAM_API_URLS")
				os.Unsetenv("BLOCKSTREAM_API_URL")
				os.Unsetenv("BTC_ENDPOINT_TIMEOUT")
				os.Unsetenv("BTC_ENDPOINT_RETRY_DELAY")
				os.Unsetenv("BTC_CIRCUIT_BREAKER_THRESHOLD")
			})

			It("should parse comma-separated URLs from environment", func() {
				os.Setenv("BLOCKSTREAM_API_URLS", "https://api1.example.com,https://api2.example.com,https://api3.example.com")

				// Test that config parsing would handle this correctly
				envValue := os.Getenv("BLOCKSTREAM_API_URLS")
				Expect(envValue).To(Equal("https://api1.example.com,https://api2.example.com,https://api3.example.com"))
				
				// This would be handled in config.New() implementation
				urls := parseCommaSeparatedURLs(envValue)
				Expect(urls).To(HaveLen(3))
				Expect(urls[0]).To(Equal("https://api1.example.com"))
				Expect(urls[1]).To(Equal("https://api2.example.com"))
				Expect(urls[2]).To(Equal("https://api3.example.com"))
			})

			It("should support timeout configuration from environment", func() {
				os.Setenv("BTC_ENDPOINT_TIMEOUT", "45s")
				os.Setenv("BTC_ENDPOINT_RETRY_DELAY", "2m")
				os.Setenv("BTC_CIRCUIT_BREAKER_THRESHOLD", "3")

				// Test environment variable parsing
				timeoutStr := os.Getenv("BTC_ENDPOINT_TIMEOUT")
				timeout, err := time.ParseDuration(timeoutStr)
				Expect(err).To(BeNil())
				Expect(timeout).To(Equal(45 * time.Second))

				retryDelayStr := os.Getenv("BTC_ENDPOINT_RETRY_DELAY")
				retryDelay, err := time.ParseDuration(retryDelayStr)
				Expect(err).To(BeNil())
				Expect(retryDelay).To(Equal(2 * time.Minute))
			})

			It("should fallback to single URL when multiple URLs not set", func() {
				os.Setenv("BLOCKSTREAM_API_URL", "https://single-api.example.com")

				// When BLOCKSTREAM_API_URLS is not set, should use single URL
				multiURLs := os.Getenv("BLOCKSTREAM_API_URLS")
				singleURL := os.Getenv("BLOCKSTREAM_API_URL")

				Expect(multiURLs).To(BeEmpty())
				Expect(singleURL).To(Equal("https://single-api.example.com"))
			})
		})
	})

	Context("Configuration Validation", func() {
		Describe("URL Validation", func() {
			It("should validate URL format", func() {
				validURLs := []string{
					"https://blockstream.info/api",
					"http://localhost:3000/api",
					"https://mempool.space/api",
				}

				for _, url := range validURLs {
					Expect(isValidURL(url)).To(BeTrue(), "URL should be valid: %s", url)
				}
			})

			It("should reject invalid URLs", func() {
				invalidURLs := []string{
					"not-a-url",
					"ftp://example.com",
					"",
					"https://",
					"http://",
				}

				for _, url := range invalidURLs {
					Expect(isValidURL(url)).To(BeFalse(), "URL should be invalid: %s", url)
				}
			})

			It("should handle duplicate URLs", func() {
				urls := []string{
					"https://api1.example.com",
					"https://api2.example.com",
					"https://api1.example.com", // Duplicate
				}

				uniqueURLs := removeDuplicateURLs(urls)
				Expect(uniqueURLs).To(HaveLen(2))
				Expect(uniqueURLs).To(ContainElement("https://api1.example.com"))
				Expect(uniqueURLs).To(ContainElement("https://api2.example.com"))
			})
		})

		Describe("Configuration Defaults", func() {
			It("should provide sensible defaults for new fields", func() {
				config := &config.BitcoinConfig{
					BlockstreamAPIURLs: []string{"https://api.example.com"},
					WalletWIF:          "test-wif",
				}

				// Test that defaults would be applied
				defaults := applyBitcoinConfigDefaults(config)
				
				Expect(defaults.EndpointTimeout).To(Equal(30 * time.Second))
				Expect(defaults.EndpointRetryDelay).To(Equal(5 * time.Minute))
				Expect(defaults.EndpointMaxRetries).To(Equal(3))
				Expect(defaults.CircuitBreakerFailureThreshold).To(Equal(5))
				Expect(defaults.CircuitBreakerTimeout).To(Equal(10 * time.Minute))
				Expect(defaults.EndpointLoadBalancing).To(Equal("failover"))
				Expect(defaults.HealthCheckInterval).To(Equal(1 * time.Minute))
			})

			It("should not override explicitly set values", func() {
				config := &config.BitcoinConfig{
					BlockstreamAPIURLs:             []string{"https://api.example.com"},
					WalletWIF:                      "test-wif",
					EndpointTimeout:                45 * time.Second,    // Custom value
					CircuitBreakerFailureThreshold: 3,                  // Custom value
					EndpointLoadBalancing:          "round-robin",      // Custom value
				}

				defaults := applyBitcoinConfigDefaults(config)
				
				// Should keep custom values
				Expect(defaults.EndpointTimeout).To(Equal(45 * time.Second))
				Expect(defaults.CircuitBreakerFailureThreshold).To(Equal(3))
				Expect(defaults.EndpointLoadBalancing).To(Equal("round-robin"))
				
				// Should apply defaults for unset values
				Expect(defaults.EndpointRetryDelay).To(Equal(5 * time.Minute))
				Expect(defaults.EndpointMaxRetries).To(Equal(3))
			})
		})
	})

	Context("Configuration Migration", func() {
		Describe("Legacy Configuration Support", func() {
			It("should automatically migrate single URL to multiple URLs", func() {
				legacyConfig := &config.BitcoinConfig{
					BlockstreamAPIURL: "https://blockstream.info/api",
					WalletWIF:         "test-wif",
				}

				// Simulate migration logic
				migratedConfig := migrateBitcoinConfig(legacyConfig)
				
				Expect(migratedConfig.BlockstreamAPIURLs).To(HaveLen(1))
				Expect(migratedConfig.BlockstreamAPIURLs[0]).To(Equal("https://blockstream.info/api"))
				Expect(migratedConfig.BlockstreamAPIURL).To(Equal("https://blockstream.info/api")) // Preserved for compatibility
			})

			It("should prefer multiple URLs over single URL when both are set", func() {
				config := &config.BitcoinConfig{
					BlockstreamAPIURL:  "https://old-api.example.com",
					BlockstreamAPIURLs: []string{"https://new-api1.example.com", "https://new-api2.example.com"},
					WalletWIF:          "test-wif",
				}

				// New field should take precedence
				effectiveURLs := getEffectiveBlockstreamURLs(config)
				Expect(effectiveURLs).To(HaveLen(2))
				Expect(effectiveURLs).To(ContainElement("https://new-api1.example.com"))
				Expect(effectiveURLs).To(ContainElement("https://new-api2.example.com"))
				Expect(effectiveURLs).NotTo(ContainElement("https://old-api.example.com"))
			})
		})

		Describe("Version Compatibility", func() {
			It("should handle configuration from older versions", func() {
				// Simulate old configuration structure
				oldConfig := map[string]interface{}{
					"BlockstreamAPIURL": "https://blockstream.info/api",
					"WalletWIF":         "test-wif",
					"MaxTxFeeUSD":       10.0,
				}

				// Test that old config can be converted to new structure
				newConfig := convertLegacyConfig(oldConfig)
				
				Expect(newConfig.BlockstreamAPIURL).To(Equal("https://blockstream.info/api"))
				Expect(newConfig.BlockstreamAPIURLs).To(HaveLen(1))
				Expect(newConfig.BlockstreamAPIURLs[0]).To(Equal("https://blockstream.info/api"))
				Expect(newConfig.WalletWIF).To(Equal("test-wif"))
				Expect(newConfig.MaxTxFeeUSD).To(Equal(10.0))
			})

			It("should warn about deprecated configuration fields", func() {
				config := &config.BitcoinConfig{
					BlockstreamAPIURL: "https://blockstream.info/api", // Deprecated but still supported
					WalletWIF:         "test-wif",
				}

				warnings := validateConfigForDeprecation(config)
				Expect(warnings).To(HaveLen(1))
				Expect(warnings[0]).To(ContainSubstring("BlockstreamAPIURL is deprecated"))
				Expect(warnings[0]).To(ContainSubstring("use BlockstreamAPIURLs instead"))
			})
		})
	})
})

// Helper functions for testing configuration logic
func parseCommaSeparatedURLs(urlsStr string) []string {
	if urlsStr == "" {
		return nil
	}
	urls := make([]string, 0)
	for _, url := range splitAndTrim(urlsStr, ",") {
		if url != "" {
			urls = append(urls, url)
		}
	}
	return urls
}

func splitAndTrim(s, sep string) []string {
	parts := make([]string, 0)
	for _, part := range strings.Split(s, sep) {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func isValidURL(urlStr string) bool {
	if urlStr == "" {
		return false
	}
	return strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://")
}

func removeDuplicateURLs(urls []string) []string {
	seen := make(map[string]bool)
	unique := make([]string, 0)
	
	for _, url := range urls {
		if !seen[url] {
			seen[url] = true
			unique = append(unique, url)
		}
	}
	
	return unique
}

func applyBitcoinConfigDefaults(config *config.BitcoinConfig) *config.BitcoinConfig {
	result := *config // Copy
	
	if result.EndpointTimeout == 0 {
		result.EndpointTimeout = 30 * time.Second
	}
	if result.EndpointRetryDelay == 0 {
		result.EndpointRetryDelay = 5 * time.Minute
	}
	if result.EndpointMaxRetries == 0 {
		result.EndpointMaxRetries = 3
	}
	if result.CircuitBreakerFailureThreshold == 0 {
		result.CircuitBreakerFailureThreshold = 5
	}
	if result.CircuitBreakerTimeout == 0 {
		result.CircuitBreakerTimeout = 10 * time.Minute
	}
	if result.EndpointLoadBalancing == "" {
		result.EndpointLoadBalancing = "failover"
	}
	if result.HealthCheckInterval == 0 {
		result.HealthCheckInterval = 1 * time.Minute
	}
	
	return &result
}

func migrateBitcoinConfig(config *config.BitcoinConfig) *config.BitcoinConfig {
	result := *config // Copy
	
	// If only single URL is set, migrate to multiple URLs
	if result.BlockstreamAPIURL != "" && len(result.BlockstreamAPIURLs) == 0 {
		result.BlockstreamAPIURLs = []string{result.BlockstreamAPIURL}
	}
	
	return &result
}

func getEffectiveBlockstreamURLs(config *config.BitcoinConfig) []string {
	// Prefer multiple URLs over single URL
	if len(config.BlockstreamAPIURLs) > 0 {
		return config.BlockstreamAPIURLs
	}
	
	// Fallback to single URL
	if config.BlockstreamAPIURL != "" {
		return []string{config.BlockstreamAPIURL}
	}
	
	return nil
}

func convertLegacyConfig(oldConfig map[string]interface{}) *config.BitcoinConfig {
	newConfig := &config.BitcoinConfig{}
	
	if url, ok := oldConfig["BlockstreamAPIURL"].(string); ok {
		newConfig.BlockstreamAPIURL = url
		newConfig.BlockstreamAPIURLs = []string{url}
	}
	
	if wif, ok := oldConfig["WalletWIF"].(string); ok {
		newConfig.WalletWIF = wif
	}
	
	if fee, ok := oldConfig["MaxTxFeeUSD"].(float64); ok {
		newConfig.MaxTxFeeUSD = fee
	}
	
	return newConfig
}

func validateConfigForDeprecation(config *config.BitcoinConfig) []string {
	warnings := make([]string, 0)
	
	if config.BlockstreamAPIURL != "" && len(config.BlockstreamAPIURLs) == 0 {
		warnings = append(warnings, "BlockstreamAPIURL is deprecated, use BlockstreamAPIURLs instead for better availability")
	}
	
	return warnings
}

func TestBtcConfigMultiEndpoint(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BTC Configuration Multi-Endpoint Suite")
}