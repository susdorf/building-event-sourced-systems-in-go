package pkg

import "context"

// RequestContext carries tenant and identity information through the request lifecycle
type RequestContext struct {
	// Sender: Who is executing this operation
	SenderTenantUuid   string
	SenderIdentityUuid string

	// Target: Whose data is being accessed (usually same as sender)
	TargetTenantUuid    string
	TargetWorkspaceUuid string
	TargetAggregateUuid string

	// Permissions for RBAC
	Permissions []string
	IsAdmin     bool
}

// Context key for request context (unexported to prevent collisions)
type reqCtxKey struct{}

// SetReqCtxToContext adds RequestContext to the context
func SetReqCtxToContext(ctx context.Context, reqCtx *RequestContext) context.Context {
	return context.WithValue(ctx, reqCtxKey{}, reqCtx)
}

// GetReqCtxFromContext retrieves RequestContext from context
func GetReqCtxFromContext(ctx context.Context) *RequestContext {
	if reqCtx, ok := ctx.Value(reqCtxKey{}).(*RequestContext); ok {
		return reqCtx
	}
	return nil
}

// CreateContextWithTenant creates a context with tenant information for testing
func CreateContextWithTenant(senderTenant, senderIdentity string, isAdmin bool) context.Context {
	reqCtx := &RequestContext{
		SenderTenantUuid:   senderTenant,
		SenderIdentityUuid: senderIdentity,
		TargetTenantUuid:   senderTenant, // Default: target same as sender
		IsAdmin:            isAdmin,
	}
	return SetReqCtxToContext(context.Background(), reqCtx)
}

// CreateCrossTenantContext creates a context for cross-tenant operations
func CreateCrossTenantContext(senderTenant, targetTenant, senderIdentity string, isAdmin bool) context.Context {
	reqCtx := &RequestContext{
		SenderTenantUuid:   senderTenant,
		SenderIdentityUuid: senderIdentity,
		TargetTenantUuid:   targetTenant,
		IsAdmin:            isAdmin,
	}
	return SetReqCtxToContext(context.Background(), reqCtx)
}
