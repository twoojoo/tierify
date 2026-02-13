# Tierify

A flexible, composable limit engine for managing pricing plan tiers. Built as an internal backend service for consumption by other applications.

## Features

- **Flexible Tiers**: Define pricing tiers with versioning, migration, and deprecation
- **Multiple Limit Types**: Absolute, time-based, cumulative, burst, compound, and feature limits
- **Tenant Hierarchy**: Organize tenants in hierarchical trees (org → dept → user)
- **Global vs Local Limits**: Global limits apply to entire hierarchies, local limits per-tenant
- **Transactions**: Application-level transactions with locking for consistency
- **Suspension & Blocking**: Fine-grained control over tenant access
- **Database Agnostic**: Generic repository interfaces (SQLite implementation included)
- **Stateless**: Horizontally scalable with no in-process state

## Quick Start

### Prerequisites

- Go 1.21 or higher
- SQLite3

### Installation

```bash
# Clone the repository
git clone <repo-url>
cd tierify

# Install dependencies
make deps

# (Optional) Install development tools
make install
```

### Running the Server

```bash
# Build and run
make run

# Or just build
make build
./tierify

# Or run in development mode with hot reload (requires air)
make dev
```

The server will start on `http://localhost:8080` by default.

### Configuration

Copy the example environment file and customize as needed:

```bash
cp .env.example .env
```

Configuration options:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Server port |
| `HOST` | `0.0.0.0` | Server host |
| `DB_TYPE` | `sqlite` | Database type (currently only sqlite) |
| `DB_CONNECTION_STRING` | `tierify.db` | Database connection string |
| `ALLOW_USAGE_BELOW_ZERO` | `false` | Allow usage to go negative |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `LOG_FORMAT` | `json` | Log format (json, text) |

## Development

### Available Make Commands

```bash
make help              # Show all available commands
make deps              # Install dependencies
make install           # Install development tools
make build             # Build the binary
make run               # Build and run the server
make dev               # Run with hot reload (requires air)
make test              # Run all tests
make test-v            # Run tests with verbose output
make test-coverage     # Generate coverage report
make lint              # Run linter
make fmt               # Format code
make clean             # Clean build artifacts
make check             # Run all checks (fmt, vet, lint, test)
make quick             # Quick check (fmt + test)
```

### Testing

```bash
# Run all tests
make test

# Run tests with verbose output
make test-v

# Generate coverage report
make test-coverage
# Opens coverage.html in browser

# Run tests in watch mode (requires gotestsum)
make test-watch
```

**Test Status**: 154 tests passing

### Project Structure

```
tierify/
├── cmd/tierify/          # Entry point
├── internal/
│   ├── config/           # Configuration
│   ├── handlers/         # HTTP handlers
│   ├── services/         # Business logic
│   ├── repositories/     # Data access interfaces
│   ├── models/           # Domain models
│   ├── limiter/          # Limit evaluation engine
│   ├── errors/           # Error types
│   ├── logger/           # Logger interface
│   └── db/
│       └── sqlite/       # SQLite implementation
├── Makefile              # Build commands
├── PLANNING.md           # Project architecture & roadmap
└── README.md             # This file
```

## API Overview

All endpoints are prefixed with `/api/v1`.

### Tier Management

- `POST /tiers` - Create a tier
- `GET /tiers/{key}` - Get latest tier version
- `PUT /tiers/{key}` - Update tier (creates new version)
- `DELETE /tiers/{key}/versions/{version}` - Deprecate a version
- `GET /tiers/{key}/versions/{version}/limits` - Get tier limits

### Tenant Management

- `POST /tenants` - Create a tenant
- `GET /tenants/{id}` - Get tenant details
- `PUT /tenants/{id}/tier` - Assign/change tier
- `PUT /tenants/{id}/status` - Update status (active/suspended/blocked)
- `DELETE /tenants/{id}?mode={cascade|orphan}` - Delete tenant
- `GET /tenants/{id}/children` - Get child tenants
- `GET /tenants/{id}/ancestors` - Get ancestor chain
- `GET /tenants/{id}/history` - Get tier change history

### Usage Operations

- `POST /tenants/{id}/check` - Check limits (read-only)
- `POST /tenants/{id}/increment` - Increment usage
- `POST /tenants/{id}/decrement` - Decrement usage
- `POST /tenants/{id}/reset` - Reset usage
- `GET /tenants/{id}/usage` - Get all usage
- `GET /tenants/{id}/usage/{limitKey}` - Get specific usage

**Transaction headers** (optional):
- `X-Transaction-ID` - Client-provided transaction ID
- `X-Transaction-TTL` - TTL in seconds

### Transaction Management

- `POST /transactions/{id}/commit` - Commit transaction
- `POST /transactions/{id}/rollback` - Rollback transaction
- `GET /transactions/{id}` - Get transaction status

### Health Check

- `GET /health` - Health check endpoint

## Example Usage

### Create a Tier

```bash
curl -X POST http://localhost:8080/api/v1/tiers \
  -H "Content-Type: application/json" \
  -d '{
    "key": "professional",
    "name": "Professional Plan",
    "limits": [
      {
        "key": "apiCalls",
        "kind": "absolute",
        "scope": "local",
        "config": {
          "max_value": 10000
        }
      }
    ]
  }'
```

### Create a Tenant

```bash
curl -X POST http://localhost:8080/api/v1/tenants \
  -H "Content-Type: application/json" \
  -d '{
    "external_id": "user-123",
    "tier_key": "professional"
  }'
```

### Check Limits

```bash
curl -X POST http://localhost:8080/api/v1/tenants/{tenant-id}/check \
  -H "Content-Type: application/json" \
  -d '{
    "operations": [
      {"limit_key": "apiCalls", "amount": 1}
    ]
  }'
```

### Increment Usage

```bash
curl -X POST http://localhost:8080/api/v1/tenants/{tenant-id}/increment \
  -H "Content-Type: application/json" \
  -d '{
    "operations": [
      {"limit_key": "apiCalls", "amount": 1}
    ]
  }'
```

### Using Transactions

```bash
# Transactional increment
curl -X POST http://localhost:8080/api/v1/tenants/{tenant-id}/increment \
  -H "Content-Type: application/json" \
  -H "X-Transaction-ID: tx-123" \
  -H "X-Transaction-TTL: 30" \
  -d '{
    "operations": [
      {"limit_key": "apiCalls", "amount": 5}
    ]
  }'

# Commit
curl -X POST http://localhost:8080/api/v1/transactions/tx-123/commit

# Or rollback
curl -X POST http://localhost:8080/api/v1/transactions/tx-123/rollback
```

## Implementation Status

| Phase | Status | Tests |
|-------|--------|-------|
| Phase 1 - Foundation (MVP) | ✅ Complete | 93 |
| Phase 2 - Hierarchy & Advanced Limits | ✅ Complete | 111 |
| Phase 3 - Transactions | ✅ Complete | 140 |
| Phase 4 - Suspension & Blocking | ✅ Complete | 154 |
| Phase 5 - Additional DB Backends | 🔜 Planned | - |
| Phase 6 - Polish & Future | 🔜 Planned | - |

**Current test coverage**: 154 tests passing

## Architecture

Tierify follows a clean architecture pattern:

```
HTTP Request
    ↓
Handlers (chi router)
    ↓
Services (business logic)
    ↓
Repositories (generic interfaces)
    ↓
DB Implementations (SQLite, etc.)
```

### Key Design Decisions

- **Aggregated usage storage**: One record per tenant per limit, not event-sourced
- **Implicit time-based reset**: No background schedulers, checked on each operation
- **Application-level transactions**: Database-agnostic, stored as records
- **Stateless service**: No in-process state, horizontally scalable
- **Flexible limit engine**: Extensible via interface pattern

See [PLANNING.md](PLANNING.md) for detailed architecture documentation.

## License

[Your License Here]

## Contributing

[Your Contributing Guidelines Here]
