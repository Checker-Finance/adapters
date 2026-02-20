# Stage 1 — Builder
FROM golang:1.25 AS builder

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    GO111MODULE=on

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .
RUN go build -o ./bin/braza-adapter ./cmd/braza-adapter/main.go

# Stage 2 — Runtime
FROM alpine:3.20

LABEL org.opencontainers.image.title="braza-adapter" \
      org.opencontainers.image.description="Checker Finance adapter for Braza" \
      org.opencontainers.image.vendor="Checker Finance" \
      org.opencontainers.image.source="https://github.com/Checker-Finance/adapters" \
      org.opencontainers.image.licenses="Proprietary"

RUN apk add --no-cache ca-certificates tzdata
RUN addgroup -S app && adduser -S app -G app
USER app

WORKDIR /app
COPY --from=builder /app/bin/braza-adapter .

ENV LOG_LEVEL=info

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:9020/health || exit 1

ENTRYPOINT ["./braza-adapter"]
