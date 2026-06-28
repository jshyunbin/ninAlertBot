package notifier

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiscordWebhookNotify(t *testing.T) {
	var gotBody discordPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
		}
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	d := &DiscordWebhook{URL: srv.URL, Client: srv.Client()}
	if err := d.Notify(context.Background(), Alert{Content: "hello"}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if gotBody.Content != "hello" {
		t.Errorf("content = %q, want hello", gotBody.Content)
	}
}

func TestDiscordWebhookNotifyError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer srv.Close()

	d := &DiscordWebhook{URL: srv.URL, Client: srv.Client()}
	if err := d.Notify(context.Background(), Alert{Content: "x"}); err == nil {
		t.Error("Notify() expected error for non-2xx, got nil")
	}
}

func TestFormatAvailable(t *testing.T) {
	got := FormatAvailable("@here", "Nintendo Switch 2", "https://store.nintendo.co.kr/beeskb6aakor")
	if !strings.HasPrefix(got, "@here ") {
		t.Errorf("expected mention prefix, got %q", got)
	}
	if !strings.Contains(got, "Nintendo Switch 2") || !strings.Contains(got, "beeskb6aakor") {
		t.Errorf("missing name or url: %q", got)
	}
}

func TestFormatAvailableNoMention(t *testing.T) {
	got := FormatAvailable("", "X", "https://u")
	if strings.HasPrefix(got, " ") {
		t.Errorf("should not start with space when no mention: %q", got)
	}
}
