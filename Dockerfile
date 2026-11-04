# syntax=docker/dockerfile:1.7
# Patella — image unique : SPA (Vite) embarqué dans le binaire Go, runtime distroless non-root.

ARG VERSION=1.0.0

# ---------- 1) Frontend (Vite + React) ----------
FROM node:24-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# ---------- 2) Backend (Go, statique) ----------
FROM golang:1.27-alpine AS backend
ARG VERSION
WORKDIR /src/backend
ENV CGO_ENABLED=0
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY backend/ ./
# Le build Vite remplace le placeholder « build frontend absent » embarqué (//go:embed all:dist).
RUN rm -rf internal/web/dist
COPY --from=frontend /src/frontend/dist/ ./internal/web/dist/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/patella ./cmd/server \
 && mkdir -p /out/data

# ---------- 3) Runtime (distroless static, non-root, tzdata embarqué dans le binaire) ----------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=backend /out/patella /app/patella
COPY --from=backend --chown=65532:65532 /out/data /data
ENV DATA_DIR=/data \
    TZ=Europe/Paris \
    PORT=8080 \
    LOG_FORMAT=pretty
VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/app/patella", "healthcheck"]
ENTRYPOINT ["/app/patella"]
