package main

import (
	"fmt"
	"strings"
	"time"

	"testing-event-sourced-demo/pkg"
)

func main() {
	fmt.Println("=== Testing Event-Sourced Systems Demo ===")
	fmt.Println()

	// --- 1. Basic Aggregate Test ---
	fmt.Println("1. Basic Aggregate Test (Place Order):")
	demoBasicAggregateTest()

	// --- 2. Business Rule Tests ---
	fmt.Println()
	fmt.Println("2. Business Rule Tests:")
	demoBusinessRuleTests()

	// --- 3. Given-When-Then Pattern ---
	fmt.Println()
	fmt.Println("3. Given-When-Then Pattern:")
	demoGivenWhenThen()

	// --- 4. Table-Driven Tests (State Transitions) ---
	fmt.Println()
	fmt.Println("4. Table-Driven Tests (State Transitions):")
	demoTableDrivenTests()

	// --- 5. Event Replay Test ---
	fmt.Println()
	fmt.Println("5. Event Replay Test (State Reconstruction):")
	demoEventReplay()

	// --- 6. Test Fixtures Demo ---
	fmt.Println()
	fmt.Println("6. Using Test Fixtures:")
	demoTestFixtures()

	// --- 7. Simulated Benchmark ---
	fmt.Println()
	fmt.Println("7. Simulated Benchmark (Command Processing):")
	demoBenchmark()

	fmt.Println()
	fmt.Println("=== Demo Complete ===")
}

func demoBasicAggregateTest() {
	// Arrange
	order := pkg.NewOrder()
	order.AggregateUuid = "order-123"

	items := pkg.SampleItems()

	// Act
	err := order.Place(items)

	// Assert
	if err != nil {
		fmt.Printf("   FAIL: Unexpected error: %v\n", err)
		return
	}

	events := order.GetUncommittedEvents()
	if len(events) != 1 {
		fmt.Printf("   FAIL: Expected 1 event, got %d\n", len(events))
		return
	}

	if events[0].Name != "OrderPlacedEvent" {
		fmt.Printf("   FAIL: Expected OrderPlacedEvent, got %s\n", events[0].Name)
		return
	}

	evt := events[0].Data.(pkg.OrderPlacedEvent)
	fmt.Printf("   PASS: Order placed with %d items, total: %.2f\n", len(evt.Items), evt.Total)
	fmt.Printf("         Event emitted: %s\n", events[0].Name)
}

func demoBusinessRuleTests() {
	testCases := pkg.OrderBusinessRuleTests()

	passed := 0
	failed := 0

	for _, tc := range testCases {
		order := pkg.NewOrder()
		err := order.Place(tc.Items)

		success := false
		if tc.ExpectError {
			if err != nil && strings.Contains(err.Error(), tc.ErrorMsg) {
				success = true
			}
		} else {
			if err == nil {
				success = true
			}
		}

		if success {
			passed++
			fmt.Printf("   PASS: %s\n", tc.Name)
		} else {
			failed++
			fmt.Printf("   FAIL: %s (expected error=%v, got: %v)\n", tc.Name, tc.ExpectError, err)
		}
	}

	fmt.Printf("   Summary: %d passed, %d failed\n", passed, failed)
}

func demoGivenWhenThen() {
	// Given: A paid order
	fmt.Println("   Given: A paid order")
	order := pkg.GivenPaidOrder("order-123")
	fmt.Printf("         Status: %s, Total: %.2f\n", order.Status, order.Total)

	// When: Shipping the order
	fmt.Println("   When: Shipping the order")
	err := order.Ship("TRACK-456")

	// Then: Order should emit shipped event
	fmt.Println("   Then: Order should emit shipped event")
	if err != nil {
		fmt.Printf("   FAIL: Unexpected error: %v\n", err)
		return
	}

	events := order.GetUncommittedEvents()
	if len(events) != 1 || events[0].Name != "OrderShippedEvent" {
		fmt.Printf("   FAIL: Expected OrderShippedEvent\n")
		return
	}

	fmt.Printf("   PASS: Status=%s, TrackingNo=%s\n", order.Status, order.TrackingNo)
}

func demoTableDrivenTests() {
	testCases := pkg.OrderTransitionTests()

	passed := 0
	failed := 0

	for _, tc := range testCases {
		order := tc.InitialState(fmt.Sprintf("order-%d", passed+failed))
		order.ClearUncommittedEvents()

		err := tc.Action(order)

		success := false
		if tc.ExpectError {
			if err != nil {
				success = true
			}
		} else {
			if err == nil {
				events := order.GetUncommittedEvents()
				if len(events) > 0 && events[0].Name == tc.ExpectedEvent {
					success = true
				}
			}
		}

		if success {
			passed++
			status := "error expected"
			if !tc.ExpectError {
				status = fmt.Sprintf("emitted %s", tc.ExpectedEvent)
			}
			fmt.Printf("   PASS: %-35s (%s)\n", tc.Name, status)
		} else {
			failed++
			fmt.Printf("   FAIL: %s\n", tc.Name)
		}
	}

	fmt.Printf("   Summary: %d passed, %d failed\n", passed, failed)
}

func demoEventReplay() {
	// Create events representing order lifecycle
	events := []pkg.Event{
		{
			Name: "OrderPlacedEvent",
			Data: pkg.OrderPlacedEvent{
				Items: []pkg.Item{{Quantity: 2, Price: 50.0}},
				Total: 100.0,
			},
		},
		{
			Name: "OrderPaidEvent",
			Data: pkg.OrderPaidEvent{
				PaymentID: "pay-123",
				Amount:    100.0,
			},
		},
		{
			Name: "OrderShippedEvent",
			Data: pkg.OrderShippedEvent{
				TrackingNo: "TRACK-456",
			},
		},
	}

	// Replay events on a fresh aggregate
	order := pkg.NewOrder()
	fmt.Printf("   Initial state: Status='%s'\n", order.Status)

	for i, evt := range events {
		order.ApplyEvent(evt)
		fmt.Printf("   After event %d (%s): Status='%s'\n", i+1, evt.Name, order.Status)
	}

	// Verify final state
	if order.Status == "shipped" && order.Total == 100.0 && order.TrackingNo == "TRACK-456" {
		fmt.Println("   PASS: Final state matches expected state after replay")
	} else {
		fmt.Println("   FAIL: State mismatch after replay")
	}
}

func demoTestFixtures() {
	fmt.Printf("   Tenant fixtures:\n")
	fmt.Printf("     TENANT_000_UUID: %s\n", pkg.TENANT_000_UUID)
	fmt.Printf("     TENANT_001_UUID: %s\n", pkg.TENANT_001_UUID)

	fmt.Printf("   Order fixtures:\n")
	fmt.Printf("     ORDER_000_UUID: %s\n", pkg.ORDER_000_UUID)

	fmt.Printf("   State helper functions:\n")

	newOrder := pkg.GivenNewOrder("test-1")
	fmt.Printf("     GivenNewOrder():    Status='%s'\n", newOrder.Status)

	placedOrder := pkg.GivenPlacedOrder("test-2")
	fmt.Printf("     GivenPlacedOrder(): Status='%s', Total=%.2f\n", placedOrder.Status, placedOrder.Total)

	paidOrder := pkg.GivenPaidOrder("test-3")
	fmt.Printf("     GivenPaidOrder():   Status='%s', PaymentID='%s'\n", paidOrder.Status, paidOrder.PaymentID)

	shippedOrder := pkg.GivenShippedOrder("test-4")
	fmt.Printf("     GivenShippedOrder(): Status='%s', TrackingNo='%s'\n", shippedOrder.Status, shippedOrder.TrackingNo)
}

func demoBenchmark() {
	// Simulate benchmark by running operations many times
	iterations := 100000

	// Benchmark 1: Command without repository (just event emission)
	start := time.Now()
	for i := 0; i < iterations; i++ {
		order := pkg.NewOrder()
		order.Place(pkg.SingleItem())
	}
	duration := time.Since(start)
	opsPerSec := float64(iterations) / duration.Seconds()
	fmt.Printf("   Place order (no repository): %d iterations in %v\n", iterations, duration)
	fmt.Printf("     -> %.0f ops/sec, %.2f ns/op\n", opsPerSec, float64(duration.Nanoseconds())/float64(iterations))

	// Benchmark 2: Full lifecycle
	start = time.Now()
	for i := 0; i < iterations; i++ {
		order := pkg.NewOrder()
		order.Place(pkg.SingleItem())
		order.Pay("pay-123", 100.0)
		order.Ship("TRACK-123")
	}
	duration = time.Since(start)
	opsPerSec = float64(iterations) / duration.Seconds()
	fmt.Printf("   Full order lifecycle: %d iterations in %v\n", iterations, duration)
	fmt.Printf("     -> %.0f ops/sec, %.2f ns/op\n", opsPerSec, float64(duration.Nanoseconds())/float64(iterations))

	// Benchmark 3: Event replay
	events := []pkg.Event{
		{Name: "OrderPlacedEvent", Data: pkg.OrderPlacedEvent{Items: pkg.SingleItem(), Total: 100.0}},
		{Name: "OrderPaidEvent", Data: pkg.OrderPaidEvent{PaymentID: "pay-123", Amount: 100.0}},
		{Name: "OrderShippedEvent", Data: pkg.OrderShippedEvent{TrackingNo: "TRACK-456"}},
	}
	start = time.Now()
	for i := 0; i < iterations; i++ {
		order := pkg.NewOrder()
		for _, evt := range events {
			order.ApplyEvent(evt)
		}
	}
	duration = time.Since(start)
	opsPerSec = float64(iterations) / duration.Seconds()
	fmt.Printf("   Event replay (3 events): %d iterations in %v\n", iterations, duration)
	fmt.Printf("     -> %.0f ops/sec, %.2f ns/op\n", opsPerSec, float64(duration.Nanoseconds())/float64(iterations))
}
