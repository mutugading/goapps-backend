# Load Testing

This directory contains load testing scripts using [k6](https://k6.io/).

## Prerequisites

Install k6:
```bash
# macOS
brew install k6

# Linux
sudo gpg -k
sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" | sudo tee /etc/apt/sources.list.d/k6.list
sudo apt-get update
sudo apt-get install k6

# Docker
docker run -i grafana/k6 run - < loadtest.js
```

## Running Tests

### Quick Test
```bash
# Smoke test (1 VU, 10s)
k6 run --vus 1 --duration 10s loadtest.js
```

### Full Test Suite
```bash
# Run all scenarios (smoke, load, stress)
k6 run loadtest.js
```

### Custom Configuration
```bash
# Custom VUs and duration
k6 run --vus 20 --duration 1m loadtest.js

# Different gRPC address
GRPC_ADDR=localhost:50051 k6 run loadtest.js
```

## Test Scenarios

| Scenario | VUs | Duration | Purpose |
|----------|-----|----------|---------|
| Smoke | 1 | 10s | Verify basic functionality |
| Load | 10 | 2m | Normal load testing |
| Stress | 50 | 2m | Find breaking points |

## Metrics

- `grpc_duration` - Response time (p95 < 500ms)
- `grpc_success` - Success rate (> 95%)
- `grpc_errors` - Error count (< 10)

## Sample Output

```
     ✓ ListUOMs status is OK
     ✓ ListUOMs has data
     ✓ CreateUOM status is OK

     checks.........................: 100.00% ✓ 300  ✗ 0
     grpc_duration..................: avg=45.23ms  min=12ms  p(95)=89.12ms
     grpc_errors....................: 0
     grpc_success...................: 100.00% ✓ 300  ✗ 0
     iterations.....................: 100     3.33/s
     vus............................: 10      min=0     max=10
```

## Grafana Dashboard

Export results to Grafana Cloud:
```bash
k6 run --out cloud loadtest.js
```

Or to InfluxDB:
```bash
k6 run --out influxdb=http://localhost:8086/k6 loadtest.js
```
