# Multi-stage Dockerfile for the Payroll Service.
# Build from repository ROOT: docker build -t payroll-service -f services/payroll/Dockerfile .
# Stage 1: Build
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies.
RUN apk add --no-cache git ca-certificates tzdata

# Copy gen module first (needed for the `replace gen => ../../gen` directive).
COPY gen/ ./gen/

# Copy Payroll service.
COPY services/payroll/ ./services/payroll/

WORKDIR /app/services/payroll

# Download dependencies.
RUN go mod download

# Build arguments.
ARG VERSION=dev
ARG BUILD_TIME

# Build server binary with optimizations.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}" \
    -o /bin/payroll-service \
    ./cmd/server/main.go

# Install golang-migrate (for running migrations via K8s Job).
RUN go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.1 \
    && cp $(go env GOPATH)/bin/migrate /bin/migrate

# Stage 2: Runtime
FROM alpine:3.20

WORKDIR /app

# Install ca-certificates for HTTPS and tzdata for timezone.
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user with specific UID/GID.
RUN addgroup -g 1000 appgroup && \
    adduser -u 1000 -G appgroup -s /bin/sh -D appuser

# Copy binaries from builder.
COPY --from=builder /bin/payroll-service /app/payroll-service
COPY --from=builder /bin/migrate /app/migrate

# Copy config (can be overridden by ConfigMap in Kubernetes).
COPY services/payroll/config.yaml /app/config.yaml

# Copy migration files (for running migrations via K8s Job).
COPY services/payroll/migrations/postgres/ /app/migrations/

# Set ownership.
RUN chown -R appuser:appgroup /app

# Switch to non-root user.
USER appuser

# Expose ports.
# 50054 - gRPC
# 8084  - HTTP (health, readiness, liveness, metrics)
EXPOSE 50054 8084

# Health check using HTTP endpoint.
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8084/healthz || exit 1

# Labels for metadata.
LABEL maintainer="mutugading" \
    service="payroll-service" \
    description="Payroll microservice"

ENTRYPOINT ["/app/payroll-service"]
