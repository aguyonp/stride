# Stride — development shortcuts
VERSION ?= 2.0.0
BIN     ?= bin/stride
LDFLAGS := -s -w -X main.version=$(VERSION)
# the backend reads ENV_FILE (default ./.env relative to backend/)
ENV_FILE_ARG := $(if $(wildcard .env),ENV_FILE=../.env,)

.PHONY: help dev dev-frontend frontend-build build test docker-up docker-down logs keygen clean

help:
	@echo "make keygen        how to generate STRIDE_MASTER_KEY"
	@echo "make dev           Go API on :8080 (in-memory cache, ./data, root .env)"
	@echo "make dev-frontend  Vite on :5173 (proxy /api -> :8080)"
	@echo "make build         $(BIN) with the embedded SPA"
	@echo "make test          go vet + go test + vitest"
	@echo "make docker-up     build and start docker compose (stride + valkey)"
	@echo "make logs          follow the stride container logs"

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
	cd frontend && npx vitest run

docker-up:
	mkdir -p data
	STRIDE_UID=$$(id -u) STRIDE_GID=$$(id -g) STRIDE_VERSION=$(VERSION) docker compose up -d --build

docker-down:
	docker compose down

logs:
	docker compose logs -f --tail=200 stride

clean:
	rm -rf bin .build
