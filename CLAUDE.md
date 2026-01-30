# Gonka NOP CLI (Go)

## Development Priorities

1. **Production-ready** - Tested, reliable, proper logging
2. **Security first** - Safe key handling, input validation, no secrets in logs
3. **Go best practices** - Idiomatic code, effective error handling
4. **DRY/KISS/SOLID** - Simple, maintainable, well-structured

## Commands

```bash
go build -o gonka-nop ./cmd/gonka-nop
go test ./...
go run ./cmd/gonka-nop setup
```

## MANDATORY: Before Finishing Go Changes

**Always run before committing or saying work is complete:**

```bash
gofmt -w .              # Fix formatting (struct alignment, spacing)
go vet ./...            # Static analysis
golangci-lint run       # Full lint (catches gosec, misspell, unparam)
```

Common gotchas:
- Struct fields must be aligned (gofmt enforces this)
- Use American English: `canceled` not `cancelled`
- File permissions: `0600` for files, `0750` for directories
- Unused params: prefix with `_` like `_ context.Context`

## Architecture

```
cmd/gonka-nop/main.go     # Entry point
internal/
├── cmd/                   # Cobra commands (root.go, setup.go)
├── phases/                # Setup phases (01-06)
├── config/                # State management
├── status/                # Status display
└── ui/                    # Terminal UI (spinner, prompt, output)
```

## Code Style

- Use standard Go formatting (gofmt/goimports)
- Error wrapping: `fmt.Errorf("context: %w", err)`
- Cobra for CLI commands
- Survey v2 for interactive prompts
- Briandowns/spinner for progress indicators

## Phase Implementation Pattern

Each phase in `internal/phases/` follows:
```go
type Phase interface {
    Name() string
    Run(state *config.State) error
}
```

## GPU Detection

Use `nvidia-smi` subprocess:
```go
cmd := exec.Command("nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader")
```

## Template Generation

Templates in `templates/` directory, use Go text/template:
- `docker-compose.yml.tmpl`
- `config.env.tmpl`
- `node-config.json.tmpl`

## Reference Patterns

- CLI wizard flow: @../airdao-nop/src/start.ts
- Docker configs: @../gonka/deploy/join/docker-compose.yml
- Node configs: @../gonka/deploy/join/node-config-*.json
