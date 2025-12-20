package pkg

import (
	"context"
	"fmt"
)

// Commands - express intent (what we want to happen)

type PlaceOrder struct {
	OrderID    string
	CustomerID string
	Items      []OrderItem
}

type CancelOrder struct {
	OrderID string
	Reason  string
}

type ShipOrder struct {
	OrderID    string
	TrackingNo string
}

// OrderCommandHandlers contains all order-related command handlers
type OrderCommandHandlers struct {
	Repository *AggregateRepository
}

func (h *OrderCommandHandlers) HandlePlaceOrder(ctx context.Context, cmd PlaceOrder) error {
	// Validate
	if cmd.OrderID == "" {
		return fmt.Errorf("order ID is required")
	}
	if cmd.CustomerID == "" {
		return fmt.Errorf("customer ID is required")
	}
	if len(cmd.Items) == 0 {
		return fmt.Errorf("at least one item is required")
	}

	// Create new aggregate
	order := NewOrderAggregate(cmd.OrderID)

	// Execute business logic (emits events)
	if err := order.Place(cmd.CustomerID, cmd.Items); err != nil {
		return err
	}

	// Persist
	return h.Repository.Save(ctx, order)
}

func (h *OrderCommandHandlers) HandleCancelOrder(ctx context.Context, cmd CancelOrder) error {
	// Load existing aggregate
	order, err := h.Repository.Load(ctx, cmd.OrderID)
	if err != nil {
		return err
	}

	// Execute business logic
	if err := order.Cancel(cmd.Reason); err != nil {
		return err
	}

	// Persist
	return h.Repository.Save(ctx, order)
}

func (h *OrderCommandHandlers) HandleShipOrder(ctx context.Context, cmd ShipOrder) error {
	// Validate
	if cmd.TrackingNo == "" {
		return fmt.Errorf("tracking number is required")
	}

	// Load existing aggregate
	order, err := h.Repository.Load(ctx, cmd.OrderID)
	if err != nil {
		return err
	}

	// Execute business logic
	if err := order.Ship(cmd.TrackingNo); err != nil {
		return err
	}

	// Persist
	return h.Repository.Save(ctx, order)
}

// SetupCommandHandlers registers all order command handlers
func SetupCommandHandlers(dispatcher *CommandDispatcher, repo *AggregateRepository) {
	handlers := &OrderCommandHandlers{Repository: repo}
	RegisterCommand(dispatcher, handlers.HandlePlaceOrder)
	RegisterCommand(dispatcher, handlers.HandleCancelOrder)
	RegisterCommand(dispatcher, handlers.HandleShipOrder)
}
