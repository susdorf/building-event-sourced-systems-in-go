package pkg

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"
)

// generateUUID creates a simple UUID v4
func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// Facade is the central entry point for dispatching commands and queries
type Facade struct {
	store              *AuthStore
	commandMiddlewares []CommandMiddleware
	queryMiddlewares   []QueryMiddleware
}

// NewFacade creates a new facade
func NewFacade(store *AuthStore) *Facade {
	return &Facade{
		store:              store,
		commandMiddlewares: make([]CommandMiddleware, 0),
		queryMiddlewares:   make([]QueryMiddleware, 0),
	}
}

// AddCommandMiddleware adds a command middleware
func (f *Facade) AddCommandMiddleware(m CommandMiddleware) {
	f.commandMiddlewares = append(f.commandMiddlewares, m)
}

// AddQueryMiddleware adds a query middleware
func (f *Facade) AddQueryMiddleware(m QueryMiddleware) {
	f.queryMiddlewares = append(f.queryMiddlewares, m)
}

// DispatchCommand dispatches a command through the middleware chain
func (f *Facade) DispatchCommand(ctx context.Context, cmd *Command) *CommandResult {
	// Build the middleware chain
	handler := f.executeCommand

	// Apply middlewares in reverse order
	for i := len(f.commandMiddlewares) - 1; i >= 0; i-- {
		m := f.commandMiddlewares[i]
		next := handler
		handler = func(ctx context.Context, cmd *Command) *CommandResult {
			return m(ctx, cmd, next)
		}
	}

	return handler(ctx, cmd)
}

// DispatchQuery dispatches a query through the middleware chain
func (f *Facade) DispatchQuery(ctx context.Context, qry *Query) *QueryResult {
	// Build the middleware chain
	handler := f.executeQuery

	// Apply middlewares in reverse order
	for i := len(f.queryMiddlewares) - 1; i >= 0; i-- {
		m := f.queryMiddlewares[i]
		next := handler
		handler = func(ctx context.Context, qry *Query) *QueryResult {
			return m(ctx, qry, next)
		}
	}

	return handler(ctx, qry)
}

// executeCommand is the final command handler that actually executes the command
func (f *Facade) executeCommand(ctx context.Context, cmd *Command) *CommandResult {
	// Create an event from the command
	event := &Event{
		EventUuid:     generateUUID(),
		TenantUuid:    cmd.TenantUuid,
		WorkspaceUuid: cmd.WorkspaceUuid,
		AggregateUuid: cmd.AggregateUuid,
		Domain:        cmd.Domain,
		EventType:     cmd.CommandType + "Executed",
		Timestamp:     time.Now(),
		Data:          cmd.Data,
	}

	// Store the event
	f.store.SaveEvent(event)

	return &CommandResult{
		Success: true,
		Events:  []*Event{event},
	}
}

// executeQuery is the final query handler that actually executes the query
func (f *Facade) executeQuery(ctx context.Context, qry *Query) *QueryResult {
	// Load events for the aggregate
	events := f.store.LoadEventsForAggregate(qry.TenantUuid, qry.AggregateUuid)

	return &QueryResult{
		Success: true,
		Data:    events,
	}
}

// NewCommand creates a new command
func NewCommand(tenantUuid, workspaceUuid, domain, commandType, aggregateUuid string) *Command {
	return &Command{
		CommandUuid:   generateUUID(),
		TenantUuid:    tenantUuid,
		WorkspaceUuid: workspaceUuid,
		AggregateUuid: aggregateUuid,
		Domain:        domain,
		CommandType:   commandType,
	}
}

// NewQuery creates a new query
func NewQuery(tenantUuid, workspaceUuid, domain, queryType, aggregateUuid string) *Query {
	return &Query{
		TenantUuid:    tenantUuid,
		WorkspaceUuid: workspaceUuid,
		AggregateUuid: aggregateUuid,
		Domain:        domain,
		QueryType:     queryType,
	}
}
