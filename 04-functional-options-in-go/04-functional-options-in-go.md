# Functional Options in Go

*This is Part 4 of the "Building Event-Sourced Systems in Go" series. This series originated from the development of comby – an application framework developed using the principles of Event Sourcing and Command Query Responsibility Segregation (CQRS) - which ist still closed source today. Code with concepts and techniques is presented, but unlike comby, it is not always suitable for product use.*

---

## Introduction

You might wonder: why dedicate an entire article to a configuration pattern in a series about event sourcing?

The answer comes from years of experience building comby. Event-sourced systems are inherently complex — they involve event stores, command handlers, projections, repositories, and numerous components that need to be configured and wired together. Early versions of comby used traditional approaches: long parameter lists, config structs, builder patterns. Every time we added a new option or changed a default, it triggered a cascade of changes across the codebase. Methods that called other methods needed updating. Tests broke. The maintenance burden grew exponentially.

**Functional Options solved this problem completely.** Once we refactored (more than once) comby to use this pattern, adding new configuration options became trivial — existing code continued to work unchanged. This single pattern transformed our ability to evolve the framework without breaking existing applications.

If you're serious about building professional Go applications — especially complex systems like event-sourced architectures — understanding **Functional Options** isn't optional. It's a pattern you'll encounter in `grpc`, `zap`, `fx`, and virtually every well-designed Go library. More importantly, it's a pattern that will save you countless hours of refactoring.

Let's understand why it works so well and how to implement it.

## The Problem: Configuration Complexity

Consider creating an event store for an event-sourced system. It needs many optional settings for listing events:

```go
// Attempt 1: Single function with all parameters
func (s *EventStore) List(
    ctx context.Context,
    tenantUuid string,
    aggregateUuid string,
    domains []string,
    dataType string,
    before int64,
    after int64,
    offset int64,
    limit int64,
    orderBy string,
    ascending bool,
) ([]Event, int64, error)

// Usage is unwieldy
events, total, err := store.List(
    ctx,
    "tenant-123",
    "",     // What is this?
    nil,    // And this?
    "",     // Hard to remember
    0,      // Is 0 "no filter" or "filter for timestamp 0"?
    0,
    0,
    100,
    "created_at",
    true,
)
```

Problems:
- Parameter order is arbitrary and error-prone
- Empty/zero values for optional parameters are unclear
- Adding new parameters breaks all call sites
- No good defaults

```go
// Attempt 2: Config struct
type ListConfig struct {
    TenantUuid    string
    AggregateUuid string
    Domains       []string
    DataType      string
    Before        int64
    After         int64
    Offset        int64
    Limit         int64
    OrderBy       string
    Ascending     bool
}

func (s *EventStore) List(ctx context.Context, config ListConfig) ([]Event, int64, error)

// Better, but...
events, total, err := store.List(ctx, ListConfig{
    TenantUuid: "tenant-123",
    Limit:      100,
    // All other fields get zero values - are those the defaults we want?
    // Is Ascending: false intentional or just unset?
})
```

Problems:
- Zero values might not be valid defaults
- Can't validate configuration at construction time
- No way to distinguish "not set" from "set to zero value"

## The Solution: Functional Options

Functional Options use functions to configure a method call or object:

```go
// Option is a function that modifies options
type EventStoreListOption func(opt *EventStoreListOptions) (*EventStoreListOptions, error)

// EventStoreListOptions holds all configuration for listing events
type EventStoreListOptions struct {
    TenantUuid    string
    AggregateUuid string
    Domains       []string
    DataType      string
    Before        int64
    After         int64
    Offset        int64
    Limit         int64
    OrderBy       string
    Ascending     bool
}

// List accepts variadic options
func (s *EventStore) List(ctx context.Context, opts ...EventStoreListOption) ([]Event, int64, error) {
    // Start with defaults
    options := &EventStoreListOptions{
        Limit:     100,
        OrderBy:   "created_at",
        Ascending: false,
    }

    // Apply each option
    for _, opt := range opts {
        var err error
        options, err = opt(options)
        if err != nil {
            return nil, 0, fmt.Errorf("invalid option: %w", err)
        }
    }

    // Now use options to build and execute the query...
    return s.executeListQuery(ctx, options)
}
```

## Creating Option Functions

Each option is a function that returns an `EventStoreListOption`:

```go
// EventStoreListOptionWithTenantUuid filters events by tenant
func EventStoreListOptionWithTenantUuid(tenantUuid string) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        opt.TenantUuid = tenantUuid
        return opt, nil
    }
}

// EventStoreListOptionWithAggregateUuid filters events by aggregate
func EventStoreListOptionWithAggregateUuid(aggregateUuid string) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        opt.AggregateUuid = aggregateUuid
        return opt, nil
    }
}

// EventStoreListOptionWithDomains filters events by domain(s)
func EventStoreListOptionWithDomains(domains ...string) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        opt.Domains = append(opt.Domains, domains...)
        return opt, nil
    }
}

// EventStoreListOptionLimit sets the maximum number of events to return
func EventStoreListOptionLimit(limit int64) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        if limit <= 0 {
            return nil, errors.New("limit must be positive")
        }
        opt.Limit = limit
        return opt, nil
    }
}

// EventStoreListOptionAscending sets the sort order
func EventStoreListOptionAscending(ascending bool) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        opt.Ascending = ascending
        return opt, nil
    }
}
```

## Convenience Options

Options can encapsulate complex logic or combine multiple settings:

```go
// EventStoreListOptionBefore filters events created before a timestamp
func EventStoreListOptionBefore(unixTimestamp int64) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        opt.Before = unixTimestamp
        return opt, nil
    }
}

// EventStoreListOptionAfter filters events created after a timestamp
func EventStoreListOptionAfter(unixTimestamp int64) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        opt.After = unixTimestamp
        return opt, nil
    }
}

// EventStoreListOptionTimeRange combines before and after into a single option
func EventStoreListOptionTimeRange(start, end time.Time) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        if end.Before(start) {
            return nil, errors.New("end time must be after start time")
        }
        opt.After = start.UnixNano()
        opt.Before = end.UnixNano()
        return opt, nil
    }
}

// EventStoreListOptionPagination combines offset and limit
func EventStoreListOptionPagination(page, pageSize int64) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        if page < 1 {
            return nil, errors.New("page must be >= 1")
        }
        if pageSize <= 0 {
            return nil, errors.New("pageSize must be positive")
        }
        opt.Offset = (page - 1) * pageSize
        opt.Limit = pageSize
        return opt, nil
    }
}
```

## Usage: Clean and Readable

```go
// Get all events for an aggregate
events, total, err := store.List(ctx,
    EventStoreListOptionWithAggregateUuid("order-123"),
)

// Get recent events for a tenant with pagination
events, total, err := store.List(ctx,
    EventStoreListOptionWithTenantUuid("tenant-456"),
    EventStoreListOptionPagination(1, 50),
    EventStoreListOptionAscending(false),
)

// Get events in a time range for specific domains
events, total, err := store.List(ctx,
    EventStoreListOptionWithDomains("orders", "inventory"),
    EventStoreListOptionTimeRange(startOfDay, endOfDay),
    EventStoreListOptionLimit(1000),
)

// Minimal call - uses all defaults
events, total, err := store.List(ctx)
```

## Real-World Example from Comby

The comby framework uses this pattern extensively. Here's a view from the actual `EventStore` interface:

```go
// From comby/store.event.go

// EventStore defines the interface for an event storage system.
type EventStore interface {
    Init(ctx context.Context, opts ...EventStoreOption) error
    Create(ctx context.Context, opts ...EventStoreCreateOption) error
    Get(ctx context.Context, opts ...EventStoreGetOption) (Event, error)
    List(ctx context.Context, opts ...EventStoreListOption) ([]Event, int64, error)
    Update(ctx context.Context, opts ...EventStoreUpdateOption) error
    Delete(ctx context.Context, opts ...EventStoreDeleteOption) error
    // ... more methods
}

// EventStoreListOptions holds options for listing events.
type EventStoreListOptions struct {
    TenantUuid    string
    AggregateUuid string
    Domains       []string
    DataType      string
    Before        int64
    After         int64
    Offset        int64
    Limit         int64
    OrderBy       string
    Ascending     bool
    Attributes    *Attributes
}

// EventStoreListOption defines a function type for setting options when listing events.
type EventStoreListOption func(opt *EventStoreListOptions) (*EventStoreListOptions, error)

// Option implementations
func EventStoreListOptionWithTenantUuid(tenantUuid string) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        opt.TenantUuid = tenantUuid
        return opt, nil
    }
}

func EventStoreListOptionWithAggregateUuid(aggregateUuid string) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        opt.AggregateUuid = aggregateUuid
        return opt, nil
    }
}

func EventStoreListOptionWithDomains(domain ...string) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        opt.Domains = append(opt.Domains, domain...)
        return opt, nil
    }
}

func EventStoreListOptionBefore(unixTimestamp int64) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        opt.Before = unixTimestamp
        return opt, nil
    }
}

func EventStoreListOptionAfter(unixTimestamp int64) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        opt.After = unixTimestamp
        return opt, nil
    }
}

func EventStoreListOptionOffset(offset int64) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        opt.Offset = offset
        return opt, nil
    }
}

func EventStoreListOptionLimit(limit int64) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        opt.Limit = limit
        return opt, nil
    }
}
```

## The Pattern for Other Components

The same pattern works for any configurable operation — not just infrastructure components like stores, but also in your domain layer. In comby, we use Functional Options extensively in **aggregates** to keep domain methods flexible and maintainable.

Here's an example from the `Account` aggregate's login functionality:

```go
// From comby/domain/account/aggregate/login.go (simplified)

// LoginOptions contains options for the login process.
type LoginOptions struct {
    AuthModel   string // Authentication model: "emailPassword" or "opaque"
    Email       string
    Password    string
    SessionData []byte // For OPAQUE authentication
}

// LoginOption defines a function type for setting options.
type LoginOption func(opt *LoginOptions) (*LoginOptions, error)

// LoginOptionWithEmailPassword sets email and password for standard authentication.
func LoginOptionWithEmailPassword(email, password string) LoginOption {
    return func(opt *LoginOptions) (*LoginOptions, error) {
        opt.AuthModel = "emailPassword"
        opt.Email = email
        opt.Password = password
        return opt, nil
    }
}

// LoginOptionWithOpaque sets credentials for OPAQUE authentication.
func LoginOptionWithOpaque(email string, sessionData []byte) LoginOption {
    return func(opt *LoginOptions) (*LoginOptions, error) {
        opt.AuthModel = "opaque"
        opt.Email = email
        opt.SessionData = sessionData
        return opt, nil
    }
}

// Usage: The caller chooses the authentication method
func (agg *Account) ValidateLoginCredentials(opts ...LoginOption) error {
    options := LoginOptions{
        AuthModel: "emailPassword", // default
    }
    for _, opt := range opts {
        if _, err := opt(&options); err != nil {
            return err
        }
    }
    // ... validate based on options.AuthModel
}

// Clean usage - authentication method is explicit
err := account.ValidateLoginCredentials(
    LoginOptionWithEmailPassword("user@example.com", "secret"),
)

// Or with OPAQUE
err := account.ValidateLoginCredentials(
    LoginOptionWithOpaque("user@example.com", opaqueSessionData),
)
```

## Advanced: Conditional Options

Options can include conditional logic:

```go
// EventStoreListOptionForEnvironment adjusts defaults based on environment
func EventStoreListOptionForEnvironment(env string) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        switch env {
        case "production":
            opt.Limit = 100 // Conservative limit in production
        case "development":
            opt.Limit = 1000 // More permissive in dev
        case "test":
            opt.Limit = 10 // Small limit for tests
        default:
            return nil, fmt.Errorf("unknown environment: %s", env)
        }
        return opt, nil
    }
}

// EventStoreListOptionFromRequest builds options from an HTTP request
func EventStoreListOptionFromRequest(r *http.Request) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        q := r.URL.Query()

        if tenant := q.Get("tenant"); tenant != "" {
            opt.TenantUuid = tenant
        }

        if limitStr := q.Get("limit"); limitStr != "" {
            limit, err := strconv.ParseInt(limitStr, 10, 64)
            if err != nil {
                return nil, fmt.Errorf("invalid limit: %w", err)
            }
            opt.Limit = limit
        }

        if domains := q["domain"]; len(domains) > 0 {
            opt.Domains = domains
        }

        return opt, nil
    }
}
```

## Composing Options

Options can be composed into higher-level configurations:

```go
// RecentEventsDefaults returns options for fetching recent events
func RecentEventsDefaults() []EventStoreListOption {
    return []EventStoreListOption{
        EventStoreListOptionLimit(50),
        EventStoreListOptionAscending(false), // newest first
    }
}

// AuditLogDefaults returns options suitable for audit log queries
func AuditLogDefaults() []EventStoreListOption {
    return []EventStoreListOption{
        EventStoreListOptionLimit(1000),
        EventStoreListOptionAscending(true), // chronological order
    }
}

// Usage - combine defaults with specific filters
events, total, err := store.List(ctx,
    append(RecentEventsDefaults(),
        EventStoreListOptionWithTenantUuid("tenant-123"),
        EventStoreListOptionWithDomains("orders"),
    )...,
)

// Or for audit logs
events, total, err := store.List(ctx,
    append(AuditLogDefaults(),
        EventStoreListOptionWithAggregateUuid(orderID),
    )...,
)
```

## Error Handling in Options

Options can validate their inputs:

```go
func EventStoreListOptionLimit(limit int64) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        if limit <= 0 {
            return nil, errors.New("limit must be positive")
        }
        if limit > 10000 {
            return nil, errors.New("limit exceeds maximum (10000)")
        }
        opt.Limit = limit
        return opt, nil
    }
}

func EventStoreListOptionTimeRange(start, end time.Time) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        if end.Before(start) {
            return nil, fmt.Errorf("invalid time range: end (%v) is before start (%v)", end, start)
        }
        if end.Sub(start) > 365*24*time.Hour {
            return nil, errors.New("time range exceeds maximum (1 year)")
        }
        opt.After = start.UnixNano()
        opt.Before = end.UnixNano()
        return opt, nil
    }
}

func EventStoreListOptionWithAttribute(key string, value any) EventStoreListOption {
    return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
        if key == "" {
            return nil, errors.New("attribute key cannot be empty")
        }
        if opt.Attributes == nil {
            opt.Attributes = NewAttributes()
        }
        err := opt.Attributes.Set(key, value)
        return opt, err
    }
}
```

## Functional Options vs. Config Struct Pointer

You might wonder: why not just use a config struct pointer? It's simpler and requires less code:

```go
type ListConfig struct {
    TenantUuid string
    Limit      int64
    Ascending  bool
}

func (s *EventStore) List(ctx context.Context, cfg *ListConfig) ([]Event, int64, error)
```

Both approaches work, but they have different trade-offs:

| Aspect | `*Config` Struct | Functional Options |
|--------|------------------|-------------------|
| **Zero-value problem** | `Limit: 0` — intentional or forgotten? Can't distinguish | Unset = default is used |
| **Validation** | Collected in constructor | Each option validates immediately with clear error attribution |
| **Complex setup logic** | Must happen externally | Can be encapsulated in option (`EventStoreListOptionFromRequest(r)` parses HTTP params) |
| **Composability** | Difficult | Easy: `AuditLogDefaults()` returns `[]Option` |
| **API evolution** | New exported fields = potential breaking change | New options = no breaking change |

### Concrete Example

```go
// With struct: Is Limit 0 "no limit" or "forgot to set"?
events, _, err := store.List(ctx, &ListConfig{
    TenantUuid: "tenant-123",
    // Limit: 0 ← zero value, but what's the default?
    // Ascending: false ← intentional or just unset?
})

// With Functional Options: Explicit about what's set
events, _, err := store.List(ctx,
    EventStoreListOptionWithTenantUuid("tenant-123"),
    // Limit automatically uses the default (e.g., 100)
    // Ascending automatically uses the default (e.g., false)
)
```

### When to Use a Config Struct Instead

A config struct pointer is perfectly valid when:

- **Few, mostly required fields** — no zero-value ambiguity
- **Serialization needed** — loading config from JSON/YAML
- **Performance-critical** — no function calls per option
- **Simplicity preferred** — less boilerplate code

For simple cases with 2-3 fields, a `*Config` struct is sufficient and requires less code. Functional Options shine when you have **many optional parameters** with **sensible defaults** and **validation requirements**.

## Benefits of Functional Options

1. **Self-documenting**: Option names describe what they do
2. **Extensible**: Add new options without breaking existing code
3. **Composable**: Combine options in any order
4. **Validated**: Each option can validate its input
5. **Defaulted**: Missing options get sensible defaults
6. **Testable**: Easy to create test configurations

## Running an Example

Source: https://github.com/susdorf/building-event-sourced-systems-in-go/tree/main/04-functional-options-in-go/code

When you run the complete example, you'll see functional options in action:

```
=== Functional Options Demo ===

Event store contains 10 events

1. Query with defaults (no options):
   -> Found 10 events (total: 10)
   -> Default limit: 100, Default order: descending (newest first)
   -> First event: UserLoggedIn (domain: accounts)

2. Filter by tenant:
   -> Found 6 events for tenant-A (total: 6)

3. Filter by domain(s):
   -> Found 6 events in 'orders' domain
   -> Found 8 events in 'orders' OR 'inventory' domains

4. Combine multiple options:
   -> Found 4 events (tenant-A + orders + limit 5 + ascending)
      [1] OrderPlaced
      [2] OrderPaid
      [3] OrderShipped
      [4] OrderPlaced

5. Pagination (page 2, 3 items per page):
   -> Page 2 contains 3 events (total: 10)
      [1] OrderPlaced (aggregate: order-2)
      [2] StockReserved (aggregate: inv-1)
      [3] StockDeducted (aggregate: inv-1)

6. Using preset defaults (RecentEventsDefaults):
   -> Found 4 recent events for tenant-B

7. Time range filtering:
   -> Found 10 events from the last hour

8. Option validation (invalid inputs):
   -> WithLimit(-5): invalid option: limit must be positive
   -> WithLimit(50000): invalid option: limit exceeds maximum (10000)
   -> WithPagination(0, 10): invalid option: page must be >= 1
   -> WithTimeRange(future, past): invalid option: invalid time range...

9. Inspect applied options:
   -> TenantUuid: tenant-X
   -> Domains: [orders payments]
   -> Offset: 50 (page 3)
   -> Limit: 25

10. Extensibility - adding new options is easy:
    -> Existing code continues to work unchanged!
    -> Found 3 OrderPlaced events

=== Demo Complete ===
```

## Summary

Functional Options provide a clean, extensible way to configure complex objects. The pattern:

1. Define an `Options` struct with all configurable fields
2. Define an `Option` function type that modifies options
3. Create named option functions for each setting
4. Accept variadic options in the constructor
5. Apply options in order, checking for errors

This pattern appears throughout the comby framework and many Go libraries. Once you recognize it, you'll see opportunities to use it in your own code.

---

## What's Next

In the next post, we'll explore **Part 05: Go Generics for Type-Safe Repositories** — using Go 1.18+ generics to create strongly-typed data access while maintaining flexibility.
