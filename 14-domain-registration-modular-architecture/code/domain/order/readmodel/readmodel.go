package readmodel

import (
	"context"
	"fmt"
	"sync"

	"domain-registration-demo/domain/order/aggregate"
	"domain-registration-demo/pkg"
)

// OrderModel is the read model for orders.
type OrderModel struct {
	OrderID    string
	CustomerID string
	Status     string
	Total      float64
	ItemCount  int
}

// OrderReadmodel maintains a queryable view of orders.
type OrderReadmodel struct {
	Domain string
	Name   string

	orders map[string]*OrderModel
	mu     sync.RWMutex
}

// NewOrderReadmodel creates a new OrderReadmodel.
func NewOrderReadmodel() *OrderReadmodel {
	return &OrderReadmodel{
		Domain: "Order",
		Name:   "OrderReadmodel",
		orders: make(map[string]*OrderModel),
	}
}

func (rm *OrderReadmodel) GetDomain() string     { return rm.Domain }
func (rm *OrderReadmodel) GetName() string       { return rm.Name }
func (rm *OrderReadmodel) GetIdentifier() string { return fmt.Sprintf("%s.%s", rm.Domain, rm.Name) }

// GetEvents returns the events this readmodel subscribes to.
func (rm *OrderReadmodel) GetEvents() []string {
	return []string{
		"Order.OrderPlaced",
		"Order.OrderPaid",
		"Order.OrderShipped",
		"Order.OrderCancelled",
	}
}

// Handle processes an event and updates the read model.
func (rm *OrderReadmodel) Handle(ctx context.Context, evt pkg.Event) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	switch evt.EventType {
	case "OrderPlaced":
		data := evt.Data.(*aggregate.OrderPlacedEvent)
		rm.orders[data.OrderID] = &OrderModel{
			OrderID:    data.OrderID,
			CustomerID: data.CustomerID,
			Status:     "placed",
			Total:      data.Total,
			ItemCount:  len(data.Items),
		}
		fmt.Printf("      [OrderReadmodel] Created order: %s (customer: %s, total: %.2f)\n",
			data.OrderID, data.CustomerID, data.Total)

	case "OrderPaid":
		data := evt.Data.(*aggregate.OrderPaidEvent)
		if order, ok := rm.orders[data.OrderID]; ok {
			order.Status = "paid"
			fmt.Printf("      [OrderReadmodel] Order %s marked as paid\n", data.OrderID)
		}

	case "OrderShipped":
		data := evt.Data.(*aggregate.OrderShippedEvent)
		if order, ok := rm.orders[data.OrderID]; ok {
			order.Status = "shipped"
			fmt.Printf("      [OrderReadmodel] Order %s marked as shipped\n", data.OrderID)
		}

	case "OrderCancelled":
		data := evt.Data.(*aggregate.OrderCancelledEvent)
		if order, ok := rm.orders[data.OrderID]; ok {
			order.Status = "cancelled"
			fmt.Printf("      [OrderReadmodel] Order %s cancelled: %s\n", data.OrderID, data.Reason)
		}
	}

	return nil
}

// GetOrder returns an order by ID.
func (rm *OrderReadmodel) GetOrder(orderID string) (*OrderModel, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	order, ok := rm.orders[orderID]
	return order, ok
}

// GetAllOrders returns all orders.
func (rm *OrderReadmodel) GetAllOrders() []*OrderModel {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]*OrderModel, 0, len(rm.orders))
	for _, order := range rm.orders {
		result = append(result, order)
	}
	return result
}

// GetOrderCount returns the total number of orders.
func (rm *OrderReadmodel) GetOrderCount() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.orders)
}

// GetOrdersByStatus returns orders filtered by status.
func (rm *OrderReadmodel) GetOrdersByStatus(status string) []*OrderModel {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var result []*OrderModel
	for _, order := range rm.orders {
		if order.Status == status {
			result = append(result, order)
		}
	}
	return result
}
