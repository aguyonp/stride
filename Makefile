# Stride — development shortcuts
VERSION ?= $(shell cat VERSION)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
BIN     ?= bin/stride
LDFLAGS := -s -w -X stride/internal/buildinfo.Version=$(VERSION) -X stride/internal/buildinfo.Commit=$(COMMIT) -X stride/internal/buildinfo.Date=$(BUILD_DATE)
# the backend reads ENV_FILE (default ./.env relative to backend/)
ENV_FILE_ARG := $(if $(wildcard .env),ENV_FILE=../.env,)

COMPOSE_DEV := docker compose -f compose.yml -f compose.dev.yml

.PHONY: help dev dev-frontend frontend-build build test test-e2e up docker-up docker-down logs keygen changelog release clean diagram

help:
	@echo "make keygen        how to generate STRIDE_MASTER_KEY"
	@echo "make dev           Go API on :8080 (in-memory cache, ./data, root .env)"
	@echo "make dev-frontend  Vite on :5173 (proxy /api -> :8080)"
	@echo "make build         $(BIN) with the embedded SPA"
	@echo "make test          go vet + go test + vitest"
	@echo "make test-e2e      Playwright browser smoke against the mock build (Chromium)"
	@echo "make up            pull and start the published image (STRIDE_TAG=latest|prerelease|dev|X.Y.Z)"
	@echo "make changelog     preview the changelog since the last release tag"
	@echo "make release       tag main as \$$(cat VERSION) with its changelog and push (CI builds latest + X.Y.Z)"
	@echo "make docker-up     build locally and start docker compose (stride + valkey)"
	@echo "make logs          follow the stride container logs"
	@echo "make diagram       regenerate docs/architecture.svg from scripts/gen_architecture.py (CI checks byte-identity)"

keygen:
	@echo "Generate the master key and paste it into .env as STRIDE_MASTER_KEY=<value>:"
	@echo "  openssl rand -base64 32"
	@echo "Keep it safe: losing it makes the stored provider credentials unreadable."

dev:
	cd backend && $(ENV_FILE_ARG) STRIDE_DATA_DIR=../data LOG_LEVEL=$${LOG_LEVEL:-debug} go run ./cmd/stride

dev-frontend:
	cd frontend && npm run dev

frontend-build:
	cd frontend && npm ci --no-audit --no-fund && npm run build

# Builds from a temporary copy so the committed SPA placeholder stays untouched.
build: frontend-build
	rm -rf .build && mkdir -p .build bin
	cp -R backend .build/backend
	rm -rf .build/backend/internal/web/dist && cp -R frontend/dist .build/backend/internal/web/dist
	cd .build/backend && CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o ../../$(BIN) ./cmd/stride
	rm -rf .build
	@echo "✅ $(BIN) ($(VERSION))"

test:
	cd backend && go vet ./... && go test ./...
	cd frontend && npm test

# Browser smoke (issue #91): builds the mock-mode production build and runs Playwright
# (Chromium) against it. No Go backend or secrets required.
test-e2e:
	cd frontend && npm run test:e2e

up:
	mkdir -p data
	docker compose pull && docker compose up -d

docker-up:
	mkdir -p data
	STRIDE_UID=$$(id -u) STRIDE_GID=$$(id -g) STRIDE_BUILD_VERSION=$(VERSION) STRIDE_COMMIT=$(COMMIT) STRIDE_BUILD_DATE=$(BUILD_DATE) $(COMPOSE_DEV) up -d --build

docker-down:
	docker compose down

logs:
	docker compose logs -f --tail=200 stride

# Regenerate the architecture diagram from its Python generator. CI runs --check and
# fails when the checked-in SVG is not byte-identical to the generated output.
diagram:
	uv run scripts/gen_architecture.py

changelog:
	@scripts/changelog.sh HEAD

release:
	@test "$$(git rev-parse --abbrev-ref HEAD)" = main || { echo "release: switch to main first"; exit 1; }
	@test -z "$$(git status --porcelain)" || { echo "release: working tree not clean"; exit 1; }
	@git fetch --quiet --tags origin main
	@test "$$(git rev-parse HEAD)" = "$$(git rev-parse origin/main)" || { echo "release: main is not in sync with origin/main"; exit 1; }
	@if git rev-parse -q --verify "refs/tags/$(VERSION)" >/dev/null; then echo "release: tag $(VERSION) exists, bump the VERSION file"; exit 1; fi
	git tag -a "$(VERSION)" -m "Stride $(VERSION)" -m "$$(scripts/changelog.sh HEAD)"
	git push origin "$(VERSION)"

clean:
	rm -rf bin .build
