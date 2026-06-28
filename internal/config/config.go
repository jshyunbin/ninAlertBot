// Package config loads and validates the ninAlertBot configuration file.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Product is a single monitored item.
type Product struct {
	Name string `yaml:"name"`
	Slug string `yaml:"slug"`
}

// Config is the full application configuration.
type Config struct {
	DiscordWebhookURL    string        `yaml:"discord_webhook_url"`
	Interval             time.Duration `yaml:"interval"`
	Mention              string        `yaml:"mention"`
	RenotifyAfter        time.Duration `yaml:"renotify_after"`
	NotifyOnScraperBreak bool          `yaml:"notify_on_scraper_break"`
	Products             []Product     `yaml:"products"`
}

// Defaults applied before validation when fields are omitted.
const (
	DefaultInterval = 60 * time.Second
	MinInterval     = 10 * time.Second
)

// Load reads, parses, and validates a YAML config file.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return Parse(raw)
}

// Parse parses and validates YAML config bytes.
func Parse(raw []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Interval == 0 {
		c.Interval = DefaultInterval
	}
	c.DiscordWebhookURL = strings.TrimSpace(c.DiscordWebhookURL)
}

func (c *Config) validate() error {
	if c.DiscordWebhookURL == "" {
		return fmt.Errorf("discord_webhook_url is required")
	}
	if !strings.HasPrefix(c.DiscordWebhookURL, "https://") {
		return fmt.Errorf("discord_webhook_url must be an https URL")
	}
	if c.Interval < MinInterval {
		return fmt.Errorf("interval must be at least %s (got %s)", MinInterval, c.Interval)
	}
	if len(c.Products) == 0 {
		return fmt.Errorf("at least one product is required")
	}
	seen := make(map[string]bool, len(c.Products))
	for i, p := range c.Products {
		if strings.TrimSpace(p.Slug) == "" {
			return fmt.Errorf("products[%d]: slug is required", i)
		}
		if strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("products[%d] (%s): name is required", i, p.Slug)
		}
		if seen[p.Slug] {
			return fmt.Errorf("duplicate product slug %q", p.Slug)
		}
		seen[p.Slug] = true
	}
	return nil
}
