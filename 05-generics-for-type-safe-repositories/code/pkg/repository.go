package pkg

import "fmt"

// NewAggregateFunc is a factory function type that creates new aggregate instances.
// This is required because Go generics cannot instantiate type parameters directly,
// and aggregates need proper initialization (base aggregate, domain, event handlers).
type NewAggregateFunc[T Aggregate] func() T

// AggregateRepository is a generic repository for any aggregate type.
// The type parameter T must implement the Aggregate interface.
type AggregateRepository[T Aggregate] struct {
	eventStore       *EventStore
	newAggregateFunc NewAggregateFunc[T]
}

// NewAggregateRepository creates a new typed repository.
func NewAggregateRepository[T Aggregate](
	eventStore *EventStore,
	newFunc NewAggregateFunc[T],
) *AggregateRepository[T] {
	return &AggregateRepository[T]{
		eventStore:       eventStore,
		newAggregateFunc: newFunc,
	}
}

// GetAggregate loads an aggregate by ID.
// Returns the concrete type T, not an interface - this is the key benefit!
func (r *AggregateRepository[T]) GetAggregate(aggregateID string) (T, error) {
	var nilT T // Zero value for return on error

	// Load events from event store
	events, err := r.eventStore.Load(aggregateID)
	if err != nil {
		return nilT, fmt.Errorf("failed to load events: %w", err)
	}

	// No events = aggregate doesn't exist
	if len(events) == 0 {
		return nilT, nil
	}

	// Create new aggregate using the factory function
	// This properly initializes: base aggregate, domain, and event handlers
	agg := r.newAggregateFunc()
	agg.SetID(aggregateID)

	// Domain mismatch protection
	if agg.GetDomain() != events[0].Domain {
		return nilT, fmt.Errorf("domain mismatch: expected %s, got %s", agg.GetDomain(), events[0].Domain)
	}

	// Replay events to reconstruct state
	for _, event := range events {
		if err := agg.ApplyEvent(event); err != nil {
			return nilT, fmt.Errorf("failed to apply event %s: %w", event.EventType, err)
		}
	}

	return agg, nil
}

// Save persists the uncommitted events of an aggregate.
func (r *AggregateRepository[T]) Save(agg T) error {
	events := agg.GetUncommittedEvents()
	if len(events) == 0 {
		return nil // Nothing to save
	}

	if err := r.eventStore.Append(agg.GetID(), events); err != nil {
		return fmt.Errorf("failed to append events: %w", err)
	}

	agg.ClearUncommittedEvents()
	return nil
}

// GetAggregateAtVersion loads an aggregate at a specific version (for temporal queries).
func (r *AggregateRepository[T]) GetAggregateAtVersion(aggregateID string, version int64) (T, error) {
	var nilT T

	events, err := r.eventStore.LoadUpToVersion(aggregateID, version)
	if err != nil {
		return nilT, fmt.Errorf("failed to load events: %w", err)
	}

	if len(events) == 0 {
		return nilT, nil
	}

	agg := r.newAggregateFunc()
	agg.SetID(aggregateID)

	for _, event := range events {
		if err := agg.ApplyEvent(event); err != nil {
			return nilT, fmt.Errorf("failed to apply event: %w", err)
		}
	}

	return agg, nil
}

// Exists checks if an aggregate exists without loading all events.
func (r *AggregateRepository[T]) Exists(aggregateID string) (bool, error) {
	events, err := r.eventStore.Load(aggregateID)
	if err != nil {
		return false, err
	}
	return len(events) > 0, nil
}

// --- Contrast: Non-Generic Repository (The Old Way) ---

// NonGenericRepository demonstrates the pre-generics approach for comparison.
type NonGenericRepository struct {
	eventStore *EventStore
}

// Load returns an interface - caller must type assert.
func (r *NonGenericRepository) Load(aggregateID string) (Aggregate, error) {
	// Problem: How do we know which type to create?
	// We'd need to store type information or use a registry.
	return nil, fmt.Errorf("cannot determine aggregate type")
}

// Save works with any Aggregate, but we lose type information.
func (r *NonGenericRepository) Save(agg Aggregate) error {
	events := agg.GetUncommittedEvents()
	if len(events) == 0 {
		return nil
	}
	return r.eventStore.Append(agg.GetID(), events)
}
