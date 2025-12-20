package pkg

import "context"

// Queries - ask questions (what we want to know)

type GetOrder struct {
	OrderID string
}

type ListOrdersByCustomer struct {
	CustomerID string
	Status     string // Optional filter
	Page       int
	PageSize   int
}

// OrderQueryHandlers contains all order-related query handlers
type OrderQueryHandlers struct {
	ReadModel *OrderReadModel
}

func (h *OrderQueryHandlers) HandleGetOrder(ctx context.Context, q GetOrder) (*OrderView, error) {
	return h.ReadModel.GetOrder(q.OrderID)
}

func (h *OrderQueryHandlers) HandleListByCustomer(ctx context.Context, q ListOrdersByCustomer) ([]*OrderView, error) {
	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize < 1 {
		pageSize = 10
	}
	return h.ReadModel.ListByCustomer(q.CustomerID, q.Status, page, pageSize)
}

// SetupQueryHandlers registers all order query handlers
func SetupQueryHandlers(dispatcher *QueryDispatcher, readModel *OrderReadModel) {
	handlers := &OrderQueryHandlers{ReadModel: readModel}
	RegisterQuery(dispatcher, handlers.HandleGetOrder)
	RegisterQuery(dispatcher, handlers.HandleListByCustomer)
}
