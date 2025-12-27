// Package domain provides a simplified CQRS domain layer for demonstration.
// In a real application, this would be much more sophisticated with event sourcing,
// aggregates, projections, and proper persistence.
package domain

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Domain errors - these map to HTTP status codes in the API layer
var (
	ErrNotFound         = errors.New("not found")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrPermissionDenied = errors.New("permission denied")
	ErrConflict         = errors.New("conflict")
	ErrBadRequest       = errors.New("bad request")
)

// Order represents our domain aggregate
type Order struct {
	OrderUuid    string    `json:"orderUuid"`
	TenantUuid   string    `json:"tenantUuid"`
	CustomerName string    `json:"customerName"`
	Status       string    `json:"status"`
	Items        []Item    `json:"items"`
	Total        float64   `json:"total"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	CreatedBy    string    `json:"createdBy,omitempty"`
}

type Item struct {
	Name     string  `json:"name"`
	Quantity int     `json:"quantity"`
	Price    float64 `json:"price"`
}

// RequestContext carries authentication and routing information from HTTP to domain
type RequestContext struct {
	SenderTenantUuid   string
	SenderIdentityUuid string
	SenderAccountUuid  string
	SenderSessionUuid  string
	TargetTenantUuid   string
}

// Command represents a write operation
type Command struct {
	CommandUuid string
	Domain      string
	TenantUuid  string
	ReqCtx      *RequestContext
	Payload     any
}

// Query represents a read operation
type Query struct {
	QueryUuid  string
	Domain     string
	TenantUuid string
	ReqCtx     *RequestContext
	Payload    any
}

// --- Domain Commands ---

type PlaceOrderCommand struct {
	OrderUuid    string
	CustomerName string
	Items        []Item
}

type UpdateOrderCommand struct {
	OrderUuid    string
	CustomerName string
	Items        []Item
}

type ShipOrderCommand struct {
	OrderUuid string
}

type CancelOrderCommand struct {
	OrderUuid string
}

// --- Domain Queries ---

type GetOrderQuery struct {
	OrderUuid string
}

type ListOrdersQuery struct {
	Page     int
	PageSize int
	Status   string
}

// --- Query Responses ---

type OrderListResponse struct {
	Data     []*Order `json:"data"`
	Total    int      `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"pageSize"`
}

// Facade is the central orchestrator that routes commands and queries
type Facade struct {
	mu     sync.RWMutex
	orders map[string]*Order // tenantUuid:orderUuid -> Order

	// Middleware chains
	commandMiddlewares []CommandMiddleware
	queryMiddlewares   []QueryMiddleware

	// Metrics (simplified)
	commandCount int
	queryCount   int
}

// CommandMiddleware wraps command handling
type CommandMiddleware func(ctx context.Context, cmd *Command, next CommandHandler) error
type CommandHandler func(ctx context.Context, cmd *Command) error

// QueryMiddleware wraps query handling
type QueryMiddleware func(ctx context.Context, qry *Query, next QueryHandler) (any, error)
type QueryHandler func(ctx context.Context, qry *Query) (any, error)

// NewFacade creates a new facade instance
func NewFacade() *Facade {
	return &Facade{
		orders: make(map[string]*Order),
	}
}

// AddCommandMiddleware adds middleware to the command processing pipeline
func (f *Facade) AddCommandMiddleware(mw CommandMiddleware) {
	f.commandMiddlewares = append(f.commandMiddlewares, mw)
}

// AddQueryMiddleware adds middleware to the query processing pipeline
func (f *Facade) AddQueryMiddleware(mw QueryMiddleware) {
	f.queryMiddlewares = append(f.queryMiddlewares, mw)
}

// DispatchCommand routes a command to its handler with middleware
func (f *Facade) DispatchCommand(ctx context.Context, cmd *Command) error {
	// Build the handler chain with middleware
	handler := f.handleCommand

	// Apply middleware in reverse order so first registered runs first
	for i := len(f.commandMiddlewares) - 1; i >= 0; i-- {
		mw := f.commandMiddlewares[i]
		next := handler
		handler = func(ctx context.Context, cmd *Command) error {
			return mw(ctx, cmd, next)
		}
	}

	f.mu.Lock()
	f.commandCount++
	f.mu.Unlock()

	return handler(ctx, cmd)
}

// handleCommand is the core command handler
func (f *Facade) handleCommand(ctx context.Context, cmd *Command) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch payload := cmd.Payload.(type) {
	case *PlaceOrderCommand:
		return f.placeOrder(cmd, payload)
	case *UpdateOrderCommand:
		return f.updateOrder(cmd, payload)
	case *ShipOrderCommand:
		return f.shipOrder(cmd, payload)
	case *CancelOrderCommand:
		return f.cancelOrder(cmd, payload)
	default:
		return fmt.Errorf("unknown command type: %T", payload)
	}
}

func (f *Facade) placeOrder(cmd *Command, payload *PlaceOrderCommand) error {
	key := cmd.TenantUuid + ":" + payload.OrderUuid

	if _, exists := f.orders[key]; exists {
		return fmt.Errorf("%w: order already exists", ErrConflict)
	}

	var total float64
	for _, item := range payload.Items {
		total += item.Price * float64(item.Quantity)
	}

	order := &Order{
		OrderUuid:    payload.OrderUuid,
		TenantUuid:   cmd.TenantUuid,
		CustomerName: payload.CustomerName,
		Status:       "pending",
		Items:        payload.Items,
		Total:        total,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		CreatedBy:    cmd.ReqCtx.SenderIdentityUuid,
	}

	f.orders[key] = order
	return nil
}

func (f *Facade) updateOrder(cmd *Command, payload *UpdateOrderCommand) error {
	key := cmd.TenantUuid + ":" + payload.OrderUuid

	order, exists := f.orders[key]
	if !exists {
		return fmt.Errorf("%w: order not found", ErrNotFound)
	}

	if order.Status == "shipped" || order.Status == "cancelled" {
		return fmt.Errorf("%w: cannot update %s order", ErrBadRequest, order.Status)
	}

	order.CustomerName = payload.CustomerName
	order.Items = payload.Items

	var total float64
	for _, item := range payload.Items {
		total += item.Price * float64(item.Quantity)
	}
	order.Total = total
	order.UpdatedAt = time.Now()

	return nil
}

func (f *Facade) shipOrder(cmd *Command, payload *ShipOrderCommand) error {
	key := cmd.TenantUuid + ":" + payload.OrderUuid

	order, exists := f.orders[key]
	if !exists {
		return fmt.Errorf("%w: order not found", ErrNotFound)
	}

	if order.Status != "pending" {
		return fmt.Errorf("%w: can only ship pending orders", ErrBadRequest)
	}

	order.Status = "shipped"
	order.UpdatedAt = time.Now()
	return nil
}

func (f *Facade) cancelOrder(cmd *Command, payload *CancelOrderCommand) error {
	key := cmd.TenantUuid + ":" + payload.OrderUuid

	order, exists := f.orders[key]
	if !exists {
		return fmt.Errorf("%w: order not found", ErrNotFound)
	}

	if order.Status == "shipped" {
		return fmt.Errorf("%w: cannot cancel shipped order", ErrBadRequest)
	}

	order.Status = "cancelled"
	order.UpdatedAt = time.Now()
	return nil
}

// DispatchQuery routes a query to its handler with middleware
func (f *Facade) DispatchQuery(ctx context.Context, qry *Query) (any, error) {
	// Build the handler chain with middleware
	handler := f.handleQuery

	// Apply middleware in reverse order
	for i := len(f.queryMiddlewares) - 1; i >= 0; i-- {
		mw := f.queryMiddlewares[i]
		next := handler
		handler = func(ctx context.Context, qry *Query) (any, error) {
			return mw(ctx, qry, next)
		}
	}

	f.mu.Lock()
	f.queryCount++
	f.mu.Unlock()

	return handler(ctx, qry)
}

// handleQuery is the core query handler
func (f *Facade) handleQuery(ctx context.Context, qry *Query) (any, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	switch payload := qry.Payload.(type) {
	case *GetOrderQuery:
		return f.getOrder(qry, payload)
	case *ListOrdersQuery:
		return f.listOrders(qry, payload)
	default:
		return nil, fmt.Errorf("unknown query type: %T", payload)
	}
}

func (f *Facade) getOrder(qry *Query, payload *GetOrderQuery) (*Order, error) {
	key := qry.TenantUuid + ":" + payload.OrderUuid

	order, exists := f.orders[key]
	if !exists {
		return nil, fmt.Errorf("%w: order not found", ErrNotFound)
	}

	return order, nil
}

func (f *Facade) listOrders(qry *Query, payload *ListOrdersQuery) (*OrderListResponse, error) {
	var orders []*Order

	for key, order := range f.orders {
		// Filter by tenant
		if qry.TenantUuid != "" && order.TenantUuid != qry.TenantUuid {
			continue
		}
		// Filter by status if specified
		if payload.Status != "" && order.Status != payload.Status {
			continue
		}
		_ = key
		orders = append(orders, order)
	}

	total := len(orders)

	// Apply pagination
	page := payload.Page
	if page < 1 {
		page = 1
	}
	pageSize := payload.PageSize
	if pageSize < 1 {
		pageSize = 10
	}

	start := (page - 1) * pageSize
	end := start + pageSize

	if start >= len(orders) {
		orders = []*Order{}
	} else {
		if end > len(orders) {
			end = len(orders)
		}
		orders = orders[start:end]
	}

	return &OrderListResponse{
		Data:     orders,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// GetMetrics returns command and query counts
func (f *Facade) GetMetrics() (commands, queries int) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.commandCount, f.queryCount
}

// NewCommand creates a new command with proper initialization
func NewCommand(domain string, tenantUuid string, reqCtx *RequestContext, payload any) *Command {
	return &Command{
		CommandUuid: uuid.New().String(),
		Domain:      domain,
		TenantUuid:  tenantUuid,
		ReqCtx:      reqCtx,
		Payload:     payload,
	}
}

// NewQuery creates a new query with proper initialization
func NewQuery(domain string, tenantUuid string, reqCtx *RequestContext, payload any) *Query {
	return &Query{
		QueryUuid:  uuid.New().String(),
		Domain:     domain,
		TenantUuid: tenantUuid,
		ReqCtx:     reqCtx,
		Payload:    payload,
	}
}
