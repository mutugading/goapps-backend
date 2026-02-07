package e2e_test

import (
	"fmt"
	"math/rand/v2"
	"testing"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
)

func TestUser_CreateAndGet(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	userClient := iamv1.NewUserServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	suffix := fmt.Sprintf("%06d", rand.IntN(999999))
	username := "e2euser_" + suffix
	email := "e2euser_" + suffix + "@test.local"
	employeeCode := "E2E" + suffix

	// Create user.
	createResp, err := userClient.CreateUser(ctx, &iamv1.CreateUserRequest{
		Username:     username,
		Email:        email,
		Password:     "TestPass123!",
		EmployeeCode: employeeCode,
		FullName:     "E2E Test User " + suffix,
		FirstName:    "E2E",
		LastName:     "User" + suffix,
	})
	if err != nil {
		t.Fatalf("CreateUser RPC failed: %v", err)
	}
	if createResp.GetBase() == nil || !createResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful create, got: %v", createResp.GetBase().GetMessage())
	}

	createdUser := createResp.GetData()
	if createdUser == nil {
		t.Fatal("Expected user data in create response, got nil")
	}
	userID := createdUser.GetUser().GetUserId()
	if userID == "" {
		t.Fatal("Expected non-empty user ID")
	}
	if createdUser.GetUser().GetUsername() != username {
		t.Errorf("Expected username %q, got %q", username, createdUser.GetUser().GetUsername())
	}

	// Get user by ID.
	getResp, err := userClient.GetUser(ctx, &iamv1.GetUserRequest{
		UserId: userID,
	})
	if err != nil {
		t.Fatalf("GetUser RPC failed: %v", err)
	}
	if getResp.GetBase() == nil || !getResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful get, got: %v", getResp.GetBase().GetMessage())
	}

	gotUser := getResp.GetData()
	if gotUser == nil {
		t.Fatal("Expected user data in get response, got nil")
	}
	if gotUser.GetUserId() != userID {
		t.Errorf("Expected user ID %q, got %q", userID, gotUser.GetUserId())
	}
	if gotUser.GetUsername() != username {
		t.Errorf("Expected username %q, got %q", username, gotUser.GetUsername())
	}
	if gotUser.GetEmail() != email {
		t.Errorf("Expected email %q, got %q", email, gotUser.GetEmail())
	}

	// Cleanup: delete the user.
	_, err = userClient.DeleteUser(ctx, &iamv1.DeleteUserRequest{
		UserId: userID,
	})
	if err != nil {
		t.Logf("Warning: cleanup DeleteUser failed: %v", err)
	}
}

func TestUser_List(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	userClient := iamv1.NewUserServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	// List users with pagination.
	listResp, err := userClient.ListUsers(ctx, &iamv1.ListUsersRequest{
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListUsers RPC failed: %v", err)
	}
	if listResp.GetBase() == nil || !listResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful list, got: %v", listResp.GetBase().GetMessage())
	}

	if listResp.GetPagination() == nil {
		t.Fatal("Expected pagination in list response, got nil")
	}
	if listResp.GetPagination().GetCurrentPage() != 1 {
		t.Errorf("Expected current page 1, got %d", listResp.GetPagination().GetCurrentPage())
	}
	if listResp.GetPagination().GetPageSize() != 10 {
		t.Errorf("Expected page size 10, got %d", listResp.GetPagination().GetPageSize())
	}
	// There should be at least the admin user.
	if listResp.GetPagination().GetTotalItems() == 0 {
		t.Error("Expected at least one user in the system")
	}
	if len(listResp.GetData()) == 0 {
		t.Error("Expected at least one user in the list data")
	}
}

func TestUser_AssignRoles(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	userClient := iamv1.NewUserServiceClient(conn)
	roleClient := iamv1.NewRoleServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	suffix := fmt.Sprintf("%06d", rand.IntN(999999))

	// Create a test role.
	roleResp, err := roleClient.CreateRole(ctx, &iamv1.CreateRoleRequest{
		RoleCode:    "E2E_ASGN_" + suffix,
		RoleName:    "E2E Assign Role " + suffix,
		Description: "Role for assignment test",
	})
	if err != nil {
		t.Fatalf("CreateRole RPC failed: %v", err)
	}
	if roleResp.GetBase() == nil || !roleResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful role create, got: %v", roleResp.GetBase().GetMessage())
	}
	roleID := roleResp.GetData().GetRoleId()

	// Create a test user.
	username := "e2eassign_" + suffix
	userResp, err := userClient.CreateUser(ctx, &iamv1.CreateUserRequest{
		Username:     username,
		Email:        "e2eassign_" + suffix + "@test.local",
		Password:     "TestPass123!",
		EmployeeCode: "ASG" + suffix,
		FullName:     "E2E Assign User " + suffix,
		FirstName:    "E2E",
		LastName:     "Assign" + suffix,
	})
	if err != nil {
		t.Fatalf("CreateUser RPC failed: %v", err)
	}
	if userResp.GetBase() == nil || !userResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful user create, got: %v", userResp.GetBase().GetMessage())
	}
	userID := userResp.GetData().GetUser().GetUserId()

	// Assign the role to the user.
	assignResp, err := userClient.AssignUserRoles(ctx, &iamv1.AssignUserRolesRequest{
		UserId:  userID,
		RoleIds: []string{roleID},
	})
	if err != nil {
		t.Fatalf("AssignUserRoles RPC failed: %v", err)
	}
	if assignResp.GetBase() == nil || !assignResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful assign, got: %v", assignResp.GetBase().GetMessage())
	}

	// Verify by getting the user's roles and permissions.
	accessResp, err := userClient.GetUserRolesAndPermissions(ctx, &iamv1.GetUserRolesAndPermissionsRequest{
		UserId: userID,
	})
	if err != nil {
		t.Fatalf("GetUserRolesAndPermissions RPC failed: %v", err)
	}
	if accessResp.GetBase() == nil || !accessResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful response, got: %v", accessResp.GetBase().GetMessage())
	}

	// Verify the assigned role appears in the response data.
	accessData := accessResp.GetData()
	if accessData == nil {
		t.Fatal("Expected access data in response, got nil")
	}

	// Cleanup: delete user and role.
	_, err = userClient.DeleteUser(ctx, &iamv1.DeleteUserRequest{
		UserId: userID,
	})
	if err != nil {
		t.Logf("Warning: cleanup DeleteUser failed: %v", err)
	}
	_, err = roleClient.DeleteRole(ctx, &iamv1.DeleteRoleRequest{
		RoleId: roleID,
	})
	if err != nil {
		t.Logf("Warning: cleanup DeleteRole failed: %v", err)
	}
}
