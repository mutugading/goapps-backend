package grpc

import (
	"context"

	"google.golang.org/grpc"
	"testing"
	"time"
)

func TestRunRPCsHavePermissions(t *testing.T) {
	for _, m := range []string{"CreateRun", "GetRun", "ListRuns", "ListRunLines", "ListPeriods", "ApprovePeriod", "ClosePeriod", "ReopenPeriod"} {
		if rpcPermissions["/payroll.v1.PayrollService/"+m] == "" {
			t.Errorf("%s has no permission mapping (fail closed)", m)
		}
	}
}

func TestMask(t *testing.T) {
	if mask(true, "12.00") != "12.00" || mask(false, "12.00") != "***" {
		t.Fatal("mask")
	}
}

func TestCreateRunTimeout(t *testing.T) {
	if methodTimeouts["/payroll.v1.PayrollService/CreateRun"] != 120*time.Second {
		t.Fatal("CreateRun timeout must be 120 s (D-51)")
	}
}

func TestTimeoutInterceptor_LongRPCDoesNotLeak(t *testing.T) {
	ic := TimeoutInterceptor(60 * time.Second)
	remaining := func(method string) time.Duration {
		var d time.Duration
		_, err := ic(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: method},
			func(ctx context.Context, _ any) (any, error) {
				dl, ok := ctx.Deadline()
				if !ok {
					t.Fatal("no deadline")
				}
				d = time.Until(dl)
				return nil, nil //nolint:nilnil // test handler
			})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	if d := remaining("/payroll.v1.PayrollService/CreateRun"); d < 100*time.Second {
		t.Fatalf("CreateRun deadline %s, want ~120s", d)
	}
	if d := remaining("/payroll.v1.PayrollService/GetRun"); d > 61*time.Second {
		t.Fatalf("GetRun deadline %s after CreateRun, want <= 60s", d)
	}
}

func TestCanSeeAmounts(t *testing.T) {
	base := context.Background()
	if canSeeAmounts(base) {
		t.Fatal("no permission must be masked")
	}
	if !canSeeAmounts(context.WithValue(base, AuthPermissionsKey, []string{permSalaryAmount})) {
		t.Fatal("permission holder must see amounts")
	}
	if !canSeeAmounts(context.WithValue(base, AuthRolesKey, []string{"SUPER_ADMIN"})) {
		t.Fatal("SUPER_ADMIN bypass")
	}
}
