package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
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

// btcEmoji prefixes the embed title: the orange circle, the closest unicode to
// the Bitcoin coin and a match for the orange border. Unicode (unlike a custom
// <:name:id> emoji) DOES render in an embed title, so no thumbnail is needed.
const btcEmoji = "🟠"

// completedColor is the embed's left-border colour: Bitcoin orange. Only
// completed swaps are notified (see Telemetry.fireSwapPayoutWebhook), so a
// single colour suffices.
const completedColor = 0xF7931A

// formatUnits renders a base-unit integer string (wei-like) as a decimal with
// `decimals` places, trailing zeros trimmed. A non-integer input is returned
// unchanged so a bad value is visible rather than silently dropped.
func formatUnits(raw string, decimals int) string {
	n, ok := new(big.Int).SetString(raw, 10)
	if !ok {
		return raw
	}
	scale := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	v := new(big.Float).Quo(new(big.Float).SetInt(n), scale)
	s := v.Text('f', decimals)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}

type embedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type discordEmbed struct {
	Title       string       `json:"title"`
	Description string       `json:"description,omitempty"`
	Color       int          `json:"color"`
	Fields      []embedField `json:"fields"`
	Timestamp   string       `json:"timestamp"`
}

// truncAddr shortens a BTC address to head…tail so the description stays on one
// line; the full address is one click away via the mempool link.
func truncAddr(a string) string {
	if len(a) <= 16 {
		return a
	}
	return a[:8] + "…" + a[len(a)-6:]
}

// CallSwapPayoutWebhook posts a compact Discord embed for a completed swap. Like
// CallUptimeWebhook, it never returns an error: every failure (marshal, request,
// transport) is logged and swallowed here so a webhook outage can never affect
// the caller's settlement flow.
func (c *Client) CallSwapPayoutWebhook(ctx context.Context, webhookURL string, event SwapPayoutEvent) {
	if webhookURL == "" {
		return // Skip if webhook URL is not configured
	}

	// Three inline fields pack into one row (the amounts + treasury); the
	// destination and tx link sit in the description as a single compact line.
	fields := []embedField{
		{Name: "ICY", Value: formatUnits(event.IcyAmount, 18), Inline: true},
		{Name: "BTC", Value: formatUnits(event.BtcAmount, 8), Inline: true},
	}
	if event.VaultBalanceSats != "" {
		fields = append(fields, embedField{Name: "Vault", Value: formatUnits(event.VaultBalanceSats, 8) + " ₿", Inline: true})
	}

	desc := "`" + truncAddr(event.BtcAddress) + "`"
	if event.BtcTxHash != "" {
		desc += fmt.Sprintf(" · [view tx ↗](https://mempool.space/tx/%s)", event.BtcTxHash)
	}

	embed := discordEmbed{
		Title:       btcEmoji + " Swap completed",
		Description: desc,
		Color:       completedColor,
		Fields:      fields,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	payload, err := json.Marshal(map[string]interface{}{"embeds": []discordEmbed{embed}})
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
