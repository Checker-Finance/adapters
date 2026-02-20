# ============================================================
# Stage 1 — Builder
# ============================================================
FROM golang:1.25 AS builder

# Enable Go modules and turn on static linking
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    GO111MODULE=on

# Create app directory
WORKDIR /app

# Copy go.mod and go.sum first (for better layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source
COPY . .

# Run tests (optional but recommended)
# RUN make test

# Build binary
RUN make build || go build -o ./bin/rio-adapter ./cmd/rio-adapter/main.go

# ============================================================
# Stage 2 — Runtime (scratch/alpine for small image)
# ============================================================
FROM alpine:3.20

# OCI image metadata
LABEL org.opencontainers.image.title="rio-adapter" \
      org.opencontainers.image.description="Checker Finance adapter for Rio Trading API" \
      org.opencontainers.image.vendor="Checker Finance" \
      org.opencontainers.image.source="https://github.com/Checker-Finance/adapters" \
      org.opencontainers.image.licenses="Proprietary"

# Install certificates (needed for HTTPS calls to Rio API)
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -S app && adduser -S app -G app
USER app

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/bin/rio-adapter .

# Env vars (can be overridden in K8s/Argo)
ENV LOG_LEVEL=info \
    POLL_INTERVAL_SEC=30

# Health check for non-K8s runtimes (ECS, Docker Compose)
HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:9010/health || exit 1

# Run binary
ENTRYPOINT ["./rio-adapter"]