package main

import (
	"context"
	"fmt"

	"aggregates-demo/pkg"
)

func main() {
	fmt.Println("=== Aggregates Demo ===")
	fmt.Println()

	// Setup infrastructure
	eventStore := pkg.NewEventStore()
	repository := pkg.NewOrderRepository(eventStore)
	ctx := context.Background()

	// --- 1. Create and place an order ---
	fmt.Println("1. Creating and placing an order...")
	order := pkg.NewOrder("order-123")

	err := order.Place("customer-456", []pkg.OrderItem{
		{ProductID: "prod-1", Name: "Widget", Quantity: 2, Price: 29.99},
		{ProductID: "prod-2", Name: "Gadget", Quantity: 1, Price: 49.99},
	})
	if err != nil {
		fmt.Printf("   Error: %v\n", err)
		return
	}
	fmt.Printf("   -> Order placed (in-memory), status: %s, total: $%.2f\n", order.Status, order.Total)
	fmt.Printf("   -> Uncommitted events: %d\n", len(order.GetUncommittedEvents()))

	// Save persists the events
	if err := repository.Save(ctx, order); err != nil {
		fmt.Printf("   Error saving: %v\n", err)
		return
	}
	fmt.Println("   -> Events persisted to store")

	// --- 2. Demonstrate aggregate is short-lived: load fresh ---
	fmt.Println()
	fmt.Println("2. Loading order from event store (simulating new request)...")
	order, err = repository.Load(ctx, "order-123")
	if err != nil {
		fmt.Printf("   Error: %v\n", err)
		return
	}
	fmt.Printf("   -> Order loaded, status: %s, version: %d\n", order.Status, order.Version)

	// --- 3. Pay the order ---
	fmt.Println()
	fmt.Println("3. Processing payment...")
	err = order.Pay("payment-789", 109.97)
	if err != nil {
		fmt.Printf("   Error: %v\n", err)
		return
	}
	fmt.Printf("   -> Payment processed, status: %s\n", order.Status)

	if err := repository.Save(ctx, order); err != nil {
		fmt.Printf("   Error saving: %v\n", err)
		return
	}
	fmt.Println("   -> Events persisted")

	// --- 4. Ship the order ---
	fmt.Println()
	fmt.Println("4. Shipping the order...")
	order, _ = repository.Load(ctx, "order-123")
	err = order.Ship("TRK-12345")
	if err != nil {
		fmt.Printf("   Error: %v\n", err)
		return
	}
	fmt.Printf("   -> Order shipped, status: %s, tracking: %s\n", order.Status, order.TrackingNo)
	repository.Save(ctx, order)

	// --- 5. Try to cancel shipped order (should fail) ---
	fmt.Println()
	fmt.Println("5. Trying to cancel shipped order (business rule validation)...")
	order, _ = repository.Load(ctx, "order-123")
	err = order.Cancel("Changed my mind")
	if err != nil {
		fmt.Printf("   -> Expected error: %v\n", err)
	}

	// --- 6. Show state machine in action ---
	fmt.Println()
	fmt.Println("6. Creating another order to show state machine...")
	order2 := pkg.NewOrder("order-456")
	order2.Place("customer-789", []pkg.OrderItem{
		{ProductID: "prod-3", Name: "Thingamajig", Quantity: 1, Price: 19.99},
	})
	repository.Save(ctx, order2)

	fmt.Println("   -> Trying to ship before payment...")
	order2, _ = repository.Load(ctx, "order-456")
	err = order2.Ship("TRK-00000")
	if err != nil {
		fmt.Printf("   -> Expected error: %v\n", err)
	}

	fmt.Println("   -> Cancelling unpaid order (allowed)...")
	err = order2.Cancel("No longer needed")
	if err != nil {
		fmt.Printf("   Error: %v\n", err)
	} else {
		fmt.Printf("   -> Order cancelled, status: %s\n", order2.Status)
	}
	repository.Save(ctx, order2)

	// --- 7. Show event store contents ---
	fmt.Println()
	fmt.Println("=== Event Store Contents ===")
	allEvents := eventStore.GetAllEvents()
	for aggregateID, events := range allEvents {
		fmt.Printf("\n%s:\n", aggregateID)
		for i, evt := range events {
			fmt.Printf("   [%d] %s\n", i, evt.EventType())
		}
	}

	// --- 8. Demonstrate testability ---
	fmt.Println()
	fmt.Println("=== Testing Business Logic (no infrastructure needed) ===")
	runTests()

	fmt.Println()
	fmt.Println("=== Demo Complete ===")
}

// runTests demonstrates that aggregates can be tested without any infrastructure
func runTests() {
	tests := []struct {
		name    string
		test    func() error
		wantErr error
	}{
		{
			name: "Cannot ship unpaid order",
			test: func() error {
				order := pkg.NewOrder("test-1")
				order.Place("customer", []pkg.OrderItem{{ProductID: "p1", Quantity: 1, Price: 10}})
				return order.Ship("TRK-X")
			},
			wantErr: pkg.ErrOrderNotPaid,
		},
		{
			name: "Cannot cancel shipped order",
			test: func() error {
				order := pkg.NewOrder("test-2")
				order.Place("customer", []pkg.OrderItem{{ProductID: "p1", Quantity: 1, Price: 10}})
				order.Pay("pay-1", 10)
				order.Ship("TRK-X")
				return order.Cancel("reason")
			},
			wantErr: pkg.ErrCannotCancelShipped,
		},
		{
			name: "Cannot place empty order",
			test: func() error {
				order := pkg.NewOrder("test-3")
				return order.Place("customer", []pkg.OrderItem{})
			},
			wantErr: pkg.ErrEmptyOrder,
		},
	}

	for _, tt := range tests {
		err := tt.test()
		if err == tt.wantErr {
			fmt.Printf("   PASS: %s\n", tt.name)
		} else {
			fmt.Printf("   FAIL: %s (got %v, want %v)\n", tt.name, err, tt.wantErr)
		}
	}
}
