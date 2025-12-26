# Projections: Building Read Models

*This is Part 10 of the "Building Event-Sourced Systems in Go" series. This series originated from the development of comby – an application framework developed using the principles of Event Sourcing and Command Query Responsibility Segregation (CQRS) - which ist still closed source today. Code with concepts and techniques is presented, but unlike comby, it is not always suitable for product use.*

---

## Introduction

In Event Sourcing, the event store serves as the single source of truth for all state changes in your system. Every modification to an aggregate is captured as an immutable event, and the current state can always be reconstructed by replaying these events from the beginning. While this approach provides excellent auditability and flexibility, it comes with a significant drawback: replaying potentially thousands of events for every query would be prohibitively expensive in terms of both time and computational resources.

**Projections** solve this problem by building optimized read models — denormalized data structures specifically tailored for particular query patterns. Instead of reconstructing state on-demand, projections listen to the event stream and maintain pre-computed views that can be queried instantly.

This represents the "Query" side of CQRS (Command Query Responsibility Segregation): while commands write to the event store, queries read from these optimized projections. The separation allows each side to be optimized independently — the write side for consistency and the read side for performance.

## The Projection Concept

At its core, a projection is a specialized event handler that transforms domain events into query-optimized data structures. Unlike aggregates that enforce business rules and emit events, projections are purely reactive — they listen to events and update their internal state accordingly.

A projection performs three fundamental operations:
1. **Subscribes** to domain events — it registers interest in specific event types relevant to its purpose
2. **Processes** events to update its internal state — when an event arrives, the projection extracts relevant data and updates its read model
3. **Provides** query methods for efficient data retrieval — it exposes an API tailored to the specific queries it serves

```
Events → [Projection] → Read Model → Queries
```

Think of projections as materialized views that are incrementally updated as events occur. Unlike database materialized views that periodically refresh from source tables, projections update in near real-time as each event flows through the system. This makes them eventually consistent with the write model — there's typically a small delay between when an event is stored and when the projection reflects it, but this delay is usually measured in milliseconds.

## Basic Projection Structure

When designing a projection, you need to think about what queries it will serve and structure the data accordingly. Unlike normalized database schemas that minimize redundancy, projection read models embrace denormalization — duplicating and pre-computing data to make queries as fast as possible.

A well-designed projection typically includes:
- **Primary storage** for the main entity views
- **Secondary indexes** for different access patterns (by tenant, by status, by date range)
- **Pre-computed aggregations** for statistics and summaries
- **A reference to the event repository** for rebuilding capabilities

Here's an example of an order projection that supports multiple query patterns:

```go
type OrderProjection struct {
    // Thread-safe storage
    orders        sync.Map  // orderID -> *OrderView
    ordersByTenant sync.Map  // tenantID -> []*OrderView
    ordersByStatus sync.Map  // status -> []*OrderView

    // For rebuilding from events
    eventRepository *EventRepository
}

type OrderView struct {
    OrderID      string
    TenantID     string
    CustomerID   string
    CustomerName string     // Denormalized from Customer aggregate
    Status       string
    Items        []ItemView
    Total        float64
    ItemCount    int        // Pre-computed
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

type ItemView struct {
    ProductID   string
    ProductName string  // Denormalized
    Quantity    int
    Price       float64
    LineTotal   float64  // Pre-computed
}
```

## Implementing Event Handlers

The heart of a projection is its event handlers — functions that react to specific domain events and update the read model accordingly. Each handler receives the raw event envelope (containing metadata like aggregate ID, tenant ID, and timestamp) along with the deserialized domain event payload.

When implementing event handlers, keep these principles in mind:
- **Handlers should be idempotent** — processing the same event twice should produce the same result
- **Handlers should be fast** — they're called for every relevant event, so avoid expensive operations
- **Handlers should not throw exceptions for expected scenarios** — missing data or out-of-order events should be handled gracefully

Projections implement the EventHandler interface. The registration pattern uses generics to provide type-safe handler functions:

```go
func NewOrderProjection(eventRepository *EventRepository) *OrderProjection {
    p := &OrderProjection{
        eventRepository: eventRepository,
    }

    // Register event handlers
    comby.AddDomainEventHandler(p, p.handleOrderPlaced)
    comby.AddDomainEventHandler(p, p.handleOrderPaid)
    comby.AddDomainEventHandler(p, p.handleOrderShipped)
    comby.AddDomainEventHandler(p, p.handleOrderCancelled)
    comby.AddDomainEventHandler(p, p.handleItemAdded)

    return p
}

func (p *OrderProjection) handleOrderPlaced(ctx context.Context, evt comby.Event, domainEvt *OrderPlacedEvent) error {
    view := &OrderView{
        OrderID:     evt.AggregateUuid,
        TenantID:    evt.TenantUuid,
        CustomerID:  domainEvt.CustomerID,
        Status:      "placed",
        Total:       domainEvt.Total,
        ItemCount:   len(domainEvt.Items),
        CreatedAt:   time.Unix(0, evt.EventTime),
        UpdatedAt:   time.Unix(0, evt.EventTime),
    }

    // Convert items
    for _, item := range domainEvt.Items {
        view.Items = append(view.Items, ItemView{
            ProductID: item.ProductID,
            Quantity:  item.Quantity,
            Price:     item.Price,
            LineTotal: float64(item.Quantity) * item.Price,
        })
    }

    // Store in multiple indexes
    p.orders.Store(view.OrderID, view)
    p.addToTenantIndex(view)
    p.addToStatusIndex(view)

    return nil
}

func (p *OrderProjection) handleOrderShipped(ctx context.Context, evt comby.Event, domainEvt *OrderShippedEvent) error {
    if view, ok := p.orders.Load(evt.AggregateUuid); ok {
        orderView := view.(*OrderView)
        oldStatus := orderView.Status

        // Update view
        orderView.Status = "shipped"
        orderView.UpdatedAt = time.Unix(0, evt.EventTime)

        // Update status index
        p.removeFromStatusIndex(orderView.OrderID, oldStatus)
        p.addToStatusIndex(orderView)
    }

    return nil
}
```

## Index Management

One of the key advantages of projections is the ability to maintain multiple indexes optimized for different query patterns. In a traditional database, you might create indexes on specific columns, but with projections, you have complete control over the data structure and can organize it exactly as needed for your queries.

The challenge with in-memory indexes is maintaining consistency during concurrent updates. Go's `sync.Map` provides thread-safe access, but updating a list stored in the map requires careful handling to avoid lost updates. The compare-and-swap pattern shown below ensures atomic updates even under concurrent access:

Maintaining multiple indexes enables fast queries:

```go
func (p *OrderProjection) addToTenantIndex(view *OrderView) {
    key := view.TenantID

    for {
        existing, _ := p.ordersByTenant.Load(key)
        var list []*OrderView
        if existing != nil {
            list = existing.([]*OrderView)
        }

        newList := append(list, view)

        if existing == nil {
            if p.ordersByTenant.CompareAndSwap(key, nil, newList) {
                return
            }
        } else {
            if p.ordersByTenant.CompareAndSwap(key, existing, newList) {
                return
            }
        }
        // Retry on concurrent modification
    }
}

func (p *OrderProjection) addToStatusIndex(view *OrderView) {
    key := view.Status
    // Similar implementation
}

func (p *OrderProjection) removeFromStatusIndex(orderID, status string) {
    if existing, ok := p.ordersByStatus.Load(status); ok {
        list := existing.([]*OrderView)
        newList := make([]*OrderView, 0, len(list)-1)
        for _, v := range list {
            if v.OrderID != orderID {
                newList = append(newList, v)
            }
        }
        p.ordersByStatus.Store(status, newList)
    }
}
```

## Query Methods

The query API is where projections deliver their value — providing fast, specialized access to data without touching the event store. When designing query methods, consider returning copies of internal data rather than references to prevent external code from accidentally modifying the projection's state.

A good projection query API typically includes:
- **Get by ID** — retrieve a single entity by its primary key
- **List with filtering** — retrieve multiple entities matching certain criteria
- **List with pagination** — handle large result sets efficiently
- **Aggregate queries** — pre-computed statistics and summaries

Projections expose query methods tailored to specific use cases:

```go
func (p *OrderProjection) GetOrder(orderID string) (*OrderView, error) {
    if view, ok := p.orders.Load(orderID); ok {
        // Return a copy to prevent modification
        return copyOrderView(view.(*OrderView)), nil
    }
    return nil, ErrOrderNotFound
}

func (p *OrderProjection) ListByTenant(tenantID string, page, pageSize int) ([]*OrderView, int, error) {
    var orders []*OrderView

    if existing, ok := p.ordersByTenant.Load(tenantID); ok {
        orders = existing.([]*OrderView)
    }

    // Total count before pagination
    total := len(orders)

    // Paginate
    start := (page - 1) * pageSize
    if start >= len(orders) {
        return []*OrderView{}, total, nil
    }

    end := start + pageSize
    if end > len(orders) {
        end = len(orders)
    }

    // Return copies
    result := make([]*OrderView, end-start)
    for i, order := range orders[start:end] {
        result[i] = copyOrderView(order)
    }

    return result, total, nil
}

func (p *OrderProjection) ListByStatus(status string) []*OrderView {
    if existing, ok := p.ordersByStatus.Load(status); ok {
        orders := existing.([]*OrderView)
        result := make([]*OrderView, len(orders))
        for i, order := range orders {
            result[i] = copyOrderView(order)
        }
        return result
    }
    return []*OrderView{}
}

func (p *OrderProjection) GetStatistics(tenantID string) *OrderStats {
    var stats OrderStats

    if existing, ok := p.ordersByTenant.Load(tenantID); ok {
        orders := existing.([]*OrderView)
        stats.TotalOrders = len(orders)

        for _, order := range orders {
            stats.TotalRevenue += order.Total
            switch order.Status {
            case "placed":
                stats.PlacedCount++
            case "paid":
                stats.PaidCount++
            case "shipped":
                stats.ShippedCount++
            }
        }
    }

    return &stats
}
```

## Generic Projection for Aggregates

Writing individual event handlers for every event type can become tedious, especially when the projection simply needs to maintain the current state of aggregates. For this common case, a generic projection can automatically handle all events by replaying them onto the aggregate whenever an update occurs.

This approach trades some flexibility for convenience — you get automatic state synchronization without writing individual handlers, but you lose the ability to structure the read model differently from the aggregate itself. It's particularly useful for administrative interfaces or debugging tools where you need access to the full aggregate state.

Comby provides a generic projection that works with any aggregate:

```go
type ProjectionAggregate[T Aggregate] struct {
    *Projection[T]
    NewAggregateFunc NewAggregateFunc[T]
    eventRepository  *EventRepository
}

func NewProjectionAggregate[T Aggregate](
    fc *Facade,
    NewAggregateFunc NewAggregateFunc[T],
) *ProjectionAggregate[T] {
    pa := &ProjectionAggregate[T]{
        NewAggregateFunc: NewAggregateFunc,
        eventRepository:  fc.GetEventRepository(),
    }

    pa.Projection = NewProjection[T](NewAggregateFunc().GetDomain(), fc)

    // Register handlers for all events this aggregate produces
    for _, domainEvt := range NewAggregateFunc().GetDomainEvents() {
        // Dynamic handler registration
        pa.registerEventHandler(domainEvt)
    }

    return pa
}

func (pa *ProjectionAggregate[T]) HandleDomainEvents(ctx context.Context, evt comby.Event, domainEvt any) error {
    // Rebuild aggregate from all events
    agg := pa.NewAggregateFunc()

    events, err := pa.eventRepository.GetEventsForAggregate(ctx, evt.TenantUuid, evt.AggregateUuid, 0)
    if err != nil {
        return err
    }

    for _, e := range events {
        if err := ApplyEvent(ctx, agg, e); err != nil {
            return err
        }
    }

    // Store the rebuilt aggregate
    pa.Projection.Store(evt.AggregateUuid, agg)

    return nil
}
```

## Rebuilding Projections

One of the most powerful features of event sourcing is the ability to rebuild projections from scratch. Since the event store contains the complete history of all state changes, you can always reconstruct any read model by replaying events from the beginning.

Rebuilding is useful in several scenarios:
- **Schema changes** — when you need to add new fields or restructure the read model
- **Bug fixes** — when a bug in event handling corrupted the projection data
- **New projections** — when you add a completely new read model to an existing system
- **Data recovery** — when the projection storage fails and needs to be reconstructed

The rebuild process is straightforward: clear existing data, load all relevant events from the store, and replay them in order:

Projections can be rebuilt from the event store:

```go
func (p *OrderProjection) Rebuild(ctx context.Context) error {
    // Clear existing data
    p.orders = sync.Map{}
    p.ordersByTenant = sync.Map{}
    p.ordersByStatus = sync.Map{}

    // Load all events for this domain
    events, err := p.eventRepository.GetAllEventsForDomain(ctx, "Order")
    if err != nil {
        return err
    }

    // Replay events in order
    for _, evt := range events {
        if err := p.handleEvent(ctx, evt); err != nil {
            return err
        }
    }

    return nil
}

func (p *OrderProjection) handleEvent(ctx context.Context, evt Event) error {
    // Dispatch to appropriate handler based on event type
    switch evt.GetDomainEvtName() {
    case "OrderPlacedEvent":
        var domainEvt OrderPlacedEvent
        json.Unmarshal(evt.GetDomainEvtBytes(), &domainEvt)
        return p.handleOrderPlaced(ctx, evt, &domainEvt)
    // ... other event types
    }
    return nil
}
```

## Snapshotting for Performance

As aggregates accumulate events over time, rebuilding their state from scratch becomes increasingly expensive. An order that has been modified hundreds of times would require replaying all those events just to determine its current state. Snapshots solve this by periodically capturing the aggregate's complete state, allowing subsequent rebuilds to start from the snapshot rather than from the beginning.

The snapshotting strategy involves a trade-off between storage space and rebuild performance. More frequent snapshots mean faster rebuilds but more storage overhead. A common approach is to snapshot every N events (e.g., every 100 events), which provides predictable rebuild times regardless of total event count.

For aggregates with many events, store periodic snapshots:

```go
type ProjectionWithSnapshots[T Aggregate] struct {
    *ProjectionAggregate[T]
    snapshotStore   SnapshotStore
    snapshotEvery   int64  // Snapshot every N events
}

func (p *ProjectionWithSnapshots[T]) GetAggregate(ctx context.Context, aggregateID string) (T, error) {
    var nilT T

    // Try to load snapshot
    snapshot, snapshotVersion, err := p.snapshotStore.Load(ctx, aggregateID)
    if err != nil {
        return nilT, err
    }

    agg := p.NewAggregateFunc()

    // Apply snapshot if exists
    if snapshot != nil {
        if err := json.Unmarshal(snapshot, agg); err != nil {
            return nilT, err
        }
    }

    // Load events after snapshot
    events, err := p.eventRepository.GetEventsFromVersion(ctx, aggregateID, snapshotVersion)
    if err != nil {
        return nilT, err
    }

    // Apply remaining events
    for _, evt := range events {
        if err := ApplyEvent(ctx, agg, evt); err != nil {
            return nilT, err
        }
    }

    // Create new snapshot if needed
    if agg.GetVersion()-snapshotVersion >= p.snapshotEvery {
        go p.createSnapshot(ctx, aggregateID, agg)
    }

    return agg, nil
}

func (p *ProjectionWithSnapshots[T]) createSnapshot(ctx context.Context, aggregateID string, agg T) {
    data, err := json.Marshal(agg)
    if err != nil {
        return
    }
    p.snapshotStore.Save(ctx, aggregateID, agg.GetVersion(), data)
}
```

## Multiple Projections from Same Events

One of the most powerful aspects of event sourcing is the ability to create multiple projections from the same event stream. Different parts of your application — or different stakeholders — often need to see the same data in different ways. A customer viewing their order history has different needs than an operations team monitoring fulfillment, which has different needs than the finance team tracking revenue.

Instead of trying to create a single "universal" data model that serves all purposes poorly, you can create specialized projections that each serve their specific use case perfectly. All projections subscribe to the same events but extract and organize different aspects of the data.

Different consumers need different views:

```go
// Customer-facing view
type CustomerOrderProjection struct {
    // Shows: order status, items, estimated delivery
}

// Operations dashboard view
type OperationsOrderProjection struct {
    // Shows: order metrics, SLA compliance, processing time
}

// Finance view
type FinanceOrderProjection struct {
    // Shows: revenue, refunds, payment status
}

// All subscribe to the same events, build different views
func RegisterProjections(fc *Facade) {
    customerProj := NewCustomerOrderProjection()
    opsProj := NewOperationsOrderProjection()
    financeProj := NewFinanceOrderProjection()

    // All receive order events
    comby.RegisterEventHandler(fc, customerProj)
    comby.RegisterEventHandler(fc, opsProj)
    comby.RegisterEventHandler(fc, financeProj)
}
```

## Async vs Sync Projections

A critical architectural decision is whether projections update synchronously (within the same transaction as the command) or asynchronously (via a background process that subscribes to the event stream).

**Synchronous projections** provide strong consistency — after a command completes, the read model immediately reflects the changes. This simplifies client code since there's no need to handle eventual consistency. However, synchronous projections add latency to command processing and create coupling between the write and read paths. If a projection fails, it can block command processing entirely.

**Asynchronous projections** decouple the write and read paths, allowing commands to complete faster and projections to be updated in parallel. This approach is more scalable and resilient — a slow or failing projection doesn't impact command processing. The trade-off is eventual consistency: there's a window of time where queries might return stale data.

Most production systems use asynchronous projections for their scalability benefits, accepting the eventual consistency trade-off. For use cases that require immediate consistency (like showing a user their just-submitted order), you can either use synchronous projections selectively or implement optimistic UI updates on the client.

**Synchronous projections** update immediately:
```go
func (fc *Facade) DispatchCommand(ctx context.Context, cmd Command) ([]Event, error) {
    events, err := handler.Handle(ctx, cmd)
    if err != nil {
        return nil, err
    }

    // Save events
    fc.eventStore.Append(ctx, events)

    // Update projections synchronously
    for _, projection := range fc.projections {
        for _, evt := range events {
            projection.Handle(ctx, evt)
        }
    }

    return events, nil
}
```

**Asynchronous projections** update via event subscription:
```go
func (fc *Facade) StartProjections(ctx context.Context) {
    for _, projection := range fc.projections {
        go func(p EventHandler) {
            // Subscribe to event stream
            events := fc.eventBroker.Subscribe(p.GetDomain())

            for evt := range events {
                if err := p.Handle(ctx, evt); err != nil {
                    Logger.Error("projection error", "error", err)
                }
            }
        }(projection)
    }
}
```

## Checkpoint and Position Tracking

In production systems, restarting a projection shouldn't require replaying millions of events from the beginning. **Checkpoint tracking** (also called position tracking) solves this by recording the last successfully processed event position. When the projection restarts, it resumes from the checkpoint rather than starting over.

A robust checkpoint implementation needs to track:
- **Last processed event timestamp or sequence number** — where to resume from
- **Metrics** — how many events have been processed, applied, skipped
- **State** — whether the projection is currently restoring, running, or paused

```go
type ProjectionCheckpoint struct {
    ProjectionName     string
    LastEventTimestamp int64     // Unix nano timestamp of last processed event
    LastEventUUID      string    // UUID for deduplication
    LastEventVersion   int64     // Event sequence number
    UpdatedAt          time.Time
}

type ProjectionWithCheckpoint struct {
    *OrderProjection
    checkpointStore CheckpointStore
    checkpoint      *ProjectionCheckpoint

    // Metrics
    metrics struct {
        eventsProcessed int64
        eventsApplied   int64
        eventsSkipped   int64
        lastEventTime   int64
    }
}

func (p *ProjectionWithCheckpoint) Start(ctx context.Context) error {
    // Load last checkpoint
    checkpoint, err := p.checkpointStore.Load(ctx, p.Name())
    if err != nil {
        return err
    }
    p.checkpoint = checkpoint

    // Subscribe to events starting from checkpoint position
    var startFrom int64
    if checkpoint != nil {
        startFrom = checkpoint.LastEventTimestamp
    }

    events, err := p.eventRepository.GetEventsAfter(ctx, p.Domain(), startFrom)
    if err != nil {
        return err
    }

    // Process events
    for _, evt := range events {
        if err := p.handleEventWithCheckpoint(ctx, evt); err != nil {
            return err
        }
    }

    return nil
}

func (p *ProjectionWithCheckpoint) handleEventWithCheckpoint(ctx context.Context, evt Event) error {
    // Skip if already processed (idempotency check)
    if p.checkpoint != nil && evt.GetEventUUID() == p.checkpoint.LastEventUUID {
        p.metrics.eventsSkipped++
        return nil
    }

    // Process event
    if err := p.handleEvent(ctx, evt); err != nil {
        return err
    }

    // Update checkpoint
    p.checkpoint = &ProjectionCheckpoint{
        ProjectionName:     p.Name(),
        LastEventTimestamp: evt.GetCreatedAt(),
        LastEventUUID:      evt.GetEventUUID(),
        LastEventVersion:   evt.GetVersion(),
        UpdatedAt:          time.Now(),
    }

    // Persist checkpoint periodically (e.g., every 100 events)
    p.metrics.eventsProcessed++
    if p.metrics.eventsProcessed%100 == 0 {
        if err := p.checkpointStore.Save(ctx, p.checkpoint); err != nil {
            return err
        }
    }

    return nil
}
```

The checkpoint store can be backed by the same database as the event store, a dedicated key-value store, or even the file system for simpler deployments. The key requirement is durability — losing a checkpoint means potentially replaying many events on restart.

## State Restoration with Live Events

A subtle but critical challenge arises when restoring projection state: what happens to events that arrive while the restoration is in progress? If you simply ignore them, you'll miss data. If you process them immediately, you might apply them out of order.

The solution is a **catch-up mechanism** that buffers incoming live events during restoration and applies them after the historical replay completes. This ensures no events are lost while maintaining correct ordering.

```go
type RestorationState int

const (
    StateIdle RestorationState = iota
    StateRestoring
    StateRunning
)

type ProjectionWithRestoration struct {
    *OrderProjection

    state              RestorationState
    stateMu            sync.RWMutex

    // Buffer for events that arrive during restoration
    catchBuffer        []Event
    catchBufferMu      sync.Mutex

    lastEventTimestamp int64
    restorationDone    chan struct{}
}

func (p *ProjectionWithRestoration) OnRestoreState(ctx context.Context, fromZero bool) <-chan struct{} {
    p.restorationDone = make(chan struct{})

    go func() {
        defer close(p.restorationDone)

        // Set state to restoring
        p.stateMu.Lock()
        p.state = StateRestoring
        p.stateMu.Unlock()

        // Determine starting point
        var startFrom int64
        if !fromZero {
            startFrom = p.lastEventTimestamp
        }

        // Load historical events
        events, err := p.eventRepository.GetEventsAfter(ctx, p.Domain(), startFrom)
        if err != nil {
            Logger.Error("restoration failed", "error", err)
            return
        }

        // Process historical events
        for _, evt := range events {
            if err := p.handleEvent(ctx, evt); err != nil {
                Logger.Error("failed to handle event during restoration", "error", err)
                continue
            }
            p.lastEventTimestamp = evt.GetCreatedAt()
        }

        // Process any events that arrived during restoration
        p.catchBufferMu.Lock()
        bufferedEvents := p.catchBuffer
        p.catchBuffer = nil
        p.catchBufferMu.Unlock()

        for _, evt := range bufferedEvents {
            // Skip if already processed during restoration
            if evt.GetCreatedAt() <= p.lastEventTimestamp {
                continue
            }
            if err := p.handleEvent(ctx, evt); err != nil {
                Logger.Error("failed to handle buffered event", "error", err)
            }
        }

        // Set state to running
        p.stateMu.Lock()
        p.state = StateRunning
        p.stateMu.Unlock()
    }()

    return p.restorationDone
}

// HandleEvent is called for incoming live events
func (p *ProjectionWithRestoration) HandleEvent(ctx context.Context, evt Event) error {
    p.stateMu.RLock()
    state := p.state
    p.stateMu.RUnlock()

    switch state {
    case StateRestoring:
        // Buffer the event for later processing
        p.catchBufferMu.Lock()
        p.catchBuffer = append(p.catchBuffer, evt)
        p.catchBufferMu.Unlock()
        return nil

    case StateRunning:
        // Process normally
        return p.handleEvent(ctx, evt)

    default:
        // Projection not started yet
        return nil
    }
}
```

This pattern ensures that:
1. No events are lost during restoration
2. Events are processed in the correct order
3. The projection transitions smoothly from restoration to live processing
4. Duplicate events are handled gracefully

## Event Handler Middleware

Cross-cutting concerns like logging, metrics collection, error handling, and tracing are common across all event handlers. Rather than duplicating this logic in every handler, the **middleware pattern** allows you to wrap handlers with reusable functionality.

Middleware functions take a handler and return a new handler with additional behavior. Multiple middleware can be chained together, with each layer wrapping the next:

```go
type DomainEventHandlerFunc func(ctx context.Context, evt Event, domainEvt any) error

type EventHandlerMiddleware func(next DomainEventHandlerFunc) DomainEventHandlerFunc

// Logging middleware
func WithLogging(logger *slog.Logger) EventHandlerMiddleware {
    return func(next DomainEventHandlerFunc) DomainEventHandlerFunc {
        return func(ctx context.Context, evt Event, domainEvt any) error {
            logger.Debug("handling event",
                "eventType", evt.GetDomainEvtName(),
                "aggregateID", evt.GetAggregateUUID(),
            )

            err := next(ctx, evt, domainEvt)

            if err != nil {
                logger.Error("event handling failed",
                    "eventType", evt.GetDomainEvtName(),
                    "error", err,
                )
            }

            return err
        }
    }
}

// Metrics middleware
func WithMetrics(metrics *ProjectionMetrics) EventHandlerMiddleware {
    return func(next DomainEventHandlerFunc) DomainEventHandlerFunc {
        return func(ctx context.Context, evt Event, domainEvt any) error {
            start := time.Now()

            err := next(ctx, evt, domainEvt)

            duration := time.Since(start)
            metrics.RecordEventProcessed(evt.GetDomainEvtName(), duration, err)

            return err
        }
    }
}

// Recovery middleware (prevents panics from crashing the projection)
func WithRecovery(logger *slog.Logger) EventHandlerMiddleware {
    return func(next DomainEventHandlerFunc) DomainEventHandlerFunc {
        return func(ctx context.Context, evt Event, domainEvt any) (err error) {
            defer func() {
                if r := recover(); r != nil {
                    logger.Error("panic in event handler",
                        "eventType", evt.GetDomainEvtName(),
                        "panic", r,
                        "stack", string(debug.Stack()),
                    )
                    err = fmt.Errorf("panic recovered: %v", r)
                }
            }()

            return next(ctx, evt, domainEvt)
        }
    }
}

// Chain multiple middleware together
func ChainMiddleware(handler DomainEventHandlerFunc, middlewares ...EventHandlerMiddleware) DomainEventHandlerFunc {
    // Apply in reverse order so the first middleware is the outermost
    for i := len(middlewares) - 1; i >= 0; i-- {
        handler = middlewares[i](handler)
    }
    return handler
}

// Usage
func NewOrderProjectionWithMiddleware(logger *slog.Logger, metrics *ProjectionMetrics) *OrderProjection {
    p := &OrderProjection{}

    // Wrap handlers with middleware
    handler := ChainMiddleware(
        p.handleOrderPlaced,
        WithRecovery(logger),
        WithMetrics(metrics),
        WithLogging(logger),
    )

    comby.AddDomainEventHandler(p, handler)
    return p
}
```

The middleware pattern keeps event handlers focused on business logic while centralizing operational concerns. It also makes it easy to add new cross-cutting functionality without modifying existing handlers.

## History Event Tracking

For audit requirements, compliance, or debugging purposes, it's often valuable to track the complete history of events that affected each projected entity. This **history tracking** captures not just what changed, but who initiated the change and when.

```go
type HistoryEventModel struct {
    EventUUID          string    `json:"eventUuid"`
    DomainEvtName      string    `json:"domainEvtName"`
    DomainEvtData      string    `json:"domainEvtData"`  // JSON serialized
    Version            int64     `json:"version"`
    CreatedAt          int64     `json:"createdAt"`

    // Who initiated this change (from command context)
    SenderTenantUUID   string    `json:"senderTenantUuid,omitempty"`
    SenderIdentityUUID string    `json:"senderIdentityUuid,omitempty"`
}

type ProjectionModelWithHistory[T any] struct {
    TenantUUID    string              `json:"tenantUuid"`
    AggregateUUID string              `json:"aggregateUuid"`
    Model         T                   `json:"model"`
    Version       int64               `json:"version"`
    CreatedAt     int64               `json:"createdAt"`
    UpdatedAt     int64               `json:"updatedAt"`

    // Optional history tracking
    HistoryEvents []*HistoryEventModel `json:"historyEvents,omitempty"`
}

type ProjectionWithHistory[T Aggregate] struct {
    *ProjectionAggregate[T]
    historyEnabled    bool
    commandRepository *CommandRepository  // To look up who initiated changes
}

func (p *ProjectionWithHistory[T]) HandleDomainEvents(ctx context.Context, evt Event, domainEvt any) error {
    // Get or create model
    model, _ := p.GetModel(evt.GetAggregateUUID())
    if model == nil {
        model = &ProjectionModelWithHistory[T]{
            TenantUUID:    evt.GetTenantUUID(),
            AggregateUUID: evt.GetAggregateUUID(),
            CreatedAt:     evt.GetCreatedAt(),
        }
    }

    // Apply domain event to aggregate
    agg := p.NewAggregateFunc()
    // ... apply event logic ...
    model.Model = agg
    model.Version = evt.GetVersion()
    model.UpdatedAt = evt.GetCreatedAt()

    // Track history if enabled
    if p.historyEnabled {
        historyEntry := p.createHistoryEvent(ctx, evt, domainEvt)
        model.HistoryEvents = append(model.HistoryEvents, historyEntry)
    }

    return p.Store(evt.GetAggregateUUID(), model)
}

func (p *ProjectionWithHistory[T]) createHistoryEvent(ctx context.Context, evt Event, domainEvt any) *HistoryEventModel {
    history := &HistoryEventModel{
        EventUUID:     evt.GetEventUUID(),
        DomainEvtName: evt.GetDomainEvtName(),
        Version:       evt.GetVersion(),
        CreatedAt:     evt.GetCreatedAt(),
    }

    // Serialize domain event data
    if data, err := json.Marshal(domainEvt); err == nil {
        history.DomainEvtData = string(data)
    }

    // Look up command context to find who initiated this
    if p.commandRepository != nil {
        if cmd, err := p.commandRepository.GetByCorrelationID(ctx, evt.GetCorrelationID()); err == nil {
            history.SenderTenantUUID = cmd.SenderTenantUUID
            history.SenderIdentityUUID = cmd.SenderIdentityUUID
        }
    }

    return history
}
```

History tracking is optional and can be enabled per-projection. The query API can expose options to include or exclude history data, allowing clients to request the full audit trail only when needed.

## Store Backend Abstraction

The examples so far have used `sync.Map` for in-memory storage, which is suitable for development and testing but may not meet production requirements. A **store backend abstraction** allows projections to work with different storage implementations — memory, Redis, PostgreSQL, or custom backends — without changing the projection logic.

```go
// ReadmodelStoreBackend defines the storage interface
type ReadmodelStoreBackend interface {
    // Initialize the store with options
    Init(opts ...ReadmodelStoreOption)

    // Get retrieves a value by namespace and key
    Get(namespace string, key string) (any, error)

    // Set stores a value
    Set(namespace string, key string, value any) error

    // Remove deletes a value
    Remove(namespace string, key string) error

    // Range iterates over all values in a namespace
    Range(namespace string, fn func(key string, value any) bool)

    // Reset clears all data in the specified namespaces
    Reset(namespaces ...string) error
}

// MemoryStoreBackend is the default in-memory implementation
type MemoryStoreBackend struct {
    namespaces sync.Map  // namespace -> *sync.Map
}

func NewMemoryStoreBackend() *MemoryStoreBackend {
    return &MemoryStoreBackend{}
}

func (m *MemoryStoreBackend) getNamespace(namespace string) *sync.Map {
    ns, _ := m.namespaces.LoadOrStore(namespace, &sync.Map{})
    return ns.(*sync.Map)
}

func (m *MemoryStoreBackend) Get(namespace, key string) (any, error) {
    ns := m.getNamespace(namespace)
    if value, ok := ns.Load(key); ok {
        return value, nil
    }
    return nil, ErrNotFound
}

func (m *MemoryStoreBackend) Set(namespace, key string, value any) error {
    ns := m.getNamespace(namespace)
    ns.Store(key, value)
    return nil
}

func (m *MemoryStoreBackend) Remove(namespace, key string) error {
    ns := m.getNamespace(namespace)
    ns.Delete(key)
    return nil
}

func (m *MemoryStoreBackend) Range(namespace string, fn func(key string, value any) bool) {
    ns := m.getNamespace(namespace)
    ns.Range(func(k, v any) bool {
        return fn(k.(string), v)
    })
}

func (m *MemoryStoreBackend) Reset(namespaces ...string) error {
    for _, ns := range namespaces {
        m.namespaces.Delete(ns)
    }
    return nil
}

// Type-safe wrapper for projections
type ReadmodelStore[T any] struct {
    backend   ReadmodelStoreBackend
    namespace string
}

func NewReadmodelStore[T any](backend ReadmodelStoreBackend, namespace string) *ReadmodelStore[T] {
    return &ReadmodelStore[T]{
        backend:   backend,
        namespace: namespace,
    }
}

func (s *ReadmodelStore[T]) Get(key string) (T, bool, error) {
    var zero T
    value, err := s.backend.Get(s.namespace, key)
    if err != nil {
        if err == ErrNotFound {
            return zero, false, nil
        }
        return zero, false, err
    }
    return value.(T), true, nil
}

func (s *ReadmodelStore[T]) Set(key string, value T) error {
    return s.backend.Set(s.namespace, key, value)
}

// Usage in projection
type OrderProjectionWithBackend struct {
    store *ReadmodelStore[*OrderView]
}

func NewOrderProjectionWithBackend(backend ReadmodelStoreBackend) *OrderProjectionWithBackend {
    return &OrderProjectionWithBackend{
        store: NewReadmodelStore[*OrderView](backend, "projection:order"),
    }
}
```

The namespace pattern allows multiple projections to share the same backend while keeping their data isolated. This is particularly useful in multi-tenant systems where you might have separate namespaces per tenant, or in systems where you want to reset specific projections without affecting others.

## Running an Example

Source: https://github.com/susdorf/building-event-sourced-systems-in-go/tree/main/10-projections-building-read-models/code

When you run the complete example, you'll see projections in action — building read models from events, querying via multiple indexes, and handling state restoration:

```
=== Projections Demo ===

Event repository contains 9 events

1. Building projection from events:
   -> Projection contains 4 orders
   -> Events processed: 9

2. Query single order by ID:
   -> Order: order-1
      Customer: Alice Johnson (cust-1)
      Status: shipped
      Items: 2, Total: $1059.97
      Created: 11:46:07

3. Query orders by tenant (secondary index):
   -> Tenant A has 2 orders:
      - order-1: shipped ($1059.97)
      - order-2: placed ($149.99)
   -> Tenant B has 2 orders:
      - order-3: cancelled ($999.99)
      - order-4: paid ($319.98)

4. Query orders by status (secondary index):
   -> placed: 1 orders
   -> paid: 1 orders
   -> shipped: 1 orders
   -> cancelled: 1 orders

5. Aggregate statistics per tenant:
   -> Tenant A:
      Orders: 2, Revenue: $1209.96
      Placed: 1, Paid: 0, Shipped: 1
   -> Tenant B:
      Orders: 2, Revenue: $1319.97
      Placed: 0, Paid: 1, Cancelled: 1

6. Event handler middleware:
   Processing event through middleware chain:
      [LOG] Processing event: OrderPlacedEvent (aggregate: order-demo)
      [LOG] Event processed successfully: OrderPlacedEvent
   -> Metrics: processed=1, failed=0, avgDuration=5.042µs

7. Store backend abstraction:
   -> Get(namespace1, key1) = value1
   -> namespace1 has 2 items, namespace2 has 1 items
   -> After Reset(namespace1): Get returns error=not found

8. Type-safe store wrapper with generics:
   -> Product: Laptop - $999.99
   -> Total products: 2

9. Checkpoint tracking:
   -> Last event timestamp: 1766746447269106000
   -> Total events processed: 9
   -> After processing new event:
      Last event timestamp: 1766746567269457000
      Total events processed: 10

10. State restoration with live events:
    -> Initial state: idle
      [Restoration] Started, processing historical events...
      [Restoration] Processed 9 historical events
      [Restoration] Complete, projection is now running
      [Live Event] Processing normally: OrderPlacedEvent
    -> Final state: running
    -> Orders in projection: 5

11. Multiple projections from same events:
    Different views serve different needs:
    - CustomerOrderProjection: order status, items, delivery
    - OperationsOrderProjection: metrics, SLA compliance
    - FinanceOrderProjection: revenue, refunds, payments
    All subscribe to the same Order events!

12. Denormalized data in projections:
    Order order-4 contains:
    - CustomerName: Dan Wilson (denormalized from Customer aggregate)
    - ItemCount: 2 (pre-computed)
    - Items with LineTotal (pre-computed):
      * Monitor: 1 x $299.99 = $299.99
      * Cable: 1 x $19.99 = $19.99

13. All orders (sorted by creation time):
    [1] order-4 | tenant-B | paid | $319.98 | Dan Wilson
    [2] order-3 | tenant-B | cancelled | $999.99 | Carol Davis
    [3] order-2 | tenant-A | placed | $149.99 | Bob Smith
    [4] order-1 | tenant-A | shipped | $1059.97 | Alice Johnson

14. Event filtering middleware:
      [LOG] Processing event: OrderPlacedEvent (aggregate: o)
      [LOG] Event processed successfully: OrderPlacedEvent
      [FILTER] Skipping event: OrderShippedEvent (not in allowed types)

15. Projection rebuild:
    -> Before rebuild: 4 orders, 10 events processed
    -> After rebuild: 4 orders, 9 events processed
    Rebuild is useful for:
    - Schema changes in read model
    - Bug fixes in event handlers
    - Adding new projections to existing system

=== Demo Complete ===
```

## Summary

Projections form the foundation of the read side in a CQRS architecture. They transform the append-only event stream into query-optimized data structures that can serve application queries efficiently. Throughout this post, we've explored the key concepts and patterns for building production-ready projections:

**Core Concepts:**
- **Event handlers** process domain events and update read models
- **Denormalized views** optimize for specific query patterns, embracing data duplication for performance
- **Multiple indexes** enable different access patterns from the same underlying data
- **Generic projections** reduce boilerplate for common aggregate-to-view scenarios

**Production Essentials:**
- **Checkpoint tracking** enables incremental updates without replaying the entire event history
- **State restoration** handles the transition from historical replay to live event processing
- **Middleware** centralizes cross-cutting concerns like logging, metrics, and error recovery
- **History tracking** provides audit trails for compliance and debugging

**Architectural Choices:**
- **Sync vs async** — trade-off between consistency and performance
- **Snapshots** — periodic state captures for aggregates with many events
- **Store backend abstraction** — flexibility to use different storage implementations

The projection pattern enables fast queries without replaying events, different views for different consumers, eventually consistent read models, and clean separation of read and write concerns. When designed well, projections can evolve independently of the write model, allowing you to add new views or restructure existing ones without modifying the core domain logic.

---

## What's Next

In the next post, we'll explore **Async Command Completion** — how to wait for commands to fully complete in an eventually consistent system.
