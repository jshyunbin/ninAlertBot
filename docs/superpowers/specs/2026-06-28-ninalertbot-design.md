# ninAlertBot — Design

**Date:** 2026-06-28
**Status:** Approved (design), implementing

## Goal

A local background service that polls chosen Nintendo Korea store (`store.nintendo.co.kr`)
product pages and sends a Discord message with the purchase link the moment a product
flips from sold-out to purchasable. Cross-platform, minimal resource use, runs 24/7 on a
Windows desktop. User can configure any number of products (e.g. regular Switch 2 and the
Switch 2 + Pokémon Pokopia bundle).

## Decisions

- **Runtime:** Go — single static binary, ~10–15 MB RAM idle, no runtime to install,
  cross-compiles to Windows/Linux/macOS.
- **Discord delivery:** Incoming webhook URL (no bot, one-way notifications).
- **Detection:** Server-rendered stock element on the product page.

## Availability detection (verified against live pages)

Each product lives at `https://store.nintendo.co.kr/{slug}` and renders:

- Sold out: `<div class="stock unavailable" title="Availability"><span>품절</span></div>`
- In stock: `<div class="stock available" title="Availability"><span>구매 가능</span></div>`

Detection rule:

- `stock available` present → **Available**
- `stock unavailable` present → **SoldOut**
- neither → **Unknown** (likely a layout change). Treated as *not available* for alerting
  (no false buy alerts); logged, and optionally one diagnostic Discord ping.

The cart button (`장바구니`) appears on both states, so it is NOT used as a signal.

Known slugs: `beeskb6aakor` (Switch 2, ₩648,000), `beeskb6nfkor` (Switch 2 + Pokémon
Pokopia Set, ₩718,000), `beeskb6nakor` (Switch 2 + Mario Kart World Set, ₩688,000).

## Architecture

```
cmd/ninalertbot/main.go   load config, build deps, run monitor, handle SIGINT/SIGTERM
internal/config           parse + validate config.yaml
internal/store            fetch a product page → Available / SoldOut / Unknown
internal/notifier         Discord webhook client + message formatting
internal/monitor          per-product scheduler, state transitions, dedup
internal/state            persist last-known status to state.json
```

**Data flow:** monitor ticks every `interval` (jittered) → `store.Check(slug)` per product →
compare to persisted state → on **SoldOut→Available** transition call `notifier.Notify()` →
write `state.json`.

Interfaces (for isolation + testing):

- `store.Checker.Check(ctx, slug) (Status, error)` — concrete `HTTPChecker`.
- `notifier.Notifier.Notify(ctx, Alert) error` — concrete `DiscordWebhook`.
- `state.Store` load/save `map[slug]ProductState` — concrete file-backed JSON store.

## Anti-spam / state

- Alert **once** on sold-out→available. Stay silent while it stays available. Re-arm when it
  returns to sold-out.
- Optional `renotify_after` (default off): re-ping if still available after the duration.
- `state.json` survives restarts so a reboot does not re-spam.
- `Unknown` never overwrites a known `available`/`sold_out` status.

## Config (`config.yaml`)

```yaml
discord_webhook_url: "https://discord.com/api/webhooks/..."
interval: 60s                 # poll cadence, jittered ±20%
mention: "@here"              # optional, prepended to alerts
renotify_after: 0s            # 0 = alert once per restock
notify_on_scraper_break: true
products:
  - name: "Nintendo Switch 2"
    slug: "beeskb6aakor"
  - name: "Switch 2 + Pokémon Pokopia Set"
    slug: "beeskb6nfkor"
```

## Error handling

- Network/HTTP errors: logged, counted, **not** alerted (avoids spam); next tick retries.
- Polite polling: jittered interval, realistic User-Agent, per-request timeout, one in-flight
  request per product.

## Testing

- `store` parser against saved HTML fixtures (sold-out, in-stock, unknown) — riskiest logic.
- `notifier` against an `httptest` server.
- `monitor` transitions with a fake checker: sold-out→available fires once; available→available
  silent; →sold-out re-arms; unknown does not clobber state.
- `state` round-trip through a temp file.

## Running 24/7 on Windows

- Foreground: `ninalertbot.exe -config config.yaml`.
- Background: Task Scheduler ("At log on", restart on failure) or `sc.exe` service wrapper —
  documented in README. Cross-compiles via `GOOS=windows GOARCH=amd64 go build`.
