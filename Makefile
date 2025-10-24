.PHONY: help test build lint docker-build docker-test clean coverage

# Variables
BINARY_NAME := cherry-pick-action
DOCKER_IMAGE := rancher/cherry-pick-action
GO_VERSION := 1.22
GOLANGCI_LINT_VERSION := v1.55.2

# Default target
.DEFAULT_GOAL := help

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

test: ## Run all tests
	@echo "Running tests..."
	go test -v -race -timeout=5m ./...

coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	go test -v -race -timeout=5m -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report saved to coverage.html"

build: ## Build the binary locally
	@echo "Building $(BINARY_NAME)..."
	CGO_ENABLED=0 go build -ldflags='-w -s' -o bin/$(BINARY_NAME) ./cmd/cherry-pick-action
	@echo "Binary built at bin/$(BINARY_NAME)"

lint: ## Run linter
	@echo "Running golangci-lint..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)" && exit 1)
	golangci-lint run --timeout=5m ./...

fmt: ## Format code
	@echo "Formatting code..."
	go fmt ./...
	gofmt -s -w .

vet: ## Run go vet
	@echo "Running go vet..."
	go vet ./...

tidy: ## Tidy go modules
	@echo "Tidying go modules..."
	go mod tidy

docker-build: ## Build Docker image
	@echo "Building Docker image $(DOCKER_IMAGE):local..."
	docker build -t $(DOCKER_IMAGE):local .
	@echo "Docker image built: $(DOCKER_IMAGE):local"

docker-test: docker-build ## Test Docker image (dry-run)
	@echo "Testing Docker image with dry-run..."
	@mkdir -p /tmp/cherry-pick-test
	@echo '{"action":"closed","pull_request":{"number":1,"merged":true,"head":{"sha":"abc123"},"base":{"ref":"main"}},"repository":{"owner":{"login":"test"},"name":"repo"}}' > /tmp/cherry-pick-test/event.json
	docker run --rm \
		-e GITHUB_EVENT_NAME=pull_request \
		-e GITHUB_EVENT_PATH=/tmp/event.json \
		-e INPUT_GITHUB_TOKEN=test-token \
		-e INPUT_DRY_RUN=true \
		-e INPUT_LOG_LEVEL=debug \
		-v /tmp/cherry-pick-test:/tmp:ro \
		$(DOCKER_IMAGE):local
	@rm -rf /tmp/cherry-pick-test

clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -f coverage.out coverage.html
	rm -f $(BINARY_NAME)
	@echo "Clean complete"

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	go mod download

verify: fmt vet lint test ## Run all verification checks (format, vet, lint, test)
	@echo "All checks passed!"

.PHONY: all
all: verify build docker-build ## Run all checks and build everything
