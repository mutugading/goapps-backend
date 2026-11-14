package grpc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/config"
)

const testSecret = "test-secret"

type fakeResolver struct {
	scopes map[string][]scope.Scope
	err    error
}

func (f fakeResolver) Resolve(_ context.Context, userID string) ([]scope.Scope, error) {
	return f.scopes[userID], f.err
}

func signToken(t *testing.T, secret, tokenType, userID string, exp time.Duration) string {
	t.Helper()
	claims := JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(exp))},
		TokenType:        tokenType,
		UserID:           userID,
		Permissions:      []string{"payroll.run.run.view"},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func ctxWithToken(token string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+token))
}

func TestAuthInterceptor_Scenarios(t *testing.T) {
	solo, _ := scope.New(1, "SOLO") //nolint:errcheck // valid literal
	resolver := fakeResolver{scopes: map[string][]scope.Scope{"u1": {solo}}}
	info := &grpc.UnaryServerInfo{FullMethod: "/payroll.v1.PayrollService/X"}

	tests := []struct {
		name      string
		ctx       context.Context
		resolver  scope.Resolver
		wantCode  codes.Code
		wantScope string
	}{
		{"valid token resolves scope", ctxWithToken(signToken(t, testSecret, "access", "u1", time.Hour)), resolver, codes.OK, "1:SOLO"},
		{"user without scope gets empty scope", ctxWithToken(signToken(t, testSecret, "access", "u2", time.Hour)), resolver, codes.OK, ""},
		{"missing header", context.Background(), resolver, codes.Unauthenticated, ""},
		{"wrong secret", ctxWithToken(signToken(t, "other", "access", "u1", time.Hour)), resolver, codes.Unauthenticated, ""},
		{"refresh token", ctxWithToken(signToken(t, testSecret, "refresh", "u1", time.Hour)), resolver, codes.Unauthenticated, ""},
		{"expired", ctxWithToken(signToken(t, testSecret, "access", "u1", -time.Minute)), resolver, codes.Unauthenticated, ""},
		{"resolver error", ctxWithToken(signToken(t, testSecret, "access", "u1", time.Hour)), fakeResolver{err: errors.New("db down")}, codes.Internal, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			icp := AuthInterceptor(&config.JWTConfig{AccessTokenSecret: testSecret}, tt.resolver)
			var gotScope string
			_, err := icp(tt.ctx, nil, info, func(ctx context.Context, _ any) (any, error) {
				id, ok := scope.FromContext(ctx)
				if !ok {
					t.Fatal("identity missing in handler context")
				}
				gotScope = scope.Join(id.Scopes)
				return nil, nil
			})
			if status.Code(err) != tt.wantCode {
				t.Fatalf("code = %v, want %v (err=%v)", status.Code(err), tt.wantCode, err)
			}
			if gotScope != tt.wantScope {
				t.Fatalf("scope = %q, want %q", gotScope, tt.wantScope)
			}
		})
	}
}

func TestAuthInterceptor_HealthIsPublic(t *testing.T) {
	icp := AuthInterceptor(&config.JWTConfig{AccessTokenSecret: testSecret}, fakeResolver{})
	info := &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}
	if _, err := icp(context.Background(), nil, info, func(context.Context, any) (any, error) { return nil, nil }); err != nil {
		t.Fatalf("health must be public: %v", err)
	}
}

func TestPermissionInterceptor_Scenarios(t *testing.T) {
	const method = "/payroll.v1.PayrollService/Test"
	rpcPermissions[method] = "payroll.run.run.commit"
	t.Cleanup(func() { delete(rpcPermissions, method) })
	info := &grpc.UnaryServerInfo{FullMethod: method}
	ok := func(context.Context, any) (any, error) { return nil, nil }

	base := context.Background()
	tests := []struct {
		name     string
		ctx      context.Context
		wantCode codes.Code
	}{
		{"has permission", context.WithValue(base, AuthPermissionsKey, []string{"payroll.run.run.commit"}), codes.OK},
		{"missing permission", context.WithValue(base, AuthPermissionsKey, []string{"payroll.run.run.view"}), codes.PermissionDenied},
		{"super admin bypass", context.WithValue(base, AuthRolesKey, []string{"SUPER_ADMIN"}), codes.OK},
	}
	t.Run("unmapped RPC is denied", func(t *testing.T) {
		ctx := context.WithValue(base, AuthPermissionsKey, []string{"payroll.run.run.commit"})
		unmapped := &grpc.UnaryServerInfo{FullMethod: "/payroll.v1.PayrollService/Unmapped"}
		if _, err := PermissionInterceptor()(ctx, nil, unmapped, ok); status.Code(err) != codes.PermissionDenied {
			t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
		}
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := PermissionInterceptor()(tt.ctx, nil, info, ok)
			if status.Code(err) != tt.wantCode {
				t.Fatalf("code = %v, want %v", status.Code(err), tt.wantCode)
			}
		})
	}
}

func TestValidateAccessToken_RejectsWeakTokens(t *testing.T) {
	const secret = "test-secret"
	claims := JWTClaims{TokenType: "access", UserID: "u1"}
	sign := func(m jwt.SigningMethod, c JWTClaims) string {
		s, err := jwt.NewWithClaims(m, c).SignedString([]byte(secret))
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		return s
	}
	withExp := claims
	withExp.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Minute))
	none, err := jwt.NewWithClaims(jwt.SigningMethodNone, withExp).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}
	tests := map[string]string{
		"no exp":   sign(jwt.SigningMethodHS256, claims),
		"HS512":    sign(jwt.SigningMethodHS512, withExp),
		"alg none": none,
	}
	for name, tok := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := validateAccessToken(tok, []byte(secret)); err == nil {
				t.Fatal("want error, got nil")
			}
		})
	}
	if _, err := validateAccessToken(sign(jwt.SigningMethodHS256, withExp), []byte(secret)); err != nil {
		t.Fatalf("valid HS256 token rejected: %v", err)
	}
}

// Every master RPC must be mapped (fail-closed) and view-only users must not reach writes.
func TestMasterRPCPermissionMap(t *testing.T) {
	const svc = "/payroll.v1.PayrollService/"
	methods := []string{"CreateCurrency", "ListCurrencies", "UpdateCurrency", "CreateCompany", "GetCompany", "ListCompanies",
		"UpdateCompany", "CreateDivision", "ListDivisions", "UpdateDivision", "CreateEmployee", "GetEmployee", "ListEmployees", "UpdateEmployee",
		"CreateComponent", "GetComponent", "ListComponents", "UpdateComponent", "RetireComponent", "GetComponentTemplate", "PutComponentTemplate"}
	if len(methods) != 21 {
		t.Fatal("expected 21 master RPCs")
	}
	viewOnly := context.WithValue(context.Background(), AuthPermissionsKey,
		[]string{permEmployeeView, permCompanyView, permCurrencyView, permComponentView, permTemplateView})
	for _, m := range methods {
		perm, ok := rpcPermissions[svc+m]
		if !ok || perm == "" {
			t.Errorf("%s is not mapped", m)
			continue
		}
		isRead := strings.HasPrefix(m, "Get") || strings.HasPrefix(m, "List") // Retire/Put/Create are writes
		if got := HasPermission(viewOnly, perm); got != isRead {
			t.Errorf("%s: view-only user allowed=%v, want %v (perm %s)", m, got, isRead, perm)
		}
	}
}
