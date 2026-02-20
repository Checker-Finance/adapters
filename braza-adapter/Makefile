# ─────────────────────────────────────────────
# Service Metadata
# ─────────────────────────────────────────────
SERVICE        := braza-adapter
PKG            := github.com/Checker-Finance/adapters/braza-adapter
BIN            := ./bin/$(SERVICE)
MAIN           := ./cmd/$(SERVICE)/main.go

# ─────────────────────────────────────────────
# ⚙️ Build Settings
# ─────────────────────────────────────────────
GO             := go
GOFLAGS        := -trimpath -mod=readonly
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
# 🧰 Build & Run
# ─────────────────────────────────────────────
.PHONY: build
build:
	@echo "Building $(SERVICE)..."
	@$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BIN) $(MAIN)

.PHONY: run
run:
	@echo "Running $(SERVICE)..."
	@$(GO) run $(MAIN)

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
	@$(GO) test ./... -count=1 -race -v

.PHONY: bench
bench:
	@echo "⚡ Running benchmarks..."
	@$(GO) test -bench=. -benchmem ./...

.PHONY: cover
cover:
	@echo "📊 Running coverage..."
	@$(GO) test ./... -coverprofile=coverage.out
	@$(GO) tool cover -func=coverage.out

.PHONY: fmt
fmt:
	@echo "✨ Formatting code..."
	@$(GO) fmt ./...

.PHONY: lint
lint:
	@echo "🔎 Linting..."
	@golangci-lint run --timeout=5m

# ─────────────────────────────────────────────
# 🐳 Docker Build & Push
# ─────────────────────────────────────────────
.PHONY: docker-build
docker-build:
	@echo "🐳 Building Docker image $(IMAGE)..."
	@docker build -t $(IMAGE) .

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

.PHONY: generate-k8s

generate-k8s:
	@if [ -z "$(ENV)" ]; then \
		echo "❌ ENV not set. Use: make generate-k8s ENV=prod"; \
		exit 1; \
	fi
	@if [ ! -f "secrets/$(ENV).env" ]; then \
		echo "❌ secrets/$(ENV).env not found."; \
		exit 1; \
	fi
	@echo "📦 Generating Deployment and ExternalSecret for $(ENV)..."

	@mkdir -p k8s/generated/$(ENV)

	@echo "---" > k8s/generated/$(ENV)/deployment.yaml
	@echo "apiVersion: apps/v1" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "kind: Deployment" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "metadata:" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "  name: $(SERVICE)" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "  namespace: $(ENV)" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "spec:" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "  replicas: 1" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "  selector:" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "    matchLabels:" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "      app: $(SERVICE)" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "  template:" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "    metadata:" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "      labels:" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "        app: $(SERVICE)" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "    spec:" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "      containers:" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "      - name: $(SERVICE)" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "        image: $(ECR_REPO)" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "        imagePullPolicy: Always" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "        envFrom:" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "        - secretRef:" >> k8s/generated/$(ENV)/deployment.yaml
	@echo "            name: $(SERVICE)-secret" >> k8s/generated/$(ENV)/deployment.yaml

	@echo "---" > k8s/generated/$(ENV)/external-secret.yaml
	@echo "apiVersion: external-secrets.io/v1" >> k8s/generated/$(ENV)/external-secret.yaml
	@echo "kind: ExternalSecret" >> k8s/generated/$(ENV)/external-secret.yaml
	@echo "metadata:" >> k8s/generated/$(ENV)/external-secret.yaml
	@echo "  name: $(SERVICE)-secret" >> k8s/generated/$(ENV)/external-secret.yaml
	@echo "  namespace: $(ENV)" >> k8s/generated/$(ENV)/external-secret.yaml
	@echo "spec:" >> k8s/generated/$(ENV)/external-secret.yaml
	@echo "  refreshInterval: 1h" >> k8s/generated/$(ENV)/external-secret.yaml
	@echo "  secretStoreRef:" >> k8s/generated/$(ENV)/external-secret.yaml
	@echo "    name: aws-secret-store" >> k8s/generated/$(ENV)/external-secret.yaml
	@echo "    kind: ClusterSecretStore" >> k8s/generated/$(ENV)/external-secret.yaml
	@echo "  target:" >> k8s/generated/$(ENV)/external-secret.yaml
	@echo "    name: $(    SERVICE)-secret" >> k8s/generated/$(ENV)/external-secret.yaml
	@echo "  data:" >> k8s/generated/$(ENV)/external-secret.yaml

	@cat secrets/$(ENV).env | grep -v '^#' | grep '=' | while IFS== read -r key value; do \
		echo "  - secretKey: $$key" >> k8s/generated/$(ENV)/external-secret.yaml; \
		echo "    remoteRef:" >> k8s/generated/$(ENV)/external-secret.yaml; \
		echo "      key: $(ENV)/$(    SERVICE)" >> k8s/generated/$(ENV)/external-secret.yaml; \
		echo "      property: $$key" >> k8s/generated/$(ENV)/external-secret.yaml; \
	done

	@echo "✅ Generated: k8s/generated/$(ENV)/deployment.yaml"
	@echo "✅ Generated: k8s/generated/$(ENV)/external-secret.yaml"

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
