# Stride

Stride merges your Garmin and Strava data, computes biomechanics, efficiency, impact, load and readiness, and turns each session into an AI-coach prompt (copied, or sent to your own LLM). Plans export to `.FIT`. Multi-user, passkeys only.

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
- **Back up `STRIDE_MASTER_KEY`**: it encrypts every stored credential.
- Passkeys need HTTPS except on `localhost`. Private image: `docker login ghcr.io` (`read:packages`).

## Architecture

![Architecture](docs/architecture.svg)

## Release cycle

One SemVer string, no `v`: `VERSION` file = git tag = image tag = version in Settings.

| Trigger | Image `ghcr.io/aguyonp/stride` | Version |
| --- | --- | --- |
| push to a branch | `dev` | `3.0.0-dev` |
| merge into `main` | `prerelease` | `3.0.0-pre` |
| tag `3.0.0` | `latest` + `3.0.0` + GitHub release | `3.0.0` |

1. Bump `VERSION` in your PR (`make changelog` previews the notes), merge, validate `prerelease`.
2. On an up-to-date `main`, `make release` pushes the tag with its changelog; CI builds `latest` + `3.0.0` and publishes the release (a tag ≠ `VERSION` fails).
3. Deploy or roll back: `STRIDE_TAG=<tag> docker compose up -d`.

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
