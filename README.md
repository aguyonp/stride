# Stride

Stride pulls your training and recovery data (Garmin Connect Personal API, Strava, Open-Meteo weather), computes biomechanics, fatigue, load and recovery insights, tracks your plan, goals and pain, and turns any session into a compact Markdown prompt for your AI coach. Multi-user, passkey-only sign-in.

## Quick start

```bash
git clone git@github.com:aguyonp/stride.git && cd stride
grep -qs STRIDE_MASTER_KEY .env || printf 'STRIDE_MASTER_KEY=%s\nSTRIDE_UID=%s\nSTRIDE_GID=%s\n' \
  "$(openssl rand -base64 32)" "$(id -u)" "$(id -g)" >> .env
mkdir -p data && docker compose up -d
open http://localhost:8080
```

- The **first account becomes admin**; others need an invitation from `/admin` (or open registration).
- Connect Garmin (personal API key) and/or Strava in the onboarding wizard; both are optional.
- **Back up `STRIDE_MASTER_KEY`**: it encrypts every stored provider credential.
- Passkeys require HTTPS except on `localhost`: behind a reverse proxy, set `STRIDE_PUBLIC_URL=https://…`.
- Private image: `docker login ghcr.io -u <github-user>` with a token that has `read:packages`.

## Images

CI (`.github/workflows/ci.yml`) runs Go and frontend tests, then pushes a multi-arch image (amd64, arm64) to `ghcr.io/aguyonp/stride`:

| Branch | Tag |
| --- | --- |
| `main` | `latest` (production) |
| any other branch | `dev` |
| every build | `sha-<commit>` |

Run the dev image with `STRIDE_TAG=dev docker compose up -d`; update with `docker compose pull && docker compose up -d`.

## Configuration (`.env`)

| Variable | Default | Purpose |
| --- | --- | --- |
| `STRIDE_MASTER_KEY` | **required** | AES-256 key for credentials at rest |
| `STRIDE_PUBLIC_URL` | `http://localhost:8080` | Public URL (passkeys, OAuth, invite links) |
| `STRIDE_REGISTRATION` | `invite` | `invite` or `open` |
| `STRIDE_STRAVA_CLIENT_ID` / `_SECRET` / `_WEBHOOK_TOKEN` | — | Enable Strava |
| `STRIDE_HOST_BIND` / `STRIDE_HOST_PORT` | `127.0.0.1` / `8080` | Published address |
| `STRIDE_TAG` | `latest` | Image tag |

Full list: [docs/SPEC.md](docs/SPEC.md) §9.

## Development

```bash
make dev            # Go API on :8080 (in-memory cache)
make dev-frontend   # Vite on :5173, cd frontend && npm run dev:mock for mocked data
make test           # Go + frontend tests
make docker-up      # build the image locally (compose.dev.yml)
```

Docs: [specification](docs/SPEC.md) · [Garmin API facts](docs/GARMIN_API.md).
