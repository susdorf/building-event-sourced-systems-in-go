package pkg

import (
	"errors"
	"fmt"
)

// Event represents a domain event
type Event struct {
	Name string
	Data interface{}
}

// Item represents an order item
type Item struct {
	ItemUid  string
	Sku      string
	Quantity int
	Price    float64
}

// OrderPlacedEvent is emitted when an order is placed
type OrderPlacedEvent struct {
	Items []Item
	Total float64
}

// OrderPaidEvent is emitted when an order is paid
type OrderPaidEvent struct {
	PaymentID string
	Amount    float64
}

// OrderShippedEvent is emitted when an order is shipped
type OrderShippedEvent struct {
	TrackingNo string
}

// Order is an aggregate that enforces business rules for orders
type Order struct {
	AggregateUuid     string
	Status            string
	Items             []Item
	Total             float64
	PaymentID         string
	TrackingNo        string
	uncommittedEvents []Event
}

// NewOrder creates a new Order aggregate
func NewOrder() *Order {
	return &Order{
		Status:            "",
		uncommittedEvents: []Event{},
	}
}

// GetUncommittedEvents returns events that haven't been persisted yet
func (o *Order) GetUncommittedEvents() []Event {
	return o.uncommittedEvents
}

// ClearUncommittedEvents clears the uncommitted events
func (o *Order) ClearUncommittedEvents() {
	o.uncommittedEvents = []Event{}
}

// Place places a new order with the given items
func (o *Order) Place(items []Item) error {
	// Business rule: order must have at least one item
	if len(items) == 0 {
		return errors.New("order must have at least one item")
	}

	// Business rule: cannot place already placed order
	if o.Status != "" {
		return errors.New("order already placed")
	}

	// Business rule: all items must have positive quantity
	for _, item := range items {
		if item.Quantity <= 0 {
			return errors.New("quantity must be positive")
		}
		if item.Price < 0 {
			return errors.New("price cannot be negative")
		}
	}

	// Calculate total
	var total float64
	for _, item := range items {
		total += float64(item.Quantity) * item.Price
	}

	// Apply event
	evt := OrderPlacedEvent{
		Items: items,
		Total: total,
	}
	o.applyOrderPlaced(evt)
	o.uncommittedEvents = append(o.uncommittedEvents, Event{
		Name: "OrderPlacedEvent",
		Data: evt,
	})

	return nil
}

func (o *Order) applyOrderPlaced(evt OrderPlacedEvent) {
	o.Status = "placed"
	o.Items = evt.Items
	o.Total = evt.Total
}

// Pay marks the order as paid
func (o *Order) Pay(paymentID string, amount float64) error {
	// Business rule: must be in placed status
	if o.Status != "placed" {
		return fmt.Errorf("order must be in 'placed' status, currently: '%s'", o.Status)
	}

	// Business rule: payment amount must match total
	if amount != o.Total {
		return fmt.Errorf("payment amount (%.2f) does not match order total (%.2f)", amount, o.Total)
	}

	// Apply event
	evt := OrderPaidEvent{
		PaymentID: paymentID,
		Amount:    amount,
	}
	o.applyOrderPaid(evt)
	o.uncommittedEvents = append(o.uncommittedEvents, Event{
		Name: "OrderPaidEvent",
		Data: evt,
	})

	return nil
}

func (o *Order) applyOrderPaid(evt OrderPaidEvent) {
	o.Status = "paid"
	o.PaymentID = evt.PaymentID
}

// Ship marks the order as shipped
func (o *Order) Ship(trackingNo string) error {
	// Business rule: must be paid before shipping
	if o.Status != "paid" {
		return errors.New("order must be paid before shipping")
	}

	// Business rule: tracking number required
	if trackingNo == "" {
		return errors.New("tracking number is required")
	}

	// Apply event
	evt := OrderShippedEvent{
		TrackingNo: trackingNo,
	}
	o.applyOrderShipped(evt)
	o.uncommittedEvents = append(o.uncommittedEvents, Event{
		Name: "OrderShippedEvent",
		Data: evt,
	})

	return nil
}

func (o *Order) applyOrderShipped(evt OrderShippedEvent) {
	o.Status = "shipped"
	o.TrackingNo = evt.TrackingNo
}

// ApplyEvent applies a historical event to reconstruct state
func (o *Order) ApplyEvent(evt Event) {
	switch e := evt.Data.(type) {
	case OrderPlacedEvent:
		o.applyOrderPlaced(e)
	case OrderPaidEvent:
		o.applyOrderPaid(e)
	case OrderShippedEvent:
		o.applyOrderShipped(e)
	}
}
