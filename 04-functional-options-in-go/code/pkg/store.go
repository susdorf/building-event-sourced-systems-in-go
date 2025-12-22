package pkg

import (
	"fmt"
	"sort"
	"time"
)

// Event represents a domain event stored in the event store.
type Event struct {
	Uuid          string
	TenantUuid    string
	AggregateUuid string
	Domain        string
	DataType      string
	Data          string
	CreatedAt     int64
}

// EventStore is an in-memory event store for demonstration purposes.
type EventStore struct {
	events []Event
}

// NewEventStore creates a new in-memory event store.
func NewEventStore() *EventStore {
	return &EventStore{
		events: make([]Event, 0),
	}
}

// Add adds an event to the store.
func (s *EventStore) Add(evt Event) {
	if evt.CreatedAt == 0 {
		evt.CreatedAt = time.Now().UnixNano()
	}
	s.events = append(s.events, evt)
}

// List retrieves events using functional options.
// This is the key method demonstrating the pattern.
func (s *EventStore) List(opts ...EventStoreListOption) ([]Event, int64, error) {
	// Start with defaults
	options := &EventStoreListOptions{
		Limit:     100,
		OrderBy:   "created_at",
		Ascending: false,
	}

	// Apply each option
	for _, opt := range opts {
		var err error
		options, err = opt(options)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid option: %w", err)
		}
	}

	// Filter events based on options
	var filtered []Event
	for _, evt := range s.events {
		if !s.matchesFilters(evt, options) {
			continue
		}
		filtered = append(filtered, evt)
	}

	// Sort
	sort.Slice(filtered, func(i, j int) bool {
		if options.Ascending {
			return filtered[i].CreatedAt < filtered[j].CreatedAt
		}
		return filtered[i].CreatedAt > filtered[j].CreatedAt
	})

	// Total before pagination
	total := int64(len(filtered))

	// Apply pagination
	if options.Offset > 0 && options.Offset < int64(len(filtered)) {
		filtered = filtered[options.Offset:]
	} else if options.Offset >= int64(len(filtered)) {
		filtered = nil
	}

	if options.Limit > 0 && int64(len(filtered)) > options.Limit {
		filtered = filtered[:options.Limit]
	}

	return filtered, total, nil
}

// matchesFilters checks if an event matches all specified filters.
func (s *EventStore) matchesFilters(evt Event, opts *EventStoreListOptions) bool {
	if opts.TenantUuid != "" && evt.TenantUuid != opts.TenantUuid {
		return false
	}
	if opts.AggregateUuid != "" && evt.AggregateUuid != opts.AggregateUuid {
		return false
	}
	if opts.DataType != "" && evt.DataType != opts.DataType {
		return false
	}
	if len(opts.Domains) > 0 {
		found := false
		for _, d := range opts.Domains {
			if evt.Domain == d {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if opts.After > 0 && evt.CreatedAt <= opts.After {
		return false
	}
	if opts.Before > 0 && evt.CreatedAt >= opts.Before {
		return false
	}
	return true
}

// Count returns the total number of events in the store.
func (s *EventStore) Count() int {
	return len(s.events)
}

// GetAppliedOptions is a helper to show what options were applied (for demo purposes).
func GetAppliedOptions(opts ...EventStoreListOption) (*EventStoreListOptions, error) {
	options := &EventStoreListOptions{
		Limit:     100,
		OrderBy:   "created_at",
		Ascending: false,
	}
	for _, opt := range opts {
		var err error
		options, err = opt(options)
		if err != nil {
			return nil, err
		}
	}
	return options, nil
}
