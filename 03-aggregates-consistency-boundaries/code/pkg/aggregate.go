package pkg

import (
	"errors"
	"time"
)

// Business rule errors
var (
	ErrOrderAlreadyPlaced  = errors.New("order already placed")
	ErrEmptyOrder          = errors.New("order must have at least one item")
	ErrInvalidQuantity     = errors.New("quantity must be positive")
	ErrOrderNotPlaced      = errors.New("order must be placed before payment")
	ErrInsufficientPayment = errors.New("payment amount insufficient")
	ErrOrderNotPaid        = errors.New("order must be paid before shipping")
	ErrOrderAlreadyShipped = errors.New("order already shipped")
	ErrCannotCancelShipped = errors.New("cannot cancel shipped order")
)

// OrderItem represents an item in an order
type OrderItem struct {
	ProductID string
	Name      string
	Quantity  int
	Price     float64
}

// Order is the aggregate root
type Order struct {
	// Identity
	ID string

	// State
	CustomerID string
	Status     string
	Items      []OrderItem
	Total      float64
	TrackingNo string

	// Version for optimistic concurrency
	Version int64

	// Uncommitted events (changes not yet persisted)
	changes []Event
}

// NewOrder creates a new Order aggregate
func NewOrder(id string) *Order {
	return &Order{
		ID:      id,
		Status:  "new",
		Items:   make([]OrderItem, 0),
		changes: make([]Event, 0),
	}
}

// --- Intentions (Business Logic) ---

// Place validates and places a new order
func (o *Order) Place(customerID string, items []OrderItem) error {
	if o.Status != "new" {
		return ErrOrderAlreadyPlaced
	}
	if len(items) == 0 {
		return ErrEmptyOrder
	}

	var total float64
	for _, item := range items {
		if item.Quantity <= 0 {
			return ErrInvalidQuantity
		}
		total += float64(item.Quantity) * item.Price
	}

	event := &OrderPlacedEvent{
		OrderID:    o.ID,
		CustomerID: customerID,
		Items:      items,
		Total:      total,
		PlacedAt:   time.Now(),
	}

	o.apply(event, true)
	return nil
}

// Pay marks the order as paid
func (o *Order) Pay(paymentID string, amount float64) error {
	if o.Status != "placed" {
		return ErrOrderNotPlaced
	}
	if amount < o.Total {
		return ErrInsufficientPayment
	}

	event := &OrderPaidEvent{
		OrderID:   o.ID,
		PaymentID: paymentID,
		Amount:    amount,
		PaidAt:    time.Now(),
	}

	o.apply(event, true)
	return nil
}

// Ship marks the order as shipped
func (o *Order) Ship(trackingNo string) error {
	if o.Status == "shipped" {
		return ErrOrderAlreadyShipped
	}
	if o.Status != "paid" {
		return ErrOrderNotPaid
	}

	event := &OrderShippedEvent{
		OrderID:    o.ID,
		TrackingNo: trackingNo,
		ShippedAt:  time.Now(),
	}

	o.apply(event, true)
	return nil
}

// Cancel cancels the order
func (o *Order) Cancel(reason string) error {
	if o.Status == "shipped" {
		return ErrCannotCancelShipped
	}

	event := &OrderCancelledEvent{
		OrderID:     o.ID,
		Reason:      reason,
		CancelledAt: time.Now(),
	}

	o.apply(event, true)
	return nil
}

// --- Event Handlers ---

func (o *Order) apply(event Event, isNew bool) {
	switch e := event.(type) {
	case *OrderPlacedEvent:
		o.onOrderPlaced(e)
	case *OrderPaidEvent:
		o.onOrderPaid(e)
	case *OrderShippedEvent:
		o.onOrderShipped(e)
	case *OrderCancelledEvent:
		o.onOrderCancelled(e)
	}

	o.Version++

	if isNew {
		o.changes = append(o.changes, event)
	}
}

func (o *Order) onOrderPlaced(e *OrderPlacedEvent) {
	o.CustomerID = e.CustomerID
	o.Items = e.Items
	o.Total = e.Total
	o.Status = "placed"
}

func (o *Order) onOrderPaid(e *OrderPaidEvent) {
	o.Status = "paid"
}

func (o *Order) onOrderShipped(e *OrderShippedEvent) {
	o.Status = "shipped"
	o.TrackingNo = e.TrackingNo
}

func (o *Order) onOrderCancelled(e *OrderCancelledEvent) {
	o.Status = "cancelled"
}

// --- Event Management ---

// GetUncommittedEvents returns events that haven't been persisted
func (o *Order) GetUncommittedEvents() []Event {
	return o.changes
}

// ClearUncommittedEvents clears the list after persistence
func (o *Order) ClearUncommittedEvents() {
	o.changes = make([]Event, 0)
}

// LoadFromHistory rebuilds the aggregate from events
func (o *Order) LoadFromHistory(events []Event) {
	for _, event := range events {
		o.apply(event, false)
	}
}
