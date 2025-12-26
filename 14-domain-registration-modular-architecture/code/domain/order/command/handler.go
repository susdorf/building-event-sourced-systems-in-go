package command

import (
	"context"
	"fmt"

	"domain-registration-demo/domain/order/aggregate"
	"domain-registration-demo/pkg"
)

// PlaceOrderCommand is the command to place a new order.
type PlaceOrderCommand struct {
	OrderID    string
	CustomerID string
	Items      []aggregate.OrderItem
}

// PayOrderCommand is the command to pay for an order.
type PayOrderCommand struct {
	OrderID       string
	PaymentMethod string
	Amount        float64
}

// CancelOrderCommand is the command to cancel an order.
type CancelOrderCommand struct {
	OrderID string
	Reason  string
}

// OrderCommandHandler handles all Order commands.
type OrderCommandHandler struct {
	Domain string
	Name   string
}

// NewCommandHandler creates a new OrderCommandHandler.
func NewCommandHandler() *OrderCommandHandler {
	return &OrderCommandHandler{
		Domain: "Order",
		Name:   "OrderCommandHandler",
	}
}

func (h *OrderCommandHandler) GetDomain() string     { return h.Domain }
func (h *OrderCommandHandler) GetIdentifier() string { return fmt.Sprintf("%s.%s", h.Domain, h.Name) }

func (h *OrderCommandHandler) GetCommands() []string {
	return []string{"PlaceOrder", "PayOrder", "CancelOrder"}
}

// Handle processes a command and returns resulting events.
func (h *OrderCommandHandler) Handle(ctx context.Context, cmd pkg.Command) ([]pkg.Event, error) {
	switch cmd.CommandType {
	case "PlaceOrder":
		return h.handlePlaceOrder(ctx, cmd)
	case "PayOrder":
		return h.handlePayOrder(ctx, cmd)
	case "CancelOrder":
		return h.handleCancelOrder(ctx, cmd)
	default:
		return nil, fmt.Errorf("unknown command: %s", cmd.CommandType)
	}
}

func (h *OrderCommandHandler) handlePlaceOrder(ctx context.Context, cmd pkg.Command) ([]pkg.Event, error) {
	data := cmd.Data.(*PlaceOrderCommand)

	// Create new aggregate
	agg := aggregate.NewAggregate().(*aggregate.Order)

	// Execute business logic
	if err := agg.PlaceOrder(data.OrderID, data.CustomerID, data.Items); err != nil {
		return nil, err
	}

	// Convert domain events to pkg.Event
	return h.toEvents(cmd, agg), nil
}

func (h *OrderCommandHandler) handlePayOrder(ctx context.Context, cmd pkg.Command) ([]pkg.Event, error) {
	data := cmd.Data.(*PayOrderCommand)

	// In real implementation, load aggregate from repository
	// For demo, create and apply previous state
	agg := aggregate.NewAggregate().(*aggregate.Order)
	agg.Uuid = data.OrderID
	agg.Status = "placed" // Simulate loaded state

	if err := agg.PayOrder(data.PaymentMethod, data.Amount); err != nil {
		return nil, err
	}

	return h.toEvents(cmd, agg), nil
}

func (h *OrderCommandHandler) handleCancelOrder(ctx context.Context, cmd pkg.Command) ([]pkg.Event, error) {
	data := cmd.Data.(*CancelOrderCommand)

	agg := aggregate.NewAggregate().(*aggregate.Order)
	agg.Uuid = data.OrderID
	agg.Status = "placed"
	agg.Items = []aggregate.OrderItem{{ProductID: "prod-1", Quantity: 2}}

	if err := agg.CancelOrder(data.Reason); err != nil {
		return nil, err
	}

	return h.toEvents(cmd, agg), nil
}

func (h *OrderCommandHandler) toEvents(cmd pkg.Command, agg *aggregate.Order) []pkg.Event {
	var events []pkg.Event
	for _, domainEvt := range agg.GetEvents() {
		var eventType string
		switch domainEvt.(type) {
		case *aggregate.OrderPlacedEvent:
			eventType = "OrderPlaced"
		case *aggregate.OrderPaidEvent:
			eventType = "OrderPaid"
		case *aggregate.OrderShippedEvent:
			eventType = "OrderShipped"
		case *aggregate.OrderCancelledEvent:
			eventType = "OrderCancelled"
		}

		events = append(events, pkg.Event{
			Uuid:          fmt.Sprintf("evt-%s-%d", cmd.AggregateUuid, len(events)+1),
			TenantUuid:    cmd.TenantUuid,
			AggregateUuid: cmd.AggregateUuid,
			Domain:        h.Domain,
			EventType:     eventType,
			Data:          domainEvt,
			Version:       agg.GetVersion(),
		})
	}
	return events
}
