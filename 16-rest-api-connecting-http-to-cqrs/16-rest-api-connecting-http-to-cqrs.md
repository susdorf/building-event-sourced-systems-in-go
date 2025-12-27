# REST API: Connecting HTTP to CQRS

*This is Part 16 of the "Building Event-Sourced Systems in Go" series. This series originated from the development of comby – an application framework developed using the principles of Event Sourcing and Command Query Responsibility Segregation (CQRS) - which ist still closed source today. Code with concepts and techniques is presented, but unlike comby, it is not always suitable for product use.*

---

## Introduction

Event Sourcing and CQRS provide powerful patterns for managing application state. But applications need to communicate with the outside world — and for most systems, that means HTTP APIs. Whether you're building a web application, a mobile backend, or integrating with third-party services, REST remains the lingua franca of modern software architecture.

The challenge is more nuanced than it first appears: How do you expose a CQRS system through REST while maintaining clean separation between HTTP concerns and domain logic? HTTP is inherently synchronous — clients send a request and wait for a response. But event-sourced systems are often asynchronous by design: commands trigger events, events update projections, and the eventual state emerges from this flow. Bridging these two paradigms requires careful architectural decisions.

Additionally, REST APIs introduce concerns that don't exist in your domain layer: authentication headers, cookies, HTTP status codes, content negotiation, pagination, and rate limiting. These are HTTP-specific concerns that shouldn't leak into your aggregates or command handlers.

In this post, we'll explore how to build a REST API layer that elegantly bridges HTTP requests to Commands and Queries, using **Huma** as our HTTP framework. We'll see how middleware pipelines can enrich requests with authentication context, how thin handlers translate HTTP to domain operations, and how consistent patterns make the codebase maintainable as it grows.

## The Bridge Problem

In a CQRS system, we have two distinct paths for data flow. Write operations travel through the command side, where business rules are enforced, events are generated, and state changes are persisted. Read operations travel through the query side, where denormalized read models provide optimized views of the data. This separation is fundamental to CQRS — but HTTP doesn't care about your architectural patterns.

When a client sends a POST request, it expects a response. When it sends a GET request, it expects data. The API layer must understand both worlds: the stateless, request-response nature of HTTP on one side, and the command-event-projection flow of CQRS on the other.

```
┌─────────────────────────────────────────────────────────────┐
│                        Client                                │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                      REST API Layer                          │
│   ┌─────────────────┐           ┌─────────────────────┐     │
│   │ POST /orders    │           │ GET /orders         │     │
│   │ PATCH /orders/1 │           │ GET /orders/1       │     │
│   │ DELETE /orders/1│           │                     │     │
│   └────────┬────────┘           └──────────┬──────────┘     │
└────────────┼────────────────────────────────┼───────────────┘
             │                                │
             ▼                                ▼
      ┌──────────────┐               ┌──────────────┐
      │   Commands   │               │   Queries    │
      │  (Writes)    │               │   (Reads)    │
      └──────┬───────┘               └──────┬───────┘
             │                                │
             ▼                                ▼
      ┌──────────────┐               ┌──────────────┐
      │   Aggregates │               │  Readmodels  │
      │   & Events   │               │              │
      └──────────────┘               └──────────────┘
```

The API layer serves as a translation layer with several distinct responsibilities:

1. **Parse HTTP requests into domain objects**: Extract path parameters, query strings, headers, and request bodies, then assemble them into strongly-typed command or query objects that the domain layer understands.

2. **Handle authentication and authorization**: Validate session cookies, bearer tokens, or API keys. Determine who the caller is (authentication) and whether they're allowed to perform the requested operation (authorization). This context must flow into commands so the domain can enforce business rules.

3. **Route writes to Commands and reads to Queries**: This is the core CQRS mapping. POST, PUT, PATCH, and DELETE typically become commands. GET requests become queries. The API layer knows HTTP verbs; the domain layer knows commands and queries.

4. **Transform domain responses back to HTTP responses**: Commands might return the UUID of a created resource. Queries return domain objects. These must be serialized to JSON with appropriate structure, pagination metadata, and hypermedia links if you're building a HATEOAS API.

5. **Map errors to appropriate HTTP status codes**: A domain `NotFound` error becomes HTTP 404. A validation error becomes 400. A permission denied becomes 403. Consistent error mapping ensures clients can programmatically handle failures.

## Why Huma?

When choosing an HTTP framework for a CQRS system, we need something that stays out of our way while providing essential features. We don't want a framework that imposes its own opinions about how to structure business logic — that's what our domain layer is for. Instead, we need a framework that excels at the HTTP-specific concerns: routing, validation, serialization, and documentation.

**Huma** is a modern Go framework for building APIs with automatic OpenAPI documentation. Unlike heavier frameworks that try to be everything, Huma focuses on being an excellent HTTP layer. It integrates with standard Go HTTP routers (Chi, Gin, Echo, Fiber) rather than replacing them, giving you flexibility in your stack.

Key features that make Huma well-suited for CQRS systems:

- **Schema-driven request/response validation**: Define your request and response types as Go structs with validation tags. Huma automatically validates incoming requests and rejects malformed data before it reaches your handlers. This keeps validation logic out of your domain layer where it doesn't belong.

- **Automatic OpenAPI 3.1 generation**: Every endpoint you register automatically appears in the generated OpenAPI specification. This means your API documentation is always in sync with your code — no more outdated Swagger files.

- **Middleware support**: Huma's middleware system lets you build pipelines that process requests before they reach handlers. Authentication, logging, rate limiting, and context enrichment all happen in middleware, keeping handlers focused on their single responsibility.

- **Clean separation between routes and handlers**: Registration is declarative — you specify the HTTP method, path, and metadata separately from the handler function. This makes it easy to see at a glance what endpoints exist and how they're configured.

```go
import (
    "github.com/danielgtaylor/huma/v2"
    "github.com/danielgtaylor/huma/v2/adapters/humachi"
    "github.com/go-chi/chi/v5"
)

func main() {
    router := chi.NewRouter()
    api := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))

    // Register endpoints...

    http.ListenAndServe(":8080", router)
}
```

The `huma.DefaultConfig` call creates a configuration with sensible defaults for JSON serialization, error formatting, and OpenAPI metadata. The resulting `api` object is what you'll use to register all your endpoints and middleware.

## Resource-Based Structure

A common mistake when organizing API code is to group by HTTP method — putting all POST handlers in one file, all GET handlers in another. This approach falls apart quickly. When you need to understand or modify the "orders" functionality, you end up jumping between multiple files, piecing together how the resource works.

Instead, organize API code by resource. Each resource gets its own package containing everything related to that domain concept. This mirrors how you think about your API: "What can I do with orders?" rather than "What POST endpoints exist?"

```
api/
├── api.go                  # Central registration
└── order/
    ├── resource.go         # Resource struct & registration
    ├── create.go           # POST /orders
    ├── list.go             # GET /orders
    ├── retrieve.go         # GET /orders/{orderUuid}
    ├── patch.go            # PATCH /orders/{orderUuid}
    └── delete.go           # DELETE /orders/{orderUuid}
```

This structure has several advantages. First, it's intuitive to navigate — need to change how orders are created? Look in `api/order/create.go`. Second, it limits the blast radius of changes — modifying the order resource doesn't require touching files that other resources depend on. Third, it scales well — adding a new resource means adding a new package, not modifying existing files.

### The Resource Pattern

Each resource encapsulates its endpoints and holds references to the dependencies it needs — primarily the facade that provides access to the command and query dispatchers:

```go
package order

import (
    "context"
    "net/http"

    "github.com/danielgtaylor/huma/v2"
    "github.com/gradientzero/comby/v2"
    "github.com/gradientzero/comby/v2/api"
)

type Resource struct {
    fc  *comby.Facade
    api huma.API
}

func NewResource(fc *comby.Facade, api huma.API) *Resource {
    return &Resource{fc: fc, api: api}
}

func (rs *Resource) Register() {
    huma.Register(rs.api, huma.Operation{
        OperationID: "order-create",
        Description: "Create a new order",
        Method:      http.MethodPost,
        Path:        "/api/tenants/{tenantUuid}/orders",
        Tags:        []string{"Order"},
    }, rs.Create)

    huma.Register(rs.api, huma.Operation{
        OperationID: "order-list",
        Description: "List all orders",
        Method:      http.MethodGet,
        Path:        "/api/tenants/{tenantUuid}/orders",
        Tags:        []string{"Order"},
    }, rs.List)

    huma.Register(rs.api, huma.Operation{
        OperationID: "order-retrieve",
        Description: "Retrieve an order",
        Method:      http.MethodGet,
        Path:        "/api/tenants/{tenantUuid}/orders/{orderUuid}",
        Tags:        []string{"Order"},
    }, rs.Retrieve)
}
```

### Helper Functions

Each resource package defines helper functions that encapsulate the boilerplate of creating commands and queries. These helpers serve two purposes: they reduce repetition in handlers, and they ensure consistency — every command for this domain automatically gets the correct domain name set.

```go
// Create a command for this domain
func NewCommand(ctx context.Context, domainCmd comby.DomainCmd) comby.Command {
    return api.NewCommand(ctx, "Order", domainCmd)
}

// Create a query for this domain
func NewQuery(ctx context.Context, domainQry comby.DomainQry) comby.Query {
    return api.NewQuery(ctx, "Order", domainQry)
}

// Response type aliases for cleaner signatures
type ResponseOrderModel = api.QueryResponseWithBody[*aggregate.Order]
type ResponseOrderList = api.QueryResponseListWithBody[*aggregate.Order]
```

The `NewCommand` and `NewQuery` functions wrap the generic API helpers, binding them to the "Order" domain. This means handlers don't need to remember to set the domain name — a common source of bugs when copy-pasting between resources.

Type aliases for responses serve a similar purpose. Without them, handler signatures become verbose and harder to read. Compare `func (rs *Resource) Retrieve(...) (*api.QueryResponseWithBody[*aggregate.Order], error)` with the cleaner `func (rs *Resource) Retrieve(...) (*ResponseOrderModel, error)`. The alias also provides a single place to change if you need to modify the response wrapper type.

## Bridging HTTP to Commands

Commands represent write operations — creating, updating, or deleting resources. In a CQRS system, commands don't return the modified data directly. Instead, they trigger events that eventually update read models. This creates a challenge for REST APIs: clients expect to receive the created or updated resource in the response.

The solution is a multi-step process in the handler:

1. **Parse the request**: Extract data from path parameters, query strings, and the request body. Huma handles this automatically based on your request struct tags.

2. **Create a Command**: Instantiate the domain command with the extracted data. The command carries both the operation data and contextual information (who's performing the action, which tenant, etc.).

3. **Dispatch to the facade**: Send the command to the command bus. In a distributed system, this might publish to a message broker. In a simpler setup, it goes directly to the command handler.

4. **Wait for completion**: Block until the command has been fully processed — events generated, projections updated. This is where we bridge the async nature of event sourcing with the sync nature of HTTP.

5. **Return the result**: Query the updated state from a read model and return it to the client. This ensures the response reflects the actual state after the command executed.

```go
package order

import (
    "context"

    "github.com/yourapp/domain/order/aggregate"
    "github.com/yourapp/domain/order/command"
    "github.com/gradientzero/comby/v2/api"
)

type RequestOrderCreate struct {
    TenantUuid string `path:"tenantUuid" validate:"required"`
    Body       struct {
        OrderUuid    string  `json:"orderUuid" validate:"required"`
        CustomerName string  `json:"customerName" validate:"required"`
        Items        []Item  `json:"items" validate:"required,min=1"`
    }
}

func (rs *Resource) Create(ctx context.Context, req *RequestOrderCreate) (*ResponseOrderModel, error) {
    // 1. Create the domain command
    cmd := NewCommand(ctx, &command.PlaceOrder{
        OrderUuid:    req.Body.OrderUuid,
        CustomerName: req.Body.CustomerName,
        Items:        req.Body.Items,
    })

    // 2. Dispatch to command bus
    if _, err := rs.fc.DispatchCommand(ctx, cmd); err != nil {
        return nil, api.SchemaError(err)
    }

    // 3. Wait for command to complete
    if err := rs.fc.WaitForCmd(ctx, cmd); err != nil {
        return nil, api.SchemaError(err)
    }

    // 4. Retrieve and return the created resource
    return rs.Retrieve(ctx, &RequestOrderModel{
        TenantUuid: req.TenantUuid,
        OrderUuid:  req.Body.OrderUuid,
    })
}
```

### The WaitForCmd Pattern

In an event-sourced system, commands are often processed asynchronously. A command might be placed on a queue, processed by a worker, and only then generate events that update projections. But HTTP clients expect synchronous responses — they send a request and wait for the result.

`WaitForCmd` bridges this gap using a hook-based mechanism. When a command is dispatched with the `ExecuteWaitToFinish` flag set, the facade registers a hook for that command's UUID. The hook blocks until the command handler signals completion (either success or failure). This creates a synchronous facade over an asynchronous system.

```go
// Dispatch returns immediately after the command is accepted
_, err := rs.fc.DispatchCommand(ctx, cmd)

// WaitForCmd blocks until the command is fully processed
err = rs.fc.WaitForCmd(ctx, cmd)
```

The implementation uses channels internally — `WaitForCmd` waits on a channel that the command handler closes when processing completes. A timeout (configurable via `ExecuteTimeout` in the request context) prevents indefinite blocking if something goes wrong.

This pattern provides several benefits:

- **Consistency**: The client sees the result of their action. When the response arrives, the read model has already been updated with the new state.

- **Error reporting**: Command processing errors — validation failures, business rule violations, concurrency conflicts — reach the client as HTTP errors rather than being silently lost in an async queue.

- **Simplicity**: Handlers don't need to manage async callbacks, polling, or webhooks. The complexity is encapsulated in the facade.

- **Timeout handling**: If command processing takes too long, the wait times out and returns an error. The client can decide whether to retry or notify the user.

For scenarios where you explicitly want fire-and-forget behavior (long-running operations, background jobs), you can skip `WaitForCmd` and return immediately after dispatch. The client would then poll a status endpoint or receive a webhook when processing completes.

## Bridging HTTP to Queries

Queries are simpler than commands because they're synchronous by nature. A query reads from a denormalized read model and returns data — there's no state change, no events to wait for, no eventual consistency to bridge.

The query side of CQRS shines in REST APIs. Read models are specifically designed to serve query patterns efficiently. Need to list orders with pagination? The read model stores orders in a queryable format. Need to filter by status or date range? The read model can have appropriate indexes. This is fundamentally different from the command side, where the event store is optimized for append operations, not queries.

Query handlers run synchronously but still benefit from the facade's infrastructure. They execute within a timeout context (preventing runaway queries from blocking indefinitely), pass through query middleware (for logging, caching, metrics), and return strongly-typed responses.

```go
type RequestOrderList struct {
    TenantUuid string `path:"tenantUuid" validate:"required"`
    api.PageSizeOrderBy  // Embedded pagination: Page, PageSize, OrderBy
}

func (rs *Resource) List(ctx context.Context, req *RequestOrderList) (*ResponseOrderList, error) {
    // Extract pagination with defaults
    Page, PageSize, OrderBy := api.ExtractPaginationFrom(ctx, req.PageSizeOrderBy)

    // Create query
    domainQry := &projection.QueryListRequest[*aggregate.Order]{
        TenantUuid: req.TenantUuid,
        Page:       Page,
        PageSize:   PageSize,
        OrderBy:    OrderBy,
    }
    qry := NewQuery(ctx, domainQry)

    // Dispatch and get result
    res, err := rs.fc.DispatchQuery(ctx, qry)
    if err != nil {
        return nil, api.SchemaError(err)
    }

    // Transform to HTTP response
    return api.QueryResponseListWithBodyFrom[*aggregate.Order](res)
}
```

### Single Resource Retrieval

```go
type RequestOrderModel struct {
    TenantUuid string `path:"tenantUuid" validate:"required"`
    OrderUuid  string `path:"orderUuid" validate:"required"`
}

func (rs *Resource) Retrieve(ctx context.Context, req *RequestOrderModel) (*ResponseOrderModel, error) {
    domainQry := &comby.ProjectionQueryModelRequest[*aggregate.Order]{
        AggregateUuid: req.OrderUuid,
    }
    qry := NewQuery(ctx, domainQry)

    res, err := rs.fc.DispatchQuery(ctx, qry)
    if err != nil {
        return nil, api.SchemaError(err)
    }

    return api.QueryResponseModelWithBodyFrom[*aggregate.Order](res)
}
```

## The HTTP Middleware Pipeline

Cross-cutting concerns — authentication, logging, rate limiting, request tracing — shouldn't be implemented in every handler. That leads to code duplication and inconsistency. Instead, these concerns belong in middleware: functions that wrap handlers and execute before (and optionally after) the handler runs.

Middleware forms a pipeline. Each request flows through the pipeline from outer to inner middleware, reaches the handler, and then flows back out. This gives middleware the opportunity to modify the request context on the way in and the response on the way out.

For a CQRS system, the most critical middleware responsibility is context enrichment. HTTP requests arrive with authentication credentials (cookies, tokens, headers), but your command and query handlers need structured context: Who is this user? What tenant do they belong to? What permissions do they have? Middleware extracts this information from HTTP artifacts and places it in the request context where handlers can access it.

The pipeline processes requests in order before they reach handlers:

```
Request
   │
   ▼
┌──────────────────────────┐
│  1. Anonymous Context    │  Set default context values
└──────────────────────────┘
   │
   ▼
┌──────────────────────────┐
│  2. Cookie Parser        │  Extract session/identity cookies
└──────────────────────────┘
   │
   ▼
┌──────────────────────────┐
│  3. Authorization Header │  Parse Bearer token
└──────────────────────────┘
   │
   ▼
┌──────────────────────────┐
│  4. Target Context       │  Extract tenant/aggregate UUIDs
└──────────────────────────┘
   │
   ▼
┌──────────────────────────┐
│  5. Workspace Context    │  Extract workspace UUID (optional)
└──────────────────────────┘
   │
   ▼
┌──────────────────────────┐
│       Handler            │  Your endpoint logic
└──────────────────────────┘
```

### Middleware Registration

Middleware is registered in order on the Huma API object. The order matters — each middleware sees the context as modified by previous middleware. Authentication middleware should run early so that subsequent middleware and handlers have access to the authenticated user.

```go
func RegisterDefaults(fc *comby.Facade, api huma.API) error {
    // Add middleware chain
    api.UseMiddleware(
        middleware.AuthAnonymousCtx(),           // Default context
        middleware.AuthCookiesCtx(fc),           // Cookie authentication
        middleware.AuthAuthorizationCtx(fc),     // Bearer token authentication
        middleware.AuthTargetCtx(),              // Path parameter extraction
    )

    // Register resources...
    return nil
}
```

Let's examine each middleware in detail to understand how they build up the request context.

### Cookie Authentication

Session-based authentication is the traditional approach for web applications. The user logs in, receives a session cookie, and the browser automatically sends this cookie with every subsequent request. This middleware extracts and validates session cookies.

The implementation parses two types of cookies: a session cookie (identifying the login session) and an identity cookie (identifying which tenant identity is active). This separation supports scenarios where a user account can have multiple identities across different tenants — common in B2B SaaS applications.

```go
// Cookie format: session=<sessionUuid|sessionKey>
func AuthCookiesCtx(fc *comby.Facade) func(ctx huma.Context, next func(huma.Context)) {
    return func(ctx huma.Context, next func(huma.Context)) {
        cookie := ctx.Header("Cookie")

        // Parse session cookie
        sessionUuid, sessionKey := parseSessionCookie(cookie)
        if sessionUuid != "" && sessionKey != "" {
            // Validate session
            session := authReadmodel.GetSession(sessionUuid)
            if session != nil && session.IsValid(sessionKey) {
                // Enrich context with authentication data
                ctx = setSessionInContext(ctx, session)
                ctx = setAccountInContext(ctx, session.AccountUuid)
            }
        }

        // Parse identity cookie
        tenantUuid, identityUuid := parseIdentityCookie(cookie)
        if tenantUuid != "" && identityUuid != "" {
            ctx = setIdentityInContext(ctx, identityUuid)
            ctx = setTenantInContext(ctx, tenantUuid)
        }

        next(ctx)
    }
}
```

### Bearer Token Authentication

While cookies work well for browser-based applications, programmatic API clients (mobile apps, CLI tools, third-party integrations) typically use bearer tokens. These are passed in the `Authorization` header and don't rely on browser cookie handling.

This middleware supports two token types: session tokens (for applications that manage their own token storage) and service account tokens (for machine-to-machine communication). Service accounts are particularly useful for background jobs, CI/CD pipelines, and integrations that need API access without a human user context.

```go
// Header format: Authorization: Bearer sa=<tokenUuid|tokenKey>
// Or: Authorization: Bearer session=<sessionUuid|sessionKey>
func AuthAuthorizationCtx(fc *comby.Facade) func(ctx huma.Context, next func(huma.Context)) {
    return func(ctx huma.Context, next func(huma.Context)) {
        authHeader := ctx.Header("Authorization")

        // Extract service account token
        tokenUuid, tokenKey := parseServiceAccountToken(authHeader)
        if tokenUuid != "" && tokenKey != "" {
            // Validate and enrich context...
        }

        next(ctx)
    }
}
```

### Workspace Context Middleware

Some applications have an additional scoping level below tenants: workspaces. A tenant might have multiple workspaces representing different projects, teams, or environments. Resources within a workspace are isolated from other workspaces in the same tenant.

This middleware is applied selectively to workspace-scoped endpoints rather than globally. It extracts the workspace UUID from the path and makes it available in the request context. The middleware can also perform authorization checks — verifying that the authenticated user has access to the specified workspace.

```go
func (rs *Resource) Register() {
    // Workspace-scoped endpoint
    huma.Register(rs.api, huma.Operation{
        OperationID: "order-list-workspace",
        Method:      http.MethodGet,
        Path:        "/api/tenants/{tenantUuid}/workspaces/{workspaceUuid}/orders",
        Middlewares: huma.Middlewares{
            middleware.AuthWorkspaceCtx(
                middleware.AuthWorkspaceCtxWithWorkspaceFieldName("workspaceUuid"),
            ),
        },
    }, rs.ListByWorkspace)
}
```

## Context Enrichment

The middleware pipeline's ultimate purpose is context enrichment. Raw HTTP requests contain authentication credentials scattered across headers, cookies, and path parameters. By the time a request reaches your handler, middleware has extracted, validated, and organized this information into a clean, structured context.

This enriched context is crucial for CQRS systems. Commands need to know who issued them — for audit trails, authorization decisions, and business rules that depend on the caller's identity. Queries might filter results based on tenant or workspace membership. Rather than passing this information as explicit parameters to every function, it flows through the context.

Handlers access the enriched context through type-safe helper functions:

```go
func (rs *Resource) Create(ctx context.Context, req *RequestOrderCreate) (*ResponseOrderModel, error) {
    // These are populated by middleware
    sessionUuid := api.SessionUuidFromContext(ctx)
    accountUuid := api.AccountUuidFromContext(ctx)
    identityUuid := api.IdentityUuidFromContext(ctx)
    tenantUuid := api.TenantUuidFromContext(ctx)
    workspaceUuid := api.WorkspaceUuidFromContext(ctx)

    // Create command with full context
    cmd := NewCommand(ctx, &command.PlaceOrder{...})

    // Command automatically receives sender info from context
    // - SenderTenantUuid
    // - SenderIdentityUuid
    // - SenderAccountUuid
    // - SenderSessionUuid
    // - TargetWorkspaceUuid

    // ...
}
```

The `NewCommand` helper automatically extracts context values and packages them into a `RequestContext` that travels with the command through the system:

```go
func NewCommand(ctx context.Context, domain string, domainCmd comby.DomainCmd) comby.Command {
    cmd := comby.NewCommand(domain, domainCmd)

    reqCtx := comby.NewRequestContext()
    reqCtx.SenderSessionUuid = SessionUuidFromContext(ctx)
    reqCtx.SenderAccountUuid = AccountUuidFromContext(ctx)
    reqCtx.SenderIdentityUuid = IdentityUuidFromContext(ctx)
    reqCtx.SenderTenantUuid = TenantUuidFromContext(ctx)
    reqCtx.TargetTenantUuid = DstTenantUuidFromContext(ctx)
    reqCtx.TargetWorkspaceUuid = DstWorkspaceUuidFromContext(ctx)

    cmd.SetReqCtx(reqCtx)

    return cmd
}
```

The `RequestContext` distinguishes between "sender" fields (who is performing the action) and "target" fields (what they're acting on). This separation is important in multi-tenant systems where an administrator might act on resources in a different tenant than their own. The sender information enables audit logging, while the target information enables routing and authorization.

## Domain-Level Middleware

HTTP middleware handles HTTP-specific concerns: parsing cookies, validating tokens, extracting path parameters. But what about concerns that apply to all commands or queries regardless of how they arrive? Logging every command execution, measuring handler performance, enforcing cross-cutting authorization rules — these belong in domain-level middleware.

Domain middleware wraps command and query handlers, not HTTP handlers. This means they apply whether the command comes from an HTTP request, a message queue, a CLI tool, or a scheduled job. The middleware doesn't know or care about HTTP — it operates purely in the domain layer.

### Command Handler Middleware

Command handler middleware wraps the execution of domain commands. Each middleware receives the handler function and returns a new handler function that adds its behavior:

```go
type CommandHandlerMiddlewareFunc func(
    ctx context.Context,
    fn DomainCommandHandlerFunc,
) DomainCommandHandlerFunc
```

Middleware is applied in reverse order, creating a nested chain where the outermost middleware executes first:

```go
// Registration
fc.AddCommandHandlerMiddleware(loggingMiddleware)
fc.AddCommandHandlerMiddleware(metricsMiddleware)
fc.AddCommandHandlerMiddleware(authorizationMiddleware)

// Execution order: logging → metrics → authorization → handler
```

A typical logging middleware might look like this:

```go
func LoggingMiddleware() CommandHandlerMiddlewareFunc {
    return func(ctx context.Context, next DomainCommandHandlerFunc) DomainCommandHandlerFunc {
        return func(ctx context.Context, cmd Command, domainCmd DomainCmd) (DomainCmdResponse, error) {
            start := time.Now()
            log.Printf("Command started: %s (uuid: %s)",
                cmd.GetDomainCmdName(), cmd.GetCommandUuid())

            // Call the next handler in the chain
            res, err := next(ctx, cmd, domainCmd)

            duration := time.Since(start)
            if err != nil {
                log.Printf("Command failed: %s (uuid: %s, duration: %v, error: %v)",
                    cmd.GetDomainCmdName(), cmd.GetCommandUuid(), duration, err)
            } else {
                log.Printf("Command completed: %s (uuid: %s, duration: %v)",
                    cmd.GetDomainCmdName(), cmd.GetCommandUuid(), duration)
            }

            return res, err
        }
    }
}
```

### Query Handler Middleware

Query handlers have their own middleware chain with the same pattern:

```go
type QueryHandlerMiddlewareFunc func(
    ctx context.Context,
    fn DomainQueryHandlerFunc,
) DomainQueryHandlerFunc
```

Query middleware is useful for caching (check cache before executing, store result after), performance monitoring, and query-level authorization. The structure mirrors command middleware:

```go
func CachingMiddleware(cache Cache) QueryHandlerMiddlewareFunc {
    return func(ctx context.Context, next DomainQueryHandlerFunc) DomainQueryHandlerFunc {
        return func(ctx context.Context, qry Query, domainQry DomainQry) (QueryResponse, error) {
            // Check cache
            cacheKey := generateCacheKey(qry)
            if cached, found := cache.Get(cacheKey); found {
                return cached, nil
            }

            // Execute query
            res, err := next(ctx, qry, domainQry)
            if err != nil {
                return nil, err
            }

            // Store in cache
            cache.Set(cacheKey, res, cacheTTL)
            return res, nil
        }
    }
}
```

### Why Two Middleware Layers?

Having both HTTP middleware and domain middleware might seem redundant, but they serve different purposes:

| Concern | HTTP Middleware | Domain Middleware |
|---------|-----------------|-------------------|
| Cookie parsing | ✓ | |
| Bearer token validation | ✓ | |
| Path parameter extraction | ✓ | |
| Request/Response logging | ✓ | |
| Command execution logging | | ✓ |
| Performance metrics | | ✓ |
| Domain-level authorization | | ✓ |
| Caching | | ✓ |
| Rate limiting | ✓ | |

HTTP middleware is specific to HTTP — it deals with headers, cookies, status codes. Domain middleware is transport-agnostic — it works with commands and queries regardless of how they arrive. A well-designed system uses both layers, each handling concerns appropriate to its level.

## Error Handling

Error handling is where the API layer earns its keep as a translation layer. Your domain layer speaks in domain errors: `NotFound`, `PermissionDenied`, `ValidationFailed`, `ConcurrencyConflict`. HTTP speaks in status codes: 404, 403, 400, 409. The API layer must translate between these languages consistently.

Inconsistent error handling is a common source of frustration for API consumers. If `NotFound` returns 404 in one endpoint but 400 in another, clients can't build reliable error handling logic. A centralized error mapping function ensures consistency across all endpoints.

The `SchemaError` function examines domain errors and returns appropriate Huma status errors:

```go
func SchemaError(err error) huma.StatusError {
    if err == nil {
        return nil
    }

    // Map known errors to HTTP status codes
    switch {
    case errors.Is(err, comby.ErrUnauthorized):
        return huma.Error401Unauthorized(err.Error())

    case errors.Is(err, comby.ErrPermissionDenied):
        return huma.Error403Forbidden(err.Error())

    case errors.Is(err, comby.ErrNotFound):
        return huma.Error404NotFound(err.Error())

    case errors.Is(err, comby.ErrConflict):
        return huma.Error409Conflict(err.Error())

    case errors.Is(err, comby.ErrRequestTimeout):
        return huma.Error408RequestTimeout(err.Error())

    case errors.Is(err, comby.ErrInternal):
        return huma.Error500InternalServerError(err.Error())

    default:
        // Default to 400 Bad Request
        return huma.Error400BadRequest(err.Error())
    }
}
```

The function uses `errors.Is` to check error types, which correctly handles wrapped errors. This is important because domain errors are often wrapped with additional context as they propagate through layers.

Usage in handlers is straightforward — wrap every error with `SchemaError`:

```go
func (rs *Resource) Create(ctx context.Context, req *RequestOrderCreate) (*ResponseOrderModel, error) {
    _, err := rs.fc.DispatchCommand(ctx, cmd)
    if err != nil {
        return nil, api.SchemaError(err)  // Always wrap with SchemaError
    }
    // ...
}
```

This pattern has a subtle but important benefit: it centralizes the decision about which errors are internal implementation details versus which should be exposed to clients. A database connection error might be `ErrInternal` (don't expose details), while a business rule violation might be `ErrBadRequest` (expose the message). The mapping function makes these decisions explicit and consistent.

## Security Schemes

Not all endpoints require the same level of authentication. A registration endpoint must be accessible without authentication — otherwise, how would new users sign up? An admin endpoint might require not just authentication but specific roles. OpenAPI security schemes let you declare these requirements declaratively, and Huma uses them both for documentation and runtime enforcement.

Security schemes define what credentials are required and how they're provided. Common schemes include cookies (for browser-based sessions), bearer tokens (for API clients), and API keys (for simple integrations). Endpoints reference schemes by name, creating a clear contract between API and client.

```go
// Security scheme definitions
var SecuritySchemeEmpty = []map[string][]string{{}}  // No auth required

var SecuritySchemeSessionIdentity = []map[string][]string{
    {"sessionCookie": {}, "identityCookie": {}},  // Default: cookies required
}
```

Usage:

```go
// Public endpoint (no authentication)
huma.Register(rs.api, huma.Operation{
    OperationID: "account-register",
    Method:      http.MethodPost,
    Path:        "/api/accounts",
    Security:    api.SecuritySchemeEmpty,  // No auth required
}, rs.Register)

// Protected endpoint (default: requires authentication)
huma.Register(rs.api, huma.Operation{
    OperationID: "order-create",
    Method:      http.MethodPost,
    Path:        "/api/tenants/{tenantUuid}/orders",
    // No Security field = default authentication required
}, rs.Create)
```

## Response Patterns

Consistent response patterns make APIs predictable and easy to consume. Clients shouldn't have to guess whether a field is called `data` or `result` or `payload`. They shouldn't wonder whether pagination info is in headers or the response body. Establish patterns early and apply them consistently.

The response wrappers provide this consistency by standardizing how resources and collections are returned.

### Single Resource Response

Single resource responses wrap the domain object in a container that can include metadata. The `ProjectionModel` wrapper provides the resource data along with optional history (the events that led to the current state) — useful for debugging and audit trails.

```go
type ResponseOrderModel struct {
    Body *comby.ProjectionModel[*aggregate.Order]
}

// Helper to construct response
func QueryResponseModelWithBodyFrom[T any](res any) (*QueryResponseWithBody[T], error) {
    if model, ok := res.(*comby.ProjectionModel[T]); ok {
        return &QueryResponseWithBody[T]{Body: model}, nil
    }
    return nil, fmt.Errorf("unexpected response type")
}
```

### List Response with Pagination

List endpoints return collections with pagination metadata. The client needs to know not just the items on the current page, but how many total items exist and what page they're viewing. This enables UI pagination controls and helps clients decide whether to fetch more data.

```go
type ResponseOrderList struct {
    Body *comby.ProjectionQueryListResponse[*aggregate.Order]
}

// The response includes pagination metadata
// {
//   "data": [...],
//   "total": 100,
//   "page": 1,
//   "pageSize": 30
// }
```

The pagination parameters (`page`, `pageSize`, `orderBy`) are typically extracted from query strings using embedded structs in the request type. This keeps pagination logic consistent across all list endpoints without duplicating code.

### Delete Response

Delete operations follow REST conventions by returning 204 No Content on success. There's no body to return — the resource no longer exists. The handler dispatches the delete command, waits for it to complete, and returns nil for both the response and error.

```go
type RequestOrderDelete struct {
    TenantUuid string `path:"tenantUuid" validate:"required"`
    OrderUuid  string `path:"orderUuid" validate:"required"`
}

func (rs *Resource) Delete(ctx context.Context, req *RequestOrderDelete) (*api.ResponseEmpty, error) {
    cmd := NewCommand(ctx, &command.DeleteOrder{
        OrderUuid: req.OrderUuid,
    })

    if _, err := rs.fc.DispatchCommand(ctx, cmd); err != nil {
        return nil, api.SchemaError(err)
    }

    if err := rs.fc.WaitForCmd(ctx, cmd); err != nil {
        return nil, api.SchemaError(err)
    }

    return nil, nil  // 204 No Content
}
```

Note that in an event-sourced system, "delete" often doesn't mean physical deletion. The aggregate might record a `Deleted` event and mark itself as deleted, but all previous events remain in the event store. This enables audit trails and potential recovery. The API returns 204 regardless — from the client's perspective, the resource is gone.

## Path Conventions

URL design matters more than you might think. Well-designed paths are intuitive, predictable, and self-documenting. A developer looking at `/api/tenants/{tenantUuid}/orders/{orderUuid}` immediately understands the resource hierarchy without reading documentation.

In a multi-tenant system, the tenant context should be explicit in the path. This makes it impossible to accidentally access the wrong tenant's data — the tenant UUID is right there in the URL, validated by middleware before the request reaches your handler.

Follow RESTful conventions with multi-tenant awareness:

```
# Tenant-scoped resources
POST   /api/tenants/{tenantUuid}/orders
GET    /api/tenants/{tenantUuid}/orders
GET    /api/tenants/{tenantUuid}/orders/{orderUuid}
PATCH  /api/tenants/{tenantUuid}/orders/{orderUuid}
DELETE /api/tenants/{tenantUuid}/orders/{orderUuid}

# Workspace-scoped resources
POST   /api/tenants/{tenantUuid}/workspaces/{workspaceUuid}/orders
GET    /api/tenants/{tenantUuid}/workspaces/{workspaceUuid}/orders

# Nested resources
GET    /api/tenants/{tenantUuid}/orders/{orderUuid}/items
POST   /api/tenants/{tenantUuid}/orders/{orderUuid}/items

# Custom operations
POST   /api/tenants/{tenantUuid}/orders/{orderUuid}/ship
POST   /api/tenants/{tenantUuid}/orders/{orderUuid}/cancel
```

Custom operations (like `/ship` or `/cancel`) use POST because they trigger state changes — they're commands, not resource creations. Some REST purists argue for PATCH instead, but POST makes the intent clearer: you're performing an action, not partially updating a resource.

## Central Registration

With resources split into separate packages, you need a central location that wires everything together. This is where you configure the middleware pipeline and register all resources. The central registration file serves as a table of contents for your API — a single place to see what endpoints exist.

Wire everything together in `api/api.go`:

```go
package api

import (
    "github.com/danielgtaylor/huma/v2"
    "github.com/gradientzero/comby/v2"
    "github.com/yourapp/api/order"
    "github.com/yourapp/api/customer"
)

func RegisterEndpoints(fc *comby.Facade, api huma.API) error {
    // Configure default security
    api.UseMiddleware(
        middleware.AuthAnonymousCtx(),
        middleware.AuthCookiesCtx(fc),
        middleware.AuthAuthorizationCtx(fc),
        middleware.AuthTargetCtx(),
    )

    // Register all resources
    order.NewResource(fc, api).Register()
    customer.NewResource(fc, api).Register()

    return nil
}
```

The `main.go` file bootstraps the entire system. It creates the facade (which manages command/query buses, event store, and projections), registers domain handlers, restores state from persisted events, and then wires up the HTTP layer:

```go
func main() {
    // Create facade with configured stores
    fc, _ := comby.NewFacade(...)

    // Register domain aggregates, command handlers, query handlers, and event handlers
    domain.RegisterDomains(context.Background(), fc)

    // Replay events to rebuild read model state
    fc.RestoreState()

    // Create HTTP router and Huma API
    router := chi.NewRouter()
    humaAPI := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))

    // Register middleware and API endpoints
    api.RegisterEndpoints(fc, humaAPI)

    // Start HTTP server
    http.ListenAndServe(":8080", router)
}
```

The order of operations matters: the facade must be created first, domains registered, and state restored before the API can serve requests. Otherwise, queries would return empty results because read models haven't been populated from the event store.

## Best Practices

The following practices emerge from building real CQRS APIs. They're not theoretical — they're lessons learned from maintaining production systems.

### 1. Keep Handlers Thin

The API layer is a translation layer, not a business logic layer. Handlers should be dumb pipes that transform HTTP into domain operations and back. If you find yourself writing business logic in a handler, stop — that code belongs in a command handler or aggregate.

Handlers should only:
- Parse requests (handled automatically by Huma)
- Create commands/queries (one-liner using helper functions)
- Dispatch to facade (one-liner)
- Transform responses (one-liner using response helpers)

```go
// Good: Thin handler
func (rs *Resource) Create(ctx context.Context, req *RequestOrderCreate) (*ResponseOrderModel, error) {
    cmd := NewCommand(ctx, &command.PlaceOrder{...})

    if _, err := rs.fc.DispatchCommand(ctx, cmd); err != nil {
        return nil, api.SchemaError(err)
    }

    if err := rs.fc.WaitForCmd(ctx, cmd); err != nil {
        return nil, api.SchemaError(err)
    }

    return rs.Retrieve(ctx, &RequestOrderModel{...})
}

// Bad: Business logic in handler
func (rs *Resource) Create(ctx context.Context, req *RequestOrderCreate) (*ResponseOrderModel, error) {
    // Don't do this - belongs in domain layer
    if req.Body.Total <= 0 {
        return nil, errors.New("total must be positive")
    }
    // ...
}
```

### 2. Consistent Error Handling

Every error that leaves a handler should pass through `SchemaError`. This isn't just about consistency — it's about security. Unfiltered errors might leak implementation details (stack traces, database errors, internal paths) to clients. `SchemaError` ensures errors are sanitized and mapped to appropriate HTTP status codes.

```go
if err != nil {
    return nil, api.SchemaError(err)  // Consistent mapping
}
```

Make this a habit. Even if you're confident an error can't contain sensitive information, wrap it anyway. Future changes might introduce sensitive data, and the wrapper provides a safety net.

### 3. Reuse Retrieve for Create/Update

After a successful write, return the resource using the retrieve handler. This ensures the response format is identical whether the client creates a resource or fetches it later. It also means you only have one place to change if the response structure needs updating.

```go
func (rs *Resource) Create(...) (*ResponseOrderModel, error) {
    // ... dispatch command

    // Reuse Retrieve for consistent response
    return rs.Retrieve(ctx, &RequestOrderModel{
        TenantUuid: req.TenantUuid,
        OrderUuid:  req.Body.OrderUuid,
    })
}
```

This pattern also guarantees that the returned data reflects the actual state after the command executed, including any side effects or derived values computed by event handlers.

### 4. Use Type Aliases for Responses

Generic types with nested type parameters get verbose quickly. Type aliases at the resource level keep handler signatures readable and provide a single place to change if you need to modify the response wrapper.

```go
// At resource level
type ResponseOrderModel = api.QueryResponseWithBody[*aggregate.Order]
type ResponseOrderList = api.QueryResponseListWithBody[*aggregate.Order]

// Cleaner handler signatures
func (rs *Resource) Retrieve(...) (*ResponseOrderModel, error)
func (rs *Resource) List(...) (*ResponseOrderList, error)
```

This also improves IDE autocompletion — typing `Response` in the order package shows relevant response types immediately.

### 5. Secure by Default

Security should be opt-out, not opt-in. If you forget to add authentication to an endpoint, it should be protected by default. Only when you explicitly want a public endpoint should you override the default.

Omit the `Security` field for protected endpoints — they use the default authentication:

```go
// Explicit: only when you need to override
huma.Register(rs.api, huma.Operation{
    Security: api.SecuritySchemeEmpty,  // Public endpoint - explicit choice
}, rs.Register)

// Default: authenticated
huma.Register(rs.api, huma.Operation{
    // No Security field = requires authentication (safe default)
}, rs.Create)
```

This "secure by default" approach means new endpoints are protected automatically. A developer adding a new endpoint doesn't need to remember to add authentication — they need to remember to remove it if the endpoint should be public. This inverts the failure mode: forgetting something results in too much security rather than too little.

## Running an Example

Source: https://github.com/susdorf/building-event-sourced-systems-in-go/tree/main/16-rest-api-connecting-http-to-cqrs/code

The example demonstrates a complete REST API connected to a simplified CQRS backend. It includes:
- A domain layer with commands, queries, and a facade
- An API layer with HTTP middleware and handlers
- Domain-level middleware for logging
- Error mapping from domain to HTTP status codes
- Multi-tenant isolation

When you run the example, you'll see the full request/response cycle:

```
=== REST API + CQRS Demo ===

1. Creating Facade (domain orchestrator)...
   -> Facade created with in-memory storage

2. Setting up HTTP server with Huma...
   -> Router: Chi
   -> Framework: Huma v2
   -> OpenAPI: Auto-generated at /docs

3. Registering middleware and endpoints...
   -> HTTP Middleware: AuthAnonymous -> AuthBearer -> AuthTarget
   -> Domain Middleware: CommandLogging, QueryLogging
   -> Endpoints registered:
      POST   /api/tenants/{tenantUuid}/orders
      GET    /api/tenants/{tenantUuid}/orders
      GET    /api/tenants/{tenantUuid}/orders/{orderUuid}
      PATCH  /api/tenants/{tenantUuid}/orders/{orderUuid}
      POST   /api/tenants/{tenantUuid}/orders/{orderUuid}/ship
      POST   /api/tenants/{tenantUuid}/orders/{orderUuid}/cancel

4. Starting HTTP server on :8080...
   -> Server running!

=== Running Demo Requests ===

--- Demo 1: Create Order (POST) ---
   Created order: HTTP 200
   -> Order: order-001, Status: pending, Total: 1059.97

--- Demo 2: Get Order (GET) ---
   Retrieved order: HTTP 200
   -> Order: order-001, Status: pending, Total: 1059.97

--- Demo 3: List Orders (GET with pagination) ---
   Order list: HTTP 200
   -> Found 1 items (total: 1)

--- Demo 4: Update Order (PATCH) ---
   Updated order: HTTP 200
   -> Order: order-001, Status: pending, Total: 1439.96

--- Demo 5: Ship Order (POST custom action) ---
   Shipped order: HTTP 200
   -> Order: order-001, Status: shipped, Total: 1439.96

--- Demo 6: Cancel Shipped Order (should fail) ---
   Cancel attempt: HTTP 400
   -> Error: Bad Request - bad request: cannot cancel shipped order

--- Demo 7: Create and Cancel Order ---
   Created order-002: HTTP 200
   -> Order: order-002, Status: pending, Total: 59.97
   Cancelled order-002: HTTP 200
   -> Order: order-002, Status: cancelled, Total: 59.97

--- Demo 8: List Orders by Status ---
   Shipped orders: HTTP 200
   -> Found 1 items (total: 1)
   Cancelled orders: HTTP 200
   -> Found 1 items (total: 1)

--- Demo 9: Multi-Tenant Isolation ---
   Created order in tenant-B: HTTP 200
   -> Order: order-B-001, Status: pending, Total: 129.9
   Orders in tenant-A (should not include tenant-B orders): HTTP 200
   -> Found 2 items (total: 2)
   Orders in tenant-B: HTTP 200
   -> Found 1 items (total: 1)

--- Demo 10: Error Handling ---
   Get non-existent order (404): HTTP 404
   -> Error: Not Found - not found: order not found
   Create duplicate order (409): HTTP 409
   -> Error: Conflict - conflict: order already exists

=== Metrics ===
Commands executed: 8
Queries executed: 13
```

The domain-level logging middleware shows the command/query flow:

```
[CMD] Started: *domain.PlaceOrderCommand (uuid: d498790d, tenant: tenant-A)
[CMD] Completed: *domain.PlaceOrderCommand (uuid: d498790d, duration: 62.667µs)
[QRY] Started: *domain.GetOrderQuery (uuid: 84772d14, tenant: tenant-A)
[QRY] Completed: *domain.GetOrderQuery (uuid: 84772d14, duration: 1.834µs)
...
[CMD] Started: *domain.CancelOrderCommand (uuid: e8ca1449, tenant: tenant-A)
[CMD] Failed: *domain.CancelOrderCommand (uuid: e8ca1449, duration: 24.125µs, error: bad request: cannot cancel shipped order)
```

After running the demo, the server stays running. You can test it with curl:

```bash
# Create an order
curl -X POST http://localhost:8080/api/tenants/tenant-demo/orders \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer user:alice" \
  -d '{"orderUuid":"my-order","customerName":"Bob","items":[{"name":"Widget","quantity":1,"price":9.99}]}'

# List orders
curl http://localhost:8080/api/tenants/tenant-demo/orders

# View OpenAPI docs
curl http://localhost:8080/docs
```

## Summary

Building a REST API for a CQRS system requires careful attention to the boundary between HTTP and domain concerns. The API layer is a translation layer — nothing more, nothing less. It doesn't contain business logic. It doesn't make domain decisions. It translates.

The key architectural elements we've explored:

1. **Clean separation**: HTTP concerns (cookies, headers, status codes) stay in the API layer. Domain concerns (business rules, events, aggregates) stay in the domain layer. Neither leaks into the other.

2. **Resource organization**: Group endpoints by resource, not HTTP method. This mirrors how developers think about APIs and makes the codebase intuitive to navigate.

3. **Two middleware layers**: HTTP middleware handles transport-specific concerns (authentication extraction, request logging). Domain middleware handles transport-agnostic concerns (command logging, metrics, caching). Both are necessary for a complete solution.

4. **Command/Query helpers**: Centralize command and query creation to ensure consistency and reduce boilerplate. Helper functions bind context data automatically.

5. **The WaitForCmd pattern**: Bridge the async nature of event sourcing with the sync nature of HTTP. Commands are dispatched and waited upon, ensuring clients receive responses that reflect the completed operation.

6. **Consistent error mapping**: A single function translates domain errors to HTTP status codes. This ensures consistency across endpoints and prevents accidental information leakage.

7. **Type-safe responses**: Generic response wrappers with proper serialization. Type aliases keep handler signatures clean.

8. **Secure by default**: Endpoints require authentication unless explicitly configured otherwise. Forgetting something results in too much security, not too little.

The key insight is that your API layer is purely a **translation layer** — it transforms HTTP requests into Commands and Queries, dispatches them to your domain, and transforms the responses back to HTTP. When you find yourself writing business logic in a handler, you've crossed the boundary. Move that logic to where it belongs: command handlers, aggregates, or event handlers.

Keep handlers thin. Keep domain logic in your domain. Let the framework handle the HTTP ceremony.

## What's Next

This completes our series on building event-sourced systems in Go. We've covered:

- Event Sourcing and CQRS fundamentals
- Aggregates and consistency boundaries
- Go patterns: functional options, generics, reflection
- Projections and read models
- Async command processing
- Multi-tenancy and workspaces
- Testing strategies
- And now, REST APIs

With these patterns, you have a solid foundation for building robust, scalable, event-sourced applications in Go.