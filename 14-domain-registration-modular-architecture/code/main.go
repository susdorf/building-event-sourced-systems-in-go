package main

import (
	"context"
	"fmt"

	"domain-registration-demo/domain"
	"domain-registration-demo/domain/order"
	"domain-registration-demo/domain/order/aggregate"
	"domain-registration-demo/domain/order/command"
	"domain-registration-demo/pkg"
)

func main() {
	fmt.Println("=== Domain Registration Demo ===")
	fmt.Println()

	ctx := context.Background()

	// --- 1. Create Facade ---
	fmt.Println("1. Creating Facade...")
	fc, err := pkg.NewFacade(
		pkg.FacadeWithAppName("demo-app"),
	)
	if err != nil {
		fmt.Printf("Failed to create facade: %v\n", err)
		return
	}
	fmt.Printf("   Facade created: %s\n", fc.AppName)

	// --- 2. Register Middleware (before domains) ---
	fmt.Println("\n2. Registering Middleware...")

	// Metrics middleware to track command execution
	metrics := &pkg.CommandMetrics{}
	fc.AddCommandMiddleware(pkg.MetricsCommandMiddleware(metrics))
	fmt.Println("   -> Added MetricsCommandMiddleware")

	// Logging middleware
	fc.AddCommandMiddleware(pkg.LoggingCommandMiddleware())
	fmt.Println("   -> Added LoggingCommandMiddleware")

	// Authorization middleware (tenant whitelist)
	authorizedTenants := map[string]bool{
		"tenant-A": true,
		"tenant-B": true,
	}
	fc.AddCommandMiddleware(pkg.AuthorizationCommandMiddleware(authorizedTenants))
	fmt.Println("   -> Added AuthorizationCommandMiddleware")

	// Event logging middleware
	fc.AddEventMiddleware(pkg.LoggingEventMiddleware())
	fmt.Println("   -> Added LoggingEventMiddleware")

	// --- 3. Register Domains ---
	if err := domain.RegisterDomains(ctx, fc); err != nil {
		fmt.Printf("Failed to register domains: %v\n", err)
		return
	}

	// --- 4. Execute Commands ---
	fmt.Println("\n========================================")
	fmt.Println("Executing Commands")
	fmt.Println("========================================")

	// Command 1: Place an order
	fmt.Println("\n3. Placing an order...")
	placeOrderCmd := pkg.Command{
		Uuid:          "cmd-1",
		TenantUuid:    "tenant-A",
		AggregateUuid: "order-001",
		Domain:        "Order",
		CommandType:   "PlaceOrder",
		Data: &command.PlaceOrderCommand{
			OrderID:    "order-001",
			CustomerID: "customer-123",
			Items: []aggregate.OrderItem{
				{ProductID: "prod-1", Quantity: 2, Price: 29.99},
				{ProductID: "prod-2", Quantity: 1, Price: 49.99},
			},
		},
	}

	events, err := fc.ExecuteCommand(ctx, placeOrderCmd)
	if err != nil {
		fmt.Printf("   Command failed: %v\n", err)
	} else {
		fmt.Printf("   -> Success! Produced %d event(s)\n", len(events))
	}

	// Command 2: Place another order
	fmt.Println("\n4. Placing another order...")
	placeOrderCmd2 := pkg.Command{
		Uuid:          "cmd-2",
		TenantUuid:    "tenant-B",
		AggregateUuid: "order-002",
		Domain:        "Order",
		CommandType:   "PlaceOrder",
		Data: &command.PlaceOrderCommand{
			OrderID:    "order-002",
			CustomerID: "customer-456",
			Items: []aggregate.OrderItem{
				{ProductID: "prod-3", Quantity: 5, Price: 19.99},
			},
		},
	}

	events, err = fc.ExecuteCommand(ctx, placeOrderCmd2)
	if err != nil {
		fmt.Printf("   Command failed: %v\n", err)
	} else {
		fmt.Printf("   -> Success! Produced %d event(s)\n", len(events))
	}

	// Command 3: Cancel an order (demonstrates cross-domain event handling)
	fmt.Println("\n5. Cancelling order-001...")
	cancelOrderCmd := pkg.Command{
		Uuid:          "cmd-3",
		TenantUuid:    "tenant-A",
		AggregateUuid: "order-001",
		Domain:        "Order",
		CommandType:   "CancelOrder",
		Data: &command.CancelOrderCommand{
			OrderID: "order-001",
			Reason:  "Customer requested cancellation",
		},
	}

	events, err = fc.ExecuteCommand(ctx, cancelOrderCmd)
	if err != nil {
		fmt.Printf("   Command failed: %v\n", err)
	} else {
		fmt.Printf("   -> Success! Produced %d event(s)\n", len(events))
	}

	// Command 4: Unauthorized tenant (should fail)
	fmt.Println("\n6. Attempting unauthorized command...")
	unauthorizedCmd := pkg.Command{
		Uuid:          "cmd-4",
		TenantUuid:    "tenant-UNKNOWN",
		AggregateUuid: "order-003",
		Domain:        "Order",
		CommandType:   "PlaceOrder",
		Data: &command.PlaceOrderCommand{
			OrderID:    "order-003",
			CustomerID: "hacker",
			Items: []aggregate.OrderItem{
				{ProductID: "prod-1", Quantity: 100, Price: 0.01},
			},
		},
	}

	_, err = fc.ExecuteCommand(ctx, unauthorizedCmd)
	if err != nil {
		fmt.Printf("   -> Expected failure: %v\n", err)
	}

	// --- 5. Query Read Model ---
	fmt.Println("\n========================================")
	fmt.Println("Querying Read Model")
	fmt.Println("========================================")

	fmt.Println("\n7. Querying Order readmodel...")
	orders := order.Readmodel.GetAllOrders()
	fmt.Printf("   Total orders in readmodel: %d\n", len(orders))
	for _, o := range orders {
		fmt.Printf("   -> Order %s: status=%s, customer=%s, total=%.2f\n",
			o.OrderID, o.Status, o.CustomerID, o.Total)
	}

	// --- 6. Show Metrics ---
	fmt.Println("\n========================================")
	fmt.Println("Metrics Summary")
	fmt.Println("========================================")

	fmt.Printf("   Total commands executed: %d\n", metrics.TotalCommands)
	fmt.Printf("   Successful: %d\n", metrics.SuccessfulCount)
	fmt.Printf("   Failed: %d\n", metrics.FailedCount)
	fmt.Printf("   Total events produced: %d\n", metrics.TotalEventsCount)

	// --- 7. Show stored events ---
	fmt.Println("\n========================================")
	fmt.Println("Event Store Contents")
	fmt.Println("========================================")

	allEvents := fc.GetEvents()
	fmt.Printf("   Total events stored: %d\n", len(allEvents))
	for i, evt := range allEvents {
		fmt.Printf("   [%d] %s.%s (aggregate: %s)\n",
			i+1, evt.Domain, evt.EventType, evt.AggregateUuid)
	}

	fmt.Println("\n=== Demo Complete ===")
}
