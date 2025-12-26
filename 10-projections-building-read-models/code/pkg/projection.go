package pkg

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

// OrderView is the denormalized read model for orders
type OrderView struct {
	OrderID      string
	TenantID     string
	CustomerID   string
	CustomerName string
	Status       string
	Items        []ItemView
	Total        float64
	ItemCount    int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ItemView struct {
	ProductID   string
	ProductName string
	Quantity    int
	Price       float64
	LineTotal   float64
}

// OrderStats provides aggregate statistics
type OrderStats struct {
	TotalOrders    int
	TotalRevenue   float64
	PlacedCount    int
	PaidCount      int
	ShippedCount   int
	CancelledCount int
}

// OrderProjection maintains denormalized order views
type OrderProjection struct {
	mu              sync.RWMutex
	store           *ReadmodelStore[*OrderView]
	ordersByTenant  map[string][]string // tenantID -> []orderID
	ordersByStatus  map[string][]string // status -> []orderID
	eventRepository *EventRepository

	// Checkpoint tracking
	lastEventTimestamp int64
	eventsProcessed    int64

	// Middleware
	middlewares []EventHandlerMiddleware
}

func NewOrderProjection(repo *EventRepository, backend ReadmodelStoreBackend) *OrderProjection {
	return &OrderProjection{
		store:           NewReadmodelStore[*OrderView](backend, "projection:order"),
		ordersByTenant:  make(map[string][]string),
		ordersByStatus:  make(map[string][]string),
		eventRepository: repo,
	}
}

// WithMiddleware adds middleware to the projection
func (p *OrderProjection) WithMiddleware(mw ...EventHandlerMiddleware) *OrderProjection {
	p.middlewares = append(p.middlewares, mw...)
	return p
}

// HandleEvent processes a single event through the middleware chain
func (p *OrderProjection) HandleEvent(ctx context.Context, evt Event) error {
	handler := p.handleEventInternal

	// Apply middleware in reverse order
	for i := len(p.middlewares) - 1; i >= 0; i-- {
		handler = p.middlewares[i](handler)
	}

	return handler(ctx, evt)
}

func (p *OrderProjection) handleEventInternal(ctx context.Context, evt Event) error {
	return p.handleEventInternalLocked(ctx, evt)
}

func (p *OrderProjection) handleEventInternalLocked(ctx context.Context, evt Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.handleEventUnlocked(ctx, evt)
}

func (p *OrderProjection) handleEventUnlocked(ctx context.Context, evt Event) error {
	var err error
	switch evt.DataType {
	case "OrderPlacedEvent":
		var domainEvt OrderPlacedEvent
		json.Unmarshal([]byte(evt.Data), &domainEvt)
		err = p.handleOrderPlaced(evt, &domainEvt)
	case "OrderPaidEvent":
		err = p.handleOrderPaid(evt)
	case "OrderShippedEvent":
		err = p.handleOrderShipped(evt)
	case "OrderCancelledEvent":
		err = p.handleOrderCancelled(evt)
	case "ItemAddedEvent":
		var domainEvt ItemAddedEvent
		json.Unmarshal([]byte(evt.Data), &domainEvt)
		err = p.handleItemAdded(evt, &domainEvt)
	}

	if err == nil {
		p.lastEventTimestamp = evt.CreatedAt
		p.eventsProcessed++
	}

	return err
}

func (p *OrderProjection) handleOrderPlaced(evt Event, domainEvt *OrderPlacedEvent) error {
	view := &OrderView{
		OrderID:      evt.AggregateUUID,
		TenantID:     evt.TenantUUID,
		CustomerID:   domainEvt.CustomerID,
		CustomerName: domainEvt.CustomerName,
		Status:       "placed",
		Total:        domainEvt.Total,
		ItemCount:    len(domainEvt.Items),
		CreatedAt:    time.Unix(0, evt.CreatedAt),
		UpdatedAt:    time.Unix(0, evt.CreatedAt),
	}

	for _, item := range domainEvt.Items {
		view.Items = append(view.Items, ItemView{
			ProductID:   item.ProductID,
			ProductName: item.ProductName,
			Quantity:    item.Quantity,
			Price:       item.Price,
			LineTotal:   float64(item.Quantity) * item.Price,
		})
	}

	p.store.Set(view.OrderID, view)
	p.addToTenantIndex(view.TenantID, view.OrderID)
	p.addToStatusIndex("placed", view.OrderID)

	return nil
}

func (p *OrderProjection) handleOrderPaid(evt Event) error {
	view, found, _ := p.store.Get(evt.AggregateUUID)
	if !found {
		return nil
	}

	p.removeFromStatusIndex(view.Status, view.OrderID)
	view.Status = "paid"
	view.UpdatedAt = time.Unix(0, evt.CreatedAt)
	p.addToStatusIndex("paid", view.OrderID)

	return nil
}

func (p *OrderProjection) handleOrderShipped(evt Event) error {
	view, found, _ := p.store.Get(evt.AggregateUUID)
	if !found {
		return nil
	}

	p.removeFromStatusIndex(view.Status, view.OrderID)
	view.Status = "shipped"
	view.UpdatedAt = time.Unix(0, evt.CreatedAt)
	p.addToStatusIndex("shipped", view.OrderID)

	return nil
}

func (p *OrderProjection) handleOrderCancelled(evt Event) error {
	view, found, _ := p.store.Get(evt.AggregateUUID)
	if !found {
		return nil
	}

	p.removeFromStatusIndex(view.Status, view.OrderID)
	view.Status = "cancelled"
	view.UpdatedAt = time.Unix(0, evt.CreatedAt)
	p.addToStatusIndex("cancelled", view.OrderID)

	return nil
}

func (p *OrderProjection) handleItemAdded(evt Event, domainEvt *ItemAddedEvent) error {
	view, found, _ := p.store.Get(evt.AggregateUUID)
	if !found {
		return nil
	}

	newItem := ItemView{
		ProductID:   domainEvt.ProductID,
		ProductName: domainEvt.ProductName,
		Quantity:    domainEvt.Quantity,
		Price:       domainEvt.Price,
		LineTotal:   float64(domainEvt.Quantity) * domainEvt.Price,
	}

	view.Items = append(view.Items, newItem)
	view.ItemCount = len(view.Items)
	view.Total += newItem.LineTotal
	view.UpdatedAt = time.Unix(0, evt.CreatedAt)

	return nil
}

// Index management
func (p *OrderProjection) addToTenantIndex(tenantID, orderID string) {
	p.ordersByTenant[tenantID] = append(p.ordersByTenant[tenantID], orderID)
}

func (p *OrderProjection) addToStatusIndex(status, orderID string) {
	p.ordersByStatus[status] = append(p.ordersByStatus[status], orderID)
}

func (p *OrderProjection) removeFromStatusIndex(status, orderID string) {
	orders := p.ordersByStatus[status]
	for i, id := range orders {
		if id == orderID {
			p.ordersByStatus[status] = append(orders[:i], orders[i+1:]...)
			break
		}
	}
}

// Query methods

func (p *OrderProjection) GetOrder(orderID string) (*OrderView, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	view, found, _ := p.store.Get(orderID)
	if !found {
		return nil, false
	}
	return copyOrderView(view), true
}

func (p *OrderProjection) ListByTenant(tenantID string) []*OrderView {
	p.mu.RLock()
	defer p.mu.RUnlock()

	orderIDs := p.ordersByTenant[tenantID]
	result := make([]*OrderView, 0, len(orderIDs))

	for _, orderID := range orderIDs {
		if view, found, _ := p.store.Get(orderID); found {
			result = append(result, copyOrderView(view))
		}
	}

	return result
}

func (p *OrderProjection) ListByStatus(status string) []*OrderView {
	p.mu.RLock()
	defer p.mu.RUnlock()

	orderIDs := p.ordersByStatus[status]
	result := make([]*OrderView, 0, len(orderIDs))

	for _, orderID := range orderIDs {
		if view, found, _ := p.store.Get(orderID); found {
			result = append(result, copyOrderView(view))
		}
	}

	return result
}

func (p *OrderProjection) GetStatistics(tenantID string) *OrderStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := &OrderStats{}

	orderIDs := p.ordersByTenant[tenantID]
	stats.TotalOrders = len(orderIDs)

	for _, orderID := range orderIDs {
		if view, found, _ := p.store.Get(orderID); found {
			stats.TotalRevenue += view.Total
			switch view.Status {
			case "placed":
				stats.PlacedCount++
			case "paid":
				stats.PaidCount++
			case "shipped":
				stats.ShippedCount++
			case "cancelled":
				stats.CancelledCount++
			}
		}
	}

	return stats
}

func (p *OrderProjection) ListAll() []*OrderView {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var result []*OrderView
	p.store.Range(func(key string, view *OrderView) bool {
		result = append(result, copyOrderView(view))
		return true
	})

	// Sort by CreatedAt descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	return result
}

func (p *OrderProjection) Count() int {
	return p.store.Count()
}

// Rebuild rebuilds the projection from scratch
func (p *OrderProjection) Rebuild(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Clear existing data
	p.store = NewReadmodelStore[*OrderView](NewMemoryStoreBackend(), "projection:order")
	p.ordersByTenant = make(map[string][]string)
	p.ordersByStatus = make(map[string][]string)
	p.lastEventTimestamp = 0
	p.eventsProcessed = 0

	// Load all events
	events := p.eventRepository.GetAllEventsForDomain("Order")

	// Sort by CreatedAt to ensure correct order
	sort.Slice(events, func(i, j int) bool {
		return events[i].CreatedAt < events[j].CreatedAt
	})

	// Replay events (use unlocked version since we already hold the lock)
	for _, evt := range events {
		p.handleEventUnlocked(ctx, evt)
	}

	return nil
}

// GetCheckpoint returns the last processed event timestamp
func (p *OrderProjection) GetCheckpoint() int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastEventTimestamp
}

// GetEventsProcessed returns the number of events processed
func (p *OrderProjection) GetEventsProcessed() int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.eventsProcessed
}

// Helper to create a copy of OrderView
func copyOrderView(v *OrderView) *OrderView {
	if v == nil {
		return nil
	}
	copy := *v
	copy.Items = make([]ItemView, len(v.Items))
	for i, item := range v.Items {
		copy.Items[i] = item
	}
	return &copy
}

// ProjectionWithRestoration demonstrates state restoration with live events
type ProjectionWithRestoration struct {
	*OrderProjection
	state           RestorationState
	stateMu         sync.RWMutex
	catchBuffer     []Event
	catchBufferMu   sync.Mutex
	restorationDone chan struct{}
}

type RestorationState int

const (
	StateIdle RestorationState = iota
	StateRestoring
	StateRunning
)

func NewProjectionWithRestoration(repo *EventRepository, backend ReadmodelStoreBackend) *ProjectionWithRestoration {
	return &ProjectionWithRestoration{
		OrderProjection: NewOrderProjection(repo, backend),
		state:           StateIdle,
	}
}

func (p *ProjectionWithRestoration) GetState() RestorationState {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	return p.state
}

func (p *ProjectionWithRestoration) GetStateString() string {
	switch p.GetState() {
	case StateIdle:
		return "idle"
	case StateRestoring:
		return "restoring"
	case StateRunning:
		return "running"
	default:
		return "unknown"
	}
}

func (p *ProjectionWithRestoration) OnRestoreState(ctx context.Context, fromZero bool) <-chan struct{} {
	p.restorationDone = make(chan struct{})

	go func() {
		defer close(p.restorationDone)

		// Set state to restoring
		p.stateMu.Lock()
		p.state = StateRestoring
		p.stateMu.Unlock()

		fmt.Println("      [Restoration] Started, processing historical events...")

		// Determine starting point
		var startFrom int64
		if !fromZero {
			startFrom = p.OrderProjection.lastEventTimestamp
		}

		// Load historical events
		events := p.eventRepository.GetEventsAfter("Order", startFrom)

		// Sort events
		sort.Slice(events, func(i, j int) bool {
			return events[i].CreatedAt < events[j].CreatedAt
		})

		// Process historical events
		for _, evt := range events {
			p.OrderProjection.handleEventInternal(ctx, evt)
		}

		fmt.Printf("      [Restoration] Processed %d historical events\n", len(events))

		// Process any events that arrived during restoration
		p.catchBufferMu.Lock()
		bufferedEvents := p.catchBuffer
		p.catchBuffer = nil
		p.catchBufferMu.Unlock()

		if len(bufferedEvents) > 0 {
			fmt.Printf("      [Restoration] Processing %d buffered live events\n", len(bufferedEvents))
			for _, evt := range bufferedEvents {
				if evt.CreatedAt <= p.OrderProjection.lastEventTimestamp {
					continue
				}
				p.OrderProjection.handleEventInternal(ctx, evt)
			}
		}

		// Set state to running
		p.stateMu.Lock()
		p.state = StateRunning
		p.stateMu.Unlock()

		fmt.Println("      [Restoration] Complete, projection is now running")
	}()

	return p.restorationDone
}

func (p *ProjectionWithRestoration) HandleLiveEvent(ctx context.Context, evt Event) error {
	p.stateMu.RLock()
	state := p.state
	p.stateMu.RUnlock()

	switch state {
	case StateRestoring:
		// Buffer the event for later processing
		p.catchBufferMu.Lock()
		p.catchBuffer = append(p.catchBuffer, evt)
		p.catchBufferMu.Unlock()
		fmt.Printf("      [Live Event] Buffered during restoration: %s\n", evt.DataType)
		return nil

	case StateRunning:
		// Process normally
		fmt.Printf("      [Live Event] Processing normally: %s\n", evt.DataType)
		return p.OrderProjection.HandleEvent(ctx, evt)

	default:
		fmt.Printf("      [Live Event] Ignored (projection idle): %s\n", evt.DataType)
		return nil
	}
}
