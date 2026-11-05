# syntax=docker/dockerfile:1.7
# Stride: React SPA embedded in the Go binary, distroless non-root runtime.

ARG VERSION=2.0.0

FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS backend
ARG VERSION
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src/backend
ENV CGO_ENABLED=0
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY backend/ ./
# The Vite build replaces the committed placeholder embedded with //go:embed all:dist.
RUN rm -rf internal/web/dist
COPY --from=frontend /src/frontend/dist/ ./internal/web/dist/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/stride ./cmd/stride \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.title="Stride" \
      org.opencontainers.image.source="https://github.com/aguyonp/stride" \
      org.opencontainers.image.description="Sports telemetry to AI-coach prompts (Garmin, Strava)"
COPY --from=backend /out/stride /app/stride
COPY --from=backend --chown=65532:65532 /out/data /data
# readable by any UID (compose may run as the host user); the default /home/nonroot is not
WORKDIR /app
ENV STRIDE_DATA_DIR=/data \
    STRIDE_LISTEN_ADDR=:8080 \
    TZ=Europe/Paris \
    LOG_FORMAT=pretty
VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD ["/app/stride", "healthcheck"]
ENTRYPOINT ["/app/stride"]
CMD ["serve"]
