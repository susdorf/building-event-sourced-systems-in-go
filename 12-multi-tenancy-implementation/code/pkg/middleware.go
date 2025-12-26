package pkg

import (
	"context"
	"fmt"
)

// CommandHandlerFunc is the signature for command handlers
type CommandHandlerFunc func(ctx context.Context, cmd *Command) *CommandResult

// QueryHandlerFunc is the signature for query handlers
type QueryHandlerFunc func(ctx context.Context, qry *Query) *QueryResult

// CommandMiddleware wraps a command handler with additional logic
type CommandMiddleware func(next CommandHandlerFunc) CommandHandlerFunc

// QueryMiddleware wraps a query handler with additional logic
type QueryMiddleware func(next QueryHandlerFunc) QueryHandlerFunc

// AuthorizationLog tracks authorization decisions for demonstration
type AuthorizationLog struct {
	Entries []AuthLogEntry
}

type AuthLogEntry struct {
	Step     string
	Decision string
	Details  string
}

var authLog = &AuthorizationLog{}

func GetAuthLog() *AuthorizationLog {
	return authLog
}

func (l *AuthorizationLog) Clear() {
	l.Entries = nil
}

func (l *AuthorizationLog) Add(step, decision, details string) {
	l.Entries = append(l.Entries, AuthLogEntry{
		Step:     step,
		Decision: decision,
		Details:  details,
	})
}

// AuthCommandMiddleware implements multi-tier authorization for commands
func AuthCommandMiddleware(eventStore *EventStore) CommandMiddleware {
	return func(next CommandHandlerFunc) CommandHandlerFunc {
		return func(ctx context.Context, cmd *Command) *CommandResult {
			authLog.Clear()

			reqCtx := GetReqCtxFromContext(ctx)
			if reqCtx == nil {
				authLog.Add("Context Check", "DENIED", "No request context found")
				return &CommandResult{Success: false, Error: ErrUnauthorized}
			}
			authLog.Add("Context Check", "OK", fmt.Sprintf("sender=%s, target=%s", reqCtx.SenderTenantUuid, cmd.TenantUuid))

			// 1. Check skip authorization flag (for internal system calls)
			if cmd.SkipAuthorization {
				authLog.Add("Skip Auth Flag", "BYPASS", "Authorization skipped by flag")
				return next(ctx, cmd)
			}
			authLog.Add("Skip Auth Flag", "CONTINUE", "Flag not set")

			// 2. System admin bypass
			if isSystemAdmin(reqCtx) {
				authLog.Add("System Admin Check", "BYPASS", fmt.Sprintf("System admin from tenant %s", reqCtx.SenderTenantUuid))
				return next(ctx, cmd)
			}
			authLog.Add("System Admin Check", "CONTINUE", "Not a system admin")

			// 3. Validate tenant match (sender must match command's target tenant)
			if cmd.TenantUuid != reqCtx.SenderTenantUuid {
				authLog.Add("Tenant Match", "DENIED", fmt.Sprintf("sender=%s cannot access tenant=%s", reqCtx.SenderTenantUuid, cmd.TenantUuid))
				return &CommandResult{
					Success: false,
					Error:   fmt.Errorf("%w: sender %s cannot access tenant %s", ErrForbidden, reqCtx.SenderTenantUuid, cmd.TenantUuid),
				}
			}
			authLog.Add("Tenant Match", "OK", "Sender tenant matches command tenant")

			// 4. Validate aggregate ownership (if targeting existing aggregate)
			if cmd.AggregateUuid != "" {
				if !IsValidTenantAggregate(ctx, eventStore, cmd.TenantUuid, cmd.AggregateUuid) {
					authLog.Add("Aggregate Ownership", "DENIED", fmt.Sprintf("aggregate %s not owned by tenant %s", cmd.AggregateUuid, cmd.TenantUuid))
					return &CommandResult{
						Success: false,
						Error:   fmt.Errorf("%w: aggregate %s", ErrAggregateNotOwned, cmd.AggregateUuid),
					}
				}
				authLog.Add("Aggregate Ownership", "OK", fmt.Sprintf("aggregate %s belongs to tenant", cmd.AggregateUuid))
			} else {
				authLog.Add("Aggregate Ownership", "SKIP", "No aggregate UUID (new aggregate)")
			}

			// 5. All checks passed
			authLog.Add("Final Decision", "ALLOWED", "All authorization checks passed")
			return next(ctx, cmd)
		}
	}
}

// AuthQueryMiddleware implements authorization for queries
func AuthQueryMiddleware(eventStore *EventStore) QueryMiddleware {
	return func(next QueryHandlerFunc) QueryHandlerFunc {
		return func(ctx context.Context, qry *Query) *QueryResult {
			reqCtx := GetReqCtxFromContext(ctx)
			if reqCtx == nil {
				return &QueryResult{Success: false, Error: ErrUnauthorized}
			}

			// System admin can query any tenant
			if isSystemAdmin(reqCtx) {
				return next(ctx, qry)
			}

			// Tenant must match
			if qry.TenantUuid != reqCtx.SenderTenantUuid {
				return &QueryResult{
					Success: false,
					Error:   fmt.Errorf("%w: query access denied", ErrForbidden),
				}
			}

			return next(ctx, qry)
		}
	}
}

// isSystemAdmin checks if the request comes from a system administrator
func isSystemAdmin(reqCtx *RequestContext) bool {
	// Option 1: System tenant
	if reqCtx.SenderTenantUuid == SystemTenantUuid {
		return true
	}
	// Option 2: Admin flag
	if reqCtx.IsAdmin {
		return true
	}
	return false
}

// IsValidTenantAggregate checks if an aggregate belongs to the specified tenant
func IsValidTenantAggregate(ctx context.Context, store *EventStore, tenantUuid, aggregateUuid string) bool {
	// First check if events exist for this tenant+aggregate combination
	events := store.LoadForAggregate(tenantUuid, aggregateUuid)
	if len(events) > 0 {
		// Aggregate exists and belongs to this tenant
		return true
	}

	// No events found - check if aggregate exists in ANY tenant (cross-tenant access attempt)
	if store.AggregateExistsInAnyTenant(aggregateUuid) {
		// Aggregate exists but belongs to different tenant
		return false
	}

	// Truly new aggregate - ownership will be established with first event
	return true
}
