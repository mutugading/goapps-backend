# Go Apps Backend

Backend microservices using Go with gRPC.

## Structure (Future)

```
goapps-backend/
├── cmd/                    # Main applications
├── internal/               # Private application code
├── pkg/                    # Public library code  
├── api/                    # OpenAPI/Swagger specs
└── deploy/                 # Deployment configs
```

## Getting Started

Proto files are located in `../goapps-shared-proto/`

Generate Go code from proto:
```bash
cd ../goapps-shared-proto
./scripts/gen-go.sh
```
