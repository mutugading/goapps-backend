# Finance Service

> Part of GoApps Backend - Microservices for Financial Operations

## Overview

Finance Service handles master data and calculations for financial operations in the GoApps ecosystem.

| Attribute | Value |
|-----------|-------|
| Service Name | `goapps-finance-service` |
| gRPC Port | 50051 |
| HTTP Port | 8080 (Gateway/Swagger/Metrics) |
| Database | PostgreSQL 16+ |
| Cache | Redis 7+ |
| Tracing | Jaeger (OTLP) |

## Current Modules

- **UOM (Unit of Measure)** - Master data for units of measurement

---

## Quick Start

### Prerequisites

- Go 1.23+
- Docker & Docker Compose
- `grpcurl` (for testing gRPC)

Install development tools:
```bash
make install-tools
```

### Step-by-Step Setup

```bash
# 1. Start infrastructure (PostgreSQL, Redis, Jaeger)
make docker-up

# Wait until services are ready
docker compose -f deployments/docker-compose.yaml logs -f

# 2. Run database migrations
make migrate-up

# 3. Seed initial data (26 UOMs)
make seed

# 4. Run the service
make run

# Or with hot reload (requires air)
make dev
```

### Verify Service is Running

```bash
# gRPC health check
grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check

# HTTP health check
curl http://localhost:8080/healthz

# Swagger UI
open http://localhost:8080/swagger/

# Prometheus metrics
curl http://localhost:8080/metrics
```

---

## Testing

```bash
# Unit tests only (fast)
make test-unit

# All tests with coverage
make coverage

# Integration tests (requires running PostgreSQL)
INTEGRATION_TEST=true make test-integration

# E2E tests (requires running service)
E2E_TEST=true make test-e2e

# Lint code
make lint
```

---

## API Reference

### gRPC Endpoints (port 50051)

| Method | Description |
|--------|-------------|
| `CreateUOM` | Create new UOM |
| `GetUOM` | Get UOM by ID |
| `UpdateUOM` | Update UOM |
| `DeleteUOM` | Soft delete UOM |
| `ListUOMs` | List with search/filter/pagination |
| `ExportUOMs` | Export to Excel |
| `ImportUOMs` | Import from Excel |
| `DownloadTemplate` | Get import template |

### HTTP Gateway (port 8080)

All gRPC methods are also available via REST:

| HTTP Method | Path | Description |
|-------------|------|-------------|
| `POST` | `/api/v1/uoms` | Create UOM |
| `GET` | `/api/v1/uoms/{id}` | Get UOM |
| `PUT` | `/api/v1/uoms/{id}` | Update UOM |
| `DELETE` | `/api/v1/uoms/{id}` | Delete UOM |
| `GET` | `/api/v1/uoms` | List UOMs |

### API Examples

#### Create UOM
```bash
# gRPC
grpcurl -plaintext -d '{
  "uom_code": "TEST",
  "uom_name": "Test Unit",
  "uom_category": "UOM_CATEGORY_QUANTITY",
  "description": "For testing"
}' localhost:50051 finance.v1.UOMService/CreateUOM

# HTTP
curl -X POST http://localhost:8080/api/v1/uoms \
  -H "Content-Type: application/json" \
  -d '{
    "uom_code": "TEST",
    "uom_name": "Test Unit",
    "uom_category": "UOM_CATEGORY_QUANTITY"
  }'
```

#### List UOMs with Pagination
```bash
# gRPC
grpcurl -plaintext -d '{"page": 1, "page_size": 10}' \
  localhost:50051 finance.v1.UOMService/ListUOMs

# HTTP
curl "http://localhost:8080/api/v1/uoms?page=1&page_size=10"
```

#### List UOMs with Filter
```bash
# Filter by category
grpcurl -plaintext -d '{
  "page": 1, 
  "page_size": 10,
  "category": "UOM_CATEGORY_WEIGHT"
}' localhost:50051 finance.v1.UOMService/ListUOMs

# Search by name/code
grpcurl -plaintext -d '{
  "page": 1, 
  "page_size": 10,
  "search": "kilo"
}' localhost:50051 finance.v1.UOMService/ListUOMs
```

#### Get UOM by ID
```bash
grpcurl -plaintext -d '{"uom_id": "uuid-here"}' \
  localhost:50051 finance.v1.UOMService/GetUOM
```

#### Update UOM
```bash
grpcurl -plaintext -d '{
  "uom_id": "uuid-here",
  "uom_name": "Updated Name",
  "description": "Updated description"
}' localhost:50051 finance.v1.UOMService/UpdateUOM
```

#### Delete UOM
```bash
grpcurl -plaintext -d '{"uom_id": "uuid-here"}' \
  localhost:50051 finance.v1.UOMService/DeleteUOM
```

### Validation Response Format

When validation fails, the API returns a structured response (NOT gRPC status error):

```json
{
  "base": {
    "validationErrors": [
      {"field": "uom_code", "message": "value length must be at most 20 characters"},
      {"field": "uom_name", "message": "value length must be at least 1 characters"}
    ],
    "statusCode": "400",
    "message": "Validation failed",
    "isSuccess": false
  }
}
```

### UOM Categories

| Value | Description |
|-------|-------------|
| `UOM_CATEGORY_WEIGHT` | Weight units (KG, G, LB, etc.) |
| `UOM_CATEGORY_LENGTH` | Length units (M, CM, KM, etc.) |
| `UOM_CATEGORY_VOLUME` | Volume units (L, ML, M3, etc.) |
| `UOM_CATEGORY_QUANTITY` | Count units (PCS, BOX, etc.) |

### UOM Code Rules

- Must start with uppercase letter
- Only uppercase letters, numbers, and underscores allowed
- 1-20 characters max
- Examples: `KG`, `MTR_SQ`, `M3`, `PCS`

---

## Project Structure

```
services/finance/
├── cmd/server/main.go        # Entry point
├── config.yaml               # Configuration
├── Dockerfile                # Container build
├── Makefile                  # Build commands
├── internal/
│   ├── domain/uom/           # Domain layer
│   │   ├── entity.go         # UOM aggregate root
│   │   ├── value_object.go   # Code, Category VOs
│   │   ├── repository.go     # Repository interface
│   │   └── errors.go         # Domain errors
│   ├── application/uom/      # Application layer
│   │   ├── create_handler.go
│   │   ├── get_handler.go
│   │   ├── update_handler.go
│   │   ├── delete_handler.go
│   │   ├── list_handler.go
│   │   └── export_handler.go
│   ├── infrastructure/       # Infrastructure layer
│   │   ├── config/           # Viper configuration
│   │   ├── postgres/         # Repository impl
│   │   ├── redis/            # Cache impl
│   │   └── tracing/          # OpenTelemetry
│   └── delivery/grpc/        # Delivery layer
│       ├── server.go         # gRPC server setup
│       ├── uom_handler.go    # UOMService impl
│       ├── interceptors.go   # Middleware
│       └── validation_helper.go
├── tests/e2e/                # E2E tests
├── migrations/postgres/      # Database migrations
├── seeds/                    # Data seeder
└── deployments/
    ├── docker-compose.yaml   # Local dev
    └── kubernetes/           # K8s manifests
```

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `APP_ENV` | development | Environment (development/production) |
| `DATABASE_HOST` | localhost | PostgreSQL host |
| `DATABASE_PORT` | 5434 | PostgreSQL port |
| `DATABASE_USER` | finance | PostgreSQL user |
| `DATABASE_PASSWORD` | finance123 | PostgreSQL password |
| `DATABASE_NAME` | finance_db | Database name |
| `REDIS_HOST` | localhost | Redis host |
| `REDIS_PORT` | 6379 | Redis port |
| `JAEGER_ENDPOINT` | localhost:4317 | Jaeger OTLP endpoint |
| `LOG_LEVEL` | info | Log level |

---

## Docker

### Build Image
```bash
make docker-build
```

### Run with Docker Compose
```bash
# Start all services
make docker-up

# View logs
make docker-logs

# Stop all services
make docker-down
```

---

## Observability

### Swagger UI
- URL: http://localhost:8080/swagger/
- Interactive API documentation

### Prometheus Metrics
- URL: http://localhost:8080/metrics
- Includes: request count, latency, UOM operation metrics

### Jaeger Tracing
- URL: http://localhost:16686 (when running docker-compose)
- Distributed tracing for all requests

### Health Endpoints
| Endpoint | Description |
|----------|-------------|
| `/healthz` | General health check |
| `/readyz` | Readiness probe (K8s) |
| `/livez` | Liveness probe (K8s) |

---

## Contributing

See [DEVELOPMENT.md](DEVELOPMENT.md) for development guidelines.
