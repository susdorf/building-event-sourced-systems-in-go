package main

import (
	"fmt"
	"time"

	"functional-options-demo/pkg"
)

func main() {
	fmt.Println("=== Functional Options Demo ===")
	fmt.Println()

	// Setup: Create event store and populate with sample events
	store := pkg.NewEventStore()
	populateSampleEvents(store)

	fmt.Printf("Event store contains %d events\n", store.Count())
	fmt.Println()

	// --- 1. Default query (no options) ---
	fmt.Println("1. Query with defaults (no options):")
	events, total, _ := store.List()
	fmt.Printf("   -> Found %d events (total: %d)\n", len(events), total)
	fmt.Printf("   -> Default limit: 100, Default order: descending (newest first)\n")
	if len(events) > 0 {
		fmt.Printf("   -> First event: %s (domain: %s)\n", events[0].DataType, events[0].Domain)
	}

	// --- 2. Filter by tenant ---
	fmt.Println()
	fmt.Println("2. Filter by tenant:")
	events, total, _ = store.List(
		pkg.WithTenantUuid("tenant-A"),
	)
	fmt.Printf("   -> Found %d events for tenant-A (total: %d)\n", len(events), total)

	// --- 3. Filter by domain ---
	fmt.Println()
	fmt.Println("3. Filter by domain(s):")
	events, total, _ = store.List(
		pkg.WithDomains("orders"),
	)
	fmt.Printf("   -> Found %d events in 'orders' domain\n", len(events))

	events, total, _ = store.List(
		pkg.WithDomains("orders", "inventory"),
	)
	fmt.Printf("   -> Found %d events in 'orders' OR 'inventory' domains\n", len(events))

	// --- 4. Combine multiple options ---
	fmt.Println()
	fmt.Println("4. Combine multiple options:")
	events, total, _ = store.List(
		pkg.WithTenantUuid("tenant-A"),
		pkg.WithDomains("orders"),
		pkg.WithLimit(5),
		pkg.WithAscending(true),
	)
	fmt.Printf("   -> Found %d events (tenant-A + orders + limit 5 + ascending)\n", len(events))
	for i, evt := range events {
		fmt.Printf("      [%d] %s\n", i+1, evt.DataType)
	}

	// --- 5. Pagination with convenience option ---
	fmt.Println()
	fmt.Println("5. Pagination (page 2, 3 items per page):")
	events, total, _ = store.List(
		pkg.WithPagination(2, 3),
		pkg.WithAscending(true),
	)
	fmt.Printf("   -> Page 2 contains %d events (total: %d)\n", len(events), total)
	for i, evt := range events {
		fmt.Printf("      [%d] %s (aggregate: %s)\n", i+1, evt.DataType, evt.AggregateUuid)
	}

	// --- 6. Using preset defaults ---
	fmt.Println()
	fmt.Println("6. Using preset defaults (RecentEventsDefaults):")
	events, total, _ = store.List(
		append(pkg.RecentEventsDefaults(),
			pkg.WithTenantUuid("tenant-B"),
		)...,
	)
	fmt.Printf("   -> Found %d recent events for tenant-B\n", len(events))

	// --- 7. Time range filtering ---
	fmt.Println()
	fmt.Println("7. Time range filtering:")
	now := time.Now()
	events, total, _ = store.List(
		pkg.WithTimeRange(now.Add(-1*time.Hour), now),
	)
	fmt.Printf("   -> Found %d events from the last hour\n", len(events))

	// --- 8. Demonstrate validation ---
	fmt.Println()
	fmt.Println("8. Option validation (invalid inputs):")

	_, _, err := store.List(pkg.WithLimit(-5))
	if err != nil {
		fmt.Printf("   -> WithLimit(-5): %v\n", err)
	}

	_, _, err = store.List(pkg.WithLimit(50000))
	if err != nil {
		fmt.Printf("   -> WithLimit(50000): %v\n", err)
	}

	_, _, err = store.List(pkg.WithPagination(0, 10))
	if err != nil {
		fmt.Printf("   -> WithPagination(0, 10): %v\n", err)
	}

	end := time.Now()
	start := end.Add(1 * time.Hour) // end before start!
	_, _, err = store.List(pkg.WithTimeRange(start, end))
	if err != nil {
		fmt.Printf("   -> WithTimeRange(future, past): %v\n", err)
	}

	// --- 9. Show applied options (debugging) ---
	fmt.Println()
	fmt.Println("9. Inspect applied options:")
	opts, _ := pkg.GetAppliedOptions(
		pkg.WithTenantUuid("tenant-X"),
		pkg.WithDomains("orders", "payments"),
		pkg.WithPagination(3, 25),
	)
	fmt.Printf("   -> TenantUuid: %s\n", opts.TenantUuid)
	fmt.Printf("   -> Domains: %v\n", opts.Domains)
	fmt.Printf("   -> Offset: %d (page 3)\n", opts.Offset)
	fmt.Printf("   -> Limit: %d\n", opts.Limit)

	// --- 10. Extensibility demonstration ---
	fmt.Println()
	fmt.Println("10. Extensibility - adding new options is easy:")
	fmt.Println("    Adding a new option like 'WithDataType' requires:")
	fmt.Println("    1. Add field to EventStoreListOptions struct")
	fmt.Println("    2. Create the option function")
	fmt.Println("    3. Handle the field in the filter logic")
	fmt.Println("    -> Existing code continues to work unchanged!")

	events, total, _ = store.List(
		pkg.WithDataType("OrderPlaced"),
	)
	fmt.Printf("    -> Found %d OrderPlaced events\n", len(events))

	fmt.Println()
	fmt.Println("=== Demo Complete ===")
}

func populateSampleEvents(store *pkg.EventStore) {
	// Create a variety of events for demonstration
	events := []pkg.Event{
		{Uuid: "evt-1", TenantUuid: "tenant-A", AggregateUuid: "order-1", Domain: "orders", DataType: "OrderPlaced", Data: "{}"},
		{Uuid: "evt-2", TenantUuid: "tenant-A", AggregateUuid: "order-1", Domain: "orders", DataType: "OrderPaid", Data: "{}"},
		{Uuid: "evt-3", TenantUuid: "tenant-A", AggregateUuid: "order-1", Domain: "orders", DataType: "OrderShipped", Data: "{}"},
		{Uuid: "evt-4", TenantUuid: "tenant-A", AggregateUuid: "order-2", Domain: "orders", DataType: "OrderPlaced", Data: "{}"},
		{Uuid: "evt-5", TenantUuid: "tenant-A", AggregateUuid: "inv-1", Domain: "inventory", DataType: "StockReserved", Data: "{}"},
		{Uuid: "evt-6", TenantUuid: "tenant-A", AggregateUuid: "inv-1", Domain: "inventory", DataType: "StockDeducted", Data: "{}"},
		{Uuid: "evt-7", TenantUuid: "tenant-B", AggregateUuid: "order-3", Domain: "orders", DataType: "OrderPlaced", Data: "{}"},
		{Uuid: "evt-8", TenantUuid: "tenant-B", AggregateUuid: "order-3", Domain: "orders", DataType: "OrderCancelled", Data: "{}"},
		{Uuid: "evt-9", TenantUuid: "tenant-B", AggregateUuid: "user-1", Domain: "accounts", DataType: "UserRegistered", Data: "{}"},
		{Uuid: "evt-10", TenantUuid: "tenant-B", AggregateUuid: "user-1", Domain: "accounts", DataType: "UserLoggedIn", Data: "{}"},
	}

	// Add events with slight time differences
	for i, evt := range events {
		evt.CreatedAt = time.Now().Add(time.Duration(i-len(events)) * time.Minute).UnixNano()
		store.Add(evt)
	}
}
