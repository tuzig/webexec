# webexec Development Guide

## Language & Build System

**Primary Language:** Go 1.22+

**Build System:** Just (task runner) + Go modules

## Essential Commands

### Setup & Installation
```bash
just install          # Download and verify Go dependencies
just bootstrap        # Install system dependencies (for container)
just build            # Build the webexec binary
```

### Testing
```bash
just test                    # Run all tests with race detector
just test-single TestName    # Run a specific test (e.g., TestAuth)
just test-integration        # Run integration tests
```

### Development
```bash
just run              # Run webexec in debug mode (foreground)
just lint             # Format code and run static analysis
just generate         # Generate version information
```

### Webexec Operations
```bash
just init             # Initialize webexec configuration
just start            # Start webexec agent (background)
just stop             # Stop webexec agent
just restart          # Restart webexec agent
just status           # Show agent status
```

### Build & Release
```bash
just build-release    # Build release binaries with goreleaser
just clean            # Remove build artifacts
```

### Container Management
```bash
just build-sandbox    # Build the sandbox container for Asimi
just clean-sandbox    # Remove the sandbox container
```

**Note:** The sandbox container image name is configured in `.agents/asimi.conf` under `[run_in_shell]` section as `image_name = "localhost/asimi-sandbox-daonb-webexec:latest"`.

## Code Style Guidelines

### Imports
- Order: standard library → third-party → local packages
- Use `goimports` or `go fmt` to organize automatically
- Group imports with blank lines between categories

### Formatting
- Use `go fmt` for all code (enforced by `just lint`)
- Tabs for indentation (Go standard)
- Line length: aim for 100 characters, but not strict

### Types
- Prefer explicit types for public APIs
- Use type aliases for clarity: `type AddressType string`
- Export struct fields when needed for JSON/TOML marshaling
- Use pointer receivers for methods that modify state

### Naming Conventions
- **Unexported:** camelCase (e.g., `handleResize`)
- **Exported:** PascalCase (e.g., `StartHTTPServer`)
- **Interfaces:** Single-method interfaces end in `-er` (e.g., `AuthBackend`)
- **Acronyms:** All caps (e.g., `HTTPServer`, `FP` for fingerprint, `ID`)
- **Constants:** PascalCase for exported, camelCase for unexported

### Error Handling
- Always check errors; never ignore with `_`
- Wrap errors with context: `fmt.Errorf("failed to connect: %w", err)`
- Use `Logger.Errorf()` for logging in long-running processes
- Return errors to callers; avoid panic except in init/main
- Use `cli.Exit()` for CLI command errors with appropriate exit codes

### Testing
- Use `testify/require` for assertions (preferred) or `testify/assert`
- Test files: `*_test.go` in the same package
- Integration tests: `integration_test.go` with build tags if needed
- Use helper functions like `initTest(t)` for common setup
- Table-driven tests for multiple similar cases

### Logging
- Use the global `Logger` (zap.SugaredLogger)
- Levels: `Debug`, `Info`, `Warn`, `Error`
- Include context: peer FP, connection ID, function name
- Format: `Logger.Infof("message with %s", context)`
- Structured logging for important events

### Comments
- Public APIs must have doc comments
- Doc comments start with the name: `// StartHTTPServer starts...`
- Explain "why" not "what" for complex logic
- Use `TODO:` with issue links for future work

## Project-Specific Conventions

### Architecture
- **WebRTC:** Peer connections managed via `peers` package
- **Configuration:** TOML format (see `conf.go`)
- **IPC:** Unix socket at `~/.webexec/webexec.sock`
- **Daemon:** PID file for process management
- **Signaling:** HTTP server on port 7777 (default)

### Key Components
- `webexec.go` - Main CLI and daemon logic
- `peers/` - WebRTC peer and pane management
- `httpserver/` - HTTP signaling server
- `auth.go` - Authentication backend
- `conf.go` - Configuration management
- `key.go` - Certificate generation and management

### Configuration
- Default config: `~/.webexec/`
- Certificate: `~/.webexec/certnkey.pem`
- Config file: `~/.webexec/webexec.conf`
- Logs: `~/.webexec/webexec.log`

### WebRTC Ports
- **TCP 7777:** Signaling server (configurable)
- **UDP 60000-61000:** WebRTC data channels (configurable)

### Testing Patterns
- Use `initTest(t)` for test initialization
- Mock WebRTC connections when needed
- Test both success and error paths
- Clean up resources in `defer` statements

### Dependencies
- **WebRTC:** `github.com/pion/webrtc/v3`
- **CLI:** `github.com/urfave/cli/v2`
- **Logging:** `go.uber.org/zap`
- **Config:** `github.com/pelletier/go-toml`
- **DI:** `go.uber.org/fx`

## Common Development Tasks

### Adding a New Command
1. Define command in `webexec.go` under `app.Commands`
2. Implement handler function with signature `func(c *cli.Context) error`
3. Add flags if needed
4. Update this guide with the new command

### Adding a New Control Message
1. Define message type in `peers/peer.go`
2. Add handler in `handleCTRLMsg()` in `webexec.go`
3. Implement handler function: `handleXxx(peer, msg, raw)`
4. Send ACK/NACK responses

### Debugging
- Use `just run` to run in foreground with console output
- Check logs: `~/.webexec/webexec.log`
- Check errors: `~/.webexec/webexec.err`
- Use `just status` to check agent state

## Release Process
1. Update `CHANGELOG.md`
2. Tag version: `git tag vX.Y.Z`
3. Push tag: `git push origin vX.Y.Z`
4. GitHub Actions runs goreleaser
5. Draft release is created automatically
