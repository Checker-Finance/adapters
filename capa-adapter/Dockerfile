# ============================================================
# Stage 1 — Builder
# ============================================================
FROM golang:1.25 AS builder

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    GO111MODULE=on

WORKDIR /app

# Copy go.mod and go.sum first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy shared packages
COPY pkg/ ./pkg/
COPY internal/ ./internal/

# Copy capa-adapter source
COPY capa-adapter/ ./capa-adapter/

# Build binary
RUN go build -trimpath -o ./capa-adapter/bin/capa-adapter ./capa-adapter/cmd/capa-adapter/main.go

# ============================================================
# Stage 2 — Runtime
# ============================================================
FROM alpine:3.20

LABEL org.opencontainers.image.title="capa-adapter" \
      org.opencontainers.image.description="Checker Finance adapter for Capa ramp platform" \
      org.opencontainers.image.vendor="Checker Finance" \
      org.opencontainers.image.source="https://github.com/Checker-Finance/adapters" \
      org.opencontainers.image.licenses="Proprietary"

RUN apk add --no-cache ca-certificates tzdata

RUN addgroup -S app && adduser -S app -G app
USER app

WORKDIR /app

COPY --from=builder /app/capa-adapter/bin/capa-adapter .

ENV LOG_LEVEL=info \
    CAPA_POLL_INTERVAL=30s

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:9060/health || exit 1

ENTRYPOINT ["./capa-adapter"]
