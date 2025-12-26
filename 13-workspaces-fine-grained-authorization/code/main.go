package main

import (
	"fmt"

	"workspace-authorization-demo/pkg"
)

func main() {
	fmt.Println("=== Workspace Fine-Grained Authorization Demo ===")
	fmt.Println()

	// Setup
	fmt.Println("1. Setting up authorization store and facade:")
	store := pkg.NewAuthStore()
	store.PopulateSampleData()

	facade := pkg.NewFacade(store)
	facade.AddCommandMiddleware(pkg.AuthCommandMiddleware(store))
	facade.AddQueryMiddleware(pkg.AuthQueryMiddleware(store))
	fmt.Println("   - Created identities: Alice (Tenant A Admin), Bob (no tenant perms), Carol (Tenant B)")
	fmt.Println("   - Created workspaces: Frontend Project, Backend Project")
	fmt.Println("   - Bob is member of Frontend workspace with Developer permissions")
	fmt.Println("   - Alice is member of both workspaces")
	fmt.Println()

	// === TENANT-ONLY SCENARIOS ===
	fmt.Println("=== TENANT-ONLY SCENARIOS ===")
	fmt.Println()

	// Scenario 2: Tenant admin without workspace context
	fmt.Println("2. Alice (Tenant Admin) creates order WITHOUT workspace context:")
	ctx := pkg.CreateContext("tenant-A", "alice")
	cmd := pkg.NewCommand("tenant-A", "", "orders", "PlaceOrderCommand", "")
	result := facade.DispatchCommand(ctx, cmd)
	printResult(result)
	printAuthLog()

	// Scenario 3: User without tenant permissions (no workspace context)
	fmt.Println("3. Bob (no tenant permissions) tries to create order WITHOUT workspace:")
	ctx = pkg.CreateContext("tenant-A", "bob")
	cmd = pkg.NewCommand("tenant-A", "", "orders", "PlaceOrderCommand", "")
	result = facade.DispatchCommand(ctx, cmd)
	printResult(result)
	printAuthLog()

	// === WORKSPACE SCENARIOS ===
	fmt.Println("=== WORKSPACE SCENARIOS ===")
	fmt.Println()

	// Scenario 4: Bob uses workspace permission (ADDITIVE MODEL)
	fmt.Println("4. Bob creates order IN Frontend workspace (workspace permission):")
	fmt.Println("   -> ADDITIVE MODEL: Bob has no tenant permission but HAS workspace permission")
	ctx = pkg.CreateWorkspaceContext("tenant-A", "bob", "workspace-frontend")
	cmd = pkg.NewCommand("tenant-A", "workspace-frontend", "orders", "PlaceOrderCommand", "")
	result = facade.DispatchCommand(ctx, cmd)
	printResult(result)
	printAuthLog()

	// Scenario 5: Bob tries Backend workspace (not a member)
	fmt.Println("5. Bob tries to create order IN Backend workspace (not a member):")
	ctx = pkg.CreateWorkspaceContext("tenant-A", "bob", "workspace-backend")
	cmd = pkg.NewCommand("tenant-A", "workspace-backend", "orders", "PlaceOrderCommand", "")
	result = facade.DispatchCommand(ctx, cmd)
	printResult(result)
	printAuthLog()

	// Scenario 6: Alice uses tenant permission in workspace context
	fmt.Println("6. Alice creates order in Frontend workspace (tenant permission works in workspace):")
	fmt.Println("   -> Alice has tenant-level permission, so she can access ANY workspace")
	ctx = pkg.CreateWorkspaceContext("tenant-A", "alice", "workspace-frontend")
	cmd = pkg.NewCommand("tenant-A", "workspace-frontend", "orders", "PlaceOrderCommand", "")
	result = facade.DispatchCommand(ctx, cmd)
	printResult(result)
	printAuthLog()

	// Scenario 7: Alice accesses workspace she's not explicitly a member of
	fmt.Println("7. Alice ships order in Backend workspace (tenant admin can access all workspaces):")
	ctx = pkg.CreateWorkspaceContext("tenant-A", "alice", "workspace-backend")
	cmd = pkg.NewCommand("tenant-A", "workspace-backend", "orders", "ShipOrderCommand", "order-2")
	result = facade.DispatchCommand(ctx, cmd)
	printResult(result)
	printAuthLog()

	// === AGGREGATE OWNERSHIP SCENARIOS ===
	fmt.Println("=== AGGREGATE OWNERSHIP SCENARIOS ===")
	fmt.Println()

	// Scenario 8: Accessing aggregate that belongs to workspace
	fmt.Println("8. Bob modifies order-1 (belongs to Frontend workspace):")
	ctx = pkg.CreateWorkspaceContext("tenant-A", "bob", "workspace-frontend")
	cmd = pkg.NewCommand("tenant-A", "workspace-frontend", "orders", "PlaceOrderCommand", "order-1")
	result = facade.DispatchCommand(ctx, cmd)
	printResult(result)
	printAuthLog()

	// Scenario 9: Accessing aggregate from different workspace
	fmt.Println("9. Bob tries to access order-2 (belongs to Backend workspace) from Frontend:")
	ctx = pkg.CreateWorkspaceContext("tenant-A", "bob", "workspace-frontend")
	cmd = pkg.NewCommand("tenant-A", "workspace-frontend", "orders", "PlaceOrderCommand", "order-2")
	result = facade.DispatchCommand(ctx, cmd)
	printResult(result)
	printAuthLog()

	// === CROSS-TENANT SCENARIOS ===
	fmt.Println("=== CROSS-TENANT SCENARIOS ===")
	fmt.Println()

	// Scenario 10: Cross-tenant access denied
	fmt.Println("10. Alice (Tenant A) tries to access Tenant B:")
	ctx = pkg.CreateContext("tenant-A", "alice")
	cmd = pkg.NewCommand("tenant-B", "", "orders", "PlaceOrderCommand", "")
	result = facade.DispatchCommand(ctx, cmd)
	printResult(result)
	printAuthLog()

	// === SYSTEM ADMIN SCENARIOS ===
	fmt.Println("=== SYSTEM ADMIN SCENARIOS ===")
	fmt.Println()

	// Scenario 11: System admin bypasses all
	fmt.Println("11. System Admin accesses Tenant B workspace:")
	ctx = pkg.CreateSystemAdminContext()
	cmd = pkg.NewCommand("tenant-B", "", "orders", "PlaceOrderCommand", "order-3")
	result = facade.DispatchCommand(ctx, cmd)
	printResult(result)
	printAuthLog()

	// === SKIP AUTHORIZATION SCENARIOS ===
	fmt.Println("=== SKIP AUTHORIZATION SCENARIOS ===")
	fmt.Println()

	// Scenario 12: Skip authorization for system operations
	fmt.Println("12. System operation with SkipAuthorization flag:")
	ctx = pkg.CreateContext("tenant-A", "bob") // Bob normally can't do this
	cmd = pkg.NewCommand("tenant-B", "", "system", "MigrateData", "")
	cmd.SkipAuthorization = true
	result = facade.DispatchCommand(ctx, cmd)
	printResult(result)
	printAuthLog()

	// === QUERY SCENARIOS ===
	fmt.Println("=== QUERY SCENARIOS ===")
	fmt.Println()

	// Scenario 13: Query with tenant permission
	fmt.Println("13. Alice queries order (tenant permission):")
	ctx = pkg.CreateContext("tenant-A", "alice")
	qry := pkg.NewQuery("tenant-A", "", "orders", "GetOrderQuery", "order-1")
	qryResult := facade.DispatchQuery(ctx, qry)
	printQueryResult(qryResult)
	printAuthLog()

	// Scenario 14: Query with workspace permission
	fmt.Println("14. Bob queries order in Frontend workspace (workspace permission):")
	ctx = pkg.CreateWorkspaceContext("tenant-A", "bob", "workspace-frontend")
	qry = pkg.NewQuery("tenant-A", "workspace-frontend", "orders", "GetOrderQuery", "order-1")
	qryResult = facade.DispatchQuery(ctx, qry)
	printQueryResult(qryResult)
	printAuthLog()

	// === OWNERSHIP vs MEMBERSHIP ===
	fmt.Println("=== OWNERSHIP vs MEMBERSHIP ===")
	fmt.Println()

	fmt.Println("15. Important: Workspace owner without group membership:")
	fmt.Println("    Alice OWNS the Frontend workspace but has no workspace groups assigned.")
	fmt.Println("    She can still access it because she has TENANT-level permissions.")
	fmt.Println("    If she had NO tenant permissions, ownership alone would NOT grant access.")
	fmt.Println()

	// === SUMMARY ===
	fmt.Println("=== AUTHORIZATION SUMMARY ===")
	fmt.Println()
	fmt.Println("The Additive Permission Model:")
	fmt.Println("  Access = (Tenant Permission) OR (Workspace Permission)")
	fmt.Println()
	fmt.Println("Key Points Demonstrated:")
	fmt.Println("  1. Tenant admins can access ALL workspaces without explicit membership")
	fmt.Println("  2. Workspace permissions work for users WITHOUT tenant permissions")
	fmt.Println("  3. Workspace membership is REQUIRED for workspace-level permissions")
	fmt.Println("  4. Aggregate ownership is validated at both tenant and workspace level")
	fmt.Println("  5. Cross-tenant access is ALWAYS denied (except system admin)")
	fmt.Println("  6. System admin bypasses ALL authorization checks")
	fmt.Println("  7. SkipAuthorization flag allows system operations to bypass checks")
	fmt.Println("  8. Ownership != Membership: Owners need explicit group assignment for permissions")
	fmt.Println()

	fmt.Println("=== Demo Complete ===")
}

func printResult(result *pkg.CommandResult) {
	if result.Success {
		fmt.Println("   -> ALLOWED: Command executed successfully")
		if len(result.Events) > 0 {
			fmt.Printf("   -> Created event: %s\n", result.Events[0].EventType)
		}
	} else {
		fmt.Printf("   -> DENIED: %v\n", result.Error)
	}
}

func printQueryResult(result *pkg.QueryResult) {
	if result.Success {
		fmt.Println("   -> ALLOWED: Query executed successfully")
		if events, ok := result.Data.([]*pkg.Event); ok {
			fmt.Printf("   -> Returned %d events\n", len(events))
		}
	} else {
		fmt.Printf("   -> DENIED: %v\n", result.Error)
	}
}

func printAuthLog() {
	log := pkg.GetAuthLog()
	fmt.Println("   Authorization flow:")
	for _, entry := range log.Entries {
		symbol := "→"
		if entry.Decision == "ALLOW" {
			symbol = "✓"
		} else if entry.Decision == "DENY" {
			symbol = "✗"
		}
		fmt.Printf("      %s [%s] %s: %s\n", symbol, entry.Decision, entry.Step, entry.Details)
	}
	fmt.Println()
}
