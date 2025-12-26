package reactor

import (
	"context"
	"fmt"

	"domain-registration-demo/domain/order/aggregate"
	"domain-registration-demo/pkg"
)

// OrderEventsReactor handles Order domain events to update inventory.
// This demonstrates cross-domain event handling.
type OrderEventsReactor struct {
	Domain string
	Name   string

	// Track reservations made (simplified for demo)
	reservations map[string][]reservation // orderID -> []reservation
}

type reservation struct {
	productID string
	quantity  int
}

// NewOrderEventsReactor creates a new reactor for Order events.
func NewOrderEventsReactor() *OrderEventsReactor {
	return &OrderEventsReactor{
		Domain:       "Inventory",
		Name:         "OrderEventsReactor",
		reservations: make(map[string][]reservation),
	}
}

func (r *OrderEventsReactor) GetDomain() string     { return r.Domain }
func (r *OrderEventsReactor) GetName() string       { return r.Name }
func (r *OrderEventsReactor) GetIdentifier() string { return fmt.Sprintf("%s.%s", r.Domain, r.Name) }

// GetEvents returns the events this reactor subscribes to.
// Note: These are from the ORDER domain, not Inventory!
func (r *OrderEventsReactor) GetEvents() []string {
	return []string{
		"Order.OrderPlaced",     // Reserve inventory when order is placed
		"Order.OrderCancelled",  // Release inventory when order is cancelled
	}
}

// Handle processes Order events and updates inventory accordingly.
func (r *OrderEventsReactor) Handle(ctx context.Context, evt pkg.Event) error {
	switch evt.EventType {
	case "OrderPlaced":
		return r.handleOrderPlaced(ctx, evt)
	case "OrderCancelled":
		return r.handleOrderCancelled(ctx, evt)
	}
	return nil
}

func (r *OrderEventsReactor) handleOrderPlaced(ctx context.Context, evt pkg.Event) error {
	data := evt.Data.(*aggregate.OrderPlacedEvent)

	fmt.Printf("      [Inventory:Reactor] Order %s placed - reserving inventory for %d items\n",
		data.OrderID, len(data.Items))

	// In a real implementation, this would:
	// 1. Load the Inventory aggregate for each product
	// 2. Call inventory.ReserveStock(orderID, quantity)
	// 3. Save the aggregate

	// For demo, we track reservations locally
	var reservations []reservation
	for _, item := range data.Items {
		reservations = append(reservations, reservation{
			productID: item.ProductID,
			quantity:  item.Quantity,
		})
		fmt.Printf("        -> Reserved %d of product %s\n", item.Quantity, item.ProductID)
	}
	r.reservations[data.OrderID] = reservations

	return nil
}

func (r *OrderEventsReactor) handleOrderCancelled(ctx context.Context, evt pkg.Event) error {
	data := evt.Data.(*aggregate.OrderCancelledEvent)

	fmt.Printf("      [Inventory:Reactor] Order %s cancelled - releasing inventory\n", data.OrderID)

	// In a real implementation, this would:
	// 1. Load the Inventory aggregate for each product
	// 2. Call inventory.ReleaseReservation(orderID, reason)
	// 3. Save the aggregate

	if reservations, ok := r.reservations[data.OrderID]; ok {
		for _, res := range reservations {
			fmt.Printf("        -> Released %d of product %s (reason: %s)\n",
				res.quantity, res.productID, data.Reason)
		}
		delete(r.reservations, data.OrderID)
	}

	return nil
}

// GetReservationCount returns the number of active reservations (for demo).
func (r *OrderEventsReactor) GetReservationCount() int {
	return len(r.reservations)
}
