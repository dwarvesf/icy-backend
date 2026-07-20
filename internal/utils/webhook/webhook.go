package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
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

	// A non-2xx here means the heartbeat did NOT register. Swallowing it makes
	// a dead monitor look healthy: the job keeps "pinging", the monitor never
	// hears it, and nobody learns the difference until an incident. Still never
	// fatal to the caller, but it must be loud in the logs.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logger.Error("Uptime webhook rejected the ping, heartbeat did NOT register", map[string]string{
			"url":         webhookURL,
			"status_code": resp.Status,
		})
		return
	}

	// Log successful webhook call
	c.logger.Info("Successfully called uptime webhook", map[string]string{
		"url":         webhookURL,
		"status_code": resp.Status,
	})
}

// SwapPayoutEvent carries the swap fields for a BTC payout notification. It fires
// both when a swap is first DETECTED on-chain (status "pending", the payout row
// was just created) and when the payout reaches a terminal settlement state
// (completed / failed / needs_reconcile). This is detection, not prevention: it
// exists so a swap, drain, or anomaly is visible somewhere other than the
// in-page browser toast.
type SwapPayoutEvent struct {
	Status     string // pending | completed | failed | needs_reconcile
	IcyAmount  string
	BtcAmount  string
	BtcAddress string
	BtcTxHash  string
	// VaultBalanceSats is the treasury BTC balance in satoshi at post time,
	// fetched best-effort by the emitter. Empty = the lookup failed or was
	// skipped; the message simply omits the line rather than showing a stale
	// or bogus number.
	VaultBalanceSats string
}

// icyEmoji is the Dwarves server's custom animated ICY emoji. Discord renders a
// literal <a:name:id> token in a message's content field as the emoji, so it can
// prefix the swap notification directly.
const icyEmoji = "<a:icy:1192768878183465062>"

// CallSwapPayoutWebhook posts a Discord-formatted notification for a settled
// BTC payout. Like CallUptimeWebhook, it never returns an error: every failure
// (marshal, request, transport) is logged and swallowed here so a webhook
// outage can never affect the caller's settlement flow.
func (c *Client) CallSwapPayoutWebhook(ctx context.Context, webhookURL string, event SwapPayoutEvent) {
	if webhookURL == "" {
		return // Skip if webhook URL is not configured
	}

	content := fmt.Sprintf(
		"%s BTC payout **%s**\nICY amount: `%s`\nBTC amount: `%s`\nDestination: `%s`\nTx hash: `%s`",
		icyEmoji, event.Status, event.IcyAmount, event.BtcAmount, event.BtcAddress, event.BtcTxHash,
	)
	if event.VaultBalanceSats != "" {
		content += "\nVault balance: `" + event.VaultBalanceSats + " sats`"
		if sats, ok := new(big.Int).SetString(event.VaultBalanceSats, 10); ok {
			btc := new(big.Float).Quo(new(big.Float).SetInt(sats), big.NewFloat(1e8))
			content += fmt.Sprintf(" (~%s BTC)", btc.Text('f', 4))
		}
	}

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
