package pkg

import (
	"context"
	"errors"
	"sync"
)

var ErrOrderNotFound = errors.New("order not found")

// EventStore is a simple in-memory event store
type EventStore struct {
	mu     sync.RWMutex
	events map[string][]Event
}

func NewEventStore() *EventStore {
	return &EventStore{
		events: make(map[string][]Event),
	}
}

func (s *EventStore) Append(ctx context.Context, aggregateID string, events []Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events[aggregateID] = append(s.events[aggregateID], events...)
	return nil
}

func (s *EventStore) Load(ctx context.Context, aggregateID string) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events, ok := s.events[aggregateID]
	if !ok {
		return nil, nil
	}
	return events, nil
}

// GetAllEvents returns all events for debugging/display
func (s *EventStore) GetAllEvents() map[string][]Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string][]Event)
	for k, v := range s.events {
		result[k] = v
	}
	return result
}
