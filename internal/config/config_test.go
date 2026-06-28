package config

import (
	"testing"
	"time"
)

const validYAML = `
discord_webhook_url: "https://discord.com/api/webhooks/123/abc"
interval: 30s
mention: "@here"
products:
  - name: "Nintendo Switch 2"
    slug: "beeskb6aakor"
  - name: "Switch 2 + Pokopia"
    slug: "beeskb6nfkor"
`

func TestParseValid(t *testing.T) {
	cfg, err := Parse([]byte(validYAML))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.Interval != 30*time.Second {
		t.Errorf("Interval = %v, want 30s", cfg.Interval)
	}
	if len(cfg.Products) != 2 {
		t.Fatalf("got %d products, want 2", len(cfg.Products))
	}
	if cfg.Products[0].Slug != "beeskb6aakor" {
		t.Errorf("Products[0].Slug = %q", cfg.Products[0].Slug)
	}
}

func TestParseAppliesDefaultInterval(t *testing.T) {
	cfg, err := Parse([]byte(`
discord_webhook_url: "https://x"
products:
  - {name: a, slug: s1}
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.Interval != DefaultInterval {
		t.Errorf("Interval = %v, want default %v", cfg.Interval, DefaultInterval)
	}
}

func TestParsePerProductMentions(t *testing.T) {
	cfg, err := Parse([]byte(`
discord_webhook_url: "https://x"
mention: "@here"
products:
  - name: "A only"
    slug: "s1"
    mentions: ["<@111>", "<@222>"]
  - name: "Default"
    slug: "s2"
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got, want := cfg.Products[0].MentionString(cfg.Mention), "<@111> <@222>"; got != want {
		t.Errorf("Products[0].MentionString() = %q, want %q", got, want)
	}
	if got, want := cfg.Products[1].MentionString(cfg.Mention), "@here"; got != want {
		t.Errorf("Products[1].MentionString() = %q, want %q (global fallback)", got, want)
	}
}

func TestParseInvalid(t *testing.T) {
	cases := map[string]string{
		"missing webhook": `
products:
  - {name: a, slug: s1}
`,
		"non-https webhook": `
discord_webhook_url: "http://x"
products:
  - {name: a, slug: s1}
`,
		"no products": `
discord_webhook_url: "https://x"
`,
		"missing slug": `
discord_webhook_url: "https://x"
products:
  - {name: a, slug: ""}
`,
		"missing name": `
discord_webhook_url: "https://x"
products:
  - {name: "", slug: s1}
`,
		"duplicate slug": `
discord_webhook_url: "https://x"
products:
  - {name: a, slug: s1}
  - {name: b, slug: s1}
`,
		"interval too small": `
discord_webhook_url: "https://x"
interval: 1s
products:
  - {name: a, slug: s1}
`,
	}
	for name, yaml := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(yaml)); err == nil {
				t.Errorf("Parse() expected error, got nil")
			}
		})
	}
}
