# Multi-stage Dockerfile for the Payroll Service.
# Build from repository ROOT: docker build -t payroll-service -f services/payroll/Dockerfile .
# Stage 1: Build
FROM golang:1.27-alpine AS builder

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

# Build the operator CLIs (D-62 (3)): run by IT with `kubectl exec` / a manual Job, never exposed as a service.
#   payroll-sync   ETL input + HRIS attendance sync (D-58)
#   payroll-report bank / reco / GL / slip files (D-62 (3)(7))
#   payroll-mailer slip distribution (D-62 (5))
#   payroll-recon  reconciliation against Oracle (D-62 (2))
RUN for c in sync report mailer recon; do \
      CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /bin/payroll-$c ./cmd/$c || exit 1; \
    done

# Install golang-migrate (for running migrations via K8s Job).
RUN go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.1 \
    && cp $(go env GOPATH)/bin/migrate /bin/migrate

# Stage 2: Runtime
FROM alpine:3.24

WORKDIR /app

# Install ca-certificates for HTTPS and tzdata for timezone.
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user with specific UID/GID.
RUN addgroup -g 1000 appgroup && \
    adduser -u 1000 -G appgroup -s /bin/sh -D appuser

# Copy binaries from builder.
COPY --from=builder /bin/payroll-service /app/payroll-service
COPY --from=builder /bin/payroll-sync /bin/payroll-report /bin/payroll-mailer /bin/payroll-recon /app/
COPY --from=builder /bin/migrate /app/migrate

# Copy config (can be overridden by ConfigMap in Kubernetes).
COPY services/payroll/config.yaml /app/config.yaml

# Copy migration files (for running migrations via K8s Job).
COPY services/payroll/migrations/postgres/ /app/migrations/

# Object grants run by the owner after every `migrate up` (D-66 C(2)). The migrate Job copies this file to an
# emptyDir for its psql container (postgres image); psql is deliberately NOT installed in this runtime image.
COPY services/payroll/deploy/grants.sql /app/deploy/grants.sql

# Set ownership. /out is where the CLIs write generated files (salary data: mount an emptyDir, never bake in).
RUN mkdir -p /out && chown -R appuser:appgroup /app /out

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
