package pkg

import (
	"fmt"
	"time"
)

// OrderAggregate encapsulates order business logic
type OrderAggregate struct {
	ID         string
	CustomerID string
	Items      []OrderItem
	Total      float64
	Status     string
	TrackingNo string
	Version    int

	// Uncommitted events waiting to be persisted
	uncommittedEvents []any
}

func NewOrderAggregate(id string) *OrderAggregate {
	return &OrderAggregate{
		ID:                id,
		uncommittedEvents: make([]any, 0),
	}
}

// Business operations that emit events

func (o *OrderAggregate) Place(customerID string, items []OrderItem) error {
	if o.Status != "" {
		return fmt.Errorf("order already exists")
	}

	// Calculate total
	var total float64
	for _, item := range items {
		total += item.Price * float64(item.Quantity)
	}

	// Emit event
	event := OrderPlacedEvent{
		OrderID:    o.ID,
		CustomerID: customerID,
		Items:      items,
		Total:      total,
		PlacedAt:   time.Now(),
	}

	o.apply(event)
	o.uncommittedEvents = append(o.uncommittedEvents, event)

	return nil
}

func (o *OrderAggregate) Ship(trackingNo string) error {
	if o.Status != "placed" {
		return fmt.Errorf("order cannot be shipped: status is %s", o.Status)
	}

	event := OrderShippedEvent{
		OrderID:    o.ID,
		TrackingNo: trackingNo,
		ShippedAt:  time.Now(),
	}

	o.apply(event)
	o.uncommittedEvents = append(o.uncommittedEvents, event)

	return nil
}

func (o *OrderAggregate) Cancel(reason string) error {
	if o.Status != "placed" {
		return fmt.Errorf("order cannot be cancelled: status is %s", o.Status)
	}

	event := OrderCancelledEvent{
		OrderID:     o.ID,
		Reason:      reason,
		CancelledAt: time.Now(),
	}

	o.apply(event)
	o.uncommittedEvents = append(o.uncommittedEvents, event)

	return nil
}

// apply updates aggregate state based on an event
func (o *OrderAggregate) apply(event any) {
	switch e := event.(type) {
	case OrderPlacedEvent:
		o.CustomerID = e.CustomerID
		o.Items = e.Items
		o.Total = e.Total
		o.Status = "placed"
	case OrderShippedEvent:
		o.Status = "shipped"
		o.TrackingNo = e.TrackingNo
	case OrderCancelledEvent:
		o.Status = "cancelled"
	}
	o.Version++
}

// GetUncommittedEvents returns events that haven't been persisted yet
func (o *OrderAggregate) GetUncommittedEvents() []any {
	return o.uncommittedEvents
}

// ClearUncommittedEvents clears the list after persistence
func (o *OrderAggregate) ClearUncommittedEvents() {
	o.uncommittedEvents = nil
}

// LoadFromHistory rebuilds aggregate state from stored events
func (o *OrderAggregate) LoadFromHistory(events []any) {
	for _, event := range events {
		o.apply(event)
	}
}
