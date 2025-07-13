# CloudAWSync Makefile

# Build variables
BINARY_NAME=cloudawsync
VERSION?=1.0.0
BUILD_DIR=build
DIST_DIR=dist
GO_FILES=$(shell find . -name "*.go" -type f)

# Go build flags
LDFLAGS=-ldflags="-s -w -X main.version=$(VERSION)"
RACE_FLAGS=-race

# Default target
.PHONY: all
all: build

# Build the binary
.PHONY: build
build: $(BUILD_DIR)/$(BINARY_NAME)

$(BUILD_DIR)/$(BINARY_NAME): $(GO_FILES) go.mod go.sum
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

# Build with race detection (for development)
.PHONY: build-race
build-race:
	@echo "Building $(BINARY_NAME) with race detection..."
	@mkdir -p $(BUILD_DIR)
	go build $(RACE_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-race .

# Cross-compile for different platforms
.PHONY: build-all
build-all: build-linux build-darwin build-windows

.PHONY: build-linux
build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-linux-arm64 .

.PHONY: build-darwin
build-darwin:
	@echo "Building for macOS..."
	@mkdir -p $(DIST_DIR)
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64 .

.PHONY: build-windows
build-windows:
	@echo "Building for Windows..."
	@mkdir -p $(DIST_DIR)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-windows-amd64.exe .

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	go test -v ./...

.PHONY: test-race
test-race:
	@echo "Running tests with race detection..."
	go test -race -v ./...

.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Benchmarks
.PHONY: bench
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

# Linting and formatting
.PHONY: lint
lint:
	@echo "Running linter..."
	golangci-lint run

.PHONY: fmt
fmt:
	@echo "Formatting code..."
	go fmt ./...

.PHONY: vet
vet:
	@echo "Running go vet..."
	go vet ./...

.PHONY: check
check: fmt vet lint test

# Dependencies
.PHONY: deps
deps:
	@echo "Downloading dependencies..."
	go mod download

.PHONY: deps-update
deps-update:
	@echo "Updating dependencies..."
	go get -u ./...
	go mod tidy

.PHONY: deps-verify
deps-verify:
	@echo "Verifying dependencies..."
	go mod verify

# Installation
.PHONY: install
install: build
	@echo "Installing $(BINARY_NAME)..."
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	sudo chmod +x /usr/local/bin/$(BINARY_NAME)

.PHONY: uninstall
uninstall:
	@echo "Uninstalling $(BINARY_NAME)..."
	sudo rm -f /usr/local/bin/$(BINARY_NAME)

# SystemD service
.PHONY: install-service
install-service: install
	@echo "Installing systemd service..."
	$(BUILD_DIR)/$(BINARY_NAME) -generate-systemd
	sudo cp $(BINARY_NAME).service /etc/systemd/system/
	sudo systemctl daemon-reload
	@echo "Service installed. Enable with: sudo systemctl enable $(BINARY_NAME)"

.PHONY: uninstall-service
uninstall-service:
	@echo "Uninstalling systemd service..."
	sudo systemctl stop $(BINARY_NAME) || true
	sudo systemctl disable $(BINARY_NAME) || true
	sudo rm -f /etc/systemd/system/$(BINARY_NAME).service
	sudo systemctl daemon-reload

# Configuration
.PHONY: install-config
install-config:
	@echo "Installing default configuration..."
	sudo mkdir -p /etc/$(BINARY_NAME)
	sudo cp config.yaml.example /etc/$(BINARY_NAME)/config.yaml
	@echo "Edit /etc/$(BINARY_NAME)/config.yaml to configure the service"

# Docker
.PHONY: docker-build
docker-build:
	@echo "Building Docker image..."
	docker build -t $(BINARY_NAME):$(VERSION) .
	docker tag $(BINARY_NAME):$(VERSION) $(BINARY_NAME):latest

.PHONY: docker-run
docker-run:
	@echo "Running Docker container..."
	docker run -d --name $(BINARY_NAME) \
		-v /path/to/config:/etc/$(BINARY_NAME) \
		-v /path/to/sync:/data \
		-p 9090:9090 \
		$(BINARY_NAME):latest

# Development
.PHONY: run
run: build
	@echo "Running $(BINARY_NAME) in development mode..."
	$(BUILD_DIR)/$(BINARY_NAME) -daemon=false -log-level=debug

.PHONY: run-config
run-config: build
	@echo "Generating sample configuration..."
	$(BUILD_DIR)/$(BINARY_NAME) -generate-config

# Cleaning
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR) $(DIST_DIR)
	rm -f coverage.out coverage.html
	rm -f $(BINARY_NAME).service
	rm -f cloudawsync-config.yaml

.PHONY: clean-all
clean-all: clean
	@echo "Cleaning all generated files..."
	go clean -cache -testcache -modcache

# Security scanning
.PHONY: security
security:
	@echo "Running security scan..."
	gosec ./...

# Generate documentation
.PHONY: docs
docs:
	@echo "Generating documentation..."
	godoc -http=:6060 &
	@echo "Documentation server started at http://localhost:6060"

# Release
.PHONY: release
release: clean build-all test
	@echo "Creating release $(VERSION)..."
	@mkdir -p $(DIST_DIR)/$(VERSION)
	@cp README.md LICENSE config.yaml.example $(DIST_DIR)/$(VERSION)/
	@for binary in $(DIST_DIR)/$(BINARY_NAME)-*; do \
		if [ -f "$$binary" ]; then \
			cp "$$binary" $(DIST_DIR)/$(VERSION)/; \
		fi; \
	done
	@cd $(DIST_DIR) && tar -czf $(BINARY_NAME)-$(VERSION).tar.gz $(VERSION)/
	@echo "Release package created: $(DIST_DIR)/$(BINARY_NAME)-$(VERSION).tar.gz"

# Help
.PHONY: help
help:
	@echo "CloudAWSync Build System"
	@echo ""
	@echo "Available targets:"
	@echo "  build         - Build the binary"
	@echo "  build-race    - Build with race detection"
	@echo "  build-all     - Cross-compile for all platforms"
	@echo "  test          - Run tests"
	@echo "  test-race     - Run tests with race detection"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  bench         - Run benchmarks"
	@echo "  lint          - Run linter"
	@echo "  fmt           - Format code"
	@echo "  vet           - Run go vet"
	@echo "  check         - Run fmt, vet, lint, and test"
	@echo "  deps          - Download dependencies"
	@echo "  deps-update   - Update dependencies"
	@echo "  install       - Install binary to /usr/local/bin"
	@echo "  uninstall     - Remove binary from /usr/local/bin"
	@echo "  install-service - Install systemd service"
	@echo "  uninstall-service - Remove systemd service"
	@echo "  install-config - Install default configuration"
	@echo "  docker-build  - Build Docker image"
	@echo "  docker-run    - Run Docker container"
	@echo "  run           - Run in development mode"
	@echo "  run-config    - Generate sample configuration"
	@echo "  clean         - Clean build artifacts"
	@echo "  clean-all     - Clean all generated files"
	@echo "  security      - Run security scan"
	@echo "  docs          - Generate documentation"
	@echo "  release       - Create release package"
	@echo "  help          - Show this help message"
