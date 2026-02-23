# Stage 1 — Builder
FROM golang:1.25 AS builder

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    GO111MODULE=on

WORKDIR /app

# Copy root module files first for cache efficiency
COPY go.mod go.sum ./

# Copy all shared packages
COPY pkg/ ./pkg/
COPY internal/ ./internal/

# Copy kiiex-adapter source
COPY kiiex-adapter/ ./kiiex-adapter/

RUN go mod download
RUN go build -trimpath -o ./kiiex-adapter/bin/kiiex-adapter ./kiiex-adapter/cmd/kiiex-adapter/main.go

# Stage 2 — Runtime
FROM alpine:3.20

LABEL org.opencontainers.image.title="kiiex-adapter" \
      org.opencontainers.image.description="Checker Finance adapter for Kiiex/AlphaPoint" \
      org.opencontainers.image.vendor="Checker Finance" \
      org.opencontainers.image.source="https://github.com/Checker-Finance/adapters" \
      org.opencontainers.image.licenses="Proprietary"

RUN apk add --no-cache ca-certificates tzdata
RUN addgroup -S app && adduser -S app -G app
USER app

WORKDIR /app
COPY --from=builder /app/kiiex-adapter/bin/kiiex-adapter .
COPY --from=builder /app/kiiex-adapter/configs /configs

ENV SERVER_PORT=8082
ENV KIIEX_WEBSOCKET_URL=wss://api.kiire.alphaprod.net/WSGateway
ENV CHECKER_OTC_ADAPTER_PROVIDER=kiiex
ENV AWS_REGION=us-east-2
ENV SYMBOL_MAPPING_PATH=/configs/symbol_mapping.json

EXPOSE 8082

ENTRYPOINT ["./kiiex-adapter"]
