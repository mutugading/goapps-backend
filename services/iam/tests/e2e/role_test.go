package e2e_test

import (
	"fmt"
	"math/rand/v2"
	"testing"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
)

func randomSuffix() string {
	return fmt.Sprintf("%06d", rand.IntN(999999))
}

func TestRole_CreateAndGet(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewRoleServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	suffix := randomSuffix()
	roleCode := "E2E_ROLE_" + suffix
	roleName := "E2E Test Role " + suffix

	// Create role.
	createResp, err := client.CreateRole(ctx, &iamv1.CreateRoleRequest{
		RoleCode:    roleCode,
		RoleName:    roleName,
		Description: "Role created by E2E test",
	})
	if err != nil {
		t.Fatalf("CreateRole RPC failed: %v", err)
	}
	if createResp.GetBase() == nil || !createResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful create, got: %v", createResp.GetBase().GetMessage())
	}

	createdRole := createResp.GetData()
	if createdRole == nil {
		t.Fatal("Expected role data in create response, got nil")
	}
	if createdRole.GetRoleId() == "" {
		t.Error("Expected non-empty role ID")
	}
	if createdRole.GetRoleCode() != roleCode {
		t.Errorf("Expected role code %q, got %q", roleCode, createdRole.GetRoleCode())
	}
	if createdRole.GetRoleName() != roleName {
		t.Errorf("Expected role name %q, got %q", roleName, createdRole.GetRoleName())
	}

	// Get role by ID.
	getResp, err := client.GetRole(ctx, &iamv1.GetRoleRequest{
		RoleId: createdRole.GetRoleId(),
	})
	if err != nil {
		t.Fatalf("GetRole RPC failed: %v", err)
	}
	if getResp.GetBase() == nil || !getResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful get, got: %v", getResp.GetBase().GetMessage())
	}

	gotRole := getResp.GetData()
	if gotRole == nil {
		t.Fatal("Expected role data in get response, got nil")
	}
	if gotRole.GetRoleId() != createdRole.GetRoleId() {
		t.Errorf("Expected role ID %q, got %q", createdRole.GetRoleId(), gotRole.GetRoleId())
	}
	if gotRole.GetRoleCode() != roleCode {
		t.Errorf("Expected role code %q, got %q", roleCode, gotRole.GetRoleCode())
	}
	if gotRole.GetRoleName() != roleName {
		t.Errorf("Expected role name %q, got %q", roleName, gotRole.GetRoleName())
	}

	// Cleanup: delete the role.
	_, err = client.DeleteRole(ctx, &iamv1.DeleteRoleRequest{
		RoleId: createdRole.GetRoleId(),
	})
	if err != nil {
		t.Logf("Warning: cleanup DeleteRole failed: %v", err)
	}
}

func TestRole_List(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewRoleServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	// List roles with pagination.
	listResp, err := client.ListRoles(ctx, &iamv1.ListRolesRequest{
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListRoles RPC failed: %v", err)
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
	// There should be at least one role (admin role from seed data).
	if listResp.GetPagination().GetTotalItems() == 0 {
		t.Error("Expected at least one role in the system")
	}
	if len(listResp.GetData()) == 0 {
		t.Error("Expected at least one role in the list data")
	}
}

func TestRole_Update(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewRoleServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	suffix := randomSuffix()

	// Create a role to update.
	createResp, err := client.CreateRole(ctx, &iamv1.CreateRoleRequest{
		RoleCode:    "E2E_UPD_" + suffix,
		RoleName:    "E2E Update Role " + suffix,
		Description: "Role to be updated",
	})
	if err != nil {
		t.Fatalf("CreateRole RPC failed: %v", err)
	}
	if createResp.GetBase() == nil || !createResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful create, got: %v", createResp.GetBase().GetMessage())
	}
	roleID := createResp.GetData().GetRoleId()

	// Update the role name.
	updatedName := "E2E Updated Name " + suffix
	updateResp, err := client.UpdateRole(ctx, &iamv1.UpdateRoleRequest{
		RoleId:   roleID,
		RoleName: &updatedName,
	})
	if err != nil {
		t.Fatalf("UpdateRole RPC failed: %v", err)
	}
	if updateResp.GetBase() == nil || !updateResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful update, got: %v", updateResp.GetBase().GetMessage())
	}

	// Verify the update by getting the role.
	getResp, err := client.GetRole(ctx, &iamv1.GetRoleRequest{
		RoleId: roleID,
	})
	if err != nil {
		t.Fatalf("GetRole RPC failed: %v", err)
	}
	if getResp.GetData().GetRoleName() != updatedName {
		t.Errorf("Expected updated role name %q, got %q", updatedName, getResp.GetData().GetRoleName())
	}

	// Cleanup.
	_, err = client.DeleteRole(ctx, &iamv1.DeleteRoleRequest{
		RoleId: roleID,
	})
	if err != nil {
		t.Logf("Warning: cleanup DeleteRole failed: %v", err)
	}
}

func TestRole_Delete(t *testing.T) {
	skipIfNoE2E(t)
	conn := grpcConn(t)
	client := iamv1.NewRoleServiceClient(conn)
	accessToken, _ := loginAsAdmin(t, conn)
	ctx := authCtx(accessToken)

	suffix := randomSuffix()

	// Create a role to delete.
	createResp, err := client.CreateRole(ctx, &iamv1.CreateRoleRequest{
		RoleCode:    "E2E_DEL_" + suffix,
		RoleName:    "E2E Delete Role " + suffix,
		Description: "Role to be deleted",
	})
	if err != nil {
		t.Fatalf("CreateRole RPC failed: %v", err)
	}
	if createResp.GetBase() == nil || !createResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful create, got: %v", createResp.GetBase().GetMessage())
	}
	roleID := createResp.GetData().GetRoleId()

	// Delete the role.
	deleteResp, err := client.DeleteRole(ctx, &iamv1.DeleteRoleRequest{
		RoleId: roleID,
	})
	if err != nil {
		t.Fatalf("DeleteRole RPC failed: %v", err)
	}
	if deleteResp.GetBase() == nil || !deleteResp.GetBase().GetIsSuccess() {
		t.Fatalf("Expected successful delete, got: %v", deleteResp.GetBase().GetMessage())
	}

	// Verify the role is no longer found.
	getResp, err := client.GetRole(ctx, &iamv1.GetRoleRequest{
		RoleId: roleID,
	})
	if err != nil {
		// A gRPC NotFound error is acceptable.
		t.Logf("GetRole after delete returned expected error: %v", err)
		return
	}
	if getResp.GetBase() != nil && getResp.GetBase().GetIsSuccess() {
		t.Error("Expected GetRole to fail after deletion, but it succeeded")
	}
}
