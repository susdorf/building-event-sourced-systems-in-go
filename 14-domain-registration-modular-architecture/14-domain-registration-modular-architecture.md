# Domain Registration: Modular Architecture

*This is Part 14 of the "Building Event-Sourced Systems in Go" series. This series originated from the development of comby – an application framework developed using the principles of Event Sourcing and Command Query Responsibility Segregation (CQRS) - which ist still closed source today. Code with concepts and techniques is presented, but unlike comby, it is not always suitable for product use.*

---

## Introduction

As event-sourced applications grow, managing the complexity of aggregates, command handlers, query handlers, and read models becomes increasingly challenging. Without a clear organizational strategy, codebases can quickly become tangled webs of dependencies where changes in one area ripple unpredictably through others.

A well-structured event-sourced application organizes code into **domains** — bounded contexts with clear responsibilities. Each domain represents a cohesive business capability: an Order domain handles everything related to orders, a Customer domain manages customer data, and an Inventory domain tracks stock levels. This separation ensures that each domain can evolve independently while maintaining clear contracts with other parts of the system.

**Domain Registration** is the architectural pattern that connects all these pieces to the central facade. It defines how aggregates, handlers, and read models are discovered, validated, and wired together at application startup. A clean registration pattern provides several benefits:

- **Discoverability**: New developers can quickly understand what capabilities exist by looking at registration functions
- **Consistency**: All domains follow the same registration pattern, reducing cognitive load
- **Validation**: Registration can verify that all required components are present and correctly configured
- **Testability**: Individual domains can be registered in isolation for unit testing
- **Extensibility**: Adding new domains requires no changes to existing code

This post explores how to implement a robust domain registration system that scales with your application's complexity.

## Domain Package Structure

A well-organized domain follows a consistent package structure that separates concerns and makes navigation intuitive. Each domain is a self-contained package with clearly defined subpackages for different responsibilities:

```
domain/
├── domain.go                 # Central registration entry point
└── order/                    # Order domain
    ├── aggregate/
    │   ├── aggregate.go      # Order aggregate with state and behavior
    │   └── events.go         # Domain events produced by this aggregate
    ├── command/
    │   ├── commands.go       # Command definitions (DTOs)
    │   └── handler.go        # Command handler implementation
    ├── query/
    │   ├── queries.go        # Query definitions (DTOs)
    │   └── handler.go        # Query handler implementation
    ├── readmodel/
    │   └── readmodel.go      # Projections for query optimization
    ├── reactor/              # Optional: side effects and integrations
    │   └── reactor.go
    └── register.go           # Domain registration function
```

This structure reflects the CQRS pattern directly in the file system. The `aggregate/` package contains the write model — the source of truth that processes commands and produces events. The `readmodel/` package contains projections optimized for specific query patterns. The `command/` and `query/` packages separate the two sides of CQRS.

The `reactor/` package is optional and contains event handlers that trigger side effects: sending emails, calling external APIs, or coordinating with other domains. Keeping reactors separate from read models clarifies their purpose — read models build queryable state, reactors perform actions.

The `register.go` file serves as the single entry point for wiring the entire domain into the application. This makes it immediately clear how to add a new domain: create the package structure and implement the registration function.

## The Registration Pattern

Each domain exposes a `Register` function that takes the application context and facade, then wires all components together. This function follows a consistent pattern across all domains, making the codebase predictable and easy to navigate.

The registration sequence matters: aggregates must be registered before command handlers (which need aggregate repositories), and read models must be registered before query handlers (which query the read models). This ordering ensures that dependencies are available when needed.

```go
// domain/order/register.go
package order

import (
    "context"

    "github.com/gradientzero/comby/v2"
    "github.com/yourapp/domain/order/aggregate"
    "github.com/yourapp/domain/order/command"
    "github.com/yourapp/domain/order/query"
    "github.com/yourapp/domain/order/readmodel"
)

func Register(ctx context.Context, fc *comby.Facade) error {
    // 1. Register aggregate first — this makes events known to the system
    //    and allows the event store to deserialize them correctly
    if err := comby.RegisterAggregate(fc, aggregate.NewAggregate); err != nil {
        return err
    }

    // 2. Create aggregate repository — this encapsulates loading and saving
    //    aggregates through the event store
    eventRepository := fc.GetEventRepository()
    aggregateRepository := comby.NewAggregateRepository(
        eventRepository,
        aggregate.NewAggregate,
    )

    // 3. Register command handler with its dependencies
    //    The handler needs the repository to load and save aggregates
    ch := command.NewCommandHandler(aggregateRepository)
    if err := comby.RegisterCommandHandler(fc, ch); err != nil {
        return err
    }

    // 4. Register read model as an event handler
    //    It will receive all events and build queryable projections
    rm := readmodel.NewOrderReadmodel(eventRepository)
    if err := comby.RegisterEventHandler(fc, rm); err != nil {
        return err
    }

    // 5. Register query handler with access to the read model
    //    Queries are served directly from the optimized projections
    qh := query.NewQueryHandler(rm)
    if err := comby.RegisterQueryHandler(fc, qh); err != nil {
        return err
    }

    return nil
}
```

Notice how each component receives exactly the dependencies it needs through constructor injection. The command handler gets an aggregate repository for loading and persisting aggregates. The read model receives the event repository to access historical events during restoration. The query handler receives the read model to serve queries. This explicit dependency passing makes testing straightforward — you can inject mock implementations for any component.

The registration function returns an error if any step fails, allowing the application to fail fast during startup rather than encountering runtime errors later. This is particularly important in production where a partially configured system could lead to data inconsistencies.

## Central Domain Registration

While each domain has its own registration function, the application needs a central point that orchestrates the registration of all domains. This central registration function lives in the `domain` package and provides a single place to see all domains in the system.

The order of domain registration can matter when domains have dependencies on each other. For example, if the Inventory domain subscribes to Order events, the Order domain should typically be registered first to ensure its events are known to the system.

```go
// domain/domain.go
package domain

import (
    "context"
    "fmt"

    "github.com/gradientzero/comby/v2"
    "github.com/yourapp/domain/order"
    "github.com/yourapp/domain/customer"
    "github.com/yourapp/domain/inventory"
)

// RegisterDomains wires all application domains into the facade.
// This is called once during application startup after the facade
// is created but before the server starts accepting requests.
func RegisterDomains(ctx context.Context, fc *comby.Facade) error {
    // Register domains in dependency order.
    // Domains that produce events should be registered before
    // domains that consume those events.

    // Core domains first
    if err := customer.Register(ctx, fc); err != nil {
        return fmt.Errorf("customer domain: %w", err)
    }

    // Order domain depends on Customer existing
    if err := order.Register(ctx, fc); err != nil {
        return fmt.Errorf("order domain: %w", err)
    }

    // Inventory reacts to Order events
    if err := inventory.Register(ctx, fc); err != nil {
        return fmt.Errorf("inventory domain: %w", err)
    }

    return nil
}
```

Wrapping errors with `fmt.Errorf` and `%w` preserves the error chain while adding context about which domain failed. This makes debugging startup failures much easier — you immediately know which domain caused the problem and can drill into the specific registration error.

Some applications use a more dynamic approach where domains register themselves through an init function or plugin system. While this reduces boilerplate, explicit registration provides better visibility into the system's structure and makes it easier to control the registration order.

## Aggregate Registration

Registering an aggregate serves several important purposes beyond simply making it available for use. The registration process validates the aggregate's configuration, registers all events it can produce, and stores a prototype that can be used to create new instances.

When an aggregate is registered, the system extracts metadata about the events it produces. This metadata is essential for the event store to correctly serialize and deserialize events, and for event handlers to know which events they might receive.

```go
func RegisterAggregate[T Aggregate](fc *Facade, fn NewAggregateFunc[T]) error {
    // Create a prototype instance to extract metadata
    agg := fn()

    // Validate that the aggregate has required metadata
    if len(agg.GetDomain()) == 0 {
        return errors.New("aggregate has no Domain set")
    }

    // Prevent duplicate registrations which could indicate a configuration error
    if _, exists := fc.AggregateMap.Load(agg.GetIdentifier()); exists {
        return fmt.Errorf("aggregate already registered: %s", agg.GetIdentifier())
    }

    // Store the prototype for later instantiation
    fc.AggregateMap.Store(agg.GetIdentifier(), agg)

    // Register all events this aggregate can produce.
    // The isOwner=true flag indicates this aggregate is the authoritative
    // source for these events, not just a subscriber.
    for _, domainEvt := range agg.GetDomainEvents() {
        if err := fc.DomainEventProvider.RegisterDomainEvent(agg.GetDomain(), domainEvt, true); err != nil {
            return err
        }
    }

    return nil
}
```

The generic type parameter `[T Aggregate]` ensures type safety at compile time while the factory function `NewAggregateFunc[T]` allows the registration system to create new aggregate instances without knowing the concrete type. This pattern enables the aggregate repository to instantiate aggregates during event replay.

The `isOwner` flag passed during event registration distinguishes between aggregates that produce events and handlers that merely consume them. This distinction is important for validation — you might want to ensure that every event type has exactly one owner but can have multiple subscribers.

## Command Handler Registration

Command handlers are the entry point for all write operations in the system. Each handler declares which commands it can process, and the registration system validates that no two handlers claim the same command type within a domain.

The registration process builds a routing table that maps command types to their handlers. When a command arrives, the facade looks up the appropriate handler and delegates execution. This decoupling means that command dispatchers don't need to know about specific handlers.

```go
func RegisterCommandHandler[T CommandHandler](fc *Facade, ch T) error {
    domain := ch.GetDomain()

    if domain == "" {
        return errors.New("command handler has no Domain set")
    }

    // Register each command type this handler can process.
    // This builds the type registry used for command deserialization.
    for _, domainCmd := range ch.GetDomainCommands() {
        if err := fc.DomainCommandProvider.RegisterDomainCommand(domain, domainCmd); err != nil {
            return err
        }
    }

    // Validate that no other handler already registered these commands.
    // Duplicate handlers would lead to ambiguous routing.
    var uniquePaths []string
    fc.CommandHandlerMap.Range(func(key string, existingHandler CommandHandler) bool {
        for _, cmd := range existingHandler.GetDomainCommands() {
            path := fmt.Sprintf("%s-%s", existingHandler.GetDomain(), GetTypePath(cmd))
            uniquePaths = append(uniquePaths, path)
        }
        return true
    })

    for _, domainCmd := range ch.GetDomainCommands() {
        path := fmt.Sprintf("%s-%s", domain, GetTypePath(domainCmd))
        if StringInSlice(path, uniquePaths) {
            return fmt.Errorf("duplicate command handler: %s", path)
        }
    }

    // Store the handler for runtime lookup
    fc.CommandHandlerMap.Store(domain, ch)

    return nil
}
```

The validation step prevents a common configuration error where two handlers accidentally claim the same command. Without this check, the system might silently use one handler while ignoring the other, leading to confusing behavior that's hard to debug.

## Query Handler Registration

Query handlers follow a similar pattern to command handlers but serve the read side of CQRS. They declare which query types they can handle and are validated to prevent duplicate registrations.

Unlike command handlers, which typically work with aggregate repositories, query handlers work directly with read models. This separation allows queries to be optimized independently of the write model's structure.

```go
func RegisterQueryHandler[T QueryHandler](fc *Facade, qh T) error {
    domain := qh.GetDomain()

    if domain == "" {
        return errors.New("query handler has no Domain set")
    }

    // Register query types for deserialization
    for _, domainQry := range qh.GetDomainQueries() {
        if err := fc.DomainQueryProvider.RegisterDomainQuery(domain, domainQry); err != nil {
            return err
        }
    }

    // Store handler for runtime lookup
    fc.QueryHandlerMap.Store(domain, qh)

    return nil
}
```

Query handlers are typically simpler than command handlers because they don't need to coordinate transactions or manage aggregate state. They simply retrieve data from read models and return it in the requested format.

## Event Handler Registration

Read models are registered as event handlers because they receive events and update their internal state. Unlike aggregates that own events, read models subscribe to events from their own domain or even from other domains.

The registration captures which events a read model is interested in, allowing the event dispatcher to efficiently route events only to handlers that care about them.

```go
func RegisterEventHandler[T EventHandler](fc *Facade, eh T) error {
    domain := eh.GetDomain()
    name := eh.GetName()

    if domain == "" {
        return errors.New("event handler has no Domain set")
    }

    // Register events this handler subscribes to.
    // The isOwner=false flag indicates this is a subscriber, not the source.
    // This allows multiple handlers to subscribe to the same event type.
    for _, domainEvt := range eh.GetDomainEvents() {
        if err := fc.DomainEventProvider.RegisterDomainEvent(domain, domainEvt, false); err != nil {
            return err
        }
    }

    // Use domain.name as the key to allow multiple handlers per domain.
    // For example: "Order.OrderListReadmodel" and "Order.OrderStatsReadmodel"
    key := fmt.Sprintf("%s.%s", domain, name)
    fc.EventHandlerMap.Store(key, eh)

    return nil
}
```

The compound key `domain.name` allows a single domain to have multiple read models optimized for different query patterns. An Order domain might have one read model for listing orders and another for computing statistics, each subscribing to the same events but maintaining different data structures.

## Middleware Registration

Middleware provides a powerful mechanism for implementing cross-cutting concerns that apply to multiple handlers without modifying the handlers themselves. Common use cases include authentication, authorization, logging, metrics, and webhook notifications.

The middleware pattern wraps handler functions with additional behavior. Each middleware receives the next handler in the chain and returns a new handler that adds its own logic before or after calling the wrapped handler. This creates a pipeline where each middleware can:

- **Inspect or modify input** before the handler executes
- **Short-circuit execution** by returning early (e.g., for authorization failures)
- **Inspect or modify output** after the handler executes
- **Handle errors** thrown by downstream handlers
- **Add timing and metrics** around handler execution

### Command Handler Middleware

Command middleware wraps the command execution pipeline. This is where you typically implement authorization checks, audit logging, and post-command actions like webhook notifications.

```go
// Middleware type definition
type CommandHandlerMiddlewareFunc func(
    ctx context.Context,
    fn DomainCommandHandlerFunc,
) DomainCommandHandlerFunc

// Middleware composition — applies middlewares in reverse order
// so the first middleware in the list executes first
func WithCommandHandlerMiddlewares(
    ctx context.Context,
    fn DomainCommandHandlerFunc,
    middlewares ...CommandHandlerMiddlewareFunc,
) DomainCommandHandlerFunc {
    if len(middlewares) < 1 {
        return fn
    }
    wrapped := fn
    for i := len(middlewares) - 1; i >= 0; i-- {
        wrapped = middlewares[i](ctx, wrapped)
    }
    return wrapped
}
```

### Authorization Middleware Example

This middleware checks permissions before allowing command execution. It supports multiple authorization strategies: explicit skip flags, custom permission functions, and role-based access control.

```go
func AuthCommandHandlerFunc(fc *comby.Facade) comby.CommandHandlerMiddlewareFunc {
    return func(ctx context.Context, next comby.DomainCommandHandlerFunc) comby.DomainCommandHandlerFunc {
        return func(ctx context.Context, cmd comby.Command, domainCmd comby.DomainCmd) ([]comby.Event, error) {
            reqCtx := cmd.GetReqCtx()

            // Check for system-level bypass (internal commands)
            if skip := reqCtx.Attributes.Get(ExecuteSkipAuthorization); skip != nil {
                if skipAuth, ok := skip.(bool); ok && skipAuth {
                    return next(ctx, cmd, domainCmd)
                }
            }

            // Check registered permission functions
            for _, perm := range runtime.RuntimePermissionList(ctx, fc) {
                if cmd.GetDomain() == perm.PermissionDomain &&
                   cmd.GetDomainCmdName() == perm.PermissionType {
                    if perm.CmdFunc != nil {
                        if err := perm.CmdFunc(ctx, cmd); err == nil {
                            return next(ctx, cmd, domainCmd)
                        }
                    }
                }
            }

            // Default: deny access
            return nil, runtime.ErrPermissionDenied
        }
    }
}
```

### Webhook Middleware Example

This middleware fires webhooks after successful command execution. It demonstrates post-processing middleware that acts on the results without modifying them.

```go
func WebhookCommandHandlerFunc(fc *comby.Facade) comby.CommandHandlerMiddlewareFunc {
    return func(ctx context.Context, next comby.DomainCommandHandlerFunc) comby.DomainCommandHandlerFunc {
        return func(ctx context.Context, cmd comby.Command, domainCmd comby.DomainCmd) ([]comby.Event, error) {
            // Execute the actual command handler
            events, err := next(ctx, cmd, domainCmd)
            if err != nil {
                return events, err
            }

            // Post-processing: send webhooks for matching events
            if WebhookReadmodel != nil {
                webhooks, _, _ := WebhookReadmodel.GetModelList()
                for _, webhook := range webhooks {
                    if !webhook.Active {
                        continue
                    }
                    for _, evt := range events {
                        eventIdentifier := fmt.Sprintf("%s.%s", evt.GetDomain(), evt.GetDomainEvtName())
                        if webhook.Tenant.TenantUuid == evt.GetTenantUuid() &&
                           webhook.DomainEvtIdentifier == eventIdentifier {
                            go postWebhookRequest(evt, webhook.WebhookUrl)
                        }
                    }
                }
            }

            return events, nil
        }
    }
}
```

### Query and Event Handler Middleware

The same pattern applies to queries and event handlers, enabling consistent cross-cutting concerns across all operations:

```go
// Query middleware for authorization and caching
type QueryHandlerMiddlewareFunc func(
    ctx context.Context,
    fn DomainQueryHandlerFunc,
) DomainQueryHandlerFunc

// Event middleware for logging and error handling
type EventHandlerMiddlewareFunc func(
    ctx context.Context,
    fn DomainEventHandlerFunc,
) DomainEventHandlerFunc
```

### Registering Middleware

Middleware is registered on the facade before domains are registered. The order of registration determines the execution order — middlewares registered first execute their "before" logic first and their "after" logic last.

```go
func RegisterDefaults(ctx context.Context, fc *comby.Facade, opts ...DefaultOption) error {
    // Webhook middleware executes after command handlers
    fc.AddCommandHandlerMiddleware(webhook.WebhookCommandHandlerFunc(fc))

    // Authorization middleware executes before command handlers
    if defaultOpts.AuthorizationDisabled {
        fc.AddCommandHandlerMiddleware(auth.NoAuthCommandHandlerFunc(fc))
        fc.AddQueryHandlerMiddleware(auth.NoAuthQueryHandlerFunc(fc))
    } else {
        fc.AddCommandHandlerMiddleware(auth.AuthCommandHandlerFunc(fc))
        fc.AddQueryHandlerMiddleware(auth.AuthQueryHandlerFunc(fc))
    }

    // Now register domains...
}
```

The middleware pipeline for a command might look like:

```
Request → Auth Middleware → Command Handler → Webhook Middleware → Response
              ↓                    ↓                  ↓
         Check permissions    Execute logic    Send notifications
```

## Using Projection Shortcut

For domains with straightforward query requirements, the framework provides built-in projection utilities that reduce boilerplate. These projections automatically maintain a queryable representation of aggregate state by replaying events.

This approach trades flexibility for convenience — you get standard list and get-by-ID queries without writing custom read model code, but you're limited to querying by aggregate properties.

```go
func Register(ctx context.Context, fc *comby.Facade) error {
    domain := aggregate.NewAggregate().GetDomain()

    // 1. Register aggregate
    if err := comby.RegisterAggregate(fc, aggregate.NewAggregate); err != nil {
        return err
    }

    // 2. Register command handler
    aggregateRepository := comby.NewAggregateRepository(fc, aggregate.NewAggregate)
    ch := command.NewCommandHandler(aggregateRepository)
    if err := comby.RegisterCommandHandler(fc, ch); err != nil {
        return err
    }

    // 3. Use built-in projection instead of custom read model.
    //    This automatically handles event subscription and state updates.
    projRm := projection.NewProjectionAggregate(fc, aggregate.NewAggregate)
    projRm.Domain = domain
    if err := comby.RegisterEventHandler(fc, projRm); err != nil {
        return err
    }

    // 4. Use built-in query handler that works with the projection.
    //    Supports standard queries: list with filtering, get by ID.
    projQh := projection.NewQueryHandler(projRm)
    projQh.Domain = domain
    if err := comby.RegisterQueryHandler(fc, projQh); err != nil {
        return err
    }

    return nil
}
```

The projection approach is ideal for simple CRUD-like domains where you primarily query by aggregate ID or list aggregates with basic filtering. For complex query requirements — full-text search, aggregations, joins across aggregates — you'll want custom read models with specialized data structures.

## Application Initialization

The application's `main.go` brings everything together in a carefully ordered sequence. The initialization order ensures that infrastructure is ready before domains are registered, and domains are fully wired before the system starts accepting requests.

```go
func main() {
    ctx := context.Background()

    // 1. Create facade with infrastructure configuration.
    //    The facade is the central hub that holds all stores, buses,
    //    and handler registries.
    fc, err := comby.NewFacade(
        comby.FacadeWithAppName("myapp"),
        comby.WithSQLiteEventStore("./data/events.db"),
        comby.WithSQLiteCommandStore("./data/commands.db"),
    )
    if err != nil {
        log.Fatal(err)
    }

    // 2. Register built-in domains provided by the framework.
    //    These include account management, authentication, tenants,
    //    and other common functionality.
    if err := combyDomain.RegisterDefaults(ctx, fc); err != nil {
        log.Fatal(err)
    }

    // 3. Register application-specific domains.
    //    This is where your business logic lives.
    if err := domain.RegisterDomains(ctx, fc); err != nil {
        log.Fatal(err)
    }

    // 4. Restore state by replaying events into read models.
    //    This brings read models up to date with all historical events.
    //    Must happen after registration so handlers are available.
    if err := fc.RestoreState(); err != nil {
        log.Fatal(err)
    }

    // 5. Setup HTTP server and API framework.
    router := chi.NewRouter()
    humaAPI := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))

    // 6. Register API endpoints that expose domain functionality.
    if err := api.RegisterEndpoints(fc, humaAPI); err != nil {
        log.Fatal(err)
    }

    // 7. Start server — system is now ready to handle requests.
    log.Println("Starting server on :8080")
    http.ListenAndServe(":8080", router)
}
```

The `RestoreState()` call is critical for event-sourced systems. On startup, read models are empty — they need to process all historical events to rebuild their current state. This might take time for systems with many events, which is why some deployments use snapshotting or persistent read models to reduce startup time.

## Registration Options

Domain registration often needs to be configurable to support different environments or deployment scenarios. The options pattern provides a clean way to parameterize registration without changing function signatures.

```go
// Option function type
type DefaultOption func(opt *DefaultOptions) (*DefaultOptions, error)

// Configuration struct with all possible options
type DefaultOptions struct {
    AuthorizationDisabled    bool
    DefaultSeedDisabled      bool
    IncludeHistoryForDomains []string
}

// Option constructors
func WithAuthorizationDisabled(disabled bool) DefaultOption {
    return func(opt *DefaultOptions) (*DefaultOptions, error) {
        opt.AuthorizationDisabled = disabled
        return opt, nil
    }
}

func WithIncludeHistoryForDomains(domains ...string) DefaultOption {
    return func(opt *DefaultOptions) (*DefaultOptions, error) {
        opt.IncludeHistoryForDomains = append(opt.IncludeHistoryForDomains, domains...)
        return opt, nil
    }
}

// Usage
err := domain.RegisterDefaults(ctx, fc,
    domain.WithAuthorizationDisabled(true),  // For development
    domain.WithIncludeHistoryForDomains("Account", "Order"),
)
```

This pattern allows each deployment to configure domains appropriately. Development environments might disable authorization for easier testing, while production environments enable all security features. The options are applied in order, so later options can override earlier ones if needed.

## Permission Registration

Commands and queries can have fine-grained permissions that control who can execute them. The permission system supports multiple authorization strategies: simple authentication checks, role-based access control, and custom permission functions for complex business rules.

Permissions are registered alongside domain components, keeping authorization logic close to the operations it protects. This co-location makes it easier to understand and maintain the security model.

```go
func Register(ctx context.Context, fc *comby.Facade) error {
    // ... aggregate and handler registration ...

    domain := aggregate.NewAggregate().GetDomain()

    // Register permissions for this domain's operations
    runtime.RegisterPermission(
        // Command permissions
        runtime.NewPermissionCmdFunc(
            domain,
            &command.PlaceOrder{},
            "Place order",
            runtime.AuthenticatedCmdFn,  // Any authenticated user
        ),
        runtime.NewPermissionCmdFunc(
            domain,
            &command.CancelOrder{},
            "Cancel order",
            permissionFunc("order:cancel"),  // Requires specific permission
        ),

        // Query permissions
        runtime.NewPermissionQryFunc(
            domain,
            &projection.QueryListRequest[*aggregate.Order]{},
            "List orders",
            runtime.AuthenticatedQryFn,
        ),
        runtime.NewPermissionQryFunc(
            domain,
            &comby.ProjectionQueryModelRequest[*aggregate.Order]{},
            "Get order",
            runtime.AuthenticatedQryFn,
        ),
    )

    return nil
}

// Custom permission function for complex authorization logic
func permissionFunc(permission string) runtime.CmdPermissionFunc {
    return func(ctx context.Context, cmd comby.Command, domainCmd comby.DomainCmd) error {
        reqCtx := comby.GetReqCtxFromContext(ctx)

        // Check if user has the required permission
        if !hasPermission(reqCtx, permission) {
            return comby.ErrForbidden
        }

        // Additional checks could go here:
        // - Resource ownership
        // - Time-based restrictions
        // - Rate limiting

        return nil
    }
}
```

Permission functions receive the full context and command, enabling sophisticated authorization rules. You might check whether the user owns the resource being modified, whether the operation is allowed during business hours, or whether the user's subscription tier permits the action.

## Cross-Domain Event Handling

One of the most powerful aspects of event-sourced architecture is the ability for domains to react to events from other domains. This enables loose coupling — domains don't call each other directly but instead react to events they care about.

Consider an Inventory domain that needs to reserve stock when orders are placed. Rather than the Order domain calling the Inventory domain directly, the Inventory domain subscribes to Order events and reacts accordingly.

```go
// inventory/register.go
func Register(ctx context.Context, fc *comby.Facade) error {
    // ... inventory aggregate registration ...

    // Create handler for Order domain events
    inventoryRepo := comby.NewAggregateRepository(fc, aggregate.NewAggregate)
    orderEventsHandler := NewOrderEventsHandler(inventoryRepo)
    if err := comby.RegisterEventHandler(fc, orderEventsHandler); err != nil {
        return err
    }

    return nil
}

// inventory/reactor/order_events.go
type OrderEventsHandler struct {
    *comby.BaseEventHandler
    inventoryRepo *comby.AggregateRepository[*Inventory]
}

func NewOrderEventsHandler(repo *comby.AggregateRepository[*Inventory]) *OrderEventsHandler {
    h := &OrderEventsHandler{inventoryRepo: repo}
    h.BaseEventHandler = comby.NewBaseEventHandler()
    h.Domain = "Inventory"  // This handler belongs to Inventory domain
    h.Name = "OrderEventsHandler"

    // Subscribe to events from the Order domain
    comby.AddDomainEventHandler(h, h.handleOrderPlaced)
    comby.AddDomainEventHandler(h, h.handleOrderCancelled)

    return h
}

func (h *OrderEventsHandler) handleOrderPlaced(
    ctx context.Context,
    evt comby.Event,
    domainEvt *order.OrderPlacedEvent,
) error {
    // React to order placement by reserving inventory
    for _, item := range domainEvt.Items {
        inv, err := h.inventoryRepo.GetAggregate(ctx, item.ProductID)
        if err != nil {
            return err
        }

        // Reserve stock for this order
        if err := inv.Reserve(item.Quantity, evt.AggregateUuid); err != nil {
            return err
        }

        if err := h.inventoryRepo.SaveAggregate(ctx, inv); err != nil {
            return err
        }
    }
    return nil
}

func (h *OrderEventsHandler) handleOrderCancelled(
    ctx context.Context,
    evt comby.Event,
    domainEvt *order.OrderCancelledEvent,
) error {
    // Release reserved inventory when order is cancelled
    for _, item := range domainEvt.Items {
        inv, err := h.inventoryRepo.GetAggregate(ctx, item.ProductID)
        if err != nil {
            return err
        }

        inv.ReleaseReservation(evt.AggregateUuid)

        if err := h.inventoryRepo.SaveAggregate(ctx, inv); err != nil {
            return err
        }
    }
    return nil
}
```

This pattern maintains domain boundaries while enabling coordination. The Order domain doesn't know about inventory — it simply publishes events about what happened. The Inventory domain decides how to react to those events. This makes each domain independently testable and deployable.

Be cautious about the complexity this can introduce. Event chains where Domain A triggers Domain B which triggers Domain C can be hard to reason about. Keep reactions simple and consider whether synchronous coordination (saga pattern) might be more appropriate for complex workflows.

## Testing Domain Registration

Registration tests verify that domains are correctly configured and all components are properly wired. These tests catch configuration errors early, before they cause runtime failures.

Testing registration in isolation also helps ensure that domains don't have hidden dependencies on other domains being registered first.

```go
func TestDomainRegistration(t *testing.T) {
    ctx := context.Background()

    // Create test facade with in-memory stores
    fc, err := comby.NewFacade(
        comby.WithInMemoryStores(),
    )
    require.NoError(t, err)

    // Register domain under test
    err = order.Register(ctx, fc)
    require.NoError(t, err)

    // Verify aggregate is registered and accessible
    agg := fc.GetAggregate("Order")
    require.NotNil(t, agg, "Order aggregate should be registered")

    // Verify command handler is registered
    ch := fc.GetCommandHandler("Order")
    require.NotNil(t, ch, "Order command handler should be registered")

    // Verify events are registered with correct ownership
    domains, events := fc.DomainEventProvider.GetDomainEvents()
    require.Contains(t, domains, "Order")
    require.NotEmpty(t, events["Order"], "Order domain should have events")

    // Verify commands are registered
    cmdDomains, cmds := fc.DomainCommandProvider.GetDomainCommands()
    require.Contains(t, cmdDomains, "Order")
    require.NotEmpty(t, cmds["Order"], "Order domain should have commands")

    // Verify queries are registered
    qryDomains, queries := fc.DomainQueryProvider.GetDomainQueries()
    require.Contains(t, qryDomains, "Order")
}

func TestDomainRegistrationOrder(t *testing.T) {
    ctx := context.Background()

    fc, err := comby.NewFacade(comby.WithInMemoryStores())
    require.NoError(t, err)

    // Test that registering handlers before aggregate fails
    ch := command.NewCommandHandler(nil)  // nil repo because aggregate not registered
    err = comby.RegisterCommandHandler(fc, ch)
    // This should work but handler won't function correctly

    // Proper order: aggregate first
    err = comby.RegisterAggregate(fc, aggregate.NewAggregate)
    require.NoError(t, err)
}
```

Beyond registration tests, consider integration tests that verify the full flow: send a command, verify events are produced, verify read models are updated, and verify queries return expected results. These tests exercise the entire registration and wiring.

## Best Practices

Based on experience building event-sourced systems, here are guidelines for effective domain registration:

**1. One Register function per domain**
Keep a single entry point for each domain. This makes it immediately clear how to add or modify a domain's configuration. Avoid spreading registration logic across multiple files or packages.

**2. Registration order matters**
Register aggregates before command handlers, and read models before query handlers. Document dependencies between domains and register them in the correct order. Consider adding explicit dependency declarations if the order becomes complex.

**3. Validate early and fail fast**
Check for configuration errors during registration, not at runtime. Validate that domains have names, handlers aren't duplicated, and required dependencies are available. A clear error at startup is far better than a cryptic failure in production.

**4. Use meaningful domain names**
Domain names appear in logs, metrics, API paths, and error messages. Choose names that are clear and consistent. "Order" is better than "Ord" or "OrderDomain".

**5. Keep domains focused**
Each domain should represent a single bounded context with cohesive responsibilities. If a domain is handling unrelated concerns, consider splitting it. A focused domain is easier to understand, test, and maintain.

**6. Coordinate through events, not direct calls**
Avoid domains calling each other's internal components. Instead, publish events and let interested domains react. This maintains loose coupling and enables independent evolution.

**7. Use middleware for cross-cutting concerns**
Authentication, authorization, logging, and metrics should be implemented as middleware, not repeated in every handler. This keeps handlers focused on business logic.

**8. Test registration in isolation**
Write tests that register a single domain with in-memory stores. This catches configuration errors and ensures domains don't have hidden dependencies.

## Running an Example

Source: https://github.com/susdorf/building-event-sourced-systems-in-go/tree/main/14-domain-registration-modular-architecture/code

When you run the complete example, you'll see the full domain registration pattern in action:

```
=== Domain Registration Demo ===

1. Creating Facade...
   Facade created: demo-app

2. Registering Middleware...
   -> Added MetricsCommandMiddleware
   -> Added LoggingCommandMiddleware
   -> Added AuthorizationCommandMiddleware
   -> Added LoggingEventMiddleware

========================================
Starting Domain Registration
========================================

[Order Domain] Registering components...
  [Facade] Registered aggregate: Order.Order
  [Facade] Registered command handler: Order.OrderCommandHandler (commands: [PlaceOrder PayOrder CancelOrder])
  [Facade] Registered event handler: Order.OrderReadmodel (subscribes to: [Order.OrderPlaced Order.OrderPaid Order.OrderShipped Order.OrderCancelled])
[Order Domain] Registration complete

[Inventory Domain] Registering components...
  [Facade] Registered aggregate: Inventory.Inventory
  [Facade] Registered event handler: Inventory.OrderEventsReactor (subscribes to: [Order.OrderPlaced Order.OrderCancelled])
[Inventory Domain] Registration complete

========================================
Domain Registration Complete
  Registered domains: [Order Inventory]
  Command handlers: 1
  Event handlers: 2
========================================

========================================
Executing Commands
========================================

3. Placing an order...
    [Middleware:Logging] Executing command: Order.PlaceOrder
    [Middleware:Auth] ALLOWED - Tenant tenant-A authorized
    [Middleware:Logging] Command succeeded in 3.041µs, produced 1 events
    [Middleware:EventLog] Processing event: Order.OrderPlaced (aggregate: order-001)
      [OrderReadmodel] Created order: order-001 (customer: customer-123, total: 109.97)
    [Middleware:EventLog] Processing event: Order.OrderPlaced (aggregate: order-001)
      [Inventory:Reactor] Order order-001 placed - reserving inventory for 2 items
        -> Reserved 2 of product prod-1
        -> Reserved 1 of product prod-2
   -> Success! Produced 1 event(s)

4. Placing another order...
    [Middleware:Logging] Executing command: Order.PlaceOrder
    [Middleware:Auth] ALLOWED - Tenant tenant-B authorized
    [Middleware:Logging] Command succeeded in 1.375µs, produced 1 events
    [Middleware:EventLog] Processing event: Order.OrderPlaced (aggregate: order-002)
      [OrderReadmodel] Created order: order-002 (customer: customer-456, total: 99.95)
    [Middleware:EventLog] Processing event: Order.OrderPlaced (aggregate: order-002)
      [Inventory:Reactor] Order order-002 placed - reserving inventory for 1 items
        -> Reserved 5 of product prod-3
   -> Success! Produced 1 event(s)

5. Cancelling order-001...
    [Middleware:Logging] Executing command: Order.CancelOrder
    [Middleware:Auth] ALLOWED - Tenant tenant-A authorized
    [Middleware:Logging] Command succeeded in 1.917µs, produced 1 events
    [Middleware:EventLog] Processing event: Order.OrderCancelled (aggregate: order-001)
      [OrderReadmodel] Order order-001 cancelled: Customer requested cancellation
    [Middleware:EventLog] Processing event: Order.OrderCancelled (aggregate: order-001)
      [Inventory:Reactor] Order order-001 cancelled - releasing inventory
        -> Released 2 of product prod-1 (reason: Customer requested cancellation)
        -> Released 1 of product prod-2 (reason: Customer requested cancellation)
   -> Success! Produced 1 event(s)

6. Attempting unauthorized command...
    [Middleware:Logging] Executing command: Order.PlaceOrder
    [Middleware:Auth] DENIED - Tenant tenant-UNKNOWN not authorized
    [Middleware:Logging] Command failed after 3.584µs: unauthorized: tenant tenant-UNKNOWN
   -> Expected failure: unauthorized: tenant tenant-UNKNOWN

========================================
Querying Read Model
========================================

7. Querying Order readmodel...
   Total orders in readmodel: 2
   -> Order order-002: status=placed, customer=customer-456, total=99.95
   -> Order order-001: status=cancelled, customer=customer-123, total=109.97

========================================
Metrics Summary
========================================
   Total commands executed: 4
   Successful: 3
   Failed: 1
   Total events produced: 3

========================================
Event Store Contents
========================================
   Total events stored: 3
   [1] Order.OrderPlaced (aggregate: order-001)
   [2] Order.OrderPlaced (aggregate: order-002)
   [3] Order.OrderCancelled (aggregate: order-001)

=== Demo Complete ===
```

The demo illustrates several key concepts:

1. **Domain Registration Sequence**: The Order domain registers first (produces events), then the Inventory domain (consumes Order events)
2. **Middleware Pipeline**: Authorization and logging middlewares wrap command execution
3. **Cross-Domain Event Handling**: When an order is placed, the Inventory reactor automatically reserves stock
4. **Event-Driven Coordination**: Cancelling an order triggers inventory release through event subscription
5. **Read Model Updates**: The OrderReadmodel maintains a queryable view of all orders
6. **Authorization Enforcement**: Unauthorized tenants are rejected by the middleware

## Summary

Domain registration is the architectural backbone that wires an event-sourced application together. A well-designed registration system provides:

1. **Clear package structure**: Each domain in its own package with consistent organization
2. **Single registration entry point**: One `Register` function per domain
3. **Centralized orchestration**: All domains registered from one place in the correct order
4. **Proper sequencing**: Aggregate → Command Handler → Read Model → Query Handler
5. **Middleware pipeline**: Cross-cutting concerns applied consistently
6. **State restoration**: Events replayed after registration to rebuild read models
7. **Permission integration**: Authorization defined alongside operations
8. **Cross-domain coordination**: Domains react to each other through events

This modular architecture makes it easy to add new domains without touching existing code, test domains in isolation, and reason about the system's structure. As your application grows, this foundation keeps complexity manageable.

---

## What's Next

In the final post, we'll explore **Testing Event-Sourced Systems** — strategies for testing aggregates, handlers, projections, and full integration.
