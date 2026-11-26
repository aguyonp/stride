# Stride

[![CI](https://github.com/aguyonp/stride/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/aguyonp/stride/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/aguyonp/stride?sort=semver&display_name=tag)](https://github.com/aguyonp/stride/releases)
[![Image](https://img.shields.io/badge/ghcr.io-aguyonp%2Fstride-blue?logo=docker)](https://github.com/aguyonp/stride/pkgs/container/stride)
[![Go](https://img.shields.io/github/go-mod/go-version/aguyonp/stride?filename=backend%2Fgo.mod)](backend/go.mod)
[![License](https://img.shields.io/badge/license-AGPL%20v3-blue.svg)](LICENSE)

Stride merges your Garmin and Strava telemetry, computes biomechanics, aerobic efficiency, cumulative impact, ACWR load and readiness, structures Macro/Méso/Micro training cycles, correlates localized musculoskeletal stress with shoe wear and terrain, and streams AI-coach analyses in real time. Plans export to `.FIT`, clinical views share via revocable read-only links, and authentication uses WebAuthn passkeys only.

![Stride dashboard (demo data)](docs/screenshot.png)

## Quick start

```bash
git clone git@github.com:aguyonp/stride.git && cd stride
grep -qs STRIDE_MASTER_KEY .env || printf 'STRIDE_MASTER_KEY=%s\nSTRIDE_UID=%s\nSTRIDE_GID=%s\n' \
  "$(openssl rand -base64 32)" "$(id -u)" "$(id -g)" >> .env
mkdir -p data && docker compose up -d
open http://localhost:8080
```

- **First account = admin**; others join by invitation. Integrations are optional, set per user in the app.
- **Back up `STRIDE_MASTER_KEY`**: it encrypts every stored credential and share link (`AES-256-GCM`).
- Passkeys need HTTPS except on `localhost`. Private image: `docker login ghcr.io` (`read:packages`).

## Highlights

- **One dashboard, every session**: Garmin and Strava merged, biomechanics, aerobic efficiency, cumulative impact, ACWR load and readiness, with previous / next navigation through the whole history.
- **Training cycles and planning**: macro / méso / micro phases with planned vs actual, structured sessions (warm-up, effort, recovery, cool-down, `repeat` blocks, pace / HR / cadence / power targets), Smart Workout Morphing.
- **Health as a single source**: injury focus, body map, pain and symptoms recorded once, correlations computed from measured data only.
- **AI coach, streamed**: analyses, plan generation and adaptation over SSE, coach notes from the physio injected first in every prompt.
- **Notification centre**: a per-user history of the alerts and the physio/coach messages — bell, unread badge, grouped by day, mark read.
- **Physio / coach portal**: revocable read-only share links with opt-in permissions (directives, remarks, prescribed sessions, events).
- **Export and sharing**: `.FIT` workout export with the Garmin Connect Web import checklist, an optional experimental watch push (off by default, unofficial API), clinical views by link.
- **Per-user MCP server**: expose your own data to AI assistants at `POST /mcp/{token}`, 13 tools scoped to you, with a quota and a call journal.
- **French / English** interface and AI prompts, following the browser language.
- **Self-hosted and boring to operate**: SQLite (one writer, one reader), atomic backups with S3 upload, WebAuthn passkeys only, Prometheus metrics, in-app admin control center, release notice when a newer version is published.

## Architecture

![Architecture](docs/architecture.svg)

Detailed guides: [Architecture & Data Flows](docs/ARCHITECTURE.md) · [Prometheus Metrics & Alerts](docs/METRICS.md) · [Specification](docs/SPEC.md) · [Garmin API Facts](docs/GARMIN_API.md).

## Release cycle

One SemVer string, no `v`: `VERSION` file = git tag = image tag = `GET /version` & Settings footer.

| Trigger | Image `ghcr.io/aguyonp/stride` | Version |
| --- | --- | --- |
| push to a branch | `dev` | `4.1.0-dev` |
| merge into `main` | `prerelease` | `4.1.0-pre` |
| tag `4.1.0` | `latest` + `4.1.0` + GitHub release | `4.1.0` |

1. Bump `VERSION` in your PR (`make changelog` previews the notes), merge, validate `prerelease`.
2. On an up-to-date `main`, `make release` pushes the tag with its changelog; CI builds `latest` + `4.1.0` and publishes the release (a tag ≠ `VERSION` fails).
3. Deploy or roll back: `STRIDE_TAG=<tag> docker compose up -d`.

## Configuration (`.env`)

| Variable | Default | Purpose |
| --- | --- | --- |
| `STRIDE_MASTER_KEY` | **required** | AES-256-GCM key for credentials and share links at rest |
| `STRIDE_PUBLIC_URL` | `http://localhost:8080` | Public URL (passkeys, OAuth, invite & share links) |
| `STRIDE_VALKEY_URL` | — (`redis://valkey:6379/0` in Compose) | Valkey URL for the TTL cache and SSE Pub/Sub |
| `STRIDE_METRICS_ADDR` | — (disabled) | Private listener for `GET /metrics` (e.g. `:9090`, not published) |
| `STRIDE_REGISTRATION` | `invite` | `invite` or `open` |
| `STRIDE_STRAVA_CLIENT_ID` / `_SECRET` / `_WEBHOOK_TOKEN` | — | Enable Strava OAuth & webhooks |
| `STRIDE_HOST_BIND` / `STRIDE_HOST_PORT` | `127.0.0.1` / `8080` | Published address |
| `STRIDE_ALLOW_PRIVATE_EGRESS` | `false` | Allow a local LLM / webhook / S3 endpoint (e.g. Ollama, MinIO) |
| `STRIDE_S3_ENDPOINT` / `_REGION` / `_BUCKET` / `_PREFIX` | — / `us-east-1` / — / — | Seed the backup S3 target once (then edited in Admin › Backups & S3) |
| `STRIDE_S3_ACCESS_KEY_ID` / `_SECRET_ACCESS_KEY` | — | S3 credentials (secret sealed at rest, never returned) |
| `STRIDE_S3_PATH_STYLE` / `_AUTO_UPLOAD` / `_RETENTION_DAYS` | `false` / `false` / `30` | Path-style URLs (MinIO, Garage), upload the daily snapshot, remote retention (0: keep) |

Full list: [docs/SPEC.md](docs/SPEC.md) §9.

## Development

```bash
make dev            # Go API on :8080 (in-memory cache)
make dev-frontend   # Vite on :5173 (npm run dev:mock for mocked data)
make test           # Go race tests + frontend Vitest suite
make test-e2e       # Playwright browser smoke against the mock build (Chromium, issue #91)
make docker-up      # Build locally and start docker compose (stride + valkey)
```

## License

Stride is free and open source, released under the **GNU Affero General Public License v3.0** (see [LICENSE](LICENSE)).

The AGPL's network clause means: anyone who modifies Stride and offers it as a network service (SaaS, hosted deployment for third parties) must publish the complete source of their modified version. Self-hosting and private use are unrestricted.

## Star history

[![Star History Chart](https://api.star-history.com/svg?repos=aguyonp/stride&type=Date)](https://star-history.com/#aguyonp/stride&Date)
