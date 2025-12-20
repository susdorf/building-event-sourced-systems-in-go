package pkg

import (
	"fmt"
	"sync"
	"time"
)

// OrderView is a denormalized read model optimized for queries
type OrderView struct {
	OrderID    string
	CustomerID string
	Status     string
	Items      []ItemView
	Total      float64
	ItemCount  int
	TrackingNo string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type ItemView struct {
	ProductID string
	Name      string
	Quantity  int
	Price     float64
}

// OrderReadModel is the projection that maintains query-optimized views
type OrderReadModel struct {
	mu         sync.RWMutex
	orders     map[string]*OrderView
	byCustomer map[string][]*OrderView
}

func NewOrderReadModel() *OrderReadModel {
	return &OrderReadModel{
		orders:     make(map[string]*OrderView),
		byCustomer: make(map[string][]*OrderView),
	}
}

// Event handlers - each handles a specific event type

func (rm *OrderReadModel) OnOrderPlaced(event OrderPlacedEvent) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	items := make([]ItemView, len(event.Items))
	for i, item := range event.Items {
		items[i] = ItemView{
			ProductID: item.ProductID,
			Name:      item.Name,
			Quantity:  item.Quantity,
			Price:     item.Price,
		}
	}

	view := &OrderView{
		OrderID:    event.OrderID,
		CustomerID: event.CustomerID,
		Status:     "placed",
		Items:      items,
		Total:      event.Total,
		ItemCount:  len(event.Items),
		CreatedAt:  event.PlacedAt,
		UpdatedAt:  event.PlacedAt,
	}

	rm.orders[event.OrderID] = view
	rm.byCustomer[event.CustomerID] = append(rm.byCustomer[event.CustomerID], view)

	return nil
}

func (rm *OrderReadModel) OnOrderShipped(event OrderShippedEvent) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if view, ok := rm.orders[event.OrderID]; ok {
		view.Status = "shipped"
		view.TrackingNo = event.TrackingNo
		view.UpdatedAt = event.ShippedAt
	}

	return nil
}

func (rm *OrderReadModel) OnOrderCancelled(event OrderCancelledEvent) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if view, ok := rm.orders[event.OrderID]; ok {
		view.Status = "cancelled"
		view.UpdatedAt = event.CancelledAt
	}

	return nil
}

// Query methods - simple lookups on the denormalized data

func (rm *OrderReadModel) GetOrder(orderID string) (*OrderView, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if view, ok := rm.orders[orderID]; ok {
		return view, nil
	}
	return nil, fmt.Errorf("order not found: %s", orderID)
}

func (rm *OrderReadModel) ListByCustomer(customerID, status string, page, pageSize int) ([]*OrderView, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	orders := rm.byCustomer[customerID]

	// Filter by status if provided
	if status != "" {
		filtered := make([]*OrderView, 0)
		for _, o := range orders {
			if o.Status == status {
				filtered = append(filtered, o)
			}
		}
		orders = filtered
	}

	// Paginate
	start := (page - 1) * pageSize
	if start >= len(orders) {
		return []*OrderView{}, nil
	}
	end := start + pageSize
	if end > len(orders) {
		end = len(orders)
	}

	return orders[start:end], nil
}

// SetupOrderProjection registers all event handlers for the read model
func SetupOrderProjection(dispatcher *EventDispatcher, readModel *OrderReadModel) {
	RegisterEventHandler(dispatcher, readModel.OnOrderPlaced)
	RegisterEventHandler(dispatcher, readModel.OnOrderShipped)
	RegisterEventHandler(dispatcher, readModel.OnOrderCancelled)
}
