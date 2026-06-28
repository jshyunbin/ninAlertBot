# ninAlertBot

**English** | [한국어](README.ko.md)

A tiny background service that watches the **Nintendo Korea store**
(`store.nintendo.co.kr`) and sends a **Discord** message the moment a product
flips from sold-out (품절) to purchasable (구매 가능).

- **Single binary**, ~9 MB, ~10–15 MB RAM idle — built to run 24/7.
- **Cross-platform** (Windows / Linux / macOS).
- **Customizable** — watch any number of products (regular Switch 2, the
  Pokémon Pokopia bundle, Mario Kart World set, accessories, …).
- **No spam** — alerts once per restock; remembers state across restarts.

## How it works

Each product has a page at `https://store.nintendo.co.kr/<slug>` that renders its
stock state server-side:

| State    | Markup                                  |
|----------|-----------------------------------------|
| Sold out | `<div class="stock unavailable">품절</div>`     |
| In stock | `<div class="stock available">구매 가능</div>`   |

ninAlertBot polls each configured product on an interval and fires a Discord
webhook on the **sold-out → available** transition.

## Setup

### 1. Create a Discord webhook
In your Discord server: **Server Settings → Integrations → Webhooks → New
Webhook**, pick a channel, **Copy Webhook URL**.

### 2. Configure
```bash
cp config.example.yaml config.yaml
# edit config.yaml: paste your webhook URL and choose products
```

Find a product's `slug` by opening its page on the store — it's the last part of
the URL. Known Switch 2 slugs:

| Product                                   | Slug           |
|-------------------------------------------|----------------|
| Nintendo Switch 2                         | `beeskb6aakor` |
| Nintendo Switch 2 + Pokémon Pokopia Set   | `beeskb6nfkor` |
| Nintendo Switch 2 + Mario Kart World Set  | `beeskb6nakor` |

### 3. Build
With Go 1.24+ installed:
```bash
go build -o ninalertbot ./cmd/ninalertbot          # current platform
GOOS=windows GOARCH=amd64 go build -o ninalertbot.exe ./cmd/ninalertbot   # Windows
```

### 4. Run
```bash
./ninalertbot -config config.yaml          # run forever
./ninalertbot -config config.yaml -once    # check once and exit (test)
./ninalertbot -config config.yaml -debug   # verbose logging
```

## Running 24/7 on Windows

### Option A — Task Scheduler (simplest)
1. Put `ninalertbot.exe` and `config.yaml` in e.g. `C:\ninAlertBot\`.
2. Open **Task Scheduler → Create Task**.
   - **General:** "Run whether user is logged on or not", check "Run with highest privileges".
   - **Triggers:** New → "At startup".
   - **Actions:** New → Program: `C:\ninAlertBot\ninalertbot.exe`
     Arguments: `-config config.yaml`
     Start in: `C:\ninAlertBot\`
   - **Settings:** "If the task fails, restart every 1 minute"; uncheck "Stop the task if it runs longer than".
3. Right-click the task → **Run**.

### Option B — Windows service via NSSM
```powershell
nssm install ninAlertBot C:\ninAlertBot\ninalertbot.exe -config config.yaml
nssm set ninAlertBot AppDirectory C:\ninAlertBot
nssm start ninAlertBot
```

## Configuration reference

See [`config.example.yaml`](config.example.yaml). Key fields:

| Field                     | Default | Meaning                                              |
|---------------------------|---------|------------------------------------------------------|
| `discord_webhook_url`     | —       | Required. Your Discord incoming webhook (https).     |
| `interval`                | `60s`   | Poll cadence, jittered ±20%. Minimum `10s`.          |
| `mention`                 | —       | Prepended to alerts, e.g. `@here` or `<@USER_ID>`.   |
| `renotify_after`          | `0s`    | Re-ping if still available after this long. 0 = once.|
| `notify_on_scraper_break` | `false` | Diagnostic alert if a page can't be parsed.          |
| `products[].name` / `.slug` | —     | Display name and URL slug per product.               |

State is stored in `state.json` next to the binary (override with `-state`).

## Development

```bash
go test ./...            # run all tests
go test -cover ./...     # with coverage
go vet ./...
```

Architecture (see `docs/superpowers/specs/` for the design):

```
cmd/ninalertbot   entry point, flags, signal handling
internal/config   load + validate config.yaml
internal/store    fetch product page -> Available / SoldOut / Unknown
internal/notifier Discord webhook client + message formatting
internal/monitor  scheduler, state transitions, dedup
internal/state    JSON persistence of last-known status
```
