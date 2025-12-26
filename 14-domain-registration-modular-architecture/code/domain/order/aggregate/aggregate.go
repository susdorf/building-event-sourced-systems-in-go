package aggregate

import (
	"fmt"

	"domain-registration-demo/pkg"
)

// Order aggregate represents an order in the system.
type Order struct {
	Domain  string
	Name    string
	Uuid    string
	Version int64

	// Order state
	CustomerID string
	Items      []OrderItem
	Status     string
	Total      float64

	// Uncommitted events
	events []interface{}
}

// OrderItem represents an item in an order.
type OrderItem struct {
	ProductID string
	Quantity  int
	Price     float64
}

// NewAggregate creates a new Order aggregate.
func NewAggregate() pkg.Aggregate {
	return &Order{
		Domain: "Order",
		Name:   "Order",
		Status: "new",
		Items:  make([]OrderItem, 0),
		events: make([]interface{}, 0),
	}
}

func (o *Order) GetDomain() string     { return o.Domain }
func (o *Order) GetName() string       { return o.Name }
func (o *Order) GetIdentifier() string { return fmt.Sprintf("%s.%s", o.Domain, o.Name) }
func (o *Order) GetUuid() string       { return o.Uuid }
func (o *Order) GetVersion() int64     { return o.Version }
func (o *Order) GetEvents() []interface{} { return o.events }
func (o *Order) ClearEvents()          { o.events = make([]interface{}, 0) }

// Apply applies an event to update the aggregate state.
func (o *Order) Apply(event interface{}) error {
	switch e := event.(type) {
	case *OrderPlacedEvent:
		o.Uuid = e.OrderID
		o.CustomerID = e.CustomerID
		o.Items = e.Items
		o.Status = "placed"
		o.Total = e.Total
	case *OrderPaidEvent:
		o.Status = "paid"
	case *OrderShippedEvent:
		o.Status = "shipped"
	case *OrderCancelledEvent:
		o.Status = "cancelled"
	default:
		return fmt.Errorf("unknown event type: %T", event)
	}
	o.Version++
	return nil
}

// --- Domain Events ---

type OrderPlacedEvent struct {
	OrderID    string
	CustomerID string
	Items      []OrderItem
	Total      float64
}

type OrderPaidEvent struct {
	OrderID       string
	PaymentMethod string
	Amount        float64
}

type OrderShippedEvent struct {
	OrderID        string
	TrackingNumber string
}

type OrderCancelledEvent struct {
	OrderID string
	Reason  string
	Items   []OrderItem // For inventory release
}

// --- Aggregate Methods (produce events) ---

// PlaceOrder creates a new order.
func (o *Order) PlaceOrder(orderID, customerID string, items []OrderItem) error {
	if len(items) == 0 {
		return fmt.Errorf("order must have at least one item")
	}

	var total float64
	for _, item := range items {
		total += item.Price * float64(item.Quantity)
	}

	event := &OrderPlacedEvent{
		OrderID:    orderID,
		CustomerID: customerID,
		Items:      items,
		Total:      total,
	}

	if err := o.Apply(event); err != nil {
		return err
	}
	o.events = append(o.events, event)
	return nil
}

// PayOrder marks the order as paid.
func (o *Order) PayOrder(paymentMethod string, amount float64) error {
	if o.Status != "placed" {
		return fmt.Errorf("cannot pay order in status: %s", o.Status)
	}

	event := &OrderPaidEvent{
		OrderID:       o.Uuid,
		PaymentMethod: paymentMethod,
		Amount:        amount,
	}

	if err := o.Apply(event); err != nil {
		return err
	}
	o.events = append(o.events, event)
	return nil
}

// ShipOrder marks the order as shipped.
func (o *Order) ShipOrder(trackingNumber string) error {
	if o.Status != "paid" {
		return fmt.Errorf("cannot ship order in status: %s", o.Status)
	}

	event := &OrderShippedEvent{
		OrderID:        o.Uuid,
		TrackingNumber: trackingNumber,
	}

	if err := o.Apply(event); err != nil {
		return err
	}
	o.events = append(o.events, event)
	return nil
}

// CancelOrder cancels the order.
func (o *Order) CancelOrder(reason string) error {
	if o.Status == "shipped" || o.Status == "cancelled" {
		return fmt.Errorf("cannot cancel order in status: %s", o.Status)
	}

	event := &OrderCancelledEvent{
		OrderID: o.Uuid,
		Reason:  reason,
		Items:   o.Items,
	}

	if err := o.Apply(event); err != nil {
		return err
	}
	o.events = append(o.events, event)
	return nil
}
