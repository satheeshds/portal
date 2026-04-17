# Build Stage
FROM golang:1.26-bookworm AS builder

ARG COMMIT_SHA=unknown
WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source required for the platform service
COPY db/ ./db/
COPY platform/ ./platform/

# Build with cache mounts (pure Go, no CGO needed)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /platform -ldflags="-s -w" ./platform

# --- Runtime ---
FROM debian:bookworm-slim

ARG COMMIT_SHA=unknown
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

LABEL watchlog.project-repo="satheeshds/portal"
LABEL watchlog.commit-sha="${COMMIT_SHA}"
LABEL watchlog.monitor="true"
LABEL watchlog.service="portal-platform"

COPY --from=builder /platform /usr/local/bin/platform

CMD ["platform"]
