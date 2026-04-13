# Build Stage
FROM golang:1.26-bookworm AS builder

WORKDIR /app

# Pre-install swag
RUN go install github.com/swaggo/swag/cmd/swag@latest

# Cache dependencies
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Generate Swagger docs (Cached unless models/handlers/store/main change)
COPY main.go ./
COPY handlers/ ./handlers/
COPY models/ ./models/
COPY store/ ./store/
RUN swag init -g main.go --dir . --output ./docs

# Copy remaining source (static, etc)
COPY . .

# Build with cache mounts (pure Go, no CGO needed)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /portal -ldflags="-s -w" .

# --- Runtime ---
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /portal /usr/local/bin/portal

ENV PORT=80

EXPOSE 80

CMD ["portal"]
