package webhook

import (
	"context"
	"net/http"
	"time"

	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

// Client is a simple HTTP client for making webhook calls
type Client struct {
	httpClient *http.Client
	logger     *logger.Logger
}

// New creates a new webhook client with timeout
func New(logger *logger.Logger) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// CallUptimeWebhook makes a simple GET request to the webhook URL
func (c *Client) CallUptimeWebhook(ctx context.Context, webhookURL string) {
	if webhookURL == "" {
		return // Skip if webhook URL is not configured
	}

	req, err := http.NewRequestWithContext(ctx, "GET", webhookURL, nil)
	if err != nil {
		c.logger.Error("Failed to create webhook request", map[string]string{
			"url":   webhookURL,
			"error": err.Error(),
		})
		return
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("Failed to call uptime webhook", map[string]string{
			"url":   webhookURL,
			"error": err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	// Log successful webhook call
	c.logger.Info("Successfully called uptime webhook", map[string]string{
		"url":         webhookURL,
		"status_code": resp.Status,
	})
}