# ─────────────────────────────────────────────
# Service Metadata
# ─────────────────────────────────────────────
SERVICE        := b2c2-adapter
PKG            := github.com/Checker-Finance/adapters
BIN            := ./bin/$(SERVICE)
MAIN           := ./$(SERVICE)/cmd/$(SERVICE)/main.go

# ─────────────────────────────────────────────
# Build Settings
# ─────────────────────────────────────────────
GO             := go
GOFLAGS        := -trimpath
LDFLAGS        := -s -w -X '$(PKG)/version.BuildTime=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")'

# Docker / AWS
ECR_REPO       := 730335471935.dkr.ecr.us-east-2.amazonaws.com
TAG            := $(shell cat VERSION 2>/dev/null || echo latest)
IMAGE          := $(ECR_REPO)/$(SERVICE):$(TAG)

# ─────────────────────────────────────────────
# Default Target
# ─────────────────────────────────────────────
.PHONY: all
all: build

# ─────────────────────────────────────────────
# Build & Run (run from repo root)
# ─────────────────────────────────────────────
.PHONY: build
build:
	@echo "Building $(SERVICE)..."
	@cd .. && $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(SERVICE)/$(BIN) $(MAIN)

.PHONY: run
run:
	@echo "Running $(SERVICE)..."
	@cd .. && $(GO) run $(MAIN)

.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin coverage.out

# ─────────────────────────────────────────────
# Test, Benchmark, Lint
# ─────────────────────────────────────────────
.PHONY: test
test:
	@echo "Running unit tests..."
	@cd .. && $(GO) test ./$(SERVICE)/... -count=1 -race -v

.PHONY: bench
bench:
	@echo "Running benchmarks..."
	@cd .. && $(GO) test -bench=. -benchmem ./$(SERVICE)/...

.PHONY: cover
cover:
	@echo "Running coverage..."
	@cd .. && $(GO) test ./$(SERVICE)/... -coverprofile=$(SERVICE)/coverage.out
	@$(GO) tool cover -func=$(SERVICE)/coverage.out

.PHONY: fmt
fmt:
	@echo "Formatting code..."
	@cd .. && $(GO) fmt ./$(SERVICE)/...

.PHONY: lint
lint:
	@echo "Linting..."
	@cd .. && golangci-lint run --timeout=5m

# ─────────────────────────────────────────────
# Docker
# ─────────────────────────────────────────────
.PHONY: docker-build
docker-build:
	@echo "Building Docker image $(IMAGE)..."
	@docker build -f $(SERVICE)/Dockerfile -t $(IMAGE) ..

.PHONY: docker-push
docker-push: docker-build
	@echo "Pushing image to ECR..."
	@aws ecr get-login-password --region us-east-2 | docker login --username AWS --password-stdin $(ECR_REPO)
	@docker push $(IMAGE)

# ─────────────────────────────────────────────
# Local Infrastructure
# ─────────────────────────────────────────────
.PHONY: up
up:
	@echo "Starting RabbitMQ..."
	@docker compose up -d rabbitmq

.PHONY: down
down:
	@echo "Stopping containers..."
	@docker compose down

# ─────────────────────────────────────────────
# Utility
# ─────────────────────────────────────────────
.PHONY: version
version:
	@echo "Version: $(TAG)"

.PHONY: help
help:
	@echo "Checker $(SERVICE) — available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

## ---------------------------
## Version bump
## ---------------------------

bump-patch:
	@old=$$(cat VERSION); \
	new=$$(echo $$old | awk -F. '{ $$3++; print $$1"."$$2"."$$3 }'); \
	echo $$new > VERSION; \
	echo "Bumped patch version: $$old → $$new"

bump-minor:
	@old=$$(cat VERSION); \
	new=$$(echo $$old | awk -F. '{ $$2++; $$3=0; print $$1"."$$2"."$$3 }'); \
	echo $$new > VERSION; \
	echo "Bumped minor version: $$old → $$new"

bump-major:
	@old=$$(cat VERSION); \
	new=$$(echo $$old | awk -F. '{ $$1++; $$2=0; $$3=0; print $$1"."$$2"."$$3 }'); \
	echo $$new > VERSION; \
	echo "Bumped major version: $$old → $$new"
