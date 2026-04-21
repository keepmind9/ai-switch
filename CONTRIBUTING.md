# Contributing to AI Switch

Thanks for your interest in contributing! This document covers the basics.

## Development Setup

### Prerequisites

- Go 1.25+
- Node.js 18+ (for frontend development)

### Getting Started

```bash
# Clone the repo
git clone https://github.com/keepmind9/ai-switch.git
cd ai-switch

# Copy example config
cp config.example.yaml config.yaml

# Build and run
make build
./bin/server -c config.yaml

# Or run in dev mode
make dev
```

### Frontend Development

```bash
make ui-dev    # Start Vite dev server with HMR
make build-ui  # Build frontend assets
```

## Making Changes

### Code Style

- Run `make fmt` before committing (Go formatting)
- Follow existing code patterns in the codebase

### Commit Messages

Use conventional commit prefixes:

- `feat:` new feature
- `fix:` bug fix
- `docs:` documentation
- `refactor:` code refactoring
- `opt:` performance optimization
- `security:` security fixes
- `chore:` build/tooling

Max 150 characters for the subject line.

### Testing

- All new features must include unit tests
- Run `make test` to verify
- Use `github.com/stretchr/testify` for assertions
- Table-driven tests for multiple scenarios

### Building

```bash
make build      # fmt + vet + compile
make build-all  # build frontend + Go binary
make test       # run tests
make lint       # fmt + vet
```

## Pull Requests

1. Fork the repository
2. Create a feature branch from `main`
3. Make your changes with clear, atomic commits
4. Ensure `make build` and `make test` pass
5. Submit a PR with a clear description of the change

## Reporting Issues

- Use [GitHub Issues](https://github.com/keepmind9/ai-switch/issues)
- Include steps to reproduce, expected behavior, and actual behavior
- Include relevant config (redact API keys)
