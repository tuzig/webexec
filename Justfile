PROJECT_NAME := "daonb-webexec"

# Default recipe - show available commands
default:
    @just --list

# Install Go dependencies
install:
    go mod download
    go mod verify

# Build the webexec binary
build:
    go generate .
    go build -o webexec .

# Run all tests with race detector
test:
    go test -v -race ./...

# Run a specific test
test-single TEST:
    go test -v -race -run {{TEST}} ./...

# Run linting and formatting
lint:
    go fmt ./...
    go vet ./...

# Clean build artifacts
clean:
    rm -f webexec
    rm -f xversion.go
    rm -rf dist

# Install system dependencies (for container bootstrap)
bootstrap:
    @echo "Installing system dependencies..."
    # Go is already installed in the base image
    # Install any additional tools if needed

# Run the webexec agent in debug mode
run:
    go run . start --debug

# Run integration tests
test-integration:
    go test -v -tags=integration ./...

# Generate version information
generate:
    go generate .

# Build release binaries using goreleaser
build-release:
    goreleaser build --snapshot --clean

# Show current status
status:
    ./webexec status || echo "webexec not running or not built"

# Initialize webexec configuration
init:
    ./webexec init

# Start webexec agent
start:
    ./webexec start

# Stop webexec agent
stop:
    ./webexec stop

# Restart webexec agent
restart:
    ./webexec restart

# Build the sandbox container
build-sandbox:
    podman machine init --disk-size 30 >/dev/null 2>&1 || true
    podman machine start >/dev/null 2>&1 || true
    podman build -t localhost/asimi-sandbox-{{PROJECT_NAME}}:latest -f .agents/sandbox/Dockerfile .

# Clean up the sandbox container
clean-sandbox:
    podman rmi localhost/asimi-sandbox-{{PROJECT_NAME}}:latest
