# Contributing

This guide covers how to set up a development environment and contribute to Talos.

## Prerequisites

- **Go 1.21+** -- [download](https://go.dev/dl/)
- **Docker** -- [install](https://docs.docker.com/get-docker/)
- **Git**

## Development Setup

### Clone the Repository

```bash
git clone https://github.com/logic-roastery/project-talos.git
cd project-talos
```

### Configure Environment

```bash
cp .env.example .env
```

Edit `.env` and set at minimum:

```bash
TALOS_SESSION_SECRET=dev-secret-change-me
```

### Build and Run

```bash
make build        # Build the binary to ./bin/talos
make dev          # Start dev server on port 4001
make dev-watch    # Auto-reload with air (file watcher)
make dev-fresh    # Reset database and restart
```

### Run Tests

```bash
make test         # Run tests with race detection
make test-cover   # Run tests with HTML coverage report
make vet          # Static analysis
```

## Makefile Commands

| Command | Description |
|---------|-------------|
| `make build` | Build the binary to `./bin/talos` |
| `make dev` | Start dev server on port 4001 |
| `make dev-watch` | Auto-reload on file changes (requires [air](https://github.com/air-verse/air)) |
| `make dev-fresh` | Delete database and restart |
| `make test` | Run all tests with `-race` flag |
| `make test-cover` | Generate HTML coverage report |
| `make vet` | Run `go vet` static analysis |
| `make ps` | List Talos-managed Docker containers |

## Code Structure

```
cmd/talos/              Application entrypoint
internal/
  auth/                 Authentication service, password hashing
  backup/               Backup manager, scheduler, restore
  config/               Environment variable loading
  crypto/               AES-256-GCM encryption primitives
  deploy/               Deployment engine (blue/green orchestration)
  domain/               Domain types (App, Deploy, Service, User, etc.)
  github/               GitHub App auth, webhook verification, workflow parsing
  proxy/traefik/        Traefik route management
  runtime/docker/       Docker API wrapper
  server/               HTTP server, middleware, route registration
    handlers/           Request handlers (app, deploy, service, backup, auth, pages)
  services/             Managed service provisioning, credential handling
  store/                SQLite persistence, repositories, migrations
web/                    Embedded HTML templates and static assets
```

### Key Design Decisions

- **Single binary** -- Talos compiles to a single static binary with embedded templates.
- **SQLite** -- lightweight, zero-config database for the control plane.
- **chi router** -- idiomatic Go HTTP routing with middleware support.
- **Repository pattern** -- store layer abstracts database access behind interfaces.
- **Domain types** -- pure structs in `internal/domain` with no dependencies.

## Pull Request Guidelines

### Before Submitting

1. Run `make test` and ensure all tests pass.
2. Run `make vet` and fix any warnings.
3. Run `make build` and verify the binary compiles.
4. Write tests for new functionality.

### PR Description

Include:

- **What** the change does
- **Why** the change is needed
- **How** it works (if not obvious)
- **Testing** -- how you verified the change

### Commit Messages

Use conventional commit format:

```
feat: add backup scheduling
fix: handle missing env vars in deploy
docs: update installation guide
refactor: extract credential encryption
test: add store migration tests
```

### Code Style

- Follow standard Go conventions (`gofmt`, `go vet`).
- Use `slog` for structured logging (not `fmt.Println`).
- Keep handlers thin -- business logic belongs in service/manager layers.
- Use context propagation for cancellation and timeouts.
- Add comments for exported types and functions.

## Testing

### Unit Tests

Tests live alongside the code they test (e.g., `store/app_repo_test.go`).

```bash
# Run a specific package's tests
go test ./internal/store/...

# Run a specific test
go test ./internal/store/ -run TestCreateApp
```

### Test Helpers

The `internal/store/helpers_test.go` file provides shared test utilities for setting up an in-memory SQLite database.

## Reporting Issues

Open an issue on [GitHub](https://github.com/logic-roastery/project-talos/issues) with:

- Steps to reproduce
- Expected behavior
- Actual behavior
- Talos version (`talos --version`)
- OS and Docker version

## License

By contributing, you agree that your contributions will be licensed under the project's license. See [LICENSE](https://github.com/logic-roastery/project-talos/blob/master/LICENSE).
