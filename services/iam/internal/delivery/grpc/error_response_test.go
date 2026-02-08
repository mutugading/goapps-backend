package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	// Import all proto packages to register their file descriptors.
	_ "github.com/mutugading/goapps-backend/gen/iam/v1"
)

func TestWrapErrorInResponse_IAMMethods(t *testing.T) {
	err := status.Error(codes.Unauthenticated, "authentication required")

	methods := []string{
		"/iam.v1.AuthService/Login",
		"/iam.v1.AuthService/Logout",
		"/iam.v1.AuthService/RefreshToken",
		"/iam.v1.AuthService/ForgotPassword",
		"/iam.v1.AuthService/Enable2FA",
		"/iam.v1.AuthService/GetCurrentUser",
		"/iam.v1.UserService/CreateUser",
		"/iam.v1.UserService/GetUser",
		"/iam.v1.UserService/ListUsers",
		"/iam.v1.RoleService/CreateRole",
		"/iam.v1.RoleService/ListRoles",
		"/iam.v1.PermissionService/ListPermissions",
		"/iam.v1.CompanyService/ListCompanies",
		"/iam.v1.CompanyService/GetCompany",
		"/iam.v1.DivisionService/ListDivisions",
		"/iam.v1.DepartmentService/ListDepartments",
		"/iam.v1.SectionService/ListSections",
		"/iam.v1.SessionService/ListActiveSessions",
		"/iam.v1.AuditService/ListAuditLogs",
		"/iam.v1.MenuService/ListMenus",
		"/iam.v1.MenuService/GetMenuTree",
		"/iam.v1.OrganizationService/GetOrganizationTree",
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			resp := wrapErrorInResponse(method, err)
			require.NotNil(t, resp, "wrapErrorInResponse returned nil for %s", method)

			// Verify the "base" field is set via protobuf reflection
			baseField := resp.ProtoReflect().Descriptor().Fields().ByName("base")
			require.NotNil(t, baseField, "response has no 'base' field for %s", method)

			baseMsg := resp.ProtoReflect().Get(baseField).Message()
			isSuccess := baseMsg.Get(baseMsg.Descriptor().Fields().ByName("is_success")).Bool()
			statusCode := baseMsg.Get(baseMsg.Descriptor().Fields().ByName("status_code")).String()
			message := baseMsg.Get(baseMsg.Descriptor().Fields().ByName("message")).String()

			assert.False(t, isSuccess)
			assert.Equal(t, "401", statusCode)
			assert.Equal(t, "authentication required", message)
		})
	}
}

func TestWrapErrorInResponse_UnknownMethod(t *testing.T) {
	err := status.Error(codes.Unauthenticated, "auth required")
	resp := wrapErrorInResponse("/unknown.Service/Method", err)
	assert.Nil(t, resp, "should return nil for unknown methods")
}

func TestWrapErrorInResponse_HealthCheck(t *testing.T) {
	// Health check is from grpc.health.v1, should gracefully return nil
	err := status.Error(codes.Internal, "unhealthy")
	resp := wrapErrorInResponse("/grpc.health.v1.Health/Check", err)
	// May or may not find the type - either way should not panic
	_ = resp
}

func TestGrpcCodeToHTTPStatus(t *testing.T) {
	tests := []struct {
		code     codes.Code
		expected int
	}{
		{codes.OK, 200},
		{codes.InvalidArgument, 400},
		{codes.Unauthenticated, 401},
		{codes.PermissionDenied, 403},
		{codes.NotFound, 404},
		{codes.AlreadyExists, 409},
		{codes.ResourceExhausted, 429},
		{codes.Internal, 500},
		{codes.Unavailable, 503},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, grpcCodeToHTTPStatus(tt.code))
	}
}
