package pkg

import (
	"context"
	"fmt"
	"time"
)

// System tenant UUID for admin operations
const SystemTenantUuid = "system"

// Common errors
var (
	ErrUnauthorized    = fmt.Errorf("unauthorized: no request context")
	ErrForbidden       = fmt.Errorf("forbidden: cross-tenant access denied")
	ErrTenantRequired  = fmt.Errorf("tenant UUID required")
	ErrTenantMismatch  = fmt.Errorf("aggregate does not belong to tenant")
	ErrAggregateNotOwned = fmt.Errorf("aggregate ownership validation failed")
)

// Command represents a command in the system
type Command struct {
	CmdUuid           string
	TenantUuid        string
	AggregateUuid     string
	Domain            string
	CmdType           string
	Data              string
	SkipAuthorization bool
}

func (c *Command) GetCmdUuid() string       { return c.CmdUuid }
func (c *Command) GetTenantUuid() string    { return c.TenantUuid }
func (c *Command) GetAggregateUuid() string { return c.AggregateUuid }
func (c *Command) GetDomain() string        { return c.Domain }
func (c *Command) GetCmdType() string       { return c.CmdType }

// NewCommand creates a command with tenant from context
func NewCommand(ctx context.Context, domain, cmdType, aggregateUuid string) *Command {
	cmd := &Command{
		CmdUuid:       fmt.Sprintf("cmd-%d", time.Now().UnixNano()),
		Domain:        domain,
		CmdType:       cmdType,
		AggregateUuid: aggregateUuid,
	}

	// Extract tenant from context
	if reqCtx := GetReqCtxFromContext(ctx); reqCtx != nil {
		cmd.TenantUuid = reqCtx.SenderTenantUuid
	}

	return cmd
}

// NewCommandForTenant creates a command targeting a specific tenant
func NewCommandForTenant(ctx context.Context, targetTenant, domain, cmdType, aggregateUuid string) *Command {
	cmd := NewCommand(ctx, domain, cmdType, aggregateUuid)
	cmd.TenantUuid = targetTenant
	return cmd
}

// Event represents an event in the system
type Event struct {
	EventUuid     string
	TenantUuid    string
	AggregateUuid string
	Domain        string
	EventType     string
	Version       int
	Data          string
	CreatedAt     int64
}

func (e *Event) GetEventUuid() string     { return e.EventUuid }
func (e *Event) GetTenantUuid() string    { return e.TenantUuid }
func (e *Event) GetAggregateUuid() string { return e.AggregateUuid }
func (e *Event) GetDomain() string        { return e.Domain }
func (e *Event) GetEventType() string     { return e.EventType }

// Query represents a query in the system
type Query struct {
	QueryUuid     string
	TenantUuid    string
	AggregateUuid string
	Domain        string
	QueryType     string
}

func (q *Query) GetQueryUuid() string     { return q.QueryUuid }
func (q *Query) GetTenantUuid() string    { return q.TenantUuid }
func (q *Query) GetAggregateUuid() string { return q.AggregateUuid }

// NewQuery creates a query with tenant from context
func NewQuery(ctx context.Context, domain, queryType, aggregateUuid string) *Query {
	qry := &Query{
		QueryUuid:     fmt.Sprintf("qry-%d", time.Now().UnixNano()),
		Domain:        domain,
		QueryType:     queryType,
		AggregateUuid: aggregateUuid,
	}

	if reqCtx := GetReqCtxFromContext(ctx); reqCtx != nil {
		qry.TenantUuid = reqCtx.SenderTenantUuid
	}

	return qry
}

// CommandResult represents the result of command execution
type CommandResult struct {
	Success bool
	Events  []*Event
	Error   error
}

// QueryResult represents the result of query execution
type QueryResult struct {
	Success bool
	Data    interface{}
	Error   error
}
