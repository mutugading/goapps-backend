package grpc

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/idempotency"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
)

type fakeIdemStore struct {
	rows      map[idempotency.Key]fakeIdemRow
	completes int
	aborts    int
}

type fakeIdemRow struct {
	req, scope string
	done       bool
	resp       []byte
}

func (f *fakeIdemStore) Begin(_ context.Context, k idempotency.Key, req, sc string) (idempotency.State, string, []byte, error) {
	row, ok := f.rows[k]
	if !ok {
		f.rows[k] = fakeIdemRow{req: req, scope: sc}
		return idempotency.Started, "c", nil, nil
	}
	switch {
	case row.req != req || row.scope != sc:
		return idempotency.Conflict, "", nil, nil
	case row.done:
		return idempotency.Replay, "", row.resp, nil
	}
	return idempotency.InProgress, "", nil, nil
}

func (f *fakeIdemStore) Complete(_ context.Context, k idempotency.Key, _ string, b []byte) error {
	f.completes++
	row := f.rows[k]
	row.done, row.resp = true, b
	f.rows[k] = row
	return nil
}

func (f *fakeIdemStore) Abort(_ context.Context, k idempotency.Key, _ string) error {
	f.aborts++
	delete(f.rows, k)
	return nil
}

func idemCtx(user, key string, scopes ...scope.Scope) context.Context {
	ctx := context.WithValue(context.Background(), AuthUserIDKey, user)
	ctx = scope.WithIdentity(ctx, scope.Identity{UserID: user, Scopes: scopes})
	if key != "" {
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("idempotency-key", key))
	}
	return ctx
}

func TestIdempotencyInterceptor(t *testing.T) {
	solo, _ := scope.New(1, "SOLO")   //nolint:errcheck // valid literal
	jkt, _ := scope.New(1, "JAKARTA") //nolint:errcheck // valid literal
	info := &grpc.UnaryServerInfo{FullMethod: "/payroll.v1.PayrollService/CreateCompany"}
	req := &payrollv1.CreateCompanyRequest{Code: "MGT", Name: "A"}

	newResp := func(ok bool) *payrollv1.CreateCompanyResponse {
		return &payrollv1.CreateCompanyResponse{Base: &commonv1.BaseResponse{IsSuccess: ok, StatusCode: "200", Message: "m"}}
	}
	calls := 0
	okHandler := func(context.Context, any) (any, error) { calls++; return newResp(true), nil }

	t.Run("replay without rerunning handler", func(t *testing.T) {
		calls = 0
		st := &fakeIdemStore{rows: map[idempotency.Key]fakeIdemRow{}}
		ic := IdempotencyInterceptor(st)
		if _, err := ic(idemCtx("u1", "k1", solo), req, info, okHandler); err != nil {
			t.Fatal(err)
		}
		got, err := ic(idemCtx("u1", "k1", solo), req, info, okHandler)
		if err != nil || calls != 1 {
			t.Fatalf("err=%v calls=%d, want replay with 1 call", err, calls)
		}
		if !proto.Equal(got.(proto.Message), newResp(true)) {
			t.Fatalf("replayed response differs: %v", got)
		}
	})

	t.Run("different body is a conflict", func(t *testing.T) {
		st := &fakeIdemStore{rows: map[idempotency.Key]fakeIdemRow{}}
		ic := IdempotencyInterceptor(st)
		_, _ = ic(idemCtx("u1", "k1", solo), req, info, okHandler) //nolint:errcheck // seeding
		_, err := ic(idemCtx("u1", "k1", solo), &payrollv1.CreateCompanyRequest{Code: "X", Name: "B"}, info, okHandler)
		if c, _ := CodeFromError(err); c != apperr.CodeIdemConflict {
			t.Fatalf("got %v, want IDEMPOTENCY_KEY_CONFLICT", err)
		}
	})

	t.Run("changed scope is a conflict", func(t *testing.T) {
		st := &fakeIdemStore{rows: map[idempotency.Key]fakeIdemRow{}}
		ic := IdempotencyInterceptor(st)
		_, _ = ic(idemCtx("u1", "k1", solo, jkt), req, info, okHandler) //nolint:errcheck // seeding
		_, err := ic(idemCtx("u1", "k1", solo), req, info, okHandler)
		if c, _ := CodeFromError(err); c != apperr.CodeIdemConflict {
			t.Fatalf("got %v, want IDEMPOTENCY_KEY_CONFLICT", err)
		}
	})

	t.Run("permission change is a conflict", func(t *testing.T) {
		st := &fakeIdemStore{rows: map[idempotency.Key]fakeIdemRow{}}
		ic := IdempotencyInterceptor(st)
		withPerm := context.WithValue(idemCtx("u1", "k1", solo), AuthPermissionsKey, []string{"payroll.master.employee.update"})
		_, _ = ic(withPerm, req, info, okHandler) //nolint:errcheck // seeding
		_, err := ic(idemCtx("u1", "k1", solo), req, info, okHandler)
		if c, _ := CodeFromError(err); c != apperr.CodeIdemConflict {
			t.Fatalf("got %v, want IDEMPOTENCY_KEY_CONFLICT", err)
		}
	})

	t.Run("panic releases the claim", func(t *testing.T) {
		st := &fakeIdemStore{rows: map[idempotency.Key]fakeIdemRow{}}
		ic := IdempotencyInterceptor(st)
		func() {
			defer func() { _ = recover() }()                                                                           //nolint:errcheck // expected panic
			_, _ = ic(idemCtx("u1", "k1", solo), req, info, func(context.Context, any) (any, error) { panic("boom") }) //nolint:errcheck // panics
		}()
		if st.aborts != 1 || len(st.rows) != 0 {
			t.Fatalf("aborts=%d rows=%d, want claim released", st.aborts, len(st.rows))
		}
	})

	t.Run("in progress", func(t *testing.T) {
		st := &fakeIdemStore{rows: map[idempotency.Key]fakeIdemRow{}}
		ic := IdempotencyInterceptor(st)
		_, _ = ic(idemCtx("u1", "k1", solo), req, info, func(ctx context.Context, r any) (any, error) { //nolint:errcheck // seeding
			_, err := ic(idemCtx("u1", "k1", solo), req, info, okHandler) // concurrent retry
			if c, _ := CodeFromError(err); c != apperr.CodeIdemInProgress {
				t.Errorf("got %v, want IDEMPOTENCY_IN_PROGRESS", err)
			}
			return okHandler(ctx, r)
		})
	})

	t.Run("errors and failed base are not stored", func(t *testing.T) {
		st := &fakeIdemStore{rows: map[idempotency.Key]fakeIdemRow{}}
		ic := IdempotencyInterceptor(st)
		_, err := ic(idemCtx("u1", "k1", solo), req, info, func(context.Context, any) (any, error) { return nil, errors.New("boom") })
		if err == nil || st.aborts != 1 || len(st.rows) != 0 {
			t.Fatalf("err=%v aborts=%d rows=%d", err, st.aborts, len(st.rows))
		}
		_, _ = ic(idemCtx("u1", "k2", solo), req, info, func(context.Context, any) (any, error) { return newResp(false), nil }) //nolint:errcheck // asserting store state
		if st.completes != 0 || len(st.rows) != 0 {
			t.Fatalf("failed base stored: completes=%d rows=%d", st.completes, len(st.rows))
		}
	})

	t.Run("no header, reads and other users bypass or isolate", func(t *testing.T) {
		st := &fakeIdemStore{rows: map[idempotency.Key]fakeIdemRow{}}
		ic := IdempotencyInterceptor(st)
		_, _ = ic(idemCtx("u1", "", solo), req, info, okHandler)                                                                            //nolint:errcheck // asserting store state
		_, _ = ic(idemCtx("u1", "k1", solo), req, &grpc.UnaryServerInfo{FullMethod: "/payroll.v1.PayrollService/ListCompanies"}, okHandler) //nolint:errcheck // asserting store state
		if len(st.rows) != 0 {
			t.Fatalf("bypassed calls touched the store: %v", st.rows)
		}
		_, _ = ic(idemCtx("u1", "k1", solo), req, info, okHandler) //nolint:errcheck // seeding
		calls = 0
		if _, err := ic(idemCtx("u2", "k1", solo), req, info, okHandler); err != nil || calls != 1 {
			t.Fatalf("other user must run its own handler: err=%v calls=%d", err, calls)
		}
	})

	t.Run("key too long", func(t *testing.T) {
		ic := IdempotencyInterceptor(&fakeIdemStore{rows: map[idempotency.Key]fakeIdemRow{}})
		long := make([]byte, 129)
		for i := range long {
			long[i] = 'a'
		}
		_, err := ic(idemCtx("u1", string(long), solo), req, info, okHandler)
		if c, _ := CodeFromError(err); c != apperr.CodeValidation {
			t.Fatalf("got %v (%v), want VALIDATION_ERROR", err, codes.Unknown)
		}
	})
}
