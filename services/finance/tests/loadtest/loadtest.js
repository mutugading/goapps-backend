// k6 Load Testing Script for Finance Service
// https://k6.io/docs/
//
// Run: k6 run --vus 10 --duration 30s loadtest.js
// Or: k6 run loadtest.js

import grpc from 'k6/net/grpc';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

// Custom metrics
const grpcErrors = new Counter('grpc_errors');
const grpcSuccess = new Rate('grpc_success');
const grpcDuration = new Trend('grpc_duration');

// Configuration
const GRPC_ADDR = __ENV.GRPC_ADDR || 'localhost:50051';

// Test options
export const options = {
    scenarios: {
        // Smoke test: minimal load
        smoke: {
            executor: 'constant-vus',
            vus: 1,
            duration: '10s',
            startTime: '0s',
        },
        // Load test: normal load
        load: {
            executor: 'ramping-vus',
            startVUs: 0,
            stages: [
                { duration: '30s', target: 10 },  // Ramp up
                { duration: '1m', target: 10 },   // Stay at 10
                { duration: '30s', target: 0 },   // Ramp down
            ],
            startTime: '10s',
        },
        // Stress test: high load
        stress: {
            executor: 'ramping-vus',
            startVUs: 0,
            stages: [
                { duration: '30s', target: 50 },  // Ramp up
                { duration: '1m', target: 50 },   // Stay at 50
                { duration: '30s', target: 0 },   // Ramp down
            ],
            startTime: '2m30s',
        },
    },
    thresholds: {
        'grpc_duration': ['p(95)<500'],  // 95% of requests under 500ms
        'grpc_success': ['rate>0.95'],   // 95% success rate
        'grpc_errors': ['count<10'],     // Less than 10 errors
    },
};

const client = new grpc.Client();

export function setup() {
    // Load proto file
    client.load(['../../goapps-shared-proto/finance/v1'], 'uom.proto');
}

export default function () {
    // Connect to gRPC server
    client.connect(GRPC_ADDR, { plaintext: true });

    // Test: List UOMs
    testListUOMs();

    // Test: Create UOM (with unique code)
    testCreateUOM();

    // Close connection
    client.close();

    sleep(0.1); // Small delay between iterations
}

function testListUOMs() {
    const startTime = Date.now();

    const response = client.invoke('finance.v1.UOMService/ListUOMs', {
        page: 1,
        page_size: 10,
    });

    const duration = Date.now() - startTime;
    grpcDuration.add(duration);

    const success = check(response, {
        'ListUOMs status is OK': (r) => r && r.status === grpc.StatusOK,
        'ListUOMs has data': (r) => r && r.message && r.message.base && r.message.base.isSuccess,
    });

    if (success) {
        grpcSuccess.add(1);
    } else {
        grpcSuccess.add(0);
        grpcErrors.add(1);
    }
}

function testCreateUOM() {
    // Generate unique code for each request
    const uniqueCode = `LOAD_${Date.now()}_${Math.random().toString(36).substring(7).toUpperCase()}`;

    const startTime = Date.now();

    const response = client.invoke('finance.v1.UOMService/CreateUOM', {
        uom_code: uniqueCode.substring(0, 20), // Max 20 chars
        uom_name: 'Load Test UOM',
        uom_category: 4, // QUANTITY
        description: 'Created by k6 load test',
    });

    const duration = Date.now() - startTime;
    grpcDuration.add(duration);

    const success = check(response, {
        'CreateUOM status is OK': (r) => r && r.status === grpc.StatusOK,
        'CreateUOM is success or duplicate': (r) => {
            if (!r || !r.message || !r.message.base) return false;
            // Accept both success and "already exists" (from concurrent tests)
            return r.message.base.isSuccess || r.message.base.statusCode === '409';
        },
    });

    if (success) {
        grpcSuccess.add(1);
    } else {
        grpcSuccess.add(0);
        grpcErrors.add(1);
    }
}

export function teardown(data) {
    console.log('Load test completed');
}
