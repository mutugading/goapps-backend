# Development Guide

## Architecture

Finance Service follows **Clean Architecture + DDD** pattern:

```
┌─────────────────────────────────────────┐
│           DELIVERY LAYER                │
│     (gRPC Handlers, Interceptors)       │
└────────────────┬────────────────────────┘
                 │ calls
┌────────────────▼────────────────────────┐
│        APPLICATION LAYER                │
│     (Command/Query Handlers)            │
└────────────────┬────────────────────────┘
                 │ uses interfaces
┌────────────────▼────────────────────────┐
│          DOMAIN LAYER                   │
│  (Entities, VOs, Repository Interface)  │
└────────────────┬────────────────────────┘
                 │ implements
┌────────────────▼────────────────────────┐
│       INFRASTRUCTURE LAYER              │
│    (PostgreSQL, Redis, Tracing)         │
└─────────────────────────────────────────┘
```

## Layer Dependencies

- ❌ Domain layer MUST NOT import from other layers
- ❌ Application layer MUST NOT import from Infrastructure or Delivery
- ✅ Infrastructure implements interfaces defined in Domain
- ✅ Delivery calls Application layer

---

## First Time Setup

### 1. Install Required Tools

```bash
# Install all development tools
make install-tools

# This installs:
# - golangci-lint (linting)
# - air (hot reload)
# - goimports (formatting)
# - grpcurl (gRPC testing)
# - migrate (database migrations)
```

### 2. Start Infrastructure

```bash
# Start PostgreSQL, Redis, Jaeger
make docker-up

# Verify services are running
docker ps

# Expected output:
# CONTAINER ID   IMAGE            STATUS         PORTS
# xxx            postgres:16      Up             0.0.0.0:5434->5432/tcp
# xxx            redis:7          Up             0.0.0.0:6379->6379/tcp
# xxx            jaegertracing    Up             various ports
```

### 3. Setup Database

```bash
# Run migrations
make migrate-up

# Seed initial data (26 UOMs)
make seed
```

### 4. Run Service

```bash
# Standard run
make run

# With hot reload (recommended for development)
make dev
```

### 5. Verify Everything Works

```bash
# Health check
curl http://localhost:8080/healthz

# List UOMs
grpcurl -plaintext -d '{"page":1,"page_size":5}' localhost:50051 finance.v1.UOMService/ListUOMs

# Create test UOM
grpcurl -plaintext -d '{"uom_code":"DEV_TEST","uom_name":"Dev Test","uom_category":"UOM_CATEGORY_QUANTITY"}' localhost:50051 finance.v1.UOMService/CreateUOM
```

---

## Common Issues & Solutions

### Issue: `go mod tidy` required
```
go: updates to go.mod needed; to update it: go mod tidy
```
**Solution:**
```bash
go mod tidy
```

### Issue: Database connection refused
```
failed to connect to postgres: connection refused
```
**Solution:**
```bash
# Check if PostgreSQL is running
docker ps | grep postgres

# Restart if needed
make docker-down && make docker-up
```

### Issue: Proto files not found
```
cannot find package "github.com/ilramdhan/goapps-backend/gen/..."
```
**Solution:**
Proto files are generated from `goapps-shared-proto`. Regenerate if needed:
```bash
cd ../../goapps-shared-proto
./scripts/gen-go.sh
```

### Issue: Redis connection error (non-critical)
```
redis: connection refused
```
**Impact:** Caching disabled, service still works with slight performance impact.
**Solution:**
```bash
docker compose -f deployments/docker-compose.yaml up -d redis
```

### Issue: OpenTelemetry schema mismatch warning
```
Dropping data due to mismatched schema version
```
**Impact:** Minor warning only, tracing still works.
**Note:** This is a non-critical warning from OTel library version mismatch.

---

## Coding Standards

### Naming Conventions

| Type | Convention | Example |
|------|------------|---------|
| Files | snake_case | `uom_repository.go` |
| Packages | lowercase | `postgres` |
| Structs/Types | PascalCase | `UOMRepository` |
| Methods | PascalCase | `CreateUOM` |
| Private | camelCase | `validateCode` |

### Error Handling

```go
// 1. Define domain errors in errors.go
var (
    ErrNotFound = errors.New("uom not found")
    ErrAlreadyExists = errors.New("uom already exists")
    ErrEmptyCode = errors.New("uom code cannot be empty")
)

// 2. Wrap errors with context
return fmt.Errorf("failed to create uom: %w", err)

// 3. Check specific errors
if errors.Is(err, uom.ErrNotFound) {
    return NotFoundResponse("UOM not found")
}
```

### Validation Response Pattern

Validation errors should be returned in `BaseResponse`, NOT as gRPC status errors:

```go
// ❌ Don't do this (returns gRPC error)
if err := h.validator.Validate(req); err != nil {
    return nil, status.Error(codes.InvalidArgument, err.Error())
}

// ✅ Do this (returns structured BaseResponse)
if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
    return &financev1.CreateUOMResponse{Base: baseResp}, nil
}
```

---

## Adding a New Module

### 1. Domain Layer (`internal/domain/newmodule/`)

```go
// entity.go - Aggregate root
type NewModule struct {
    id        uuid.UUID
    code      Code
    name      string
    createdAt time.Time
    createdBy string
}

// value_object.go - Value objects
type Code struct { value string }

// repository.go - Repository interface
type Repository interface {
    Create(ctx context.Context, entity *NewModule) error
    GetByID(ctx context.Context, id uuid.UUID) (*NewModule, error)
    // ...
}

// errors.go - Domain errors
var ErrNotFound = errors.New("newmodule not found")
```

### 2. Infrastructure Layer (`internal/infrastructure/postgres/`)

```go
// newmodule_repository.go
type NewModuleRepository struct {
    db *DB
}

func (r *NewModuleRepository) Create(ctx context.Context, entity *newmodule.NewModule) error {
    // Implementation
}
```

### 3. Application Layer (`internal/application/newmodule/`)

```go
// create_handler.go
type CreateHandler struct {
    repo newmodule.Repository
}

// get_handler.go, list_handler.go, etc.
```

### 4. Delivery Layer (`internal/delivery/grpc/`)

```go
// newmodule_handler.go
type NewModuleHandler struct {
    financev1.UnimplementedNewModuleServiceServer
    createHandler *newmodule.CreateHandler
    // ...
}
```

### 5. Proto Definition (`goapps-shared-proto/finance/v1/`)

```protobuf
// newmodule.proto
service NewModuleService {
    rpc Create(CreateRequest) returns (CreateResponse);
    // ...
}
```

### 6. Register in main.go

```go
newmoduleHandler := grpcdelivery.NewNewModuleHandler(...)
financev1.RegisterNewModuleServiceServer(grpcServer, newmoduleHandler)
```

---

## Database Migrations

```bash
# Create new migration
make migrate-create NAME=create_new_table

# Apply migrations
make migrate-up

# Rollback last migration
make migrate-down
```

### Migration Best Practices

```sql
-- Always use IF NOT EXISTS / IF EXISTS
CREATE TABLE IF NOT EXISTS new_table (...);
DROP TABLE IF EXISTS new_table;

-- Always include both UP and DOWN
-- UP: Create table
-- DOWN: Drop table

-- Add indexes for frequently queried columns
CREATE INDEX idx_new_table_code ON new_table(code);
```

---

## Testing

### Test Categories

```bash
# Unit tests (fast, no external deps)
make test-unit

# Integration tests (requires PostgreSQL)
INTEGRATION_TEST=true make test-integration

# E2E tests (requires running service)
E2E_TEST=true make test-e2e

# All tests with coverage report
make coverage
```

### Writing Unit Tests

```go
// internal/domain/uom/entity_test.go
func TestNewCode(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {"valid", "KG", false},
        {"invalid - empty", "", true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := uom.NewCode(tt.input)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### Writing Integration Tests

```go
// internal/infrastructure/postgres/uom_repository_test.go
func TestUOMRepositorySuite(t *testing.T) {
    if os.Getenv("INTEGRATION_TEST") != "true" {
        t.Skip("Skipping integration test")
    }
    suite.Run(t, new(UOMRepositorySuite))
}
```

---

## Git Workflow

1. Create feature branch: `feature/finance-add-parameter-module`
2. Write code with tests
3. Run `make lint` - must pass
4. Run `make test` - must pass
5. Create PR with descriptive title

## Commit Convention

```
feat(finance): add parameter module CRUD operations
fix(uom): handle duplicate code error correctly
docs(finance): update API documentation
test(uom): add repository integration tests
refactor(finance): extract common validation logic
chore(finance): update dependencies
```

---

## Useful Make Commands

| Command | Description |
|---------|-------------|
| `make build` | Build binary |
| `make run` | Run service |
| `make dev` | Run with hot reload |
| `make test-unit` | Run unit tests |
| `make test-integration` | Run integration tests |
| `make coverage` | Run tests with coverage |
| `make lint` | Run linter |
| `make fmt` | Format code |
| `make docker-up` | Start infrastructure |
| `make docker-down` | Stop infrastructure |
| `make docker-build` | Build Docker image |
| `make migrate-up` | Apply migrations |
| `make migrate-down` | Rollback migration |
| `make seed` | Seed database |
| `make install-tools` | Install dev tools |
