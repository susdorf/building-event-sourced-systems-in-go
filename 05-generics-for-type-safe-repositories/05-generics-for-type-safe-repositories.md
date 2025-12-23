# Generics for Type-Safe Repositories

*This is Part 5 of the "Building Event-Sourced Systems in Go" series. This series originated from the development of comby – an application framework developed using the principles of Event Sourcing and Command Query Responsibility Segregation (CQRS) - which ist still closed source today. Code with concepts and techniques is presented, but unlike comby, it is not always suitable for product use.*

---

## Introduction

Before Go 1.18, achieving type safety in generic data structures required either code generation or runtime type assertions. With generics, we can now build truly type-safe repositories that catch errors at compile time.

Knowing and applying the right patterns — such as Generic Repositories, Functional Options, or type-safe wrappers — is crucial for maintainable codebases. Experience from multiple projects has shown that without these foundational structures, technical debt accumulates rapidly. What starts as a quick fix becomes a maze of type assertions, scattered configuration, and tangled dependencies. Before long, even small changes require touching multiple files, understanding implicit contracts, and hoping nothing breaks. Investing in proper abstractions upfront pays dividends in reduced maintenance burden.

In this post, we'll explore how to use generics to create repositories that work with any aggregate type while maintaining full type safety.

## The Problem: Type Assertions Everywhere

The `Aggregate` interface shown below originates from Domain-Driven Design and Event Sourcing terminology. While naming an interface after a noun like "Aggregate" doesn't fully align with idiomatic Go style (which typically favors verb-based names like `Reader` or `Handler`), this is a pragmatic choice found in real-world systems. The DDD terminology is so widely recognized that deviating from it would create more confusion than it solves.

Pre-generics, a generic repository looked like this:

```go
// Generic interface
type Aggregate interface {
    GetID() string
    GetVersion() int64
}

type Repository struct {
    eventStore EventStore
}

func (r *Repository) Load(ctx context.Context, id string) (Aggregate, error) {
    // Returns interface{} - no type safety!
    events, err := r.eventStore.Load(ctx, id)
    if err != nil {
        return nil, err
    }
    // How do we know which aggregate type to create?
    // We need external information or type switches
    return nil, errors.New("can't determine aggregate type")
}

func (r *Repository) Save(ctx context.Context, agg Aggregate) error {
    // Works, but we lose type information
    return r.eventStore.Append(ctx, agg.GetID(), agg.GetUncommittedEvents())
}
```

Using this repository required type assertions:

```go
func ProcessOrder(ctx context.Context, repo *Repository, orderID string) error {
    agg, err := repo.Load(ctx, orderID)
    if err != nil {
        return err
    }

    // Must assert type - runtime error if wrong!
    order, ok := agg.(*Order)
    if !ok {
        return errors.New("expected Order aggregate")
    }

    return order.Process()
}
```

## The Solution: Generic Repository

With generics, we can create a type-safe repository:

```go
// Aggregate is the constraint for all aggregates
type Aggregate interface {
    GetID() string
    GetVersion() int64
    GetDomain() string
    GetUncommittedEvents() []Event
    LoadFromHistory(events []Event)
}

// NewAggregateFunc creates a new instance of an aggregate
type NewAggregateFunc[T Aggregate] func() T

// AggregateRepository is a generic repository for any aggregate type
type AggregateRepository[T Aggregate] struct {
    EventStore       EventStore
    NewAggregateFunc NewAggregateFunc[T]
}

// NewAggregateRepository creates a typed repository
func NewAggregateRepository[T Aggregate](
    eventStore EventStore,
    newFunc NewAggregateFunc[T],
) *AggregateRepository[T] {
    return &AggregateRepository[T]{
        EventStore:       eventStore,
        NewAggregateFunc: newFunc,
    }
}
```

### Why NewAggregateFunc?

You might wonder why we need to pass a factory function like `NewAggregateFunc`. The reason is twofold:

First, Go generics cannot instantiate type parameters directly — `new(T)` when `T` is `*Order` gives you `**Order`, not what you need.

Second, and more importantly, creating an aggregate involves more than just allocating memory. A properly initialized aggregate requires:

```go
func NewOrder() *Order {
    agg := &Order{}
    agg.BaseAggregate = NewBaseAggregate()
    agg.Domain = "Order"

    // Register domain event handlers - essential for event replay!
    AddDomainEventHandler(agg, agg.OnOrderCreated)
    AddDomainEventHandler(agg, agg.OnOrderShipped)
    AddDomainEventHandler(agg, agg.OnOrderCancelled)

    return agg
}
```

The factory function initializes the base aggregate, sets the domain, and — critically — registers all domain event handlers. Without these handlers, the repository cannot replay events to reconstruct state. The repository has no knowledge of which handlers exist for a given aggregate type; only the aggregate itself knows its event handlers.

This pattern delegates initialization responsibility to the aggregate, where it belongs.

## Loading Aggregates with Type Safety

The `Load` method returns the concrete type:

```go
func (ar *AggregateRepository[T]) GetAggregate(
    ctx context.Context,
    aggregateID string,
) (T, error) {
    var nilT T  // Zero value for return on error

    // Load events
    events, err := ar.EventStore.Load(ctx, aggregateID)
    if err != nil {
        return nilT, fmt.Errorf("failed to load events: %w", err)
    }

    if len(events) == 0 {
        return nilT, nil  // Aggregate doesn't exist
    }

    // Create new aggregate using the factory function
    agg := ar.NewAggregateFunc()

    // Replay events
    agg.LoadFromHistory(events)

    return agg, nil
}
```

## Usage: No Type Assertions Needed

```go
// Create typed repository
orderRepo := NewAggregateRepository[*Order](
    eventStore,
    NewOrder,  // Factory function
)

// Load returns *Order directly - no assertion needed!
order, err := orderRepo.GetAggregate(ctx, "order-123")
if err != nil {
    return err
}

// order is *Order, not Aggregate interface
// Full IDE support, compile-time checking
err = order.Ship("TRACK-456")
```

## Handling the Zero Value Problem

When an aggregate doesn't exist, we need to return something. With generics, we use the zero value:

```go
func (ar *AggregateRepository[T]) GetAggregate(ctx context.Context, id string) (T, error) {
    var nilT T  // Zero value of T

    events, err := ar.EventStore.Load(ctx, id)
    if err != nil {
        return nilT, err
    }

    if len(events) == 0 {
        // Return zero value (nil for pointer types)
        return nilT, nil
    }

    agg := ar.NewAggregateFunc()
    agg.LoadFromHistory(events)
    return agg, nil
}

// Caller checks for nil
order, err := orderRepo.GetAggregate(ctx, orderID)
if err != nil {
    return err
}
if order == nil {
    return ErrOrderNotFound
}
```

## Advanced: Options with Generics

We can combine functional options with generic repositories:

```go
type AggregateRepositoryGetOptions struct {
    Version        int64
    SkipCache      bool
    IncludeDeleted bool
}

type AggregateRepositoryGetOption func(opt *AggregateRepositoryGetOptions) (*AggregateRepositoryGetOptions, error)

func WithVersion(version int64) AggregateRepositoryGetOption {
    return func(opt *AggregateRepositoryGetOptions) (*AggregateRepositoryGetOptions, error) {
        opt.Version = version
        return opt, nil
    }
}

func WithSkipCache() AggregateRepositoryGetOption {
    return func(opt *AggregateRepositoryGetOptions) (*AggregateRepositoryGetOptions, error) {
        opt.SkipCache = true
        return opt, nil
    }
}

func (ar *AggregateRepository[T]) GetAggregate(
    ctx context.Context,
    aggregateID string,
    opts ...AggregateRepositoryGetOption,
) (T, error) {
    var nilT T

    // Apply options
    options := &AggregateRepositoryGetOptions{}
    for _, opt := range opts {
        var err error
        options, err = opt(options)
        if err != nil {
            return nilT, err
        }
    }

    // Load events based on options
    var events []Event
    var err error

    if options.Version > 0 {
        events, err = ar.EventStore.LoadUpToVersion(ctx, aggregateID, options.Version)
    } else {
        events, err = ar.EventStore.Load(ctx, aggregateID)
    }

    if err != nil {
        return nilT, err
    }

    if len(events) == 0 {
        return nilT, nil
    }

    agg := ar.NewAggregateFunc()
    agg.LoadFromHistory(events)

    return agg, nil
}

// Usage
order, err := orderRepo.GetAggregate(ctx, orderID,
    WithVersion(5),
    WithSkipCache(),
)
```

## Generic Projections

Generics also work great for read models:

```go
// Projection is a generic read model
type Projection[T any] struct {
    items    map[string]T
    byTenant map[string][]T
    mu       sync.RWMutex
}

func NewProjection[T any]() *Projection[T] {
    return &Projection[T]{
        items:    make(map[string]T),
        byTenant: make(map[string][]T),
    }
}

func (p *Projection[T]) Store(id string, item T) {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.items[id] = item
}

func (p *Projection[T]) Get(id string) (T, bool) {
    p.mu.RLock()
    defer p.mu.RUnlock()
    item, ok := p.items[id]
    return item, ok
}

func (p *Projection[T]) List() []T {
    p.mu.RLock()
    defer p.mu.RUnlock()

    result := make([]T, 0, len(p.items))
    for _, item := range p.items {
        result = append(result, item)
    }
    return result
}
```

Usage:

```go
// Typed projection for orders
type OrderView struct {
    ID         string
    CustomerID string
    Status     string
    Total      float64
}

orderProjection := NewProjection[*OrderView]()

// Store - type checked
orderProjection.Store("order-123", &OrderView{
    ID:     "order-123",
    Status: "placed",
})

// Get returns *OrderView directly
view, ok := orderProjection.Get("order-123")
if ok {
    fmt.Println(view.Status)  // No assertion needed
}
```

## Generic Query Handlers

Query handlers can also benefit from generics:

```go
type QueryHandler[Q any, R any] interface {
    Handle(ctx context.Context, query Q) (R, error)
}

// Specific implementation
type OrderQueryHandler struct {
    projection *Projection[*OrderView]
}

func (h *OrderQueryHandler) Handle(ctx context.Context, query GetOrderQuery) (*OrderView, error) {
    view, ok := h.projection.Get(query.OrderID)
    if !ok {
        return nil, ErrOrderNotFound
    }
    return view, nil
}

// List query returns slice
type ListOrdersQueryHandler struct {
    projection *Projection[*OrderView]
}

func (h *ListOrdersQueryHandler) Handle(ctx context.Context, query ListOrdersQuery) ([]*OrderView, error) {
    return h.projection.List(), nil
}
```

## Real-World Example from Comby

Here's how comby implements the generic aggregate repository:

```go
// From comby/repository.aggregate.go
type AggregateRepository[T Aggregate] struct {
    EventRepository  *EventRepository
    NewAggregateFunc NewAggregateFunc[T]
}

func NewAggregateRepository[T Aggregate](
    fc *Facade,
    NewTypedAggregateFunc NewAggregateFunc[T],
) *AggregateRepository[T] {
    return &AggregateRepository[T]{
        EventRepository:  fc.GetEventRepository(),
        NewAggregateFunc: NewTypedAggregateFunc,
    }
}

func (ar *AggregateRepository[T]) GetAggregate(
    ctx context.Context,
    aggregateUuid string,
    opts ...AggregateRepositoryGetOption,
) (T, error) {
    var nilT T

    // Apply options
    options := &AggregateRepositoryGetOptions{
        Version: 0,
    }
    for _, opt := range opts {
        var err error
        options, err = opt(options)
        if err != nil {
            return nilT, err
        }
    }

    // Determine tenant from context (skipped)
    tenantUuid := "..."

    // Get events with optional version limit
    evts, err := ar.EventRepository.GetEventsForAggregate(
        ctx,
        tenantUuid,
        aggregateUuid,
        options.Version,
    )
    if err != nil {
        return nilT, err
    }
    if len(evts) == 0 {
        return nilT, nil
    }

    // Create and hydrate aggregate
    agg := ar.NewAggregateFunc()
    for _, evt := range evts {
        if err := ApplyEvent(ctx, agg, evt); err != nil {
            return nilT, err
        }
    }

    return agg, nil
}
```

## Generic SyncMap

For thread-safe storage with generics:

```go
// SyncMap is a generic thread-safe map
type SyncMap[K comparable, V any] struct {
    internalMap sync.Map
}

func (m *SyncMap[K, V]) Store(key K, value V) {
    m.internalMap.Store(key, value)
}

func (m *SyncMap[K, V]) Load(key K) (V, bool) {
    value, ok := m.internalMap.Load(key)
    if !ok {
        var zeroValue V
        return zeroValue, false
    }
    return value.(V), true
}

func (m *SyncMap[K, V]) Delete(key K) {
    m.internalMap.Delete(key)
}

func (m *SyncMap[K, V]) Range(f func(key K, value V) bool) {
    m.internalMap.Range(func(k, v interface{}) bool {
        return f(k.(K), v.(V))
    })
}

// Usage
var orders SyncMap[string, *Order]
orders.Store("order-123", order)

order, ok := orders.Load("order-123")  // Returns *Order, not interface{}
```

## Benefits of Generic Repositories

1. **Compile-time type safety**: Errors caught before runtime
2. **IDE support**: Full autocomplete and navigation
3. **No type assertions**: Cleaner, safer code
4. **Reusable patterns**: Same repository works for any aggregate
5. **Self-documenting**: Types serve as documentation

## Running an Example

Source: https://github.com/susdorf/building-event-sourced-systems-in-go/tree/main/05-generics-for-type-safe-repositories/code

When you run the complete example, you'll see generic repositories in action:

```
=== Generic Repository Demo ===

1. Creating typed repositories:
   orderRepo := NewAggregateRepository[*Order](eventStore, NewOrder)
   customerRepo := NewAggregateRepository[*Customer](eventStore, NewCustomer)
   -> Both repositories share the same event store
   -> Each is typed to its specific aggregate

2. Create and save an Order aggregate:
   -> Order created with ID: order-001
   -> Customer: customer-123
   -> Items: 2
   -> Total: $109.97
   -> Status: confirmed
   -> Uncommitted events: 4
   -> Saved! Events in store: 4

3. Load Order - returns *Order, not interface{}:
   // No type assertion needed!
   order, _ := orderRepo.GetAggregate("order-001")
   order.Ship("TRACK-123")  // Direct method call

   -> Loaded order: order-001
   -> Customer: customer-123 (directly accessible, no assertion)
   -> Status: confirmed
   -> Total: $109.97
   -> Shipped! New status: shipped

4. Type safety - compile-time checks:
   // This would NOT compile:
   // order, _ := orderRepo.GetAggregate("order-001")
   // order.UpgradeToVIP()  // ERROR: *Order has no method UpgradeToVIP

   // Customer methods only work with customerRepo:
   // customer, _ := customerRepo.GetAggregate("cust-001")
   // customer.UpgradeToVIP()  // OK: *Customer has UpgradeToVIP

5. Work with Customer aggregate (different type, same pattern):
   -> Customer: Alice Smith
   -> Email: alice@example.com
   -> VIP: true
   -> Linked orders: [order-001]

6. Load Customer - also returns concrete type:
   -> Name: Alice Smith (direct access)
   -> VIP: true (direct access)
   -> Orders: [order-001] (direct access)

7. Temporal query - load order at version 2 (before confirmation):
   -> Order at v2: status=pending, items=1
   -> (Version 3 was the confirmation)

8. Domain mismatch protection:
   // Trying to load an Order ID with customerRepo would fail:
   // customer, err := customerRepo.GetAggregate("order-001")
   // -> Error: domain mismatch: expected Customer, got Order
   -> Actual error: domain mismatch: expected Customer, got Order

9. Contrast - the old (non-generic) way:
   // agg, _ := nonGenericRepo.Load("order-001")
   // order, ok := agg.(*Order)  // Manual type assertion!
   // if !ok {
   //     return errors.New("expected Order")
   // }
   // order.Ship("TRACK-123")

   With generics, the type assertion happens at compile time,
   not runtime. The compiler ensures type safety.

10. Summary of benefits:
    - No type assertions needed when loading aggregates
    - Full IDE autocomplete for aggregate methods
    - Compile-time type checking prevents mixing aggregate types
    - Same repository pattern works for any aggregate
    - Factory function (NewAggregateFunc) ensures proper initialization

Total events in store: 8

=== Demo Complete ===
```

## Summary

Go generics enable type-safe repositories that:
- Return concrete types instead of interfaces
- Catch type errors at compile time
- Provide full IDE support
- Reduce boilerplate from type assertions
- Work seamlessly with functional options

The pattern is simple:
1. Define a type constraint (interface) for your aggregates
2. Create a factory function type `NewAggregateFunc[T]`
3. Build the repository with a type parameter `AggregateRepository[T]`
4. Methods return `T` instead of interface types

---

## What's Next

In the next post, we'll explore **Generic Wrappers** — a powerful technique for providing type-safe interfaces to non-generic systems. This pattern is essential when integrating type-safe handlers with flexible, dynamic dispatching.