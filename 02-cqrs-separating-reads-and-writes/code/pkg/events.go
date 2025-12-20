package pkg

import "time"

// Domain Events - describe what happened

type OrderPlacedEvent struct {
	OrderID    string
	CustomerID string
	Items      []OrderItem
	Total      float64
	PlacedAt   time.Time
}

type OrderShippedEvent struct {
	OrderID    string
	TrackingNo string
	ShippedAt  time.Time
}

type OrderCancelledEvent struct {
	OrderID     string
	Reason      string
	CancelledAt time.Time
}

// OrderItem represents an item in an order
type OrderItem struct {
	ProductID string
	Name      string
	Quantity  int
	Price     float64
}
