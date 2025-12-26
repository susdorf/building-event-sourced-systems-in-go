# Multi-Tenancy Implementation

*This is Part 12 of the "Building Event-Sourced Systems in Go" series. This series originated from the development of comby – an application framework developed using the principles of Event Sourcing and Command Query Responsibility Segregation (CQRS) - which ist still closed source today. Code with concepts and techniques is presented, but unlike comby, it is not always suitable for product use.*

---

## Introduction

Most SaaS applications serve multiple customers (tenants) from a single deployment. This architectural pattern — known as multi-tenancy — allows providers to share infrastructure costs while maintaining logical separation between customers. Each tenant's data must be isolated, secure, and performant. A tenant typically represents a company, organization, or distinct customer account.

In event-sourced systems, tenant isolation presents unique challenges. Unlike traditional CRUD applications where you might simply add a `tenant_id` column to your tables, event sourcing requires isolation at multiple layers: the event store must never leak events across tenants, command handlers must validate tenant ownership, projections must maintain separate read models, and subscriptions must filter appropriately. A single gap in any layer could expose sensitive data to unauthorized parties.

The good news is that event sourcing's explicit event boundaries make it easier to reason about and audit tenant isolation. Every state change is captured as an event with a clear tenant context, creating an immutable audit trail that can be verified for compliance.

Let's explore how to implement robust multi-tenancy that enforces isolation from HTTP request to persisted event.

## Multi-Tenancy Strategies

Before diving into implementation, it's important to choose the right isolation strategy. The decision impacts complexity, cost, compliance, and scalability. There are three common approaches, each with distinct trade-offs.

### Strategy 1: Database per Tenant

```
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│  Tenant A DB    │  │  Tenant B DB    │  │  Tenant C DB    │
│  - events       │  │  - events       │  │  - events       │
│  - commands     │  │  - commands     │  │  - commands     │
└─────────────────┘  └─────────────────┘  └─────────────────┘
```

**Pros**: Complete isolation, easy to scale individual tenants
**Cons**: Complex connection management, expensive

### Strategy 2: Schema per Tenant

```
┌─────────────────────────────────────────────┐
│                 Database                     │
│  ┌───────────┐ ┌───────────┐ ┌───────────┐  │
│  │ Schema A  │ │ Schema B  │ │ Schema C  │  │
│  │ - events  │ │ - events  │ │ - events  │  │
│  └───────────┘ └───────────┘ └───────────┘  │
└─────────────────────────────────────────────┘
```

**Pros**: Good isolation, shared infrastructure
**Cons**: Schema management complexity

### Strategy 3: Shared Tables with Tenant ID (Recommended)

```
┌─────────────────────────────────────────────┐
│                 Database                     │
│  ┌─────────────────────────────────────────┐│
│  │              events table                ││
│  │ tenant_id | aggregate_id | event_data   ││
│  │     A     |    order-1   |    ...       ││
│  │     B     |    order-2   |    ...       ││
│  │     A     |    order-3   |    ...       ││
│  └─────────────────────────────────────────┘│
└─────────────────────────────────────────────┘
```

**Pros**: Simple, cost-effective, easy to implement
**Cons**: Requires careful query design, index strategy

For most applications, Strategy 3 provides the best balance. It keeps operational complexity low while still offering strong isolation through application-level enforcement. The remainder of this post focuses on implementing this approach correctly.

## Tenant Context Propagation

Every operation in a multi-tenant system needs to know which tenant is executing it and, in some cases, which tenant's data is being accessed. This context must flow through the entire request lifecycle — from HTTP handler to event store.

We use Go's `context.Context` to propagate tenant information. The `RequestContext` structure distinguishes between the *sender* (who is making the request) and the *target* (whose data is being accessed). This separation enables controlled cross-tenant operations for administrative scenarios while maintaining strict isolation for normal operations.

```go
// RequestContext carries tenant and identity information
type RequestContext struct {
    // Sender: Who is executing this operation
    SenderTenantUuid   string
    SenderIdentityUuid string

    // Target: Whose data is being accessed (usually same as sender)
    TargetTenantUuid    string
    TargetWorkspaceUuid string
    TargetAggregateUuid string

    Permissions []string
}

// Context key for request context
type reqCtxKey struct{}

func SetReqCtxToContext(ctx context.Context, reqCtx *RequestContext) context.Context {
    return context.WithValue(ctx, reqCtxKey{}, reqCtx)
}

func GetReqCtxFromContext(ctx context.Context) *RequestContext {
    if reqCtx, ok := ctx.Value(reqCtxKey{}).(*RequestContext); ok {
        return reqCtx
    }
    return nil
}
```

## Command Tenant Isolation

Commands are the entry point for all write operations. Every command must carry tenant context so the system knows which tenant's data will be affected. The tenant UUID is set when the command is created and validated before execution.

```go
type Command interface {
    GetCmdUuid() string
    GetDomain() string
    GetTenantUuid() string
    SetTenantUuid(tenantUuid string)
    // ... other methods
}

type BaseCommand struct {
    CmdUuid    string
    Domain     string
    TenantUuid string
    // ...
}
```

Tenant is extracted from the request context and set on the command:

```go
func NewCommand(ctx context.Context, domain string, domainCmd DomainCmd) Command {
    cmd := &BaseCommand{
        CmdUuid: NewUuid(),
        Domain:  domain,
    }

    // Extract tenant from context
    if reqCtx := GetReqCtxFromContext(ctx); reqCtx != nil {
        cmd.TenantUuid = reqCtx.SenderTenantUuid
    }

    cmd.SetDomainCmd(domainCmd)
    return cmd
}
```

## Event Tenant Isolation

Events are the source of truth in an event-sourced system — once stored, they form an immutable record. Every event must be permanently associated with its tenant. This association is inherited from the aggregate when events are created, ensuring consistent tenant context throughout the event's lifecycle.

```go
type Event interface {
    GetEventUuid() string
    GetTenantUuid() string
    GetAggregateUuid() string
    // ...
}

type BaseEvent struct {
    EventUuid     string
    TenantUuid    string
    AggregateUuid string
    Domain        string
    // ...
}
```

When events are created from commands:

```go
func (agg *BaseAggregate) ApplyDomainEvent(domainEvt DomainEvt, isNew bool) error {
    evt := &BaseEvent{
        EventUuid:     NewUuid(),
        TenantUuid:    agg.TenantUuid,  // Inherited from aggregate
        AggregateUuid: agg.AggregateUuid,
        Domain:        agg.Domain,
        // ...
    }
    // ...
}
```

## Event Store Tenant Filtering

The event store is the critical boundary for tenant isolation. Every query must include a tenant filter — there should be no way to load events without specifying which tenant's events you want. This is enforced at the interface level by making `tenantUuid` a required parameter.

```go
type EventStore interface {
    Append(ctx context.Context, events []Event) error
    Load(ctx context.Context, tenantUuid, aggregateUuid string) ([]Event, error)
    LoadAll(ctx context.Context, tenantUuid string) ([]Event, error)
}

// SQL implementation with tenant filtering
type SQLEventStore struct {
    db *sql.DB
}

func (s *SQLEventStore) Load(ctx context.Context, tenantUuid, aggregateUuid string) ([]Event, error) {
    // Always filter by tenant!
    rows, err := s.db.QueryContext(ctx, `
        SELECT event_uuid, tenant_uuid, aggregate_uuid, domain, event_type, event_data, version
        FROM events
        WHERE tenant_uuid = $1 AND aggregate_uuid = $2
        ORDER BY version ASC
    `, tenantUuid, aggregateUuid)

    if err != nil {
        return nil, err
    }
    defer rows.Close()

    return scanEvents(rows)
}

func (s *SQLEventStore) Append(ctx context.Context, events []Event) error {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    for _, evt := range events {
        _, err := tx.ExecContext(ctx, `
            INSERT INTO events (event_uuid, tenant_uuid, aggregate_uuid, domain, event_type, event_data, version)
            VALUES ($1, $2, $3, $4, $5, $6, $7)
        `, evt.GetEventUuid(), evt.GetTenantUuid(), evt.GetAggregateUuid(),
           evt.GetDomain(), evt.GetEventType(), evt.GetEventData(), evt.GetVersion())

        if err != nil {
            return err
        }
    }

    return tx.Commit()
}
```

## Repository Tenant Enforcement

Repositories serve as the gateway between application code and the event store. They add an additional layer of protection by extracting tenant context from the request and validating that loaded aggregates actually belong to the requesting tenant. This double-check prevents subtle bugs where a tenant UUID might be passed incorrectly.

```go
func (ar *AggregateRepository[T]) GetAggregate(ctx context.Context, aggregateUuid string) (T, error) {
    var nilT T

    // Extract tenant from context
    reqCtx := GetReqCtxFromContext(ctx)
    if reqCtx == nil || reqCtx.SenderTenantUuid == "" {
        return nilT, ErrTenantRequired
    }

    tenantUuid := reqCtx.SenderTenantUuid

    // Load events with tenant filter
    events, err := ar.EventRepository.GetEventsForAggregate(ctx, tenantUuid, aggregateUuid, 0)
    if err != nil {
        return nilT, err
    }

    if len(events) == 0 {
        return nilT, nil
    }

    // Verify tenant matches
    if events[0].GetTenantUuid() != tenantUuid {
        return nilT, ErrTenantMismatch
    }

    agg := ar.NewAggregateFunc()
    for _, evt := range events {
        if err := ApplyEvent(ctx, agg, evt); err != nil {
            return nilT, err
        }
    }

    return agg, nil
}
```

## Projection Tenant Isolation

Projections (read models) are optimized views built from events. In a multi-tenant system, projections must maintain strict separation — a query for Tenant A's orders should never accidentally return Tenant B's data. The simplest approach is to use the tenant UUID as the primary key prefix, effectively creating separate indexes per tenant.

Read models can also process events from multiple domains. For example, a tenant dashboard projection might listen to `TenantCreated`, `OrderPlaced`, and `IdentityCreated` events to build a comprehensive view. Each event handler must respect tenant boundaries.

```go
type OrderProjection struct {
    // Indexed by tenant first, then by order
    ordersByTenant sync.Map  // tenantUuid -> map[orderUuid]*OrderView
}

func (p *OrderProjection) handleOrderPlaced(ctx context.Context, evt Event, domainEvt *OrderPlacedEvent) error {
    view := &OrderView{
        OrderID:  evt.AggregateUuid,
        TenantID: evt.TenantUuid,
        // ...
    }

    // Store under tenant
    p.storeForTenant(evt.TenantUuid, view)
    return nil
}

func (p *OrderProjection) storeForTenant(tenantUuid string, view *OrderView) {
    // Get or create tenant map
    tenantMapRaw, _ := p.ordersByTenant.LoadOrStore(tenantUuid, &sync.Map{})
    tenantMap := tenantMapRaw.(*sync.Map)

    tenantMap.Store(view.OrderID, view)
}

func (p *OrderProjection) GetOrdersForTenant(tenantUuid string) []*OrderView {
    tenantMapRaw, ok := p.ordersByTenant.Load(tenantUuid)
    if !ok {
        return []*OrderView{}
    }

    tenantMap := tenantMapRaw.(*sync.Map)
    var orders []*OrderView

    tenantMap.Range(func(key, value interface{}) bool {
        orders = append(orders, value.(*OrderView))
        return true
    })

    return orders
}
```

## HTTP Middleware for Tenant Extraction

The HTTP layer is where tenant context first enters the system. Middleware extracts tenant information from various sources — JWT tokens, API keys, URL paths, or headers — and establishes the `RequestContext` that flows through the rest of the request. This is the first line of defense for tenant isolation.

```go
func TenantMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract tenant from JWT claims
        claims := getClaimsFromContext(r.Context())
        if claims == nil {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }

        tenantUuid := claims.TenantUuid
        identityUuid := claims.IdentityUuid

        // Create request context
        reqCtx := &RequestContext{
            SenderTenantUuid:   tenantUuid,
            SenderIdentityUuid: identityUuid,
        }

        // Add to context
        ctx := SetReqCtxToContext(r.Context(), reqCtx)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

Or from URL path:

```go
func TenantFromPathMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract from path: /api/tenants/{tenantUuid}/orders
        tenantUuid := chi.URLParam(r, "tenantUuid")

        if tenantUuid == "" {
            http.Error(w, "tenant required", http.StatusBadRequest)
            return
        }

        // Verify user belongs to this tenant
        claims := getClaimsFromContext(r.Context())
        if claims.TenantUuid != tenantUuid {
            http.Error(w, "forbidden", http.StatusForbidden)
            return
        }

        reqCtx := &RequestContext{
            SenderTenantUuid:   tenantUuid,
            SenderIdentityUuid: claims.IdentityUuid,
        }

        ctx := SetReqCtxToContext(r.Context(), reqCtx)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

## Domain-Level Authorization Middleware

While HTTP middleware handles authentication and basic tenant extraction, the domain layer enforces authorization. This separation keeps HTTP handlers simple and ensures consistent authorization regardless of how commands are dispatched (HTTP, message queue, internal calls).

The domain middleware implements a multi-tier authorization check:

```go
type AuthCommandHandlerFunc func(ctx context.Context, next DomainCommandHandlerFunc) DomainCommandHandlerFunc

func AuthMiddleware(ctx context.Context, next DomainCommandHandlerFunc) DomainCommandHandlerFunc {
    return func(ctx context.Context, cmd Command, domainCmd DomainCmd) ([]Event, error) {
        reqCtx := GetReqCtxFromContext(ctx)
        if reqCtx == nil {
            return nil, ErrUnauthorized
        }

        // 1. Check skip authorization flag (for internal system calls)
        if cmd.GetSkipAuthorization() {
            return next(ctx, cmd, domainCmd)
        }

        // 2. System admin bypass
        if isSystemAdmin(reqCtx) {
            return next(ctx, cmd, domainCmd)
        }

        // 3. Validate tenant match
        if cmd.GetTenantUuid() != reqCtx.SenderTenantUuid {
            return nil, ErrForbidden
        }

        // 4. Validate aggregate ownership (if targeting existing aggregate)
        if cmd.GetAggregateUuid() != "" {
            if !isValidTenantAggregate(ctx, cmd.GetTenantUuid(), cmd.GetAggregateUuid()) {
                return nil, ErrForbidden
            }
        }

        // 5. Check RBAC permissions
        if !hasPermission(reqCtx, cmd.GetRequiredPermission()) {
            return nil, ErrForbidden
        }

        return next(ctx, cmd, domainCmd)
    }
}
```

This layered approach ensures defense in depth — even if one check is bypassed, others remain in place.

## Cross-Tenant Prevention and Aggregate Ownership

Preventing cross-tenant access is the most critical security requirement. Beyond simple tenant UUID matching, the system must verify that any aggregate being accessed actually belongs to the requesting tenant. This prevents attacks where an attacker might guess or enumerate aggregate UUIDs belonging to other tenants.

```go
func (fc *Facade) authorizeCommand(ctx context.Context, cmd Command) error {
    reqCtx := GetReqCtxFromContext(ctx)
    if reqCtx == nil {
        return ErrUnauthorized
    }

    // Command's target tenant must match sender's tenant
    if cmd.GetTenantUuid() != reqCtx.SenderTenantUuid {
        Logger.Warn("cross-tenant access attempt",
            "sender", reqCtx.SenderTenantUuid,
            "target", cmd.GetTenantUuid(),
            "identity", reqCtx.SenderIdentityUuid,
        )
        return ErrForbidden
    }

    return nil
}
```

Additionally, aggregate ownership must be verified by checking if the aggregate actually belongs to the claimed tenant:

```go
// IsValidTenantAggregate checks if an aggregate belongs to the specified tenant
func IsValidTenantAggregate(ctx context.Context, tenantUuid, aggregateUuid string) bool {
    // Load first event to verify tenant ownership
    events, err := eventStore.Load(ctx, tenantUuid, aggregateUuid)
    if err != nil || len(events) == 0 {
        return false
    }
    return events[0].GetTenantUuid() == tenantUuid
}
```

## System Tenant for Admin Operations

Some operations legitimately need cross-tenant access: platform administration, billing reconciliation, or support tools. Rather than creating security holes, we define a special "system tenant" with elevated privileges. Operations from this tenant can access any tenant's data, but access to the system tenant itself is tightly controlled.

```go
const SystemTenantUuid = "system"

func (fc *Facade) isSystemTenant(ctx context.Context) bool {
    reqCtx := GetReqCtxFromContext(ctx)
    return reqCtx != nil && reqCtx.SenderTenantUuid == SystemTenantUuid
}

func (fc *Facade) authorizeCommand(ctx context.Context, cmd Command) error {
    // System tenant can access any tenant
    if fc.isSystemTenant(ctx) {
        return nil
    }

    reqCtx := GetReqCtxFromContext(ctx)
    if cmd.GetTenantUuid() != reqCtx.SenderTenantUuid {
        return ErrForbidden
    }

    return nil
}
```

## Database Indexing for Tenants

With the shared-table approach, proper indexing is crucial for performance. The tenant UUID should be the leading column in all indexes to ensure the database can quickly filter to the relevant tenant before applying other conditions.

```sql
-- Primary index for loading aggregate events
CREATE INDEX idx_events_tenant_aggregate
ON events (tenant_uuid, aggregate_uuid, version);

-- Index for listing all events for a tenant
CREATE INDEX idx_events_tenant_time
ON events (tenant_uuid, event_time);

-- Index for domain-specific queries within tenant
CREATE INDEX idx_events_tenant_domain
ON events (tenant_uuid, domain, event_type);
```

## Tenant Metrics and Monitoring

Monitoring per-tenant usage is essential for capacity planning, billing, and detecting anomalies. Metrics should track command execution, event creation, and query patterns per tenant.

```go
var (
    commandsByTenant = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "commands_total",
            Help: "Total commands by tenant",
        },
        []string{"tenant", "domain", "command"},
    )

    eventsByTenant = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "events_total",
            Help: "Total events by tenant",
        },
        []string{"tenant", "domain"},
    )
)

func TenantMetricsMiddleware(ctx context.Context, next DomainCommandHandlerFunc) DomainCommandHandlerFunc {
    return func(ctx context.Context, cmd Command, domainCmd DomainCmd) ([]Event, error) {
        tenant := cmd.GetTenantUuid()
        domain := cmd.GetDomain()
        cmdName := cmd.GetDomainCmdName()

        commandsByTenant.WithLabelValues(tenant, domain, cmdName).Inc()

        events, err := next(ctx, cmd, domainCmd)

        if err == nil {
            eventsByTenant.WithLabelValues(tenant, domain).Add(float64(len(events)))
        }

        return events, err
    }
}
```

## Testing Multi-Tenancy

Testing tenant isolation requires specific scenarios to verify security boundaries. Key test cases should cover:

```go
func TestTenantIsolation(t *testing.T) {
    tests := []struct {
        name          string
        senderTenant  string
        targetTenant  string
        expectAllowed bool
    }{
        // Same tenant: allowed
        {"same tenant", "tenant-A", "tenant-A", true},

        // Cross-tenant: denied
        {"cross tenant", "tenant-A", "tenant-B", false},

        // System admin to any tenant: allowed
        {"system to tenant", SystemTenantUuid, "tenant-A", true},

        // Regular tenant to system: denied
        {"tenant to system", "tenant-A", SystemTenantUuid, false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ctx := createContextWithTenant(tt.senderTenant)
            cmd := NewTestCommand(tt.targetTenant)

            err := facade.DispatchCommand(ctx, cmd)

            if tt.expectAllowed && err != nil {
                t.Errorf("expected allowed, got error: %v", err)
            }
            if !tt.expectAllowed && err == nil {
                t.Error("expected denied, got success")
            }
        })
    }
}
```

Additional scenarios to test: aggregate ownership validation, workspace-level access within tenants, and anonymous user restrictions.

## Running an Example

Source: https://github.com/susdorf/building-event-sourced-systems-in-go/tree/main/12-multi-tenancy-implementation/code

When you run the complete example, you'll see multi-tenancy authorization in action:

```
=== Multi-Tenancy Demo ===

1. Setting up event store and facade:
   Populated store with 8 events across 2 tenants
   Registered AuthCommandMiddleware and AuthQueryMiddleware

2. Same-tenant command (tenant-A -> tenant-A):
   -> ALLOWED: Command executed successfully
   -> Created event: PlaceOrderExecuted
   Authorization steps:
      [OK] Context Check - sender=tenant-A, target=tenant-A
      [CONTINUE] Skip Auth Flag - Flag not set
      [CONTINUE] System Admin Check - Not a system admin
      [OK] Tenant Match - Sender tenant matches command tenant
      [OK] Aggregate Ownership - aggregate order-new belongs to tenant
      [ALLOWED] Final Decision - All authorization checks passed

3. Cross-tenant command (tenant-A -> tenant-B):
   -> DENIED: forbidden: cross-tenant access denied
   Authorization steps:
      [OK] Context Check - sender=tenant-A, target=tenant-B
      [CONTINUE] Skip Auth Flag - Flag not set
      [CONTINUE] System Admin Check - Not a system admin
      [DENIED] Tenant Match - sender=tenant-A cannot access tenant=tenant-B

4. System admin accessing tenant-B (system -> tenant-B):
   -> ALLOWED: Command executed successfully
   Authorization steps:
      [BYPASS] System Admin Check - System admin from tenant system

5. Admin user accessing different tenant (admin flag):
   -> ALLOWED: Command executed successfully
   Authorization steps:
      [BYPASS] System Admin Check - System admin from tenant tenant-A

6. Accessing own aggregate (tenant-A -> order-1):
   -> ALLOWED: Command executed successfully
   Authorization steps:
      [OK] Aggregate Ownership - aggregate order-1 belongs to tenant
      [ALLOWED] Final Decision - All authorization checks passed

7. Accessing other tenant's aggregate (tenant-A -> order-3):
   -> DENIED: aggregate ownership validation failed
   Authorization steps:
      [OK] Tenant Match - Sender tenant matches command tenant
      [DENIED] Aggregate Ownership - aggregate order-3 not owned by tenant tenant-A

8. Event store tenant isolation:
   Total events in store: 12
   Events for tenant-A: 7
   Events for tenant-B: 5

   Loading order-1 as tenant-A: 4 events found
   Loading order-1 as tenant-B: 0 events found (isolation works!)

9. Query authorization:
   -> Query succeeded: 4 events returned
   -> Cross-tenant query denied: forbidden

10. Internal system call with SkipAuthorization:
   -> ALLOWED: Command executed successfully
   Authorization steps:
      [BYPASS] Skip Auth Flag - Authorization skipped by flag

=== Demo Complete ===
```

The example demonstrates all key scenarios: same-tenant access, cross-tenant blocking, system admin bypass, aggregate ownership validation, and event store isolation.

## Summary

Multi-tenancy in event-sourced systems requires defense in depth — multiple layers of protection working together. A single check is never enough; tenant isolation must be enforced at every layer:

1. **Tenant context propagation**: Use `RequestContext` with sender/target distinction, flowing through the entire request lifecycle
2. **Command and event isolation**: Every command and event carries immutable tenant association
3. **Event store filtering**: Make tenant a required parameter — no queries without tenant filter
4. **Repository enforcement**: Double-check that loaded aggregates belong to the requesting tenant
5. **Projection isolation**: Index by tenant first, then by entity
6. **HTTP middleware**: Extract and validate tenant from authentication tokens or URL paths
7. **Domain middleware**: Multi-tier authorization with system admin bypass, RBAC, and aggregate ownership checks
8. **Cross-tenant prevention**: Log and block all unauthorized cross-tenant access attempts
9. **Proper indexing**: Composite indexes with tenant as the leading column
10. **Testing**: Comprehensive test matrix covering same-tenant, cross-tenant, and system admin scenarios
11. **Monitoring**: Per-tenant metrics for capacity planning and anomaly detection

The shared-table approach with application-level tenant filtering is recommended for most applications — it balances simplicity, cost-effectiveness, and scalability while providing strong isolation through consistent enforcement across all layers.

---

## What's Next

In the next post, we'll explore **Workspaces** — fine-grained authorization within tenants for multi-team and multi-project scenarios.
