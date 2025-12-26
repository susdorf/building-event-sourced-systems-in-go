package main

import (
	"fmt"

	"multi-tenancy-demo/pkg"
)

func main() {
	fmt.Println("=== Multi-Tenancy Demo ===")
	fmt.Println()

	// Setup
	fmt.Println("1. Setting up event store and facade:")
	store := pkg.NewEventStore()
	store.PopulateSampleEvents()

	facade := pkg.NewFacade(store)
	facade.AddCommandMiddleware(pkg.AuthCommandMiddleware(store))
	facade.AddQueryMiddleware(pkg.AuthQueryMiddleware(store))
	fmt.Println("   Registered AuthCommandMiddleware and AuthQueryMiddleware")
	fmt.Println()

	// --- Test Scenarios ---

	// 2. Same-tenant access (allowed)
	fmt.Println("2. Same-tenant command (tenant-A -> tenant-A):")
	ctx := pkg.CreateContextWithTenant("tenant-A", "user-1", false)
	cmd := pkg.NewCommand(ctx, "orders", "PlaceOrder", "order-new")
	result := facade.DispatchCommand(ctx, cmd)
	printResult(result)
	printAuthLog()

	// 3. Cross-tenant access (denied)
	fmt.Println("3. Cross-tenant command (tenant-A -> tenant-B):")
	ctx = pkg.CreateContextWithTenant("tenant-A", "user-1", false)
	cmd = pkg.NewCommandForTenant(ctx, "tenant-B", "orders", "CancelOrder", "order-3")
	result = facade.DispatchCommand(ctx, cmd)
	printResult(result)
	printAuthLog()

	// 4. System admin bypass
	fmt.Println("4. System admin accessing tenant-B (system -> tenant-B):")
	ctx = pkg.CreateContextWithTenant(pkg.SystemTenantUuid, "admin-1", false)
	cmd = pkg.NewCommandForTenant(ctx, "tenant-B", "orders", "RefundOrder", "order-3")
	result = facade.DispatchCommand(ctx, cmd)
	printResult(result)
	printAuthLog()

	// 5. Admin flag bypass
	fmt.Println("5. Admin user accessing different tenant (admin flag):")
	ctx = pkg.CreateContextWithTenant("tenant-A", "admin-user", true) // isAdmin = true
	cmd = pkg.NewCommandForTenant(ctx, "tenant-B", "accounts", "SuspendUser", "user-2")
	result = facade.DispatchCommand(ctx, cmd)
	printResult(result)
	printAuthLog()

	// 6. Aggregate ownership validation - accessing own aggregate
	fmt.Println("6. Accessing own aggregate (tenant-A -> order-1):")
	ctx = pkg.CreateContextWithTenant("tenant-A", "user-1", false)
	cmd = pkg.NewCommand(ctx, "orders", "ShipOrder", "order-1") // order-1 belongs to tenant-A
	result = facade.DispatchCommand(ctx, cmd)
	printResult(result)
	printAuthLog()

	// 7. Aggregate ownership validation - accessing other tenant's aggregate
	fmt.Println("7. Accessing other tenant's aggregate (tenant-A -> order-3):")
	ctx = pkg.CreateContextWithTenant("tenant-A", "user-1", false)
	// Try to access order-3 which belongs to tenant-B, but claim it's tenant-A's
	cmd = pkg.NewCommand(ctx, "orders", "ShipOrder", "order-3")
	result = facade.DispatchCommand(ctx, cmd)
	printResult(result)
	printAuthLog()

	// 8. Event store tenant isolation
	fmt.Println("8. Event store tenant isolation:")
	fmt.Println("   Total events in store:", store.Count())
	fmt.Println("   Events for tenant-A:", store.CountForTenant("tenant-A"))
	fmt.Println("   Events for tenant-B:", store.CountForTenant("tenant-B"))
	fmt.Println()

	eventsA := store.LoadForAggregate("tenant-A", "order-1")
	fmt.Printf("   Loading order-1 as tenant-A: %d events found\n", len(eventsA))

	eventsB := store.LoadForAggregate("tenant-B", "order-1")
	fmt.Printf("   Loading order-1 as tenant-B: %d events found (isolation works!)\n", len(eventsB))
	fmt.Println()

	// 9. Query authorization
	fmt.Println("9. Query authorization:")
	ctx = pkg.CreateContextWithTenant("tenant-A", "user-1", false)
	qry := pkg.NewQuery(ctx, "orders", "GetOrder", "order-1")
	qryResult := facade.DispatchQuery(ctx, qry)
	if qryResult.Success {
		events := qryResult.Data.([]*pkg.Event)
		fmt.Printf("   -> Query succeeded: %d events returned\n", len(events))
	}

	// Cross-tenant query
	qry = &pkg.Query{TenantUuid: "tenant-B", AggregateUuid: "order-3"}
	qryResult = facade.DispatchQuery(ctx, qry)
	if !qryResult.Success {
		fmt.Printf("   -> Cross-tenant query denied: %v\n", qryResult.Error)
	}
	fmt.Println()

	// 10. Skip authorization flag
	fmt.Println("10. Internal system call with SkipAuthorization:")
	ctx = pkg.CreateContextWithTenant("tenant-A", "system-process", false)
	cmd = pkg.NewCommandForTenant(ctx, "tenant-B", "system", "ReconcileData", "")
	cmd.SkipAuthorization = true
	result = facade.DispatchCommand(ctx, cmd)
	printResult(result)
	printAuthLog()

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

func printAuthLog() {
	log := pkg.GetAuthLog()
	fmt.Println("   Authorization steps:")
	for _, entry := range log.Entries {
		fmt.Printf("      [%s] %s - %s\n", entry.Decision, entry.Step, entry.Details)
	}
	fmt.Println()
}
