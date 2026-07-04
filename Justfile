# This template is customized by project-init ritual based on the project's language
# and tool set. 

PROJECT_NAME := `git config --get remote.origin.url | sed -E 's/.*[:\/]([^\/]+)\/([^\/]+)\.git$/\1\/\2/'`

list:
    @just --list

# Build the sandbox container
build-sandbox:
    podman machine init --disk-size 30 >/dev/null 2>&1 || true
    podman machine start >/dev/null 2>&1 || true
    podman rmi localhost/asimi/sandbox/{{PROJECT_NAME}}:latest 2>/dev/null || true
    podman build -t localhost/asimi/sandbox/{{PROJECT_NAME}}:latest -f .agents/sandbox/Dockerfile .

# Clean up the sandbox container
clean-sandbox:
    podman rmi localhost/asimi/sandbox/{{PROJECT_NAME}}:latest

# Install project dependencies (language-specific) — customize for your project
install:
    go mod download

# Run linter & formatter (language-specific) — customize for your project
lint:
    go vet ./...
    gofmt -l -s .

# Run tests (language-specific) — customize for your project
test: install
    go test ./... -v -timeout 120s

# Start the program or server — customize for your project
run: build
    ./webexec

# Build the project — customize for your project
build: install
    go generate ./...
    go build -o webexec .

# Clean build artifacts and caches — customize for your project
clean:
    rm -f webexec
    go clean -cache -testcache

# Install system dependencies (Go toolchain + go-gitver for go generate)
bootstrap:
    @command -v go >/dev/null 2>&1 || { echo "Go 1.25+ is required. Install from https://go.dev/doc/install"; exit 1; }
    go version
    go install git.rootprojects.org/root/go-gitver/v2@latest
