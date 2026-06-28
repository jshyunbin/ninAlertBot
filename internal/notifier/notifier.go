// Package notifier delivers alerts to Discord via an incoming webhook.
package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Alert is a single message to deliver.
type Alert struct {
	// Content is the message body (may include a mention prefix).
	Content string
}

// Notifier delivers alerts.
type Notifier interface {
	Notify(ctx context.Context, a Alert) error
}

// DiscordWebhook posts messages to a Discord incoming webhook URL.
type DiscordWebhook struct {
	URL    string
	Client *http.Client
}

// NewDiscordWebhook returns a webhook notifier with a default HTTP client.
func NewDiscordWebhook(url string) *DiscordWebhook {
	return &DiscordWebhook{URL: url, Client: &http.Client{Timeout: 15 * time.Second}}
}

type discordPayload struct {
	Content         string         `json:"content"`
	AllowedMentions allowedMention `json:"allowed_mentions"`
}

type allowedMention struct {
	Parse []string `json:"parse"`
}

// Notify posts the alert to Discord. It allows @here/@everyone/role/user
// mentions so a configured mention actually pings.
func (d *DiscordWebhook) Notify(ctx context.Context, a Alert) error {
	payload := discordPayload{
		Content:         a.Content,
		AllowedMentions: allowedMention{Parse: []string{"everyone", "users", "roles"}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))

	// Discord webhooks return 204 No Content on success.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord webhook returned %d", resp.StatusCode)
	}
	return nil
}

// FormatAvailable builds the alert text for a product that just became buyable.
func FormatAvailable(mention, name, url string) string {
	var b strings.Builder
	if m := strings.TrimSpace(mention); m != "" {
		b.WriteString(m)
		b.WriteString(" ")
	}
	fmt.Fprintf(&b, "🟢 **%s** is now available to buy!\n%s", name, url)
	return b.String()
}

// FormatScraperBreak builds the diagnostic alert text when a page can no longer
// be parsed (likely a layout change).
func FormatScraperBreak(name, url string) string {
	return fmt.Sprintf("⚠️ ninAlertBot could not read the stock status for **%s**. "+
		"The page layout may have changed — check the scraper.\n%s", name, url)
}
