# ─────────────────────────────────────────────
# Service Metadata
# ─────────────────────────────────────────────
SERVICE        := rio-adapter
PKG            := github.com/Checker-Finance/adapters
BIN            := ./bin/$(SERVICE)
MAIN           := ./$(SERVICE)/cmd/$(SERVICE)/main.go

# ─────────────────────────────────────────────
# ⚙️ Build Settings
# ─────────────────────────────────────────────
GO             := go
GOFLAGS        := -trimpath
LDFLAGS        := -s -w -X '$(PKG)/version.BuildTime=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")'

# Docker / AWS
ECR_REPO       := 730335471935.dkr.ecr.us-east-2.amazonaws.com
TAG            := $(shell cat VERSION 2>/dev/null || echo latest)
IMAGE          := $(ECR_REPO)/$(SERVICE):$(TAG)

# ─────────────────────────────────────────────
# 🧩 Default Target
# ─────────────────────────────────────────────
.PHONY: all
all: build

# ─────────────────────────────────────────────
# 🧰 Build & Run (run from repo root)
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
	@echo "🧹 Cleaning build artifacts..."
	@rm -rf bin coverage.out

# ─────────────────────────────────────────────
# 🧪 Test, Benchmark, Lint
# ─────────────────────────────────────────────
.PHONY: test
test:
	@echo "🧪 Running unit tests..."
	@cd .. && $(GO) test ./$(SERVICE)/... ./internal/... ./pkg/... -count=1 -race -v

.PHONY: integration-test
integration-test:
	@echo "Running integration tests against Rio sandbox..."
	@cd .. && $(GO) test -v -tags integration -run TestIntegration ./$(SERVICE)/internal/rio/ -count=1

.PHONY: bench
bench:
	@echo "⚡ Running benchmarks..."
	@cd .. && $(GO) test -bench=. -benchmem ./$(SERVICE)/... ./internal/... ./pkg/...

.PHONY: cover
cover:
	@echo "📊 Running coverage..."
	@cd .. && $(GO) test ./$(SERVICE)/... ./internal/... ./pkg/... -coverprofile=$(SERVICE)/coverage.out
	@$(GO) tool cover -func=$(SERVICE)/coverage.out

.PHONY: fmt
fmt:
	@echo "✨ Formatting code..."
	@cd .. && $(GO) fmt ./$(SERVICE)/... ./internal/... ./pkg/...

.PHONY: lint
lint:
	@echo "🔎 Linting..."
	@cd .. && golangci-lint run --timeout=5m

# ─────────────────────────────────────────────
# 🐳 Docker Build & Push
# ─────────────────────────────────────────────
.PHONY: docker-build
docker-build:
	@echo "🐳 Building Docker image $(IMAGE)..."
	@docker build -f $(SERVICE)/Dockerfile -t $(IMAGE) ..

.PHONY: docker-push
docker-push: docker-build
	@echo "🚢 Pushing image to ECR..."
	@aws ecr get-login-password --region us-east-2 | docker login --username AWS --password-stdin $(ECR_REPO)
	@docker push $(IMAGE)

# ─────────────────────────────────────────────
# 🧩 Local Dev Tools
# ─────────────────────────────────────────────
.PHONY: up
up:
	@echo "🧩 Starting local NATS + Redis..."
	@docker compose up -d nats redis

.PHONY: down
down:
	@docker compose down

.PHONY: logs
logs:
	@docker compose logs -f $(SERVICE)

# ─────────────────────────────────────────────
# 🔍 Utility
# ─────────────────────────────────────────────
.PHONY: version
version:
	@echo "📦 Version: $(TAG)"

.PHONY: help
help:
	@echo "Checker $(SERVICE) — available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: push-secret
push-secret:
	@if [ -z "$(ENV)" ]; then \
		echo "❌ ENV not set. Use: make push-secret ENV=prod"; \
		exit 1; \
	fi
	@if [ ! -f "secrets/$(ENV).env.json" ]; then \
		echo "❌ secrets/$(ENV).env.json does not exist."; \
		exit 1; \
	fi
	@echo "🔐 Uploading secrets for environment: $(ENV)"
	@if aws secretsmanager describe-secret --secret-id $(ENV)/$(SERVICE) > /dev/null 2>&1; then \
		echo "🔁 Updating existing secret..."; \
		aws secretsmanager put-secret-value \
			--secret-id $(ENV)/$(SERVICE) \
			--secret-string file://secrets/$(ENV).env.json; \
	else \
		echo "✨ Creating new secret..."; \
		aws secretsmanager create-secret \
			--name $(ENV)/$(SERVICE) \
			--description "Secrets for $(SERVICE) $(ENV) environment" \
			--secret-string file://secrets/$(ENV).env.json; \
	fi

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
