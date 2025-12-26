package pkg

import "context"

type contextKey string

const (
	ctxKeyRequestContext contextKey = "requestContext"
)

// RequestContext carries authorization information through the system
type RequestContext struct {
	// Sender information (who is making the request)
	SenderTenantUuid   string
	SenderIdentityUuid string
	SenderAccountUuid  string
	SenderSessionUuid  string
	IsSystemAdmin      bool

	// Target information (what is being accessed)
	TargetWorkspaceUuid string
	TargetAggregateUuid string

	// Skip authorization (for system operations)
	SkipAuthorization bool
}

// NewRequestContext creates a new request context
func NewRequestContext(tenantUuid, identityUuid string) *RequestContext {
	return &RequestContext{
		SenderTenantUuid:   tenantUuid,
		SenderIdentityUuid: identityUuid,
	}
}

// WithWorkspace sets the target workspace
func (rc *RequestContext) WithWorkspace(workspaceUuid string) *RequestContext {
	rc.TargetWorkspaceUuid = workspaceUuid
	return rc
}

// WithAggregate sets the target aggregate
func (rc *RequestContext) WithAggregate(aggregateUuid string) *RequestContext {
	rc.TargetAggregateUuid = aggregateUuid
	return rc
}

// WithSession sets session information
func (rc *RequestContext) WithSession(accountUuid, sessionUuid string) *RequestContext {
	rc.SenderAccountUuid = accountUuid
	rc.SenderSessionUuid = sessionUuid
	return rc
}

// WithSystemAdmin marks this as a system admin request
func (rc *RequestContext) WithSystemAdmin() *RequestContext {
	rc.IsSystemAdmin = true
	return rc
}

// WithSkipAuthorization sets the skip authorization flag
func (rc *RequestContext) WithSkipAuthorization() *RequestContext {
	rc.SkipAuthorization = true
	return rc
}

// ContextWithRequestContext adds request context to a context
func ContextWithRequestContext(ctx context.Context, rc *RequestContext) context.Context {
	return context.WithValue(ctx, ctxKeyRequestContext, rc)
}

// GetRequestContext retrieves request context from a context
func GetRequestContext(ctx context.Context) *RequestContext {
	if rc, ok := ctx.Value(ctxKeyRequestContext).(*RequestContext); ok {
		return rc
	}
	return nil
}

// CreateContext creates a context with request context for a regular user
func CreateContext(tenantUuid, identityUuid string) context.Context {
	rc := NewRequestContext(tenantUuid, identityUuid)
	return ContextWithRequestContext(context.Background(), rc)
}

// CreateWorkspaceContext creates a context with workspace information
func CreateWorkspaceContext(tenantUuid, identityUuid, workspaceUuid string) context.Context {
	rc := NewRequestContext(tenantUuid, identityUuid).WithWorkspace(workspaceUuid)
	return ContextWithRequestContext(context.Background(), rc)
}

// CreateSystemAdminContext creates a context for system admin
func CreateSystemAdminContext() context.Context {
	rc := NewRequestContext(SystemTenantUuid, SystemIdentityUuid).WithSystemAdmin()
	return ContextWithRequestContext(context.Background(), rc)
}

// CreateAnonymousContext creates an anonymous context
func CreateAnonymousContext() context.Context {
	rc := NewRequestContext(AnonymousTenantUuid, AnonymousIdentity)
	return ContextWithRequestContext(context.Background(), rc)
}
