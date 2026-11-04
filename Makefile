# Patella — raccourcis de développement
VERSION ?= 1.0.0
BIN     ?= bin/patella
LDFLAGS := -s -w -X main.version=$(VERSION)
# .env local chargé par le backend en dev (le binaire lit .env du CWD, ici backend/)
ENV_FILE_ARG := $(if $(wildcard .env),ENV_FILE=../.env,)

.PHONY: help dev-backend dev-frontend frontend-build build test docker-up docker-down docker-logs clean

help:
	@echo "make dev-backend   API Go sur :8080 (DATA_DIR=./data, .env racine)"
	@echo "make dev-frontend  Vite sur :5173 (proxy /api → :8080)"
	@echo "make build         binaire $(BIN) avec le SPA embarqué"
	@echo "make test          go vet + go test + vitest"
	@echo "make docker-up     build + démarrage docker compose"
	@echo "make docker-logs   suivre les logs du conteneur"

dev-backend:
	cd backend && $(ENV_FILE_ARG) DATA_DIR=../data LOG_LEVEL=$${LOG_LEVEL:-debug} go run ./cmd/server

dev-frontend:
	cd frontend && npm run dev

frontend-build:
	cd frontend && npm ci --no-audit --no-fund && npm run build

# Copie temporaire du backend pour embarquer frontend/dist sans modifier le placeholder versionné.
build: frontend-build
	rm -rf .build && mkdir -p .build bin
	cp -R backend .build/backend
	rm -rf .build/backend/internal/web/dist && cp -R frontend/dist .build/backend/internal/web/dist
	cd .build/backend && CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o ../../$(BIN) ./cmd/server
	rm -rf .build
	@echo "✅ $(BIN) ($(VERSION))"

test:
	cd backend && go vet ./... && go test ./...
	cd frontend && npx vitest run

docker-up:
	mkdir -p data
	PATELLA_UID=$$(id -u) PATELLA_GID=$$(id -g) PATELLA_VERSION=$(VERSION) docker compose up -d --build

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f --tail=200 patella

clean:
	rm -rf bin .build
