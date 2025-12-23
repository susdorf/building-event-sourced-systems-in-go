package main

import (
	"fmt"

	"generics-repository-demo/pkg"
)

func main() {
	fmt.Println("=== Generic Repository Demo ===")
	fmt.Println()

	// Create shared event store
	eventStore := pkg.NewEventStore()

	// --- 1. Create typed repositories ---
	fmt.Println("1. Creating typed repositories:")
	fmt.Println("   orderRepo := NewAggregateRepository[*Order](eventStore, NewOrder)")
	fmt.Println("   customerRepo := NewAggregateRepository[*Customer](eventStore, NewCustomer)")

	orderRepo := pkg.NewAggregateRepository[*pkg.Order](eventStore, pkg.NewOrder)
	customerRepo := pkg.NewAggregateRepository[*pkg.Customer](eventStore, pkg.NewCustomer)

	fmt.Println("   -> Both repositories share the same event store")
	fmt.Println("   -> Each is typed to its specific aggregate")
	fmt.Println()

	// --- 2. Create and save an order ---
	fmt.Println("2. Create and save an Order aggregate:")

	order := pkg.NewOrder()
	order.Create("order-001", "customer-123")
	_ = order.AddItem("PROD-A", "Widget", 2, 29.99)
	_ = order.AddItem("PROD-B", "Gadget", 1, 49.99)
	_ = order.Confirm()

	fmt.Printf("   -> Order created with ID: %s\n", order.GetID())
	fmt.Printf("   -> Customer: %s\n", order.CustomerID)
	fmt.Printf("   -> Items: %d\n", len(order.Items))
	fmt.Printf("   -> Total: $%.2f\n", order.Total)
	fmt.Printf("   -> Status: %s\n", order.Status)
	fmt.Printf("   -> Uncommitted events: %d\n", len(order.GetUncommittedEvents()))

	_ = orderRepo.Save(order)
	fmt.Printf("   -> Saved! Events in store: %d\n", eventStore.Count())
	fmt.Println()

	// --- 3. Load the order back - returns *Order directly! ---
	fmt.Println("3. Load Order - returns *Order, not interface{}:")

	loadedOrder, _ := orderRepo.GetAggregate("order-001")

	fmt.Println("   // No type assertion needed!")
	fmt.Println("   order, _ := orderRepo.GetAggregate(\"order-001\")")
	fmt.Println("   order.Ship(\"TRACK-123\")  // Direct method call")
	fmt.Println()

	if loadedOrder != nil {
		fmt.Printf("   -> Loaded order: %s\n", loadedOrder.GetID())
		fmt.Printf("   -> Customer: %s (directly accessible, no assertion)\n", loadedOrder.CustomerID)
		fmt.Printf("   -> Status: %s\n", loadedOrder.Status)
		fmt.Printf("   -> Total: $%.2f\n", loadedOrder.Total)

		// Call domain method directly - this is the type safety benefit!
		_ = loadedOrder.Ship("TRACK-123")
		_ = orderRepo.Save(loadedOrder)
		fmt.Printf("   -> Shipped! New status: %s\n", loadedOrder.Status)
	}
	fmt.Println()

	// --- 4. Demonstrate type safety ---
	fmt.Println("4. Type safety - compile-time checks:")
	fmt.Println("   // This would NOT compile:")
	fmt.Println("   // order, _ := orderRepo.GetAggregate(\"order-001\")")
	fmt.Println("   // order.UpgradeToVIP()  // ERROR: *Order has no method UpgradeToVIP")
	fmt.Println()
	fmt.Println("   // Customer methods only work with customerRepo:")
	fmt.Println("   // customer, _ := customerRepo.GetAggregate(\"cust-001\")")
	fmt.Println("   // customer.UpgradeToVIP()  // OK: *Customer has UpgradeToVIP")
	fmt.Println()

	// --- 5. Create and save a Customer ---
	fmt.Println("5. Work with Customer aggregate (different type, same pattern):")

	customer := pkg.NewCustomer()
	customer.Register("cust-001", "alice@example.com", "Alice Smith")
	customer.LinkOrder("order-001")
	customer.UpgradeToVIP()

	_ = customerRepo.Save(customer)

	fmt.Printf("   -> Customer: %s\n", customer.Name)
	fmt.Printf("   -> Email: %s\n", customer.Email)
	fmt.Printf("   -> VIP: %v\n", customer.VIP)
	fmt.Printf("   -> Linked orders: %v\n", customer.OrderIDs)
	fmt.Println()

	// --- 6. Load customer back ---
	fmt.Println("6. Load Customer - also returns concrete type:")

	loadedCustomer, _ := customerRepo.GetAggregate("cust-001")
	if loadedCustomer != nil {
		fmt.Printf("   -> Name: %s (direct access)\n", loadedCustomer.Name)
		fmt.Printf("   -> VIP: %v (direct access)\n", loadedCustomer.VIP)
		fmt.Printf("   -> Orders: %v (direct access)\n", loadedCustomer.OrderIDs)
	}
	fmt.Println()

	// --- 7. Temporal query - load at specific version ---
	fmt.Println("7. Temporal query - load order at version 2 (before confirmation):")

	orderV2, _ := orderRepo.GetAggregateAtVersion("order-001", 2)
	if orderV2 != nil {
		fmt.Printf("   -> Order at v2: status=%s, items=%d\n", orderV2.Status, len(orderV2.Items))
		fmt.Println("   -> (Version 3 was the confirmation)")
	}
	fmt.Println()

	// --- 8. Domain mismatch protection ---
	fmt.Println("8. Domain mismatch protection:")
	fmt.Println("   // Trying to load an Order ID with customerRepo would fail:")
	fmt.Println("   // customer, err := customerRepo.GetAggregate(\"order-001\")")
	fmt.Println("   // -> Error: domain mismatch: expected Customer, got Order")

	_, err := customerRepo.GetAggregate("order-001")
	if err != nil {
		fmt.Printf("   -> Actual error: %v\n", err)
	}
	fmt.Println()

	// --- 9. The old way (contrast) ---
	fmt.Println("9. Contrast - the old (non-generic) way:")
	fmt.Println("   // agg, _ := nonGenericRepo.Load(\"order-001\")")
	fmt.Println("   // order, ok := agg.(*Order)  // Manual type assertion!")
	fmt.Println("   // if !ok {")
	fmt.Println("   //     return errors.New(\"expected Order\")")
	fmt.Println("   // }")
	fmt.Println("   // order.Ship(\"TRACK-123\")")
	fmt.Println()
	fmt.Println("   With generics, the type assertion happens at compile time,")
	fmt.Println("   not runtime. The compiler ensures type safety.")
	fmt.Println()

	// --- 10. Summary ---
	fmt.Println("10. Summary of benefits:")
	fmt.Println("    - No type assertions needed when loading aggregates")
	fmt.Println("    - Full IDE autocomplete for aggregate methods")
	fmt.Println("    - Compile-time type checking prevents mixing aggregate types")
	fmt.Println("    - Same repository pattern works for any aggregate")
	fmt.Println("    - Factory function (NewAggregateFunc) ensures proper initialization")
	fmt.Println()

	fmt.Printf("Total events in store: %d\n", eventStore.Count())
	fmt.Println()
	fmt.Println("=== Demo Complete ===")
}
