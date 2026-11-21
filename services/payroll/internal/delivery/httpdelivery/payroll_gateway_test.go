package httpdelivery

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	grpcdelivery "github.com/mutugading/goapps-backend/services/payroll/internal/delivery/grpc"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
)

// stub implements only UpdateEmployee: current row version is 2.
type stub struct {
	payrollv1.UnimplementedPayrollServiceServer
}

func (stub) UpdateEmployee(_ context.Context, r *payrollv1.UpdateEmployeeRequest) (*payrollv1.UpdateEmployeeResponse, error) {
	if strings.Trim(r.GetIfMatch(), `"`) != "2" {
		return nil, grpcdelivery.ToStatusError(apperr.New(apperr.CodeVersionMismatch, "Karyawan sudah berubah", nil))
	}
	return &payrollv1.UpdateEmployeeResponse{
		Base: ok(),
		Data: &payrollv1.Employee{Id: r.GetId(), Etag: "3"},
	}, nil
}

func newGateway(t *testing.T) http.Handler {
	t.Helper()
	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// Same relative order as the real chain: error wrapper outermost, If-Match
	// copy before validation.
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		grpcdelivery.StructuredErrorInterceptor(),
		grpcdelivery.IfMatchInterceptor(),
	))
	payrollv1.RegisterPayrollServiceServer(srv, stub{})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	h, err := NewGatewayMux(context.Background(), lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func put(t *testing.T, h http.Handler, ifMatch string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/payroll/employees/id/7", strings.NewReader(`{"input":{}}`))
	req.Header.Set("Content-Type", "application/json")
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestGateway_StaleIfMatchReturns412(t *testing.T) {
	rec := put(t, newGateway(t), `"1"`)
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412; body=%s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"status_code":"412"`) {
		t.Fatalf("body lacks status_code 412: %s", rec.Body)
	}
}

func TestGateway_FreshIfMatchSetsETag(t *testing.T) {
	rec := put(t, newGateway(t), `"2"`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("ETag"); got != `"3"` {
		t.Fatalf("ETag = %q, want \"3\"", got)
	}
}

func ok() *commonv1.BaseResponse { return &commonv1.BaseResponse{IsSuccess: true, StatusCode: "200"} }
