# Stride

Stride pulls your training and recovery data (Garmin Connect Personal API, Strava, Open-Meteo weather), merges duplicates, computes biomechanics, fatigue, aerobic efficiency, joint impact, load and readiness, tracks your plan, goals and pain, and turns any session into a compact Markdown prompt for your AI coach — or sends it to your own LLM (Gemini, Claude, OpenAI, Ollama). Generated or imported workouts land in the calendar and export as `.FIT` for the watch. Multi-user, passkey-only sign-in.

![Stride dashboard (demo data)](docs/screenshot.png)

## Quick start

```bash
git clone git@github.com:aguyonp/stride.git && cd stride
grep -qs STRIDE_MASTER_KEY .env || printf 'STRIDE_MASTER_KEY=%s\nSTRIDE_UID=%s\nSTRIDE_GID=%s\n' \
  "$(openssl rand -base64 32)" "$(id -u)" "$(id -g)" >> .env
mkdir -p data && docker compose up -d
open http://localhost:8080
```

- **First account = admin**; others join by invitation (`/admin`).
- Garmin, Strava, your LLM key and notification webhooks are all optional, set per user in the app.
- **Back up `STRIDE_MASTER_KEY`**: it encrypts every stored credential.
- Passkeys need HTTPS except on `localhost` (set `STRIDE_PUBLIC_URL=https://…` behind a proxy).
- Private image: `docker login ghcr.io` with a `read:packages` token.

## Architecture

![Architecture](docs/architecture.svg)

## Images

CI tests, then pushes `ghcr.io/aguyonp/stride` (amd64, arm64): `main` → `latest`, other branches → `dev`, plus `sha-<commit>`. Use `STRIDE_TAG=dev` for the dev image; update with `docker compose pull && docker compose up -d`.

## Configuration (`.env`)

| Variable | Default | Purpose |
| --- | --- | --- |
| `STRIDE_MASTER_KEY` | **required** | AES-256 key for credentials at rest |
| `STRIDE_PUBLIC_URL` | `http://localhost:8080` | Public URL (passkeys, OAuth, invite links) |
| `STRIDE_REGISTRATION` | `invite` | `invite` or `open` |
| `STRIDE_STRAVA_CLIENT_ID` / `_SECRET` / `_WEBHOOK_TOKEN` | — | Enable Strava |
| `STRIDE_HOST_BIND` / `STRIDE_HOST_PORT` | `127.0.0.1` / `8080` | Published address |
| `STRIDE_ALLOW_PRIVATE_EGRESS` | `false` | Allow a local LLM / webhook (e.g. Ollama) |

Full list: [docs/SPEC.md](docs/SPEC.md) §9.

## Development

```bash
make dev            # Go API on :8080 (in-memory cache)
make dev-frontend   # Vite on :5173 (npm run dev:mock for mocked data)
make test           # Go + frontend tests
make docker-up      # build the image locally (compose.dev.yml)
```

Docs: [specification](docs/SPEC.md) · [Garmin API facts](docs/GARMIN_API.md).
