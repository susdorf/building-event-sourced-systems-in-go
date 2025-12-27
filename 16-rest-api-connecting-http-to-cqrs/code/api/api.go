// Package api provides the REST API layer that bridges HTTP to CQRS.
package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"rest-api-cqrs-demo/domain"

	"github.com/danielgtaylor/huma/v2"
)

// Context keys for storing authentication data
type contextKey string

const (
	ctxKeyTenantUuid   contextKey = "tenantUuid"
	ctxKeyIdentityUuid contextKey = "identityUuid"
	ctxKeyAccountUuid  contextKey = "accountUuid"
	ctxKeySessionUuid  contextKey = "sessionUuid"
)

// --- Context Helper Functions ---

func TenantUuidFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyTenantUuid).(string); ok {
		return v
	}
	return ""
}

func IdentityUuidFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyIdentityUuid).(string); ok {
		return v
	}
	return ""
}

func AccountUuidFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyAccountUuid).(string); ok {
		return v
	}
	return ""
}

func SessionUuidFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeySessionUuid).(string); ok {
		return v
	}
	return ""
}

// --- HTTP Middleware ---

// AuthAnonymousCtx sets default anonymous context values
func AuthAnonymousCtx() func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		// Set default anonymous values
		newCtx := context.WithValue(ctx.Context(), ctxKeyTenantUuid, "")
		newCtx = context.WithValue(newCtx, ctxKeyIdentityUuid, "anonymous")
		newCtx = context.WithValue(newCtx, ctxKeyAccountUuid, "")
		newCtx = context.WithValue(newCtx, ctxKeySessionUuid, "")

		ctx = huma.WithContext(ctx, newCtx)
		next(ctx)
	}
}

// AuthBearerToken extracts and validates bearer token from Authorization header
// For demo purposes, we accept tokens in format: "Bearer user:<identityUuid>"
func AuthBearerToken() func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		authHeader := ctx.Header("Authorization")

		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token := authHeader[7:]

			// Simple token parsing for demo: "user:<identityUuid>"
			if len(token) > 5 && token[:5] == "user:" {
				identityUuid := token[5:]
				newCtx := context.WithValue(ctx.Context(), ctxKeyIdentityUuid, identityUuid)
				newCtx = context.WithValue(newCtx, ctxKeyAccountUuid, "account-"+identityUuid)
				newCtx = context.WithValue(newCtx, ctxKeySessionUuid, "session-"+identityUuid)
				ctx = huma.WithContext(ctx, newCtx)
			}
		}

		next(ctx)
	}
}

// AuthTargetCtx extracts tenant UUID from path parameters
func AuthTargetCtx() func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		// Extract tenantUuid from path if present
		tenantUuid := ctx.Param("tenantUuid")
		if tenantUuid != "" {
			newCtx := context.WithValue(ctx.Context(), ctxKeyTenantUuid, tenantUuid)
			ctx = huma.WithContext(ctx, newCtx)
		}

		next(ctx)
	}
}

// --- Domain-Level Middleware ---

// LoggingCommandMiddleware logs command execution
func LoggingCommandMiddleware() domain.CommandMiddleware {
	return func(ctx context.Context, cmd *domain.Command, next domain.CommandHandler) error {
		start := time.Now()
		log.Printf("[CMD] Started: %s (uuid: %s, tenant: %s)",
			fmt.Sprintf("%T", cmd.Payload), cmd.CommandUuid[:8], cmd.TenantUuid)

		err := next(ctx, cmd)

		duration := time.Since(start)
		if err != nil {
			log.Printf("[CMD] Failed: %s (uuid: %s, duration: %v, error: %v)",
				fmt.Sprintf("%T", cmd.Payload), cmd.CommandUuid[:8], duration, err)
		} else {
			log.Printf("[CMD] Completed: %s (uuid: %s, duration: %v)",
				fmt.Sprintf("%T", cmd.Payload), cmd.CommandUuid[:8], duration)
		}

		return err
	}
}

// LoggingQueryMiddleware logs query execution
func LoggingQueryMiddleware() domain.QueryMiddleware {
	return func(ctx context.Context, qry *domain.Query, next domain.QueryHandler) (any, error) {
		start := time.Now()
		log.Printf("[QRY] Started: %s (uuid: %s, tenant: %s)",
			fmt.Sprintf("%T", qry.Payload), qry.QueryUuid[:8], qry.TenantUuid)

		res, err := next(ctx, qry)

		duration := time.Since(start)
		if err != nil {
			log.Printf("[QRY] Failed: %s (uuid: %s, duration: %v, error: %v)",
				fmt.Sprintf("%T", qry.Payload), qry.QueryUuid[:8], duration, err)
		} else {
			log.Printf("[QRY] Completed: %s (uuid: %s, duration: %v)",
				fmt.Sprintf("%T", qry.Payload), qry.QueryUuid[:8], duration)
		}

		return res, err
	}
}

// --- Error Handling ---

// SchemaError maps domain errors to HTTP status codes
func SchemaError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, domain.ErrUnauthorized):
		return huma.Error401Unauthorized(err.Error())
	case errors.Is(err, domain.ErrPermissionDenied):
		return huma.Error403Forbidden(err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return huma.Error404NotFound(err.Error())
	case errors.Is(err, domain.ErrConflict):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, domain.ErrBadRequest):
		return huma.Error400BadRequest(err.Error())
	default:
		return huma.Error400BadRequest(err.Error())
	}
}

// --- Request/Response Types ---

type RequestOrderCreate struct {
	TenantUuid string `path:"tenantUuid" doc:"Tenant UUID"`
	Body       struct {
		OrderUuid    string        `json:"orderUuid" doc:"Unique order identifier"`
		CustomerName string        `json:"customerName" doc:"Customer name" minLength:"1"`
		Items        []domain.Item `json:"items" doc:"Order items" minItems:"1"`
	}
}

type RequestOrderUpdate struct {
	TenantUuid string `path:"tenantUuid" doc:"Tenant UUID"`
	OrderUuid  string `path:"orderUuid" doc:"Order UUID"`
	Body       struct {
		CustomerName string        `json:"customerName" doc:"Customer name"`
		Items        []domain.Item `json:"items" doc:"Order items"`
	}
}

type RequestOrderGet struct {
	TenantUuid string `path:"tenantUuid" doc:"Tenant UUID"`
	OrderUuid  string `path:"orderUuid" doc:"Order UUID"`
}

type RequestOrderList struct {
	TenantUuid string `path:"tenantUuid" doc:"Tenant UUID"`
	Page       int    `query:"page" default:"1" doc:"Page number"`
	PageSize   int    `query:"pageSize" default:"10" doc:"Items per page"`
	Status     string `query:"status" doc:"Filter by status"`
}

type RequestOrderAction struct {
	TenantUuid string `path:"tenantUuid" doc:"Tenant UUID"`
	OrderUuid  string `path:"orderUuid" doc:"Order UUID"`
}

type ResponseOrder struct {
	Body *domain.Order
}

type ResponseOrderList struct {
	Body *domain.OrderListResponse
}

type ResponseEmpty struct{}

// --- Resource ---

// OrderResource handles all order-related endpoints
type OrderResource struct {
	fc  *domain.Facade
	api huma.API
}

// NewOrderResource creates a new order resource
func NewOrderResource(fc *domain.Facade, api huma.API) *OrderResource {
	return &OrderResource{fc: fc, api: api}
}

// Register registers all order endpoints
func (rs *OrderResource) Register() {
	// Create order
	huma.Register(rs.api, huma.Operation{
		OperationID: "order-create",
		Summary:     "Create a new order",
		Description: "Creates a new order for a tenant",
		Method:      http.MethodPost,
		Path:        "/api/tenants/{tenantUuid}/orders",
		Tags:        []string{"Orders"},
	}, rs.Create)

	// List orders
	huma.Register(rs.api, huma.Operation{
		OperationID: "order-list",
		Summary:     "List orders",
		Description: "Lists all orders for a tenant with pagination",
		Method:      http.MethodGet,
		Path:        "/api/tenants/{tenantUuid}/orders",
		Tags:        []string{"Orders"},
	}, rs.List)

	// Get order
	huma.Register(rs.api, huma.Operation{
		OperationID: "order-get",
		Summary:     "Get order",
		Description: "Retrieves a single order by UUID",
		Method:      http.MethodGet,
		Path:        "/api/tenants/{tenantUuid}/orders/{orderUuid}",
		Tags:        []string{"Orders"},
	}, rs.Get)

	// Update order
	huma.Register(rs.api, huma.Operation{
		OperationID: "order-update",
		Summary:     "Update order",
		Description: "Updates an existing order",
		Method:      http.MethodPatch,
		Path:        "/api/tenants/{tenantUuid}/orders/{orderUuid}",
		Tags:        []string{"Orders"},
	}, rs.Update)

	// Ship order (custom action)
	huma.Register(rs.api, huma.Operation{
		OperationID: "order-ship",
		Summary:     "Ship order",
		Description: "Marks an order as shipped",
		Method:      http.MethodPost,
		Path:        "/api/tenants/{tenantUuid}/orders/{orderUuid}/ship",
		Tags:        []string{"Orders"},
	}, rs.Ship)

	// Cancel order (custom action)
	huma.Register(rs.api, huma.Operation{
		OperationID: "order-cancel",
		Summary:     "Cancel order",
		Description: "Cancels an order",
		Method:      http.MethodPost,
		Path:        "/api/tenants/{tenantUuid}/orders/{orderUuid}/cancel",
		Tags:        []string{"Orders"},
	}, rs.Cancel)
}

// Helper to build RequestContext from HTTP context
func buildReqCtx(ctx context.Context) *domain.RequestContext {
	return &domain.RequestContext{
		SenderTenantUuid:   TenantUuidFromContext(ctx),
		SenderIdentityUuid: IdentityUuidFromContext(ctx),
		SenderAccountUuid:  AccountUuidFromContext(ctx),
		SenderSessionUuid:  SessionUuidFromContext(ctx),
		TargetTenantUuid:   TenantUuidFromContext(ctx),
	}
}

// Create handles POST /api/tenants/{tenantUuid}/orders
func (rs *OrderResource) Create(ctx context.Context, req *RequestOrderCreate) (*ResponseOrder, error) {
	// Build command from HTTP request
	cmd := domain.NewCommand(
		"Order",
		req.TenantUuid,
		buildReqCtx(ctx),
		&domain.PlaceOrderCommand{
			OrderUuid:    req.Body.OrderUuid,
			CustomerName: req.Body.CustomerName,
			Items:        req.Body.Items,
		},
	)

	// Dispatch command
	if err := rs.fc.DispatchCommand(ctx, cmd); err != nil {
		return nil, SchemaError(err)
	}

	// Return created resource using Get
	return rs.Get(ctx, &RequestOrderGet{
		TenantUuid: req.TenantUuid,
		OrderUuid:  req.Body.OrderUuid,
	})
}

// List handles GET /api/tenants/{tenantUuid}/orders
func (rs *OrderResource) List(ctx context.Context, req *RequestOrderList) (*ResponseOrderList, error) {
	qry := domain.NewQuery(
		"Order",
		req.TenantUuid,
		buildReqCtx(ctx),
		&domain.ListOrdersQuery{
			Page:     req.Page,
			PageSize: req.PageSize,
			Status:   req.Status,
		},
	)

	res, err := rs.fc.DispatchQuery(ctx, qry)
	if err != nil {
		return nil, SchemaError(err)
	}

	return &ResponseOrderList{Body: res.(*domain.OrderListResponse)}, nil
}

// Get handles GET /api/tenants/{tenantUuid}/orders/{orderUuid}
func (rs *OrderResource) Get(ctx context.Context, req *RequestOrderGet) (*ResponseOrder, error) {
	qry := domain.NewQuery(
		"Order",
		req.TenantUuid,
		buildReqCtx(ctx),
		&domain.GetOrderQuery{
			OrderUuid: req.OrderUuid,
		},
	)

	res, err := rs.fc.DispatchQuery(ctx, qry)
	if err != nil {
		return nil, SchemaError(err)
	}

	return &ResponseOrder{Body: res.(*domain.Order)}, nil
}

// Update handles PATCH /api/tenants/{tenantUuid}/orders/{orderUuid}
func (rs *OrderResource) Update(ctx context.Context, req *RequestOrderUpdate) (*ResponseOrder, error) {
	cmd := domain.NewCommand(
		"Order",
		req.TenantUuid,
		buildReqCtx(ctx),
		&domain.UpdateOrderCommand{
			OrderUuid:    req.OrderUuid,
			CustomerName: req.Body.CustomerName,
			Items:        req.Body.Items,
		},
	)

	if err := rs.fc.DispatchCommand(ctx, cmd); err != nil {
		return nil, SchemaError(err)
	}

	return rs.Get(ctx, &RequestOrderGet{
		TenantUuid: req.TenantUuid,
		OrderUuid:  req.OrderUuid,
	})
}

// Ship handles POST /api/tenants/{tenantUuid}/orders/{orderUuid}/ship
func (rs *OrderResource) Ship(ctx context.Context, req *RequestOrderAction) (*ResponseOrder, error) {
	cmd := domain.NewCommand(
		"Order",
		req.TenantUuid,
		buildReqCtx(ctx),
		&domain.ShipOrderCommand{
			OrderUuid: req.OrderUuid,
		},
	)

	if err := rs.fc.DispatchCommand(ctx, cmd); err != nil {
		return nil, SchemaError(err)
	}

	return rs.Get(ctx, &RequestOrderGet{
		TenantUuid: req.TenantUuid,
		OrderUuid:  req.OrderUuid,
	})
}

// Cancel handles POST /api/tenants/{tenantUuid}/orders/{orderUuid}/cancel
func (rs *OrderResource) Cancel(ctx context.Context, req *RequestOrderAction) (*ResponseOrder, error) {
	cmd := domain.NewCommand(
		"Order",
		req.TenantUuid,
		buildReqCtx(ctx),
		&domain.CancelOrderCommand{
			OrderUuid: req.OrderUuid,
		},
	)

	if err := rs.fc.DispatchCommand(ctx, cmd); err != nil {
		return nil, SchemaError(err)
	}

	return rs.Get(ctx, &RequestOrderGet{
		TenantUuid: req.TenantUuid,
		OrderUuid:  req.OrderUuid,
	})
}

// RegisterEndpoints registers all API endpoints and middleware
func RegisterEndpoints(fc *domain.Facade, api huma.API) {
	// Add HTTP middleware pipeline
	api.UseMiddleware(
		AuthAnonymousCtx(),
		AuthBearerToken(),
		AuthTargetCtx(),
	)

	// Add domain-level middleware
	fc.AddCommandMiddleware(LoggingCommandMiddleware())
	fc.AddQueryMiddleware(LoggingQueryMiddleware())

	// Register resources
	NewOrderResource(fc, api).Register()
}
