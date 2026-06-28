package monitor

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jshyunbin/ninalertbot/internal/config"
	"github.com/jshyunbin/ninalertbot/internal/notifier"
	"github.com/jshyunbin/ninalertbot/internal/state"
	"github.com/jshyunbin/ninalertbot/internal/store"
)

// fakeChecker returns a scripted sequence of statuses per call.
type fakeChecker struct {
	seq []store.Status
	i   int
}

func (f *fakeChecker) Check(ctx context.Context, slug string) (store.Status, error) {
	s := f.seq[f.i]
	if f.i < len(f.seq)-1 {
		f.i++
	}
	return s, nil
}

type fakeURLs struct{}

func (fakeURLs) ProductURL(slug string) string { return "https://store/" + slug }

type capturingNotifier struct {
	mu    sync.Mutex
	calls []string
}

func (c *capturingNotifier) Notify(ctx context.Context, a notifier.Alert) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, a.Content)
	return nil
}

func (c *capturingNotifier) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

// memStore is an in-memory state.Store.
type memStore struct {
	m map[string]state.ProductState
}

func newMemStore() *memStore { return &memStore{m: map[string]state.ProductState{}} }
func (s *memStore) Get(slug string) state.ProductState { return s.m[slug] }
func (s *memStore) Set(slug string, st state.ProductState) error {
	s.m[slug] = st
	return nil
}

func newTestMonitor(t *testing.T, cfg *config.Config, ck store.Checker, n notifier.Notifier, st state.Store) *Monitor {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := New(cfg, ck, fakeURLs{}, n, st, log)
	m.now = func() time.Time { return time.Unix(1_000_000, 0) }
	return m
}

func baseConfig() *config.Config {
	return &config.Config{
		DiscordWebhookURL: "https://x",
		Interval:          time.Minute,
		Products:          []config.Product{{Name: "Switch 2", Slug: "s1"}},
	}
}

func TestFiresOnceOnTransitionToAvailable(t *testing.T) {
	ck := &fakeChecker{seq: []store.Status{store.SoldOut, store.Available, store.Available}}
	n := &capturingNotifier{}
	m := newTestMonitor(t, baseConfig(), ck, n, newMemStore())

	m.RunOnce(context.Background()) // sold out -> no alert
	if n.count() != 0 {
		t.Fatalf("after sold out: %d alerts, want 0", n.count())
	}
	m.RunOnce(context.Background()) // available -> 1 alert
	if n.count() != 1 {
		t.Fatalf("after available: %d alerts, want 1", n.count())
	}
	m.RunOnce(context.Background()) // still available -> no extra alert
	if n.count() != 1 {
		t.Fatalf("after still-available: %d alerts, want 1", n.count())
	}
}

func TestUsesPerProductMention(t *testing.T) {
	cfg := baseConfig()
	cfg.Mention = "@here"
	cfg.Products = []config.Product{{Name: "Switch 2", Slug: "s1", Mentions: []string{"<@999>"}}}
	ck := &fakeChecker{seq: []store.Status{store.Available}}
	n := &capturingNotifier{}
	m := newTestMonitor(t, cfg, ck, n, newMemStore())

	m.RunOnce(context.Background())
	if n.count() != 1 {
		t.Fatalf("got %d alerts, want 1", n.count())
	}
	n.mu.Lock()
	msg := n.calls[0]
	n.mu.Unlock()
	if !strings.HasPrefix(msg, "<@999> ") {
		t.Errorf("alert should use per-product mention, got %q", msg)
	}
	if strings.Contains(msg, "@here") {
		t.Errorf("alert should not fall back to global mention, got %q", msg)
	}
}

func TestReArmsAfterReturningToSoldOut(t *testing.T) {
	ck := &fakeChecker{seq: []store.Status{store.Available, store.SoldOut, store.Available}}
	n := &capturingNotifier{}
	m := newTestMonitor(t, baseConfig(), ck, n, newMemStore())

	m.RunOnce(context.Background()) // available -> alert 1
	m.RunOnce(context.Background()) // sold out -> re-arm
	m.RunOnce(context.Background()) // available again -> alert 2
	if n.count() != 2 {
		t.Fatalf("got %d alerts, want 2", n.count())
	}
}

func TestUnknownDoesNotAlertOrClobber(t *testing.T) {
	cfg := baseConfig()
	cfg.NotifyOnScraperBreak = false
	ck := &fakeChecker{seq: []store.Status{store.Available, store.Unknown, store.Available}}
	n := &capturingNotifier{}
	ms := newMemStore()
	m := newTestMonitor(t, cfg, ck, n, ms)

	m.RunOnce(context.Background()) // available -> alert 1, state=available
	m.RunOnce(context.Background()) // unknown -> no alert, state stays available
	if ms.Get("s1").Status != store.Available.String() {
		t.Errorf("unknown clobbered state: %q", ms.Get("s1").Status)
	}
	m.RunOnce(context.Background()) // available -> still considered available, no new alert
	if n.count() != 1 {
		t.Fatalf("got %d alerts, want 1", n.count())
	}
}

func TestScraperBreakDiagnosticAlert(t *testing.T) {
	cfg := baseConfig()
	cfg.NotifyOnScraperBreak = true
	ck := &fakeChecker{seq: []store.Status{store.Unknown}}
	n := &capturingNotifier{}
	m := newTestMonitor(t, cfg, ck, n, newMemStore())

	m.RunOnce(context.Background())
	if n.count() != 1 {
		t.Fatalf("got %d diagnostic alerts, want 1", n.count())
	}
}

func TestRenotifyAfter(t *testing.T) {
	cfg := baseConfig()
	cfg.RenotifyAfter = 30 * time.Minute
	ck := &fakeChecker{seq: []store.Status{store.Available}}
	n := &capturingNotifier{}
	ms := newMemStore()
	m := newTestMonitor(t, cfg, ck, n, ms)

	base := time.Unix(1_000_000, 0)
	m.now = func() time.Time { return base }
	m.RunOnce(context.Background()) // alert 1

	// Not enough time elapsed -> no renotify.
	m.now = func() time.Time { return base.Add(10 * time.Minute) }
	m.RunOnce(context.Background())
	if n.count() != 1 {
		t.Fatalf("early renotify: got %d, want 1", n.count())
	}

	// Past the window -> renotify.
	m.now = func() time.Time { return base.Add(31 * time.Minute) }
	m.RunOnce(context.Background())
	if n.count() != 2 {
		t.Fatalf("renotify: got %d, want 2", n.count())
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	ck := &fakeChecker{seq: []store.Status{store.SoldOut}}
	n := &capturingNotifier{}
	m := newTestMonitor(t, baseConfig(), ck, n, newMemStore())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}
