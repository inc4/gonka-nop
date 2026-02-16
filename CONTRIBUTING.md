# Contributing to Gonka NOP

Thank you for your interest in contributing to Gonka NOP. This project benefits from both code contributions and operational knowledge from Gonka validators.

## Ways to Contribute

- **Code** - Bug fixes, new features, test coverage improvements
- **Operational insights** - GPU configurations that work well, common pitfalls, troubleshooting tips
- **Documentation** - Improve setup guides, add examples, fix typos
- **Bug reports** - Detailed reports with logs and environment info
- **Feature requests** - Ideas for improving the operator experience

## Getting Started

### Prerequisites

- Go 1.25+
- [golangci-lint](https://golangci-lint.run/welcome/install/) v2.8.0+
- Docker (for integration testing)

### Development Setup

```bash
git clone https://github.com/inc4/gonka-nop.git
cd gonka-nop/inc4-gonka-nop
go mod download
go build ./...
go test -race ./...
golangci-lint run
```

### Build and Test

```bash
go build -o gonka-nop ./cmd/gonka-nop    # Build binary
go test -race ./...                       # Run tests with race detector
golangci-lint run                         # Run linter
```

## Contribution Workflow

1. **Find or create an issue** - Check [open issues](https://github.com/inc4/gonka-nop/issues). Look for `good first issue` or `help wanted` labels. If your idea isn't tracked yet, open an issue first to discuss it.

2. **Fork and branch** - Fork the repo and create a branch from `main`:
   ```bash
   git checkout -b fix/description-of-change
   ```

3. **Make your changes** - Follow the code standards below.

4. **Run checks before pushing**:
   ```bash
   gofmt -w .
   go vet ./...
   golangci-lint run
   go test -race ./...
   ```

5. **Open a pull request** - Target the `main` branch. Fill in the PR template.

6. **Address review feedback** - A maintainer will review your PR. Be patient and responsive to feedback.

## Code Standards

### Formatting and Linting

All code must pass:

```bash
gofmt -w .           # Format code
go vet ./...         # Static analysis
golangci-lint run    # Full lint suite
```

CI enforces these on every PR.

### Testing

- Add tests for new functionality
- Run `go test -race ./...` before submitting
- Aim to maintain or improve test coverage

### Error Handling

- Wrap errors with context: `fmt.Errorf("loading config: %w", err)`
- Return errors to callers; don't log and return
- Use `%w` for errors that callers may need to inspect

### File Permissions

- `0600` for files containing sensitive data
- `0750` for directories

## Commit Messages

Use conventional commit format:

```
feat: add NVLink topology detection
fix: correct TP/PP calculation for 4-GPU setups
docs: update GPU configuration examples
test: add config generation edge cases
chore: update golangci-lint to v2.8.0
```

Keep the first line under 72 characters. Add a blank line and longer description if needed.

## Operator Contributions

If you run Gonka nodes and have operational knowledge to share, we especially value:

- **GPU benchmarks** - Performance data for specific GPU configurations
- **Configuration examples** - Working `node-config.json` setups for different hardware
- **Troubleshooting guides** - Solutions to problems you've encountered
- **Network insights** - Reliable peers, sync strategies, DDoS mitigation

Open an issue with the `operator-insight` label to share this kind of knowledge.

## What Not to Submit

- Changes that add secrets, credentials, or API keys
- Cosmetic-only changes (formatting, whitespace) to files you didn't otherwise modify
- Large refactors without prior discussion in an issue
- Dependencies without clear justification

## Questions?

Open a [GitHub Issue](https://github.com/inc4/gonka-nop/issues) for questions about contributing.
