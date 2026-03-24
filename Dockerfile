# Build Stage
FROM golang:1.24-bookworm AS builder

WORKDIR /app

# Install build dependencies for DuckDB (C++)
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    g++ \
    libc6-dev \
    git \
    && rm -rf /var/lib/apt/lists/*

# Pre-install swag
RUN go install github.com/swaggo/swag/cmd/swag@latest

# Cache dependencies
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Generate Swagger docs (Cached unless models/handlers/main change)
COPY main.go ./
COPY handlers/ ./handlers/
COPY models/ ./models/
RUN swag init -g main.go --dir .

# Copy remaining source (static, etc)
COPY . .

# Build with cache mounts
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux go build -o /portal -ldflags="-s -w" .

# --- Runtime ---
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /portal /usr/local/bin/portal

RUN mkdir -p /data
VOLUME /data

ENV DB_PATH=/data/portal.db
ENV PORT=80

EXPOSE 80

CMD ["portal"]
