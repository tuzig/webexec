# webexec — Agent & Contributor Guide

## Project Overview

webexec is a terminal server running over WebRTC with support for
signaling over SSH or HTTP. It listens for connection requests, executes
commands over pseudo ttys, and pipes their I/O over WebRTC data channels.

## Language & Tooling

- **Language:** Go (module `github.com/tuzig/webexec`, Go 1.25+)
- **Build system:** `go build`, `go generate` (uses `go-gitver` for versioning)
- **Test framework:** Go standard `testing` + `stretchr/testify`
- **Task runner:** `just` (see `Justfile`)

## Common Commands

```bash
# Bootstrap (verify Go, install go-gitver)
just bootstrap

# Install dependencies
just install          # or: go mod download

# Run linter & formatter checks
just lint             # or: go vet ./... && gofmt -l -s .

# Run all tests (verbose, 120s timeout)
just test             # or: go test ./... -v -timeout 120s

# Build the binary (runs go generate then go build)
just build            # or: go generate ./... && go build -o webexec .

# Run the server
just run              # or: ./webexec

# Clean build artifacts
just clean            # or: rm -f webexec && go clean -cache -testcache
```

## Build Conventions

- `go generate ./...` must be run before building — it generates version
  info via `go-gitver` (see `//go:generate` directive in `webexec.go`).
- The binary output is named `webexec` and placed in the repo root.
- Build tags are used for platform-specific files (e.g.,
  `dup2_wrapper.go` vs `dup2_wrapper_armlinux.go`, `pidfile` package).

## Coding Style

- Follow `gofmt` formatting (use `gofmt -s` for simplification).
- Run `go vet ./...` before submitting — no warnings allowed.
- Package `main` contains the server entry point and core logic.
  Sub-packages: `httpserver`, `peers`, `pidfile`.
- Tests live alongside source files (`*_test.go`).
  Some test files use `//go:build ignore` and are excluded from normal
  test runs (e.g., `session_test.go`).
- Integration tests are under `aatp/` — run with `./aatp/test`.

## Ports & Networking

- **TCP 7777** — HTTP signaling server (default: `127.0.0.1:7777`).
- **UDP 60000–61000** — WebRTC data channel range.
- When running inside a sandbox, `host.containers.internal` is used to
  reach services on the host.

## Configuration

- Config file is TOML format, sections: `[log]`, `[net]`, `[timeouts]`,
  `[ice_servers]`, `[env]`, `[peerbook]`.
- Default config is embedded in `conf.go`.
- Env vars: `PEERBOOK_UID`, `PEERBOOK_HOST`, `PEERBOOK_NAME` for
  PeerBook integration.
