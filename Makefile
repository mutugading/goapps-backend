# goapps-backend Root Makefile
# Common targets for all services

.PHONY: help proto lint test build clean

# Default target
help:
	@echo "goapps-backend Makefile"
	@echo ""
	@echo "Proto targets:"
	@echo "  make proto              - Generate proto code for all services"
	@echo ""
	@echo "Lint targets:"
	@echo "  make lint               - Run golangci-lint for all services"
	@echo "  make lint-fix           - Run golangci-lint with auto-fix"
	@echo ""
	@echo "Test targets:"
	@echo "  make test               - Run tests for all services"
	@echo "  make test-coverage      - Run tests with coverage report"
	@echo ""
	@echo "Service-specific targets:"
	@echo "  make finance-run        - Run finance service locally"
	@echo "  make finance-build      - Build finance service binary"
	@echo "  make finance-migrate    - Run finance service migrations"
	@echo "  make finance-seed       - Run finance service seeders"
	@echo "  make finance-docker     - Build finance service Docker image"

# =============================================================================
# Proto Generation
# =============================================================================

proto:
	@echo "🔧 Generating proto code..."
	cd ../goapps-shared-proto && ./scripts/gen-go.sh
	@echo "✅ Proto generation complete"

# =============================================================================
# Linting
# =============================================================================

lint:
	@echo "🔍 Running golangci-lint..."
	golangci-lint run ./...
	@echo "✅ Lint passed"

lint-fix:
	@echo "🔧 Running golangci-lint with auto-fix..."
	golangci-lint run --fix ./...
	@echo "✅ Lint fix complete"

# =============================================================================
# Testing
# =============================================================================

test:
	@echo "🧪 Running tests..."
	go test -v -race ./...
	@echo "✅ All tests passed"

test-coverage:
	@echo "🧪 Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report: coverage.html"

# =============================================================================
# Finance Service
# =============================================================================

FINANCE_DIR := services/finance

finance-run:
	@echo "🚀 Running finance service..."
	cd $(FINANCE_DIR) && go run cmd/server/main.go

finance-build:
	@echo "🔨 Building finance service..."
	cd $(FINANCE_DIR) && go build -o bin/finance-service cmd/server/main.go
	@echo "✅ Built: $(FINANCE_DIR)/bin/finance-service"

finance-migrate:
	@echo "📦 Running finance migrations..."
	cd $(FINANCE_DIR) && migrate -path migrations/postgres -database "$${DATABASE_URL}" up
	@echo "✅ Migrations applied"

finance-migrate-down:
	@echo "📦 Rolling back finance migrations..."
	cd $(FINANCE_DIR) && migrate -path migrations/postgres -database "$${DATABASE_URL}" down 1

finance-seed:
	@echo "🌱 Running finance seeders..."
	cd $(FINANCE_DIR) && go run seeds/main.go
	@echo "✅ Seeding complete"

finance-docker:
	@echo "🐳 Building finance Docker image..."
	docker build -t goapps-finance-service:latest -f $(FINANCE_DIR)/Dockerfile .
	@echo "✅ Docker image built: goapps-finance-service:latest"

finance-docker-compose:
	@echo "🐳 Starting finance service with Docker Compose..."
	docker compose -f $(FINANCE_DIR)/deployments/docker-compose.yaml up -d

finance-docker-compose-down:
	@echo "🐳 Stopping finance service Docker Compose..."
	docker compose -f $(FINANCE_DIR)/deployments/docker-compose.yaml down

# =============================================================================
# Clean
# =============================================================================

clean:
	@echo "🧹 Cleaning build artifacts..."
	rm -rf services/*/bin
	rm -f coverage.out coverage.html
	@echo "✅ Clean complete"
