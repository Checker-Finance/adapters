# Braza Adapter — build and test
SERVICE        := braza-adapter
PKG            := github.com/Checker-Finance/adapters/braza-adapter
BIN            := ./bin/$(SERVICE)
MAIN           := ./cmd/$(SERVICE)/main.go

GO             := go
GOFLAGS        := -trimpath -mod=readonly

.PHONY: all build clean test fmt lint
all: build

build:
	@echo "Building $(SERVICE)..."
	@$(GO) build $(GOFLAGS) -o $(BIN) $(MAIN)

run:
	@$(GO) run $(MAIN)

clean:
	@rm -rf bin coverage.out

test:
	@$(GO) test ./... -count=1 -race

fmt:
	@$(GO) fmt ./...

lint:
	@golangci-lint run --timeout=5m

docker-build:
	@docker build -t $(SERVICE):latest .
