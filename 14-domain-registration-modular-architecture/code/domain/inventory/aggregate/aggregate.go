package aggregate

import (
	"fmt"

	"domain-registration-demo/pkg"
)

// Inventory aggregate represents inventory for a product.
type Inventory struct {
	Domain  string
	Name    string
	Uuid    string
	Version int64

	// Inventory state
	ProductID    string
	Available    int
	Reserved     int
	Reservations map[string]int // orderID -> quantity

	events []interface{}
}

// NewAggregate creates a new Inventory aggregate.
func NewAggregate() pkg.Aggregate {
	return &Inventory{
		Domain:       "Inventory",
		Name:         "Inventory",
		Reservations: make(map[string]int),
		events:       make([]interface{}, 0),
	}
}

func (i *Inventory) GetDomain() string        { return i.Domain }
func (i *Inventory) GetName() string          { return i.Name }
func (i *Inventory) GetIdentifier() string    { return fmt.Sprintf("%s.%s", i.Domain, i.Name) }
func (i *Inventory) GetUuid() string          { return i.Uuid }
func (i *Inventory) GetVersion() int64        { return i.Version }
func (i *Inventory) GetEvents() []interface{} { return i.events }
func (i *Inventory) ClearEvents()             { i.events = make([]interface{}, 0) }

// Apply applies an event to update the aggregate state.
func (i *Inventory) Apply(event interface{}) error {
	switch e := event.(type) {
	case *StockAddedEvent:
		i.Available += e.Quantity
	case *StockReservedEvent:
		i.Available -= e.Quantity
		i.Reserved += e.Quantity
		i.Reservations[e.OrderID] = e.Quantity
	case *ReservationReleasedEvent:
		if qty, ok := i.Reservations[e.OrderID]; ok {
			i.Reserved -= qty
			i.Available += qty
			delete(i.Reservations, e.OrderID)
		}
	case *StockDeductedEvent:
		if qty, ok := i.Reservations[e.OrderID]; ok {
			i.Reserved -= qty
			delete(i.Reservations, e.OrderID)
		}
	default:
		return fmt.Errorf("unknown event type: %T", event)
	}
	i.Version++
	return nil
}

// --- Domain Events ---

type StockAddedEvent struct {
	ProductID string
	Quantity  int
}

type StockReservedEvent struct {
	ProductID string
	OrderID   string
	Quantity  int
}

type ReservationReleasedEvent struct {
	ProductID string
	OrderID   string
	Reason    string
}

type StockDeductedEvent struct {
	ProductID string
	OrderID   string
	Quantity  int
}

// --- Aggregate Methods ---

// AddStock adds stock to inventory.
func (i *Inventory) AddStock(productID string, quantity int) error {
	if quantity <= 0 {
		return fmt.Errorf("quantity must be positive")
	}

	i.ProductID = productID
	event := &StockAddedEvent{
		ProductID: productID,
		Quantity:  quantity,
	}

	if err := i.Apply(event); err != nil {
		return err
	}
	i.events = append(i.events, event)
	return nil
}

// ReserveStock reserves stock for an order.
func (i *Inventory) ReserveStock(orderID string, quantity int) error {
	if quantity > i.Available {
		return fmt.Errorf("insufficient stock: available %d, requested %d", i.Available, quantity)
	}

	event := &StockReservedEvent{
		ProductID: i.ProductID,
		OrderID:   orderID,
		Quantity:  quantity,
	}

	if err := i.Apply(event); err != nil {
		return err
	}
	i.events = append(i.events, event)
	return nil
}

// ReleaseReservation releases a reservation (e.g., when order is cancelled).
func (i *Inventory) ReleaseReservation(orderID string, reason string) error {
	if _, ok := i.Reservations[orderID]; !ok {
		return fmt.Errorf("no reservation found for order: %s", orderID)
	}

	event := &ReservationReleasedEvent{
		ProductID: i.ProductID,
		OrderID:   orderID,
		Reason:    reason,
	}

	if err := i.Apply(event); err != nil {
		return err
	}
	i.events = append(i.events, event)
	return nil
}
