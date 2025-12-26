package pkg

import (
	"context"
	"fmt"
	"time"
)

// Facade orchestrates command and query dispatch with middleware
type Facade struct {
	eventStore         *EventStore
	commandMiddlewares []CommandMiddleware
	queryMiddlewares   []QueryMiddleware
}

// NewFacade creates a new facade with the given event store
func NewFacade(store *EventStore) *Facade {
	return &Facade{
		eventStore:         store,
		commandMiddlewares: make([]CommandMiddleware, 0),
		queryMiddlewares:   make([]QueryMiddleware, 0),
	}
}

// AddCommandMiddleware registers middleware for commands
func (f *Facade) AddCommandMiddleware(mw CommandMiddleware) {
	f.commandMiddlewares = append(f.commandMiddlewares, mw)
}

// AddQueryMiddleware registers middleware for queries
func (f *Facade) AddQueryMiddleware(mw QueryMiddleware) {
	f.queryMiddlewares = append(f.queryMiddlewares, mw)
}

// DispatchCommand executes a command through the middleware chain
func (f *Facade) DispatchCommand(ctx context.Context, cmd *Command) *CommandResult {
	// The actual command handler
	handler := func(ctx context.Context, cmd *Command) *CommandResult {
		// Simulate command execution - create an event
		evt := &Event{
			EventUuid:     fmt.Sprintf("evt-%d", time.Now().UnixNano()),
			TenantUuid:    cmd.TenantUuid,
			AggregateUuid: cmd.AggregateUuid,
			Domain:        cmd.Domain,
			EventType:     cmd.CmdType + "Executed",
			Version:       1,
		}

		if err := f.eventStore.Append([]*Event{evt}); err != nil {
			return &CommandResult{Success: false, Error: err}
		}

		return &CommandResult{Success: true, Events: []*Event{evt}}
	}

	// Apply middleware in reverse order (so first registered runs first)
	for i := len(f.commandMiddlewares) - 1; i >= 0; i-- {
		handler = f.commandMiddlewares[i](handler)
	}

	return handler(ctx, cmd)
}

// DispatchQuery executes a query through the middleware chain
func (f *Facade) DispatchQuery(ctx context.Context, qry *Query) *QueryResult {
	handler := func(ctx context.Context, qry *Query) *QueryResult {
		// Load events for the requested aggregate
		events := f.eventStore.LoadForAggregate(qry.TenantUuid, qry.AggregateUuid)
		return &QueryResult{Success: true, Data: events}
	}

	// Apply middleware
	for i := len(f.queryMiddlewares) - 1; i >= 0; i-- {
		handler = f.queryMiddlewares[i](handler)
	}

	return handler(ctx, qry)
}

// GetEventStore returns the event store (for testing/demo)
func (f *Facade) GetEventStore() *EventStore {
	return f.eventStore
}
