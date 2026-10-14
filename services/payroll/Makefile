# Payroll Service Makefile
# Service-specific targets

.PHONY: help run dev build test test-unit test-integration test-e2e test-coverage \
        lint fmt vet tidy clean \
        migrate-up migrate-down migrate-create \
        docker-up docker-down docker-logs \
        grpc-list grpc-health install-tools

SERVICE_NAME := payroll-service
BINARY := bin/$(SERVICE_NAME)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
DOCKER_REGISTRY ?= ghcr.io/ilramdhan
DOCKER_IMAGE := $(DOCKER_REGISTRY)/$(SERVICE_NAME)

# Auto-load .env for local development. Gitignored — copy from .env.example.
ifneq (,$(wildcard .env))
    include .env
    export
endif
ifneq (,$(wildcard .env.local))
    include .env.local
    export
endif

help:
	@echo "Payroll Service Makefile"
	@echo ""
	@echo "Development:"
	@echo "  make run              - Run service locally (gRPC :50054 + HTTP :8084)"
	@echo "  make dev              - Run with hot reload (requires air)"
	@echo "  make build            - Build binary"
	@echo ""
	@echo "Testing:"
	@echo "  make test             - Run all tests"
	@echo "  make test-unit        - Run unit tests only"
	@echo "  make lint             - Run linter"
	@echo ""
	@echo "Database (from T002/T003):"
	@echo "  make migrate-up       - Apply migrations"
	@echo "  make migrate-down     - Rollback last migration"
	@echo "  make migrate-create   - Create new migration (NAME=xxx)"

# =============================================================================
# Development
# =============================================================================

run:
	go run cmd/server/main.go

dev:
	air

build:
	@mkdir -p bin
	CGO_ENABLED=0 go build -ldflags="-s -w -X main.Version=$(VERSION)" -o $(BINARY) cmd/server/main.go
	@echo "Built $(BINARY)"

# =============================================================================
# Testing
# =============================================================================

test:
	go test -v -race ./...

test-unit:
	go test -v -race -short ./internal/...

test-integration:
	INTEGRATION_TEST=true go test -v -race ./internal/infrastructure/... ./tests/...

test-e2e:
	INTEGRATION_TEST=true go test -v ./tests/e2e/...

test-coverage:
	@mkdir -p coverage
	go test -v -race -coverprofile=coverage/coverage.out -covermode=atomic ./...
	go tool cover -func=coverage/coverage.out | tail -1
	go tool cover -html=coverage/coverage.out -o coverage/coverage.html
	@echo "Coverage report: coverage/coverage.html"

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...
	goimports -w .

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -rf bin coverage coverage.out tmp
	@echo "Cleaned build artifacts"

# =============================================================================
# Database
# =============================================================================

DATABASE_URL ?= postgres://payroll_app:payroll_app123@localhost:5437/payroll?sslmode=disable

migrate-up:
	migrate -path migrations/postgres -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path migrations/postgres -database "$(DATABASE_URL)" down 1

migrate-create:
	@if [ -z "$(NAME)" ]; then echo "Usage: make migrate-create NAME=create_xxx"; exit 1; fi
	migrate create -ext sql -dir migrations/postgres -seq $(NAME)

# =============================================================================
# gRPC Testing
# =============================================================================

grpc-list:
	grpcurl -plaintext localhost:50054 list

grpc-health:
	grpcurl -plaintext localhost:50054 grpc.health.v1.Health/Check

# =============================================================================
# Tools Installation
# =============================================================================

GOLANGCI_LINT_VERSION := v2.3.0
AIR_VERSION := v1.52.3
GOIMPORTS_VERSION := v0.28.0
GRPCURL_VERSION := v1.9.1
MIGRATE_VERSION := v4.18.1

install-tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go install github.com/air-verse/air@$(AIR_VERSION)
	go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)
	go install github.com/fullstorydev/grpcurl/cmd/grpcurl@$(GRPCURL_VERSION)
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION)
	@echo "Tools installed (pinned versions)"
