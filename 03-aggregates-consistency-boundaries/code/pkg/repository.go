package pkg

import "context"

// OrderRepository handles persistence of Order aggregates
type OrderRepository struct {
	eventStore *EventStore
}

func NewOrderRepository(eventStore *EventStore) *OrderRepository {
	return &OrderRepository{eventStore: eventStore}
}

func (r *OrderRepository) Load(ctx context.Context, orderID string) (*Order, error) {
	events, err := r.eventStore.Load(ctx, orderID)
	if err != nil {
		return nil, err
	}

	if len(events) == 0 {
		return nil, ErrOrderNotFound
	}

	order := NewOrder(orderID)
	order.LoadFromHistory(events)
	return order, nil
}

func (r *OrderRepository) Save(ctx context.Context, order *Order) error {
	events := order.GetUncommittedEvents()
	if len(events) == 0 {
		return nil // Nothing to save
	}

	err := r.eventStore.Append(ctx, order.ID, events)
	if err != nil {
		return err
	}

	order.ClearUncommittedEvents()
	return nil
}
