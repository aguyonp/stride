# Stride

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

## Highlights in 4.0.0

- **Versioned SQL Migrations (`pressly/goose`)**: Embedded `.up.sql` / `.down.sql` migrations with lossless v3.x upgrades and startup SQLite `PRAGMA` schema verification.
- **End-to-End OpenAPI v3 Contract**: Reflection-driven OpenAPI 3.0.3 generation (`GET /openapi.yaml`, `docs/openapi.yaml`) paired with `openapi-typescript` (`npm run generate:api`).
- **Valkey Async Worker (`Asynq`) & Native SSE (`/api/events/stream`)**: Weighted priority queues (`critical`, `default`, `low`) with exponential backoff retries for Garmin/Strava sync, `.FIT`/`.GPX`/`.TCX` imports, outbound webhooks, and **token-by-token LLM streaming** — zero frontend polling.
- **Prometheus Observability & Version Endpoint**: Native `GET /metrics` (HTTP P50/P95/P99, cache hits/latency, worker queues, GPS/FIT point ingestion, external API error rates) and public `GET /version`.
- **Macro / Méso / Micro Periodization (`/plan`)**: Three-tier relational cycles with assimilation weeks, zoomable Gantt timeline, and Planned vs Actual volume & load tracking.
- **Body Stress Map & Correlations (`/body`)**: 6 runner anatomical zones (`0–10` pain + `J+1` cold stiffness), automatic shoe drop/wear, `D-`, `GCT`, and impact correlation engine, plus automatic AI prompt injection.
- **Physio & Coach Read-Only Share (`/share/:token`) & Command Palette (`Cmd+K`)**: Encrypted, expiring, 1-click revocable clinical share links and 100 % keyboard-driven universal search/actions.

## Architecture

![Architecture](docs/architecture.svg)

Detailed guides: [Architecture & Data Flows](docs/ARCHITECTURE.md) · [Prometheus Metrics & Alerts](docs/METRICS.md) · [Specification](docs/SPEC.md) · [OpenAPI v3 Schema](docs/openapi.yaml) · [Garmin API Facts](docs/GARMIN_API.md).

## Release cycle

One SemVer string, no `v`: `VERSION` file = git tag = image tag = `GET /version` & Settings footer.

| Trigger | Image `ghcr.io/aguyonp/stride` | Version |
| --- | --- | --- |
| push to a branch | `dev` | `4.0.0-dev` |
| merge into `main` | `prerelease` | `4.0.0-pre` |
| tag `4.0.0` | `latest` + `4.0.0` + GitHub release | `4.0.0` |

1. Bump `VERSION` in your PR (`make changelog` previews the notes), merge, validate `prerelease`.
2. On an up-to-date `main`, `make release` pushes the tag with its changelog; CI builds `latest` + `4.0.0` and publishes the release (a tag ≠ `VERSION` fails).
3. Deploy or roll back: `STRIDE_TAG=<tag> docker compose up -d`.

## Configuration (`.env`)

| Variable | Default | Purpose |
| --- | --- | --- |
| `STRIDE_MASTER_KEY` | **required** | AES-256-GCM key for credentials and share links at rest |
| `STRIDE_PUBLIC_URL` | `http://localhost:8080` | Public URL (passkeys, OAuth, invite & share links) |
| `STRIDE_VALKEY_URL` | — (`redis://valkey:6379/0` in Compose) | Valkey URL for TTL cache, Asynq worker queues, and SSE Pub/Sub |
| `STRIDE_REGISTRATION` | `invite` | `invite` or `open` |
| `STRIDE_STRAVA_CLIENT_ID` / `_SECRET` / `_WEBHOOK_TOKEN` | — | Enable Strava OAuth & webhooks |
| `STRIDE_HOST_BIND` / `STRIDE_HOST_PORT` | `127.0.0.1` / `8080` | Published address |
| `STRIDE_ALLOW_PRIVATE_EGRESS` | `false` | Allow a local LLM / webhook (e.g. Ollama) |

Full list: [docs/SPEC.md](docs/SPEC.md) §9.

## Development

```bash
make dev            # Go API + in-memory worker on :8080
make dev-frontend   # Vite on :5173 (npm run dev:mock for mocked data)
make openapi        # Regenerate docs/openapi.{json,yaml} + frontend/src/api/schema.d.ts
make test           # Go race tests + frontend Vitest suite
make docker-up      # Build locally and start docker compose (stride + valkey)
```
