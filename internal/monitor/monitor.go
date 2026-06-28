// Package monitor schedules availability checks and turns sold-out→available
// transitions into Discord alerts, with restart-safe deduplication.
package monitor

import (
	"context"
	"log/slog"
	"math/rand"
	"time"

	"github.com/jshyunbin/ninalertbot/internal/config"
	"github.com/jshyunbin/ninalertbot/internal/notifier"
	"github.com/jshyunbin/ninalertbot/internal/state"
	"github.com/jshyunbin/ninalertbot/internal/store"
)

// URLBuilder yields the public product URL for a slug (used in alert messages).
type URLBuilder interface {
	ProductURL(slug string) string
}

// Monitor wires together checking, state, and notification.
type Monitor struct {
	cfg      *config.Config
	checker  store.Checker
	urls     URLBuilder
	notifier notifier.Notifier
	store    state.Store
	log      *slog.Logger
	now      func() time.Time
	rand     *rand.Rand
}

// New constructs a Monitor.
func New(cfg *config.Config, checker store.Checker, urls URLBuilder, n notifier.Notifier, st state.Store, log *slog.Logger) *Monitor {
	return &Monitor{
		cfg:      cfg,
		checker:  checker,
		urls:     urls,
		notifier: n,
		store:    st,
		log:      log,
		now:      time.Now,
		rand:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Run polls on a jittered interval until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context) {
	m.log.Info("monitor started",
		"products", len(m.cfg.Products), "interval", m.cfg.Interval.String())
	m.RunOnce(ctx) // check immediately on startup
	for {
		select {
		case <-ctx.Done():
			m.log.Info("monitor stopped")
			return
		case <-time.After(m.jitter(m.cfg.Interval)):
			m.RunOnce(ctx)
		}
	}
}

// RunOnce checks every configured product exactly once.
func (m *Monitor) RunOnce(ctx context.Context) {
	for _, p := range m.cfg.Products {
		if ctx.Err() != nil {
			return
		}
		m.checkProduct(ctx, p)
	}
}

func (m *Monitor) checkProduct(ctx context.Context, p config.Product) {
	status, err := m.checker.Check(ctx, p.Slug)
	if err != nil {
		// Transient: log and let the next tick retry. Never alert on errors.
		m.log.Warn("check failed", "product", p.Name, "slug", p.Slug, "err", err)
		return
	}

	st := m.store.Get(p.Slug)
	now := m.now()
	url := m.urls.ProductURL(p.Slug)
	m.log.Debug("checked", "product", p.Name, "status", status.String())

	switch status {
	case store.Available:
		m.handleAvailable(ctx, p, url, st, now)
	case store.SoldOut:
		if st.Status != store.SoldOut.String() {
			m.save(p.Slug, state.ProductState{
				Status:         store.SoldOut.String(),
				LastChangeUnix: now.Unix(),
				LastNotifyUnix: st.LastNotifyUnix,
			})
			m.log.Info("now sold out", "product", p.Name)
		}
	case store.Unknown:
		m.handleUnknown(ctx, p, url, st, now)
	}
}

func (m *Monitor) handleAvailable(ctx context.Context, p config.Product, url string, st state.ProductState, now time.Time) {
	wasAvailable := st.Status == store.Available.String()
	mention := p.MentionString(m.cfg.Mention)

	switch {
	case !wasAvailable:
		// sold-out/unknown -> available: the alert we exist to send.
		m.log.Info("AVAILABLE", "product", p.Name, "url", url)
		m.notify(ctx, notifier.FormatAvailable(mention, p.Name, url))
		m.save(p.Slug, state.ProductState{
			Status:         store.Available.String(),
			LastChangeUnix: now.Unix(),
			LastNotifyUnix: now.Unix(),
		})
	case m.cfg.RenotifyAfter > 0 && now.Sub(time.Unix(st.LastNotifyUnix, 0)) >= m.cfg.RenotifyAfter:
		// Still available after the renotify window.
		m.log.Info("still available (renotify)", "product", p.Name)
		m.notify(ctx, notifier.FormatAvailable(mention, p.Name, url))
		st.LastNotifyUnix = now.Unix()
		m.save(p.Slug, st)
	}
}

func (m *Monitor) handleUnknown(ctx context.Context, p config.Product, url string, st state.ProductState, now time.Time) {
	m.log.Warn("could not parse stock status (possible layout change)",
		"product", p.Name, "slug", p.Slug)
	if !m.cfg.NotifyOnScraperBreak {
		return
	}
	// Throttle diagnostic pings to at most once per 6h per product, reusing
	// LastNotifyUnix so we don't spam.
	const scraperBreakCooldown = 6 * time.Hour
	if st.LastNotifyUnix != 0 && now.Sub(time.Unix(st.LastNotifyUnix, 0)) < scraperBreakCooldown {
		return
	}
	m.notify(ctx, notifier.FormatScraperBreak(p.Name, url))
	st.LastNotifyUnix = now.Unix()
	m.save(p.Slug, st) // status field intentionally unchanged
}

func (m *Monitor) notify(ctx context.Context, content string) {
	if err := m.notifier.Notify(ctx, notifier.Alert{Content: content}); err != nil {
		m.log.Error("discord notify failed", "err", err)
	}
}

func (m *Monitor) save(slug string, st state.ProductState) {
	if err := m.store.Set(slug, st); err != nil {
		m.log.Error("persist state failed", "slug", slug, "err", err)
	}
}

// jitter returns d adjusted by ±20% to avoid hammering the store on a fixed beat.
func (m *Monitor) jitter(d time.Duration) time.Duration {
	delta := float64(d) * 0.2
	return time.Duration(float64(d) - delta + m.rand.Float64()*2*delta)
}
