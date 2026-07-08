package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

// SwapPayoutEvent carries the swap fields for a BTC payout that just reached a
// terminal settlement state (completed / failed / needs_reconcile). This is
// detection, not prevention: it exists so a drain or anomaly is visible
// somewhere other than the in-page browser toast.
type SwapPayoutEvent struct {
	Status     string // completed | failed | needs_reconcile
	IcyAmount  string
	BtcAmount  string
	BtcAddress string
	BtcTxHash  string
}

// CallSwapPayoutWebhook posts a Discord-formatted notification for a settled
// BTC payout. Like CallUptimeWebhook, it never returns an error: every failure
// (marshal, request, transport) is logged and swallowed here so a webhook
// outage can never affect the caller's settlement flow.
func (c *Client) CallSwapPayoutWebhook(ctx context.Context, webhookURL string, event SwapPayoutEvent) {
	if webhookURL == "" {
		return // Skip if webhook URL is not configured
	}

	content := fmt.Sprintf(
		"BTC payout **%s**\nICY amount: `%s`\nBTC amount: `%s`\nDestination: `%s`\nTx hash: `%s`",
		event.Status, event.IcyAmount, event.BtcAmount, event.BtcAddress, event.BtcTxHash,
	)

	payload, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		c.logger.Error("Failed to marshal swap payout webhook payload", map[string]string{
			"error":  err.Error(),
			"status": event.Status,
		})
		return
	}

	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(payload))
	if err != nil {
		c.logger.Error("Failed to create swap payout webhook request", map[string]string{
			"url":   webhookURL,
			"error": err.Error(),
		})
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("Failed to call swap payout webhook", map[string]string{
			"url":   webhookURL,
			"error": err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	// Log successful webhook call
	c.logger.Info("Successfully called swap payout webhook", map[string]string{
		"url":         webhookURL,
		"status_code": resp.Status,
		"status":      event.Status,
	})
}
