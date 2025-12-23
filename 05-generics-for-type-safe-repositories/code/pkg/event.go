package pkg

import "time"

// Event represents a domain event in the event store.
type Event struct {
	EventID       string
	AggregateID   string
	Domain        string
	EventType     string
	Data          any
	Version       int64
	CreatedAt     int64
}

// NewEvent creates a new event.
func NewEvent(aggregateID, domain, eventType string, data any, version int64) Event {
	return Event{
		EventID:     NewUUID(),
		AggregateID: aggregateID,
		Domain:      domain,
		EventType:   eventType,
		Data:        data,
		Version:     version,
		CreatedAt:   time.Now().UnixNano(),
	}
}

// EventStore is an in-memory event store for demonstration purposes.
type EventStore struct {
	events map[string][]Event // aggregateID -> events
}

// NewEventStore creates a new in-memory event store.
func NewEventStore() *EventStore {
	return &EventStore{
		events: make(map[string][]Event),
	}
}

// Append adds events for an aggregate.
func (s *EventStore) Append(aggregateID string, events []Event) error {
	s.events[aggregateID] = append(s.events[aggregateID], events...)
	return nil
}

// Load retrieves all events for an aggregate.
func (s *EventStore) Load(aggregateID string) ([]Event, error) {
	return s.events[aggregateID], nil
}

// LoadUpToVersion retrieves events up to a specific version.
func (s *EventStore) LoadUpToVersion(aggregateID string, version int64) ([]Event, error) {
	allEvents := s.events[aggregateID]
	var result []Event
	for _, evt := range allEvents {
		if evt.Version <= version {
			result = append(result, evt)
		}
	}
	return result, nil
}

// Count returns the total number of events in the store.
func (s *EventStore) Count() int {
	total := 0
	for _, events := range s.events {
		total += len(events)
	}
	return total
}

// ListAggregates returns all aggregate IDs in the store.
func (s *EventStore) ListAggregates() []string {
	ids := make([]string, 0, len(s.events))
	for id := range s.events {
		ids = append(ids, id)
	}
	return ids
}
