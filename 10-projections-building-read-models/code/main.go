package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"projections-demo/pkg"
)

func main() {
	fmt.Println("=== Projections Demo ===")
	fmt.Println()

	ctx := context.Background()

	// Setup: Create event repository and populate with sample events
	eventRepo := pkg.CreateSampleEvents()
	backend := pkg.NewMemoryStoreBackend()

	fmt.Printf("Event repository contains %d events\n", eventRepo.Count())
	fmt.Println()

	// --- 1. Build projection from events ---
	fmt.Println("1. Building projection from events:")
	projection := pkg.NewOrderProjection(eventRepo, backend)
	projection.Rebuild(ctx)
	fmt.Printf("   -> Projection contains %d orders\n", projection.Count())
	fmt.Printf("   -> Events processed: %d\n", projection.GetEventsProcessed())
	fmt.Println()

	// --- 2. Query by order ID ---
	fmt.Println("2. Query single order by ID:")
	if order, found := projection.GetOrder("order-1"); found {
		fmt.Printf("   -> Order: %s\n", order.OrderID)
		fmt.Printf("      Customer: %s (%s)\n", order.CustomerName, order.CustomerID)
		fmt.Printf("      Status: %s\n", order.Status)
		fmt.Printf("      Items: %d, Total: $%.2f\n", order.ItemCount, order.Total)
		fmt.Printf("      Created: %s\n", order.CreatedAt.Format("15:04:05"))
	}
	fmt.Println()

	// --- 3. Query by tenant (secondary index) ---
	fmt.Println("3. Query orders by tenant (secondary index):")
	tenantAOrders := projection.ListByTenant("tenant-A")
	fmt.Printf("   -> Tenant A has %d orders:\n", len(tenantAOrders))
	for _, order := range tenantAOrders {
		fmt.Printf("      - %s: %s ($%.2f)\n", order.OrderID, order.Status, order.Total)
	}

	tenantBOrders := projection.ListByTenant("tenant-B")
	fmt.Printf("   -> Tenant B has %d orders:\n", len(tenantBOrders))
	for _, order := range tenantBOrders {
		fmt.Printf("      - %s: %s ($%.2f)\n", order.OrderID, order.Status, order.Total)
	}
	fmt.Println()

	// --- 4. Query by status (another secondary index) ---
	fmt.Println("4. Query orders by status (secondary index):")
	statusCounts := map[string]int{
		"placed":    len(projection.ListByStatus("placed")),
		"paid":      len(projection.ListByStatus("paid")),
		"shipped":   len(projection.ListByStatus("shipped")),
		"cancelled": len(projection.ListByStatus("cancelled")),
	}
	for status, count := range statusCounts {
		if count > 0 {
			fmt.Printf("   -> %s: %d orders\n", status, count)
		}
	}
	fmt.Println()

	// --- 5. Aggregate statistics ---
	fmt.Println("5. Aggregate statistics per tenant:")
	statsA := projection.GetStatistics("tenant-A")
	fmt.Printf("   -> Tenant A:\n")
	fmt.Printf("      Orders: %d, Revenue: $%.2f\n", statsA.TotalOrders, statsA.TotalRevenue)
	fmt.Printf("      Placed: %d, Paid: %d, Shipped: %d\n",
		statsA.PlacedCount, statsA.PaidCount, statsA.ShippedCount)

	statsB := projection.GetStatistics("tenant-B")
	fmt.Printf("   -> Tenant B:\n")
	fmt.Printf("      Orders: %d, Revenue: $%.2f\n", statsB.TotalOrders, statsB.TotalRevenue)
	fmt.Printf("      Placed: %d, Paid: %d, Cancelled: %d\n",
		statsB.PlacedCount, statsB.PaidCount, statsB.CancelledCount)
	fmt.Println()

	// --- 6. Middleware demonstration ---
	fmt.Println("6. Event handler middleware:")
	metrics := &pkg.ProjectionMetrics{}
	projWithMiddleware := pkg.NewOrderProjection(eventRepo, pkg.NewMemoryStoreBackend()).
		WithMiddleware(
			pkg.WithRecovery(),
			pkg.WithMetrics(metrics),
			pkg.WithLogging(),
		)

	// Process a single event through middleware
	sampleEvent := pkg.Event{
		UUID:          "evt-demo",
		TenantUUID:    "tenant-demo",
		AggregateUUID: "order-demo",
		Domain:        "Order",
		DataType:      "OrderPlacedEvent",
		Data:          `{"customerId":"cust-demo","customerName":"Demo User","items":[],"total":99.99}`,
		Version:       1,
		CreatedAt:     time.Now().UnixNano(),
	}

	fmt.Println("   Processing event through middleware chain:")
	projWithMiddleware.HandleEvent(ctx, sampleEvent)

	processed, failed, avgDuration := metrics.GetStats()
	fmt.Printf("   -> Metrics: processed=%d, failed=%d, avgDuration=%v\n",
		processed, failed, avgDuration)
	fmt.Println()

	// --- 7. Store backend abstraction ---
	fmt.Println("7. Store backend abstraction:")
	memBackend := pkg.NewMemoryStoreBackend()

	// Store some values
	memBackend.Set("namespace1", "key1", "value1")
	memBackend.Set("namespace1", "key2", "value2")
	memBackend.Set("namespace2", "key1", "other-value")

	// Retrieve
	val, _ := memBackend.Get("namespace1", "key1")
	fmt.Printf("   -> Get(namespace1, key1) = %v\n", val)

	// Count by namespace
	count1 := 0
	memBackend.Range("namespace1", func(key string, value any) bool {
		count1++
		return true
	})
	count2 := 0
	memBackend.Range("namespace2", func(key string, value any) bool {
		count2++
		return true
	})
	fmt.Printf("   -> namespace1 has %d items, namespace2 has %d items\n", count1, count2)

	// Reset namespace
	memBackend.Reset("namespace1")
	val, err := memBackend.Get("namespace1", "key1")
	fmt.Printf("   -> After Reset(namespace1): Get returns error=%v\n", err)
	fmt.Println()

	// --- 8. Type-safe store wrapper ---
	fmt.Println("8. Type-safe store wrapper with generics:")
	type Product struct {
		ID    string
		Name  string
		Price float64
	}

	productStore := pkg.NewReadmodelStore[*Product](pkg.NewMemoryStoreBackend(), "products")
	productStore.Set("prod-1", &Product{ID: "prod-1", Name: "Laptop", Price: 999.99})
	productStore.Set("prod-2", &Product{ID: "prod-2", Name: "Mouse", Price: 29.99})

	if product, found, _ := productStore.Get("prod-1"); found {
		fmt.Printf("   -> Product: %s - $%.2f\n", product.Name, product.Price)
	}
	fmt.Printf("   -> Total products: %d\n", productStore.Count())
	fmt.Println()

	// --- 9. Checkpoint tracking ---
	fmt.Println("9. Checkpoint tracking:")
	fmt.Printf("   -> Last event timestamp: %d\n", projection.GetCheckpoint())
	fmt.Printf("   -> Total events processed: %d\n", projection.GetEventsProcessed())

	// Simulate incremental update
	newEvent := pkg.Event{
		UUID:          "evt-new",
		TenantUUID:    "tenant-A",
		AggregateUUID: "order-1",
		Domain:        "Order",
		DataType:      "OrderPaidEvent",
		Data:          `{"paymentId":"pay-new","paymentMethod":"card","amount":100}`,
		Version:       4,
		CreatedAt:     time.Now().UnixNano(),
	}
	projection.HandleEvent(ctx, newEvent)
	fmt.Printf("   -> After processing new event:\n")
	fmt.Printf("      Last event timestamp: %d\n", projection.GetCheckpoint())
	fmt.Printf("      Total events processed: %d\n", projection.GetEventsProcessed())
	fmt.Println()

	// --- 10. State restoration with live events ---
	fmt.Println("10. State restoration with live events:")
	restoreProj := pkg.NewProjectionWithRestoration(eventRepo, pkg.NewMemoryStoreBackend())
	fmt.Printf("    -> Initial state: %s\n", restoreProj.GetStateString())

	// Start restoration
	doneCh := restoreProj.OnRestoreState(ctx, true)

	// Simulate live event arriving during restoration
	time.Sleep(10 * time.Millisecond)
	liveEvent := pkg.Event{
		UUID:          "evt-live",
		TenantUUID:    "tenant-A",
		AggregateUUID: "order-5",
		Domain:        "Order",
		DataType:      "OrderPlacedEvent",
		Data:          `{"customerId":"cust-5","customerName":"Live User","items":[],"total":50.00}`,
		Version:       1,
		CreatedAt:     time.Now().UnixNano(),
	}
	restoreProj.HandleLiveEvent(ctx, liveEvent)

	// Wait for restoration to complete
	<-doneCh
	fmt.Printf("    -> Final state: %s\n", restoreProj.GetStateString())
	fmt.Printf("    -> Orders in projection: %d\n", restoreProj.Count())
	fmt.Println()

	// --- 11. Multiple projections from same events ---
	fmt.Println("11. Multiple projections from same events:")
	fmt.Println("    Different views serve different needs:")
	fmt.Println("    - CustomerOrderProjection: order status, items, delivery")
	fmt.Println("    - OperationsOrderProjection: metrics, SLA compliance")
	fmt.Println("    - FinanceOrderProjection: revenue, refunds, payments")
	fmt.Println("    All subscribe to the same Order events!")
	fmt.Println()

	// --- 12. Denormalized data example ---
	fmt.Println("12. Denormalized data in projections:")
	if order, found := projection.GetOrder("order-4"); found {
		fmt.Printf("    Order %s contains:\n", order.OrderID)
		fmt.Printf("    - CustomerName: %s (denormalized from Customer aggregate)\n", order.CustomerName)
		fmt.Printf("    - ItemCount: %d (pre-computed)\n", order.ItemCount)
		fmt.Printf("    - Items with LineTotal (pre-computed):\n")
		for _, item := range order.Items {
			fmt.Printf("      * %s: %d x $%.2f = $%.2f\n",
				item.ProductName, item.Quantity, item.Price, item.LineTotal)
		}
	}
	fmt.Println()

	// --- 13. Show all orders ---
	fmt.Println("13. All orders (sorted by creation time):")
	allOrders := projection.ListAll()
	for i, order := range allOrders {
		fmt.Printf("    [%d] %s | %s | %s | $%.2f | %s\n",
			i+1, order.OrderID, order.TenantID, order.Status,
			order.Total, order.CustomerName)
	}
	fmt.Println()

	// --- 14. Event handler with filter middleware ---
	fmt.Println("14. Event filtering middleware:")
	filteredProj := pkg.NewOrderProjection(eventRepo, pkg.NewMemoryStoreBackend()).
		WithMiddleware(
			pkg.WithEventTypeFilter("OrderPlacedEvent", "OrderPaidEvent"),
			pkg.WithLogging(),
		)

	// This event will be processed
	filteredProj.HandleEvent(ctx, pkg.Event{
		UUID: "evt-filter-1", TenantUUID: "t", AggregateUUID: "o",
		Domain: "Order", DataType: "OrderPlacedEvent",
		Data: `{"customerId":"c","customerName":"Test","items":[],"total":10}`,
		Version: 1, CreatedAt: time.Now().UnixNano(),
	})

	// This event will be filtered out
	filteredProj.HandleEvent(ctx, pkg.Event{
		UUID: "evt-filter-2", TenantUUID: "t", AggregateUUID: "o",
		Domain: "Order", DataType: "OrderShippedEvent",
		Data: `{}`,
		Version: 2, CreatedAt: time.Now().UnixNano(),
	})
	fmt.Println()

	// --- 15. Rebuild demonstration ---
	fmt.Println("15. Projection rebuild:")
	fmt.Printf("    -> Before rebuild: %d orders, %d events processed\n",
		projection.Count(), projection.GetEventsProcessed())

	projection.Rebuild(ctx)
	fmt.Printf("    -> After rebuild: %d orders, %d events processed\n",
		projection.Count(), projection.GetEventsProcessed())
	fmt.Println("    Rebuild is useful for:")
	fmt.Println("    - Schema changes in read model")
	fmt.Println("    - Bug fixes in event handlers")
	fmt.Println("    - Adding new projections to existing system")
	fmt.Println()

	fmt.Println("=== Demo Complete ===")
}

// Helper to pretty print JSON (unused but available)
func prettyJSON(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
