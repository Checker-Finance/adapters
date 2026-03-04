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

# Copy b2c2-adapter source
COPY b2c2-adapter/ ./b2c2-adapter/

RUN go mod download
RUN go build -trimpath -o ./b2c2-adapter/bin/b2c2-adapter ./b2c2-adapter/cmd/b2c2-adapter/main.go

# Stage 2 — Runtime
FROM alpine:3.20

LABEL org.opencontainers.image.title="b2c2-adapter" \
      org.opencontainers.image.description="Checker Finance adapter for B2C2 Markets" \
      org.opencontainers.image.vendor="Checker Finance" \
      org.opencontainers.image.source="https://github.com/Checker-Finance/adapters" \
      org.opencontainers.image.licenses="Proprietary"

RUN apk add --no-cache ca-certificates tzdata
RUN addgroup -S app && adduser -S app -G app
USER app

WORKDIR /app
COPY --from=builder /app/b2c2-adapter/bin/b2c2-adapter .

ENV ENV=prod
ENV AWS_REGION=us-east-2
ENV LOG_LEVEL=info
ENV HEALTH_PORT=9050
ENV B2C2_DEFAULT_BASE_URL=https://api.b2c2.net

EXPOSE 9050

ENTRYPOINT ["./b2c2-adapter"]
