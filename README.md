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

## Updating

Manually trigger a self-update from the latest GitHub release, in place. When
the bot runs and a newer release exists, it logs a one-line notice telling you
to update.

```bash
./ninalertbot -version        # show current version
./ninalertbot -check-update   # see if a newer release exists (no changes made)
./ninalertbot -update         # download latest, verify checksum, replace the binary
```

`-update` downloads the release asset for your platform, verifies it against
`SHA256SUMS.txt`, and swaps the binary (with automatic rollback on failure).
**Restart the bot (or its Windows service/task) afterward** to run the new
version. If you run it as a service, stop the service first so the file isn't
locked, run `-update`, then start it again.

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
| `mention`                 | —       | Default mention, used when a product has no `mentions`. E.g. `@here` or `<@USER_ID>`. |
| `renotify_after`          | `0s`    | Re-ping if still available after this long. 0 = once.|
| `notify_on_scraper_break` | `false` | Diagnostic alert if a page can't be parsed.          |
| `products[].name` / `.slug` | —     | Display name and URL slug per product.               |
| `products[].mentions`     | —       | Optional. Per-product mention list that **replaces** the global `mention` for that product. |

State is stored in `state.json` next to the binary (override with `-state`).

> **Note:** alert messages are sent in **Korean** (e.g. "🟢 **Nintendo Switch 2**
> 지금 구매 가능합니다!").

### Per-product targeting

Ping different people for different products with a `mentions` list on each
product. It accepts user IDs (`<@123>`) and/or role IDs (`<@&456>`), and when
present it overrides the global `mention` for that product only:

```yaml
mention: "@here"            # default for products without their own list
products:
  - name: "Nintendo Switch 2"
    slug: "beeskb6aakor"
    mentions: ["<@B_USER_ID>"]                 # B watches the Switch 2
  - name: "Nintendo Switch 2 + Pokémon Pokopia Set"
    slug: "beeskb6nfkor"
    mentions: ["<@A_USER_ID>"]                 # A watches the bundles
  - name: "Nintendo Switch 2 + Mario Kart World Set"
    slug: "beeskb6nakor"
    mentions: ["<@A_USER_ID>"]
```

To get a user ID: enable **Developer Mode** (Discord Settings → Advanced), then
right-click a user → **Copy User ID**. Multiple recipients: list several, e.g.
`mentions: ["<@A>", "<@B>"]`.

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

## Cutting a release

`release.sh` builds all platforms, packages the zips (binary + both READMEs +
example config), tags the version, pushes it, and publishes a GitHub release:

```bash
./release.sh v0.3.0             # full release (requires clean main + authenticated gh)
./release.sh v0.3.0 --dry-run   # build + package into dist/ only; no tag or publish
```

It runs `go vet` + tests first and refuses to proceed unless you're on a clean,
up-to-date `main` and the tag doesn't already exist.
