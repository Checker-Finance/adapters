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

# Copy xfx-adapter source
COPY xfx-adapter/ ./xfx-adapter/

# Build binary
RUN go build -trimpath -o ./xfx-adapter/bin/xfx-adapter ./xfx-adapter/cmd/xfx-adapter/main.go

# ============================================================
# Stage 2 — Runtime
# ============================================================
FROM alpine:3.20

LABEL org.opencontainers.image.title="xfx-adapter" \
      org.opencontainers.image.description="Checker Finance adapter for XFX Trading API" \
      org.opencontainers.image.vendor="Checker Finance" \
      org.opencontainers.image.source="https://github.com/Checker-Finance/adapters" \
      org.opencontainers.image.licenses="Proprietary"

RUN apk add --no-cache ca-certificates tzdata

RUN addgroup -S app && adduser -S app -G app
USER app

WORKDIR /app

COPY --from=builder /app/xfx-adapter/bin/xfx-adapter .

ENV LOG_LEVEL=info \
    XFX_POLL_INTERVAL=15s

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:9030/health || exit 1

ENTRYPOINT ["./xfx-adapter"]
