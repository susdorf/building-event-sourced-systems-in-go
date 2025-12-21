package pkg

import "time"

// Event is the base interface for all domain events
type Event interface {
	EventType() string
}

// OrderPlacedEvent is emitted when an order is placed
type OrderPlacedEvent struct {
	OrderID    string
	CustomerID string
	Items      []OrderItem
	Total      float64
	PlacedAt   time.Time
}

func (e OrderPlacedEvent) EventType() string { return "OrderPlaced" }

// OrderPaidEvent is emitted when an order is paid
type OrderPaidEvent struct {
	OrderID   string
	PaymentID string
	Amount    float64
	PaidAt    time.Time
}

func (e OrderPaidEvent) EventType() string { return "OrderPaid" }

// OrderShippedEvent is emitted when an order is shipped
type OrderShippedEvent struct {
	OrderID    string
	TrackingNo string
	ShippedAt  time.Time
}

func (e OrderShippedEvent) EventType() string { return "OrderShipped" }

// OrderCancelledEvent is emitted when an order is cancelled
type OrderCancelledEvent struct {
	OrderID     string
	Reason      string
	CancelledAt time.Time
}

func (e OrderCancelledEvent) EventType() string { return "OrderCancelled" }
