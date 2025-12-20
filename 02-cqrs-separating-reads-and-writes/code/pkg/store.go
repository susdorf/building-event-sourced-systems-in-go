package pkg

import (
	"context"
	"fmt"
	"sync"
)

// EventStore persists and retrieves events
type EventStore struct {
	mu          sync.RWMutex
	events      map[string][]any // aggregateID -> events
	subscribers []func(event any) error
}

func NewEventStore() *EventStore {
	return &EventStore{
		events:      make(map[string][]any),
		subscribers: make([]func(event any) error, 0),
	}
}

// Append stores new events for an aggregate
func (s *EventStore) Append(ctx context.Context, aggregateID string, events []any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events[aggregateID] = append(s.events[aggregateID], events...)

	// Notify subscribers (projections)
	for _, event := range events {
		for _, subscriber := range s.subscribers {
			if err := subscriber(event); err != nil {
				return err
			}
		}
	}

	return nil
}

// Load retrieves all events for an aggregate
func (s *EventStore) Load(ctx context.Context, aggregateID string) ([]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events := s.events[aggregateID]
	result := make([]any, len(events))
	copy(result, events)
	return result, nil
}

// Subscribe registers a handler to receive new events
func (s *EventStore) Subscribe(handler func(event any) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscribers = append(s.subscribers, handler)
}

// AggregateRepository provides load/save operations for aggregates
type AggregateRepository struct {
	store           *EventStore
	eventDispatcher *EventDispatcher
}

func NewAggregateRepository(store *EventStore, eventDispatcher *EventDispatcher) *AggregateRepository {
	return &AggregateRepository{
		store:           store,
		eventDispatcher: eventDispatcher,
	}
}

// Save persists uncommitted events from an aggregate
func (r *AggregateRepository) Save(ctx context.Context, aggregate *OrderAggregate) error {
	events := aggregate.GetUncommittedEvents()
	if len(events) == 0 {
		return nil
	}

	// Persist events
	if err := r.store.Append(ctx, aggregate.ID, events); err != nil {
		return err
	}

	// Dispatch to event handlers (projections)
	for _, event := range events {
		if err := r.eventDispatcher.Dispatch(event); err != nil {
			return err
		}
	}

	aggregate.ClearUncommittedEvents()
	return nil
}

// Load rebuilds an aggregate from its event history
func (r *AggregateRepository) Load(ctx context.Context, aggregateID string) (*OrderAggregate, error) {
	events, err := r.store.Load(ctx, aggregateID)
	if err != nil {
		return nil, err
	}

	if len(events) == 0 {
		return nil, fmt.Errorf("aggregate not found: %s", aggregateID)
	}

	aggregate := NewOrderAggregate(aggregateID)
	aggregate.LoadFromHistory(events)
	return aggregate, nil
}
