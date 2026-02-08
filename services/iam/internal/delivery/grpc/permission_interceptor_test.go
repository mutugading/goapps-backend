package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func ctxWithRolesAndPerms(roles, permissions []string) context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, UserIDKey, "user-123")
	ctx = context.WithValue(ctx, RolesKey, roles)
	ctx = context.WithValue(ctx, PermissionsKey, permissions)
	return ctx
}

func TestPermissionInterceptor_PublicMethod(t *testing.T) {
	interceptor := PermissionInterceptor()

	info := &grpc.UnaryServerInfo{FullMethod: "/iam.v1.AuthService/Login"}
	resp, err := interceptor(context.Background(), nil, info, noopHandler)

	assert.NoError(t, err)
	assert.Equal(t, "ok", resp)
}

func TestPermissionInterceptor_SuperAdminBypass(t *testing.T) {
	interceptor := PermissionInterceptor()

	// Super admin should bypass all permission checks.
	ctx := ctxWithRolesAndPerms([]string{"SUPER_ADMIN"}, nil)
	info := &grpc.UnaryServerInfo{FullMethod: "/iam.v1.UserService/CreateUser"}
	resp, err := interceptor(ctx, nil, info, noopHandler)

	assert.NoError(t, err)
	assert.Equal(t, "ok", resp)
}

func TestPermissionInterceptor_UnmappedMethodDenied(t *testing.T) {
	interceptor := PermissionInterceptor()

	// Non-super-admin on an unmapped method should be denied.
	ctx := ctxWithRolesAndPerms([]string{"ADMIN"}, []string{"iam.user.account.view"})
	info := &grpc.UnaryServerInfo{FullMethod: "/iam.v1.UnknownService/UnknownMethod"}
	_, err := interceptor(ctx, nil, info, noopHandler)

	assert.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestPermissionInterceptor_AuthenticatedOnly(t *testing.T) {
	interceptor := PermissionInterceptor()

	// Methods with empty permission (authenticated-only) should pass.
	ctx := ctxWithRolesAndPerms([]string{"USER"}, nil)
	info := &grpc.UnaryServerInfo{FullMethod: "/iam.v1.AuthService/GetCurrentUser"}
	resp, err := interceptor(ctx, nil, info, noopHandler)

	assert.NoError(t, err)
	assert.Equal(t, "ok", resp)
}

func TestPermissionInterceptor_HasPermission(t *testing.T) {
	interceptor := PermissionInterceptor()

	ctx := ctxWithRolesAndPerms([]string{"ADMIN"}, []string{"iam.user.account.view"})
	info := &grpc.UnaryServerInfo{FullMethod: "/iam.v1.UserService/ListUsers"}
	resp, err := interceptor(ctx, nil, info, noopHandler)

	assert.NoError(t, err)
	assert.Equal(t, "ok", resp)
}

func TestPermissionInterceptor_MissingPermission(t *testing.T) {
	interceptor := PermissionInterceptor()

	// User has view but not create permission.
	ctx := ctxWithRolesAndPerms([]string{"VIEWER"}, []string{"iam.user.account.view"})
	info := &grpc.UnaryServerInfo{FullMethod: "/iam.v1.UserService/CreateUser"}
	_, err := interceptor(ctx, nil, info, noopHandler)

	assert.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestPermissionInterceptor_AllMappedMethods(t *testing.T) {
	// Verify every mapped method has a valid permission code format.
	for method, req := range methodPermissions {
		t.Run(method, func(t *testing.T) {
			assert.NotEmpty(t, method, "method key should not be empty")
			// Permission can be empty (authenticated-only) but should be valid if set.
			if req.Permission != "" {
				// Format: {service}.{module}.{entity}.{action}
				parts := splitPermissionCode(req.Permission)
				assert.GreaterOrEqual(t, len(parts), 4,
					"permission %q should have at least 4 parts (service.module.entity.action)", req.Permission)
			}
		})
	}
}

func splitPermissionCode(code string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(code); i++ {
		if i == len(code) || code[i] == '.' {
			parts = append(parts, code[start:i])
			start = i + 1
		}
	}
	return parts
}

func TestPermissionInterceptor_RoleServiceMethods(t *testing.T) {
	interceptor := PermissionInterceptor()

	tests := []struct {
		method     string
		permission string
		wantAllow  bool
	}{
		{"/iam.v1.RoleService/CreateRole", "iam.rbac.role.create", true},
		{"/iam.v1.RoleService/CreateRole", "iam.rbac.role.view", false},
		{"/iam.v1.RoleService/ListRoles", "iam.rbac.role.view", true},
		{"/iam.v1.RoleService/DeleteRole", "iam.rbac.role.delete", true},
		{"/iam.v1.RoleService/DeleteRole", "iam.rbac.role.view", false},
	}

	for _, tt := range tests {
		t.Run(tt.method+"_"+tt.permission, func(t *testing.T) {
			ctx := ctxWithRolesAndPerms([]string{"ADMIN"}, []string{tt.permission})
			info := &grpc.UnaryServerInfo{FullMethod: tt.method}
			_, err := interceptor(ctx, nil, info, noopHandler)

			if tt.wantAllow {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				assert.Equal(t, codes.PermissionDenied, st.Code())
			}
		})
	}
}
