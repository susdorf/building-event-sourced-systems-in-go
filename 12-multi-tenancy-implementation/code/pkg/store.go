package pkg

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// EventStore is an in-memory event store with tenant isolation
type EventStore struct {
	mu     sync.RWMutex
	events []*Event
}

// NewEventStore creates a new event store
func NewEventStore() *EventStore {
	return &EventStore{
		events: make([]*Event, 0),
	}
}

// Append adds events to the store (tenant is part of each event)
func (s *EventStore) Append(events []*Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, evt := range events {
		if evt.TenantUuid == "" {
			return ErrTenantRequired
		}
		evt.CreatedAt = time.Now().UnixNano()
		s.events = append(s.events, evt)
	}
	return nil
}

// LoadForAggregate loads events for an aggregate, filtered by tenant
// This is the critical method - tenant filter is REQUIRED
func (s *EventStore) LoadForAggregate(tenantUuid, aggregateUuid string) []*Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Event
	for _, evt := range s.events {
		// Always filter by tenant!
		if evt.TenantUuid == tenantUuid && evt.AggregateUuid == aggregateUuid {
			result = append(result, evt)
		}
	}

	// Sort by version
	sort.Slice(result, func(i, j int) bool {
		return result[i].Version < result[j].Version
	})

	return result
}

// LoadAllForTenant loads all events for a tenant
func (s *EventStore) LoadAllForTenant(tenantUuid string) []*Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Event
	for _, evt := range s.events {
		if evt.TenantUuid == tenantUuid {
			result = append(result, evt)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt < result[j].CreatedAt
	})

	return result
}

// Count returns total event count
func (s *EventStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.events)
}

// CountForTenant returns event count for a specific tenant
func (s *EventStore) CountForTenant(tenantUuid string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, evt := range s.events {
		if evt.TenantUuid == tenantUuid {
			count++
		}
	}
	return count
}

// AggregateExistsInAnyTenant checks if an aggregate exists in any tenant
func (s *EventStore) AggregateExistsInAnyTenant(aggregateUuid string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, evt := range s.events {
		if evt.AggregateUuid == aggregateUuid {
			return true
		}
	}
	return false
}

// GetTenants returns all unique tenant UUIDs
func (s *EventStore) GetTenants() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tenantSet := make(map[string]bool)
	for _, evt := range s.events {
		tenantSet[evt.TenantUuid] = true
	}

	tenants := make([]string, 0, len(tenantSet))
	for tenant := range tenantSet {
		tenants = append(tenants, tenant)
	}
	sort.Strings(tenants)
	return tenants
}

// PopulateSampleEvents adds sample events for demonstration
func (s *EventStore) PopulateSampleEvents() {
	// Tenant A events
	s.Append([]*Event{
		{EventUuid: "evt-1", TenantUuid: "tenant-A", AggregateUuid: "order-1", Domain: "orders", EventType: "OrderPlaced", Version: 1},
		{EventUuid: "evt-2", TenantUuid: "tenant-A", AggregateUuid: "order-1", Domain: "orders", EventType: "OrderPaid", Version: 2},
		{EventUuid: "evt-3", TenantUuid: "tenant-A", AggregateUuid: "order-1", Domain: "orders", EventType: "OrderShipped", Version: 3},
		{EventUuid: "evt-4", TenantUuid: "tenant-A", AggregateUuid: "order-2", Domain: "orders", EventType: "OrderPlaced", Version: 1},
		{EventUuid: "evt-5", TenantUuid: "tenant-A", AggregateUuid: "user-1", Domain: "accounts", EventType: "UserCreated", Version: 1},
	})

	// Tenant B events
	s.Append([]*Event{
		{EventUuid: "evt-6", TenantUuid: "tenant-B", AggregateUuid: "order-3", Domain: "orders", EventType: "OrderPlaced", Version: 1},
		{EventUuid: "evt-7", TenantUuid: "tenant-B", AggregateUuid: "order-3", Domain: "orders", EventType: "OrderCancelled", Version: 2},
		{EventUuid: "evt-8", TenantUuid: "tenant-B", AggregateUuid: "user-2", Domain: "accounts", EventType: "UserCreated", Version: 1},
	})

	fmt.Printf("   Populated store with %d events across %d tenants\n", s.Count(), len(s.GetTenants()))
}
