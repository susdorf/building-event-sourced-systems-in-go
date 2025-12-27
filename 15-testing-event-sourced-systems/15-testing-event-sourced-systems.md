# Testing Event-Sourced Systems

*This is Part 15 of the "Building Event-Sourced Systems in Go" series. This series originated from the development of comby – an application framework developed using the principles of Event Sourcing and Command Query Responsibility Segregation (CQRS) - which ist still closed source today. Code with concepts and techniques is presented, but unlike comby, it is not always suitable for product use.*

---

## Introduction

Event-sourced systems offer unique and powerful testing opportunities that traditional CRUD-based applications simply cannot match. Since every state change in an event-sourced system is recorded as an immutable event, we gain unprecedented control over our test scenarios. This architectural decision transforms testing from a challenging necessity into a natural extension of how the system works.

The benefits of testing event-sourced systems include:

- **Deterministic state reconstruction**: We can set up any aggregate state by replaying a specific sequence of events, making it trivial to test edge cases and complex scenarios that would be difficult to reproduce in traditional systems.
- **Behavior verification through events**: Instead of asserting internal state, we verify that the correct events were emitted, which directly corresponds to the business intent of our operations.
- **Time-travel debugging**: When a test fails or a bug appears in production, we can replay the exact event sequence to reproduce the issue deterministically.
- **Natural isolation**: Aggregates are inherently isolated units of consistency, making them perfect candidates for unit testing without complex mocking strategies.

This post covers comprehensive strategies for testing every layer of an event-sourced application, from individual aggregates to full integration tests, including performance benchmarking and concurrency testing.

## Testing Aggregates

Aggregates are the heart of business logic in an event-sourced system. They serve as consistency boundaries that ensure business rules are enforced and valid state transitions occur. Testing aggregates follows a natural pattern that mirrors how they operate in production:

1. **Setting up initial state** by applying historical events
2. **Executing an intention** through a method call that represents a business action
3. **Verifying emitted events** to confirm the expected behavior occurred

This approach tests behavior rather than implementation details, making tests more resilient to refactoring while ensuring business rules are correctly enforced.

### Basic Aggregate Test

The fundamental aggregate test verifies that a business operation produces the expected events. Here we test placing an order, ensuring it emits an `OrderPlacedEvent` with the correct data:

```go
func TestOrder_Place(t *testing.T) {
    // Arrange: Create a fresh aggregate
    order := aggregate.NewAggregate()
    order.AggregateUuid = "order-123"

    items := []aggregate.Item{
        {ItemUid: "item-1", Sku: "SKU001", Quantity: 2, Price: 29.99},
        {ItemUid: "item-2", Sku: "SKU002", Quantity: 1, Price: 49.99},
    }

    // Act: Execute the business operation
    err := order.Place(items)

    // Assert: Verify the operation succeeded
    require.NoError(t, err)

    // Check emitted events - this is the key verification
    events := order.GetUncommittedEvents()
    require.Len(t, events, 1)

    evt := events[0]
    assert.Equal(t, "OrderPlacedEvent", evt.GetDomainEvtName())

    // Verify event payload contains correct business data
    domainEvt := evt.GetDomainEvt().(*aggregate.OrderPlacedEvent)
    assert.Len(t, domainEvt.Items, 2)
    assert.Equal(t, 109.97, domainEvt.Total)
}
```

Notice that we verify the emitted events rather than inspecting internal aggregate state. This approach tests what the aggregate communicates to the outside world, which is what matters for downstream consumers like projections and other aggregates.

### Testing Business Rules

Business rules are the core value of your domain model. Testing them ensures that invalid operations are rejected before they can corrupt the system's state. Each business rule should have dedicated tests that verify both the rejection and the error message:

```go
func TestOrder_Place_RejectsEmptyOrder(t *testing.T) {
    order := aggregate.NewAggregate()

    err := order.Place([]aggregate.Item{})

    // Verify the operation was rejected
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "at least one item")

    // Crucially: no events should be emitted for rejected operations
    assert.Empty(t, order.GetUncommittedEvents())
}

func TestOrder_Place_RejectsZeroQuantity(t *testing.T) {
    order := aggregate.NewAggregate()

    items := []aggregate.Item{
        {ItemUid: "item-1", Sku: "SKU001", Quantity: 0, Price: 29.99},
    }

    err := order.Place(items)

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "quantity must be positive")
}

func TestOrder_Ship_RequiresPaidStatus(t *testing.T) {
    order := aggregate.NewAggregate()

    // Place order (status = "placed")
    order.Place([]aggregate.Item{{Quantity: 1, Price: 10.0}})
    order.ClearUncommittedEvents()

    // Try to ship without paying - should violate business rule
    err := order.Ship("TRACK-123")

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "must be paid")
}
```

The last test demonstrates testing state-dependent rules: shipping requires the order to be in "paid" status. We first set up the aggregate in the correct initial state, then verify that the business rule is enforced.

### Given-When-Then Pattern

The Given-When-Then pattern provides a structured approach to writing expressive tests that read like specifications. This pattern is particularly well-suited for event-sourced systems because the "Given" phase naturally maps to replaying historical events:

```go
func TestOrder_Ship_Success(t *testing.T) {
    // Given: A paid order (state established through events)
    order := givenPaidOrder(t, "order-123")

    // When: Shipping the order
    err := order.Ship("TRACK-123")

    // Then: Order should emit shipped event
    require.NoError(t, err)

    events := order.GetUncommittedEvents()
    require.Len(t, events, 1)

    assert.Equal(t, "OrderShippedEvent", events[0].GetDomainEvtName())
    assert.Equal(t, "shipped", order.Status)
}

// Helper to create orders in specific states
// This encapsulates the event sequence needed to reach a particular state
func givenPaidOrder(t *testing.T, orderID string) *aggregate.Order {
    t.Helper()

    order := aggregate.NewAggregate()
    order.AggregateUuid = orderID

    // Apply historical events to reach the "paid" state
    order.Place([]aggregate.Item{{Quantity: 1, Price: 100.0}})
    order.Pay("payment-123", 100.0)

    // Clear uncommitted events - we only care about new events
    order.ClearUncommittedEvents()

    return order
}
```

The helper function `givenPaidOrder` encapsulates the event sequence required to reach a specific state. This keeps tests focused on the behavior being tested while making the preconditions explicit and reusable.

### Testing State After Event Replay

A fundamental property of event-sourced systems is that replaying the same events must always produce the same state. This test verifies that property and ensures our event handlers correctly reconstruct aggregate state:

```go
func TestOrder_StateAfterReplay(t *testing.T) {
    // Create a sequence of events representing order lifecycle
    events := []comby.Event{
        createEvent("OrderPlacedEvent", &aggregate.OrderPlacedEvent{
            Items: []aggregate.Item{{Quantity: 2, Price: 50.0}},
            Total: 100.0,
        }),
        createEvent("OrderPaidEvent", &aggregate.OrderPaidEvent{
            PaymentID: "pay-123",
            Amount:    100.0,
        }),
        createEvent("OrderShippedEvent", &aggregate.OrderShippedEvent{
            TrackingNo: "TRACK-456",
        }),
    }

    // Replay events on a fresh aggregate
    order := aggregate.NewAggregate()
    for _, evt := range events {
        comby.ApplyEvent(context.Background(), order, evt)
    }

    // Verify final state matches expected state after all events
    assert.Equal(t, "shipped", order.Status)
    assert.Equal(t, 100.0, order.Total)
    assert.Equal(t, "TRACK-456", order.TrackingNo)
}
```

This test pattern is essential for verifying that event handlers are implemented correctly. It also serves as documentation for how events affect aggregate state.

## Testing Command Handlers

Command handlers serve as the coordination layer between incoming commands and aggregates. They are responsible for loading aggregates, executing business operations, and returning the resulting events. Testing command handlers verifies this orchestration logic:

```go
func TestCommandHandler_PlaceOrder(t *testing.T) {
    ctx := context.Background()

    // Setup: Create facade with in-memory stores for isolation
    fc, _ := comby.NewFacade(comby.WithInMemoryStores())
    repo := comby.NewAggregateRepository(fc, aggregate.NewAggregate)
    handler := command.NewCommandHandler(repo)

    // Create command representing user intent
    cmd := comby.NewCommand(ctx, "Order", &command.PlaceOrder{
        OrderUuid: "order-123",
        Items:     []aggregate.Item{{Quantity: 1, Price: 50.0}},
    })

    // Execute the command through the handler
    events, err := handler.Handle(ctx, cmd)

    // Verify: Command succeeded and produced expected events
    require.NoError(t, err)
    require.Len(t, events, 1)
    assert.Equal(t, "OrderPlacedEvent", events[0].GetDomainEvtName())
}

func TestCommandHandler_PlaceOrder_DuplicateRejected(t *testing.T) {
    ctx := context.Background()

    fc, _ := comby.NewFacade(comby.WithInMemoryStores())
    repo := comby.NewAggregateRepository(fc, aggregate.NewAggregate)
    handler := command.NewCommandHandler(repo)

    // Place first order successfully
    cmd1 := comby.NewCommand(ctx, "Order", &command.PlaceOrder{
        OrderUuid: "order-123",
        Items:     []aggregate.Item{{Quantity: 1, Price: 50.0}},
    })
    events, _ := handler.Handle(ctx, cmd1)

    // Save events to make aggregate exist in repository
    fc.GetEventRepository().SaveEvents(ctx, events)

    // Try to place duplicate order with same ID
    cmd2 := comby.NewCommand(ctx, "Order", &command.PlaceOrder{
        OrderUuid: "order-123",  // Same ID - should fail
        Items:     []aggregate.Item{{Quantity: 1, Price: 50.0}},
    })

    _, err := handler.Handle(ctx, cmd2)

    // Verify duplicate was rejected
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "already exists")
}
```

Command handler tests verify both the happy path and error scenarios. The duplicate rejection test demonstrates testing idempotency, an important property for reliable distributed systems.

## Testing Projections

Projections transform events into read-optimized views. They are the "Q" in CQRS and must correctly handle every event type they subscribe to. Testing projections involves feeding events and verifying the resulting state:

```go
func TestOrderProjection_HandleOrderPlaced(t *testing.T) {
    ctx := context.Background()

    // Create projection in isolation
    projection := readmodel.NewOrderProjection(nil)

    // Feed a single event
    evt := createEvent("OrderPlacedEvent", &aggregate.OrderPlacedEvent{
        Items: []aggregate.Item{
            {Sku: "SKU001", Quantity: 2, Price: 25.0},
        },
        Total: 50.0,
    })
    evt.TenantUuid = "tenant-123"
    evt.AggregateUuid = "order-123"

    err := projection.HandleEvent(ctx, evt)
    require.NoError(t, err)

    // Query projection and verify state
    order := projection.GetOrder("order-123")
    require.NotNil(t, order)
    assert.Equal(t, "placed", order.Status)
    assert.Equal(t, 50.0, order.Total)

    // Verify secondary indexes are populated
    orders := projection.GetOrdersByTenant("tenant-123")
    assert.Len(t, orders, 1)
}

func TestOrderProjection_StatusTransitions(t *testing.T) {
    ctx := context.Background()
    projection := readmodel.NewOrderProjection(nil)

    orderUuid := "order-123"
    tenantUuid := "tenant-123"

    // Test complete lifecycle through event sequence
    events := []struct {
        name     string
        event    interface{}
        expected string
    }{
        {"OrderPlacedEvent", &aggregate.OrderPlacedEvent{Total: 100}, "placed"},
        {"OrderPaidEvent", &aggregate.OrderPaidEvent{}, "paid"},
        {"OrderShippedEvent", &aggregate.OrderShippedEvent{TrackingNo: "TRK"}, "shipped"},
    }

    for _, tc := range events {
        evt := createEvent(tc.name, tc.event)
        evt.TenantUuid = tenantUuid
        evt.AggregateUuid = orderUuid

        projection.HandleEvent(ctx, evt)

        order := projection.GetOrder(orderUuid)
        assert.Equal(t, tc.expected, order.Status, "After %s", tc.name)
    }
}
```

The status transitions test demonstrates testing a complete workflow through the projection, verifying that each event correctly updates the view state.

## Testing Query Handlers

Query handlers provide the interface for reading data from projections. They may include additional logic like pagination, filtering, or authorization checks:

```go
func TestQueryHandler_GetOrder(t *testing.T) {
    ctx := context.Background()

    // Setup projection with test data
    projection := readmodel.NewOrderProjection(nil)
    populateProjection(projection, "tenant-123", "order-123")

    handler := query.NewQueryHandler(projection)

    // Execute query
    qry := comby.NewQuery(ctx, "Order", &query.GetOrder{
        TenantUuid: "tenant-123",
        OrderUuid:  "order-123",
    })

    result, err := handler.Handle(ctx, qry)

    // Verify result
    require.NoError(t, err)
    order := result.(*readmodel.OrderView)
    assert.Equal(t, "order-123", order.OrderID)
}

func TestQueryHandler_ListOrders_Pagination(t *testing.T) {
    ctx := context.Background()

    projection := readmodel.NewOrderProjection(nil)

    // Add 25 orders for pagination testing
    for i := 0; i < 25; i++ {
        addOrderToProjection(projection, "tenant-123", fmt.Sprintf("order-%d", i))
    }

    handler := query.NewQueryHandler(projection)

    // Test first page
    qry := comby.NewQuery(ctx, "Order", &query.ListOrders{
        TenantUuid: "tenant-123",
        Page:       1,
        PageSize:   10,
    })

    result, _ := handler.Handle(ctx, qry)
    response := result.(*query.ListOrdersResponse)

    assert.Len(t, response.Orders, 10)
    assert.Equal(t, 25, response.TotalCount)
    assert.Equal(t, 1, response.Page)

    // Test last page (partial)
    qry2 := comby.NewQuery(ctx, "Order", &query.ListOrders{
        TenantUuid: "tenant-123",
        Page:       3,
        PageSize:   10,
    })

    result2, _ := handler.Handle(ctx, qry2)
    response2 := result2.(*query.ListOrdersResponse)

    assert.Len(t, response2.Orders, 5)  // Only 5 remaining on page 3
}
```

Pagination tests are important for query handlers that return collections. They verify that the handler correctly implements offset/limit logic and returns accurate total counts.

## Integration Testing

While unit tests verify individual components, integration tests ensure they work together correctly. The key to effective integration testing in event-sourced systems is using in-memory stores that provide the same behavior as production stores without external dependencies:

```go
func TestIntegration_OrderLifecycle(t *testing.T) {
    ctx := context.Background()

    // Setup: Full system with in-memory stores
    fc, _ := comby.NewFacade(comby.WithInMemoryStores())
    order.Register(ctx, fc)
    fc.RestoreState()

    // Set tenant context for multi-tenancy
    ctx = comby.SetReqCtxToContext(ctx, &comby.RequestContext{
        SenderTenantUuid:   "tenant-123",
        SenderIdentityUuid: "user-456",
    })

    orderUuid := comby.NewUuid()

    // Step 1: Place order through full command pipeline
    placeCmd := comby.NewCommand(ctx, "Order", &command.PlaceOrder{
        OrderUuid: orderUuid,
        Items:     []aggregate.Item{{Quantity: 2, Price: 50.0}},
    })
    _, err := fc.DispatchCommand(ctx, placeCmd)
    require.NoError(t, err)

    // Wait for async command processing to complete
    fc.WaitForCmd(ctx, placeCmd)

    // Step 2: Query order through full query pipeline
    qry := comby.NewQuery(ctx, "Order", &projection.QueryModelRequest{
        AggregateUuid: orderUuid,
    })
    result, err := fc.DispatchQuery(ctx, qry)
    require.NoError(t, err)

    // Verify end-to-end result
    order := result.(*aggregate.Order)
    assert.Equal(t, "placed", order.Status)
    assert.Equal(t, 100.0, order.Total)
}
```

Integration tests verify the complete request flow: command dispatch, aggregate execution, event storage, projection updates, and query responses. They catch integration issues that unit tests might miss.

## Table-Driven Tests

Table-driven tests provide comprehensive coverage of state transitions with minimal code duplication. They are particularly effective for testing aggregate state machines where many transitions need verification:

```go
func TestOrder_Transitions(t *testing.T) {
    tests := []struct {
        name          string
        initialEvents []interface{}
        action        func(*aggregate.Order) error
        expectError   bool
        expectEvent   string
    }{
        {
            name:          "can place new order",
            initialEvents: nil,
            action: func(o *aggregate.Order) error {
                return o.Place([]aggregate.Item{{Quantity: 1, Price: 10}})
            },
            expectError: false,
            expectEvent: "OrderPlacedEvent",
        },
        {
            name:          "cannot place already placed order",
            initialEvents: []interface{}{&aggregate.OrderPlacedEvent{}},
            action: func(o *aggregate.Order) error {
                return o.Place([]aggregate.Item{{Quantity: 1, Price: 10}})
            },
            expectError: true,
        },
        {
            name:          "can pay placed order",
            initialEvents: []interface{}{&aggregate.OrderPlacedEvent{Total: 100}},
            action: func(o *aggregate.Order) error {
                return o.Pay("pay-123", 100)
            },
            expectError: false,
            expectEvent: "OrderPaidEvent",
        },
        {
            name: "cannot ship unpaid order",
            initialEvents: []interface{}{
                &aggregate.OrderPlacedEvent{Total: 100},
            },
            action: func(o *aggregate.Order) error {
                return o.Ship("TRACK-123")
            },
            expectError: true,
        },
        {
            name: "can ship paid order",
            initialEvents: []interface{}{
                &aggregate.OrderPlacedEvent{Total: 100},
                &aggregate.OrderPaidEvent{},
            },
            action: func(o *aggregate.Order) error {
                return o.Ship("TRACK-123")
            },
            expectError: false,
            expectEvent: "OrderShippedEvent",
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            order := aggregate.NewAggregate()

            // Apply initial events to establish state
            for _, evt := range tc.initialEvents {
                applyEventToAggregate(order, evt)
            }
            order.ClearUncommittedEvents()

            // Execute action under test
            err := tc.action(order)

            // Verify expectations
            if tc.expectError {
                assert.Error(t, err)
                assert.Empty(t, order.GetUncommittedEvents())
            } else {
                assert.NoError(t, err)
                if tc.expectEvent != "" {
                    events := order.GetUncommittedEvents()
                    require.NotEmpty(t, events)
                    assert.Equal(t, tc.expectEvent, events[0].GetDomainEvtName())
                }
            }
        })
    }
}
```

Each test case documents a specific state transition rule. This makes it easy to see all valid and invalid transitions at a glance, serving as living documentation for the aggregate's behavior.

## Test Suite Setup

A well-designed test suite provides consistent setup and teardown across all tests. The suite pattern encapsulates complex initialization logic and ensures proper cleanup:

```go
// test/suite.go
package test

import (
    "context"
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "os"

    "github.com/gradientzero/comby/v2"
    store "github.com/gradientzero/comby-store-sqlite"
)

// Suite provides a fully configured test environment
type Suite struct {
    Facade *comby.Facade
    Mux    *http.ServeMux
    Url    string
}

// SetupSuite creates a complete test environment with:
// - SQLite stores for persistence
// - Registered domains
// - Pre-populated test data
// - HTTP test server for API testing
func SetupSuite(facadeOpts ...comby.FacadeOption) *Suite {
    dir, _ := os.Getwd()

    // Create isolated databases for each test run
    eventStore := store.NewEventStoreSQLite(
        filepath.Join(dir, "__"+comby.NewUuid()+".db"),
    )
    commandStore := store.NewCommandStoreSQLite(
        filepath.Join(dir, "__"+comby.NewUuid()+".db"),
    )

    // Configure facade with test settings
    facadeOpts = append(facadeOpts, comby.FacadeWithAppName("test-app"))
    facadeOpts = append(facadeOpts, comby.FacadeWithInstanceId(1))
    facadeOpts = append(facadeOpts, comby.FacadeWithEventStore(eventStore))
    facadeOpts = append(facadeOpts, comby.FacadeWithCommandStore(commandStore))
    fc, _ := comby.NewFacade(facadeOpts...)

    // Register domains and restore state
    ctx := context.Background()
    domain.RegisterDefaults(ctx, fc)
    fc.RestoreState()

    // Populate with standard test data
    TestFillAccounts(fc)
    TestFillTenants(fc)
    TestFillGroups(fc)
    TestFillIdentities(fc)

    // Setup HTTP test server
    mux := http.NewServeMux()
    humaApi := humago.New(mux, huma.DefaultConfig("Test API", "1.0.0"))
    combyApi.RegisterDefaults(fc, humaApi)

    return &Suite{
        Url:    "http://127.0.0.1:8041",
        Facade: fc,
        Mux:    mux,
    }
}

// CleanupSuite removes test databases and resets state
func CleanupSuite(testSuite *Suite) error {
    ctx := context.Background()
    if testSuite != nil && testSuite.Facade != nil {
        testSuite.Facade.GetEventStore().Reset(ctx)
        testSuite.Facade.GetCommandStore().Reset(ctx)
    }
    return nil
}

// ServeHttpAndRequest executes HTTP requests against the test server
func (s *Suite) ServeHttpAndRequest(req *http.Request) (int, *http.Response, error) {
    resRec := httptest.NewRecorder()
    s.Mux.ServeHTTP(resRec, req)
    resp := resRec.Result()
    defer resp.Body.Close()
    return resp.StatusCode, resp, nil
}
```

Using the suite in tests:

```go
func TestWithSuite(t *testing.T) {
    testSuite := test.SetupSuite()
    defer test.CleanupSuite(testSuite)

    // Test using the fully configured environment
    fc := testSuite.Facade
    // ... test code
}
```

## Test Command and Query Helpers

Creating commands and queries for tests involves boilerplate that can be encapsulated in helper functions. These helpers set up the necessary context and skip authorization checks that are not relevant for unit tests:

```go
// test/command.go
package test

func NewTestCommand(tenantUuid string, domain string, domainCmd comby.DomainCmd) comby.Command {
    cmd, _ := comby.NewCommand(domain, domainCmd)

    // Configure request context for testing
    reqCtx := comby.NewRequestContext()

    // Make commands synchronous for easier testing
    reqCtx.ExecuteWaitToFinish = true

    // Skip authorization in tests
    reqCtx.Attributes.Set(auth.ExecuteSkipAuthorization, true)

    cmd.SetReqCtx(reqCtx)
    cmd.SetTenantUuid(tenantUuid)

    return cmd
}

// test/query.go
package test

func NewTestQuery(tenantUuid string, domain string, domainQry comby.DomainQry) comby.Query {
    qry, _ := comby.NewQuery(domain, domainQry)

    reqCtx := comby.NewRequestContext()
    reqCtx.Attributes.Set(auth.ExecuteSkipAuthorization, true)

    qry.SetReqCtx(reqCtx)
    qry.SetTenantUuid(tenantUuid)

    return qry
}
```

These helpers make test code more concise and consistent:

```go
func TestWithHelpers(t *testing.T) {
    fc := setupFacade()
    ctx := context.Background()

    cmd := test.NewTestCommand("tenant-123", "Order", &command.PlaceOrder{
        OrderUuid: "order-123",
        Items:     []aggregate.Item{{Quantity: 1, Price: 50}},
    })

    _, err := fc.DispatchCommand(ctx, cmd)
    require.NoError(t, err)
    fc.WaitForCmd(ctx, cmd)
}
```

## Structured Test Fixtures

Test fixtures provide consistent, well-known data for tests. Using constants and helper functions ensures that test data is reusable and changes can be made in one place:

```go
// test/default.tenant.go
package test

const (
    TENANT_000_UUID string = "3f58cd86-0d14-4c2f-bcea-be60b6e32c62"
    TENANT_000_NAME string = "tenant-000"
    TENANT_001_UUID string = "52ff913d-a4a9-424d-90ca-4f049ec86eb6"
    TENANT_001_NAME string = "tenant-001"
)

// TestFillTenants populates the system with standard test tenants
func TestFillTenants(fc *comby.Facade) {
    TestAddTenant(fc, TENANT_000_UUID, TENANT_000_NAME)
    TestAddTenant(fc, TENANT_001_UUID, TENANT_001_NAME)
}

// TestAddTenant creates a tenant and returns the resulting model
func TestAddTenant(fc *comby.Facade, tenantUuid, name string) (*readmodel.TenantModel, error) {
    ctx := context.Background()
    cmd := NewTestCommand(tenantUuid, "Tenant", &command.TenantCommandCreate{
        TenantUuid: tenantUuid,
        Name:       name,
    })

    fc.DispatchCommand(ctx, cmd)
    fc.WaitForCmd(ctx, cmd)

    return TestGetTenant(fc, tenantUuid)
}

// TestGetTenant retrieves a tenant by UUID
func TestGetTenant(fc *comby.Facade, tenantUuid string) (*readmodel.TenantModel, error) {
    ctx := context.Background()
    qry := NewTestQuery(tenantUuid, "Tenant", &query.TenantQueryModel{
        TenantUuid: tenantUuid,
    })

    res, err := fc.DispatchQuery(ctx, qry)
    if err != nil {
        return nil, err
    }

    if resTyped, ok := res.(*query.TenantQueryItemResponse); ok {
        return resTyped.Item, nil
    }
    return nil, nil
}
```

Similar fixtures should be created for other domain entities (accounts, identities, groups, etc.). This creates a consistent vocabulary for tests across the codebase.

## Test Domain for Framework Testing

When testing framework functionality itself, a dedicated test domain isolates framework behavior from business logic. This dummy domain provides different handler variants to test various scenarios:

```go
// test/dummy.domain.go
package test

// Dummy aggregate for testing framework behavior
type Dummy struct {
    *comby.BaseAggregate
    Value string
}

func NewAggregate() *Dummy {
    agg := &Dummy{}
    agg.BaseAggregate = comby.NewBaseAggregate()
    agg.Domain = "Dummy"
    comby.AddDomainEventHandler(agg, agg.DummyEvent)
    return agg
}

type DummyEvent struct {
    DummyUuid string `json:"dummyUuid"`
    Value     string `json:"value"`
}

func (agg *Dummy) DoDummy(value string) error {
    domainEvt := &DummyEvent{
        DummyUuid: agg.GetAggregateUuid(),
        Value:     value,
    }
    return comby.ApplyDomainEvent(agg, domainEvt, true)
}

func (agg *Dummy) DummyEvent(ctx context.Context, evt comby.Event, domainEvt *DummyEvent) error {
    agg.Value = domainEvt.Value
    return nil
}

// Command handler with multiple variants for testing different scenarios
type cmdHandler struct {
    *comby.BaseCommandHandler
    AggregateRepository *comby.AggregateRepository[*Dummy]
}

// DummyWithoutRepository - simplest handler, no repository interaction
type DummyWithoutRepository struct {
    DummyUuid string `json:"dummyUuid"`
    Value     string `json:"value"`
}

func (cs *cmdHandler) DoDummyWithoutRepository(ctx context.Context, cmd comby.Command, domainCmd *DummyWithoutRepository) ([]comby.Event, error) {
    return []comby.Event{}, nil
}

// DummyWithRepository - full handler with repository and optional delay
type DummyWithRepository struct {
    DummyUuid string        `json:"dummyUuid"`
    Value     string        `json:"value"`
    Delay     time.Duration `json:"delay,omitempty"`
}

func (cs *cmdHandler) DoDummyWithRepository(ctx context.Context, cmd comby.Command, domainCmd *DummyWithRepository) ([]comby.Event, error) {
    if domainCmd.Delay > 0 {
        time.Sleep(domainCmd.Delay)  // Simulate slow operations
    }

    agg, _ := cs.AggregateRepository.GetAggregate(ctx, domainCmd.DummyUuid)
    if agg == nil {
        agg = NewAggregate()
        agg.SetAggregateUuid(domainCmd.DummyUuid)
    }

    agg.DoDummy(domainCmd.Value)
    return agg.GetUncommittedEvents(), nil
}

// DummyUnknownEvent - handler that returns unknown event type (for error testing)
type DummyUnknownEvent struct {
    DummyUuid string `json:"dummyUuid"`
    Value     string `json:"value"`
}

// RegisterDummy registers the complete test domain
func RegisterDummy(ctx context.Context, fc *comby.Facade) error {
    comby.RegisterAggregate(fc, NewAggregate)

    rm := NewDummyReadmodel(fc)
    comby.RegisterEventHandler(fc, rm)

    aggregateRepository := comby.NewAggregateRepository(fc, NewAggregate)
    cs := NewDummyCommandHandler(aggregateRepository)
    comby.RegisterCommandHandler(fc, cs)

    qs := NewDummyQueryhandler(rm)
    comby.RegisterQueryHandler(fc, qs)

    return nil
}
```

The dummy domain provides handlers for different testing scenarios: simple handlers without repository access, full handlers with repository interaction, and error scenarios. This allows comprehensive testing of framework behavior.

## Performance Testing with Benchmarks

Event-sourced systems must handle potentially large volumes of events. Benchmark tests verify that the system meets performance requirements and help identify regressions:

```go
// facade_benchmark_test.go
package comby_test

import (
    "context"
    "sync"
    "testing"
    "time"
)

// BenchmarkCommandInMemoryWithoutRepository measures raw command throughput
// Result: ~120K ops/second
func BenchmarkCommandInMemoryWithoutRepository(b *testing.B) {
    ctx := context.Background()

    // Setup with in-memory stores for pure performance measurement
    fc, _ := comby.NewFacade()
    test.RegisterDummy(ctx, fc)
    fc.RestoreState()

    // Create command once outside the loop
    cmd, _ := comby.NewCommand("Dummy", &test.DummyWithoutRepository{
        DummyUuid: comby.NewUuid(),
        Value:     "benchmark-value",
    })

    // Measure throughput
    start := time.Now()
    for i := 0; i < b.N; i++ {
        fc.DispatchCommand(ctx, cmd)
    }
    duration := time.Since(start)

    // Report ops/second for easier interpretation
    opsPerSecond := float64(b.N) / duration.Seconds()
    b.ReportMetric(opsPerSecond, "ops/s")
}

// BenchmarkCommandInMemoryWithRepository measures throughput with repository access
// Result: ~67K ops/second
func BenchmarkCommandInMemoryWithRepository(b *testing.B) {
    ctx := context.Background()

    fc, _ := comby.NewFacade()
    test.RegisterDummy(ctx, fc)
    fc.RestoreState()

    cmd, _ := comby.NewCommand("Dummy", &test.DummyWithRepository{
        DummyUuid: comby.NewUuid(),
        Value:     "benchmark-value",
    })

    start := time.Now()
    for i := 0; i < b.N; i++ {
        fc.DispatchCommand(ctx, cmd)
    }
    duration := time.Since(start)
    opsPerSecond := float64(b.N) / duration.Seconds()
    b.ReportMetric(opsPerSecond, "ops/s")
}

// BenchmarkCommandInMemoryWithWait measures synchronous command execution
// Result: ~17K ops/second
func BenchmarkCommandInMemoryWithWait(b *testing.B) {
    ctx := context.Background()

    fc, _ := comby.NewFacade()
    test.RegisterDummy(ctx, fc)
    fc.RestoreState()

    start := time.Now()
    for i := 0; i < b.N; i++ {
        cmd, _ := comby.NewCommand("Dummy", &test.DummyWithoutRepository{
            DummyUuid: comby.NewUuid(),
            Value:     "benchmark-value",
        })
        cmd.GetReqCtx().ExecuteWaitToFinish = true

        fc.DispatchCommand(ctx, cmd)
        fc.WaitForCmd(ctx, cmd)
    }
    duration := time.Since(start)
    opsPerSecond := float64(b.N) / duration.Seconds()
    b.ReportMetric(opsPerSecond, "ops/s")
}

// BenchmarkQueryInMemoryWithReadmodel measures query throughput
// Result: ~169K ops/second
func BenchmarkQueryInMemoryWithReadmodel(b *testing.B) {
    ctx := context.Background()

    fc, _ := comby.NewFacade()
    test.RegisterDummy(ctx, fc)
    fc.RestoreState()

    // Setup: Create data to query
    dummyUuid := comby.NewUuid()
    cmd, _ := comby.NewCommand("Dummy", &test.DummyWithRepository{
        DummyUuid: dummyUuid,
        Value:     "query-target",
    })
    cmd.GetReqCtx().ExecuteWaitToFinish = true
    fc.DispatchCommand(ctx, cmd)
    fc.WaitForCmd(ctx, cmd)

    qry, _ := comby.NewQuery("Dummy", &test.DummyQueryRequest{
        DummyUuid: dummyUuid,
    })

    start := time.Now()
    for i := 0; i < b.N; i++ {
        res, _ := fc.DispatchQuery(ctx, qry)

        // Verify each query returns valid data
        if val, ok := res.(*test.DummyQueryResponse); ok {
            if val.Model == nil || val.Model.DummyUuid != dummyUuid {
                b.Fatal("invalid response")
            }
        }
    }
    duration := time.Since(start)
    opsPerSecond := float64(b.N) / duration.Seconds()
    b.ReportMetric(opsPerSecond, "ops/s")
}
```

Benchmarks also reveal the performance impact of different implementation strategies. This example compares direct method calls vs. reflection-based dispatch:

```go
// aggregate_benchmark_test.go

// Direct method call: 0.5 ns/op (baseline)
func BenchmarkAggregateCallMethodDirect(b *testing.B) {
    agg := NewAggregate(comby.NewUuid())
    evt, _ := comby.NewEventFromAggregate(agg, &AppliedEvent{
        MyCustomField: "MyCustomField",
    })

    for i := 0; i < b.N; i++ {
        agg.OnAppliedEvent(context.Background(), evt, &AppliedEvent{
            MyCustomField: "MyCustomField",
        })
    }
}

// Function mapper: 6.97 ns/op (~14x slower than direct)
func BenchmarkAggregateCallMethodMapper(b *testing.B) {
    agg := NewAggregate(comby.NewUuid())
    domainEvt := &AppliedEvent{MyCustomField: "MyCustomField"}
    evt, _ := comby.NewEventFromAggregate(agg, domainEvt)

    mapper := agg.GetDomainEventHandlers(domainEvt)
    for i := 0; i < b.N; i++ {
        handler := mapper[0]
        fn := handler.DomainEvtHandlerFunc
        fn(context.Background(), evt, evt.GetDomainEvt())
    }
}

// Full reflection: 1355 ns/op (~2700x slower than direct)
func BenchmarkAggregateCallMethodWithReflect(b *testing.B) {
    agg := NewAggregate(comby.NewUuid())
    evt, _ := comby.NewEventFromAggregate(agg, &AppliedEvent{
        MyCustomField: "MyCustomField",
    })

    for i := 0; i < b.N; i++ {
        evtData := evt.GetDomainEvt()
        method := reflect.ValueOf(agg).MethodByName("OnAppliedEvent")
        args := []reflect.Value{
            reflect.ValueOf(context.Background()),
            reflect.ValueOf(evt),
            reflect.ValueOf(evtData),
        }
        method.Call(args)
    }
}
```

These benchmarks demonstrate why comby uses a function mapper approach instead of pure reflection — it provides the flexibility of dynamic dispatch with only a ~14x overhead compared to direct calls, versus ~2700x for full reflection.

## Concurrency Testing

Event-sourced systems often process commands concurrently. Testing concurrent execution ensures thread safety and helps identify race conditions:

```go
// Test concurrent command execution
func BenchmarkCommandInMemoryWithWaitInGoroutines(b *testing.B) {
    ctx := context.Background()

    fc, _ := comby.NewFacade()
    test.RegisterDummy(ctx, fc)
    fc.RestoreState()

    start := time.Now()
    var wg sync.WaitGroup

    for i := 0; i < b.N; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()

            ctx := context.Background()
            cmd, _ := comby.NewCommand("Dummy", &test.DummyWithoutRepository{
                DummyUuid: comby.NewUuid(),
                Value:     "concurrent-value",
            })
            cmd.GetReqCtx().ExecuteWaitToFinish = true

            _, err := fc.DispatchCommand(ctx, cmd)
            if err != nil {
                // Queue full is acceptable under load
                return
            }
            fc.WaitForCmd(ctx, cmd)
        }()
    }
    wg.Wait()

    duration := time.Since(start)
    opsPerSecond := float64(b.N) / duration.Seconds()
    b.ReportMetric(opsPerSecond, "ops/s")
}
```

Running with the race detector (`go test -race`) during development catches data races early.

## Testing Event Stores with Encryption

Event stores may support encryption for sensitive data. Testing encryption ensures that data is properly encrypted at rest and correctly decrypted when read:

```go
func TestEventStoreMemory_WithEncryption(t *testing.T) {
    ctx := context.Background()

    // Create crypto service with AES-256 key (32 bytes)
    cryptoService, err := comby.NewCryptoService(
        []byte("12345678901234567890123456789012"),
    )
    require.NoError(t, err)

    // Create store with encryption enabled
    store := comby.NewEventStoreMemory(
        comby.EventStoreOptionWithCryptoService(cryptoService),
    )
    store.Init(ctx)
    defer store.Close(ctx)

    // Create event with sensitive data
    evt := comby.NewBaseEvent()
    evt.SetEventUuid("test-evt-uuid")
    evt.SetTenantUuid("test-tenant")
    evt.SetAggregateUuid("test-aggregate")
    evt.SetDomain("TestDomain")
    evt.SetDomainEvtName("TestEvent")
    evt.SetDomainEvtBytes([]byte(`{"action":"created","data":"sensitive-data"}`))

    // Store event (domain data should be encrypted internally)
    err = store.Create(ctx, comby.EventStoreCreateOptionWithEvent(evt))
    require.NoError(t, err)

    // Retrieve event (domain data should be decrypted automatically)
    retrievedEvt, err := store.Get(ctx,
        comby.EventStoreGetOptionWithEventUuid(evt.GetEventUuid()),
    )
    require.NoError(t, err)
    require.NotNil(t, retrievedEvt)

    // Verify decrypted data matches original
    assert.Equal(t,
        string(evt.GetDomainEvtBytes()),
        string(retrievedEvt.GetDomainEvtBytes()),
    )
}

func TestEventStoreMemory_ListWithEncryption(t *testing.T) {
    ctx := context.Background()

    cryptoService, _ := comby.NewCryptoService(
        []byte("12345678901234567890123456789012"),
    )
    store := comby.NewEventStoreMemory(
        comby.EventStoreOptionWithCryptoService(cryptoService),
    )
    store.Init(ctx)
    defer store.Close(ctx)

    tenantUuid := "tenant-123"
    numEvents := 5

    // Store multiple encrypted events
    for i := 0; i < numEvents; i++ {
        evt := comby.NewBaseEvent()
        evt.SetEventUuid(fmt.Sprintf("evt-%d", i))
        evt.SetTenantUuid(tenantUuid)
        evt.SetAggregateUuid("aggregate-123")
        evt.SetDomain("TestDomain")
        evt.SetDomainEvtName("TestEvent")
        evt.SetDomainEvtBytes([]byte(fmt.Sprintf(`{"id":%d,"secret":"data"}`, i)))

        store.Create(ctx, comby.EventStoreCreateOptionWithEvent(evt))
    }

    // List all events - they should all be decrypted
    events, total, _ := store.List(ctx,
        comby.EventStoreListOptionWithTenantUuid(tenantUuid),
    )

    assert.Equal(t, int64(numEvents), total)

    // Verify all events are properly decrypted
    for _, evt := range events {
        data := string(evt.GetDomainEvtBytes())
        assert.NotEmpty(t, data)
        assert.True(t, strings.HasPrefix(data, "{"), "Expected JSON data")
    }
}
```

Encryption tests should cover all store operations: create, read, update, delete, and list. This ensures encryption is transparent to the application while data is protected at rest.

## Running an Example

Source: https://github.com/susdorf/building-event-sourced-systems-in-go/tree/main/15-testing-event-sourced-systems/code

When you run the complete example, you'll see testing patterns in action:

```
=== Testing Event-Sourced Systems Demo ===

1. Basic Aggregate Test (Place Order):
   PASS: Order placed with 2 items, total: 109.97
         Event emitted: OrderPlacedEvent

2. Business Rule Tests:
   PASS: rejects empty order
   PASS: rejects zero quantity
   PASS: rejects negative quantity
   PASS: rejects negative price
   PASS: accepts valid order
   Summary: 5 passed, 0 failed

3. Given-When-Then Pattern:
   Given: A paid order
         Status: paid, Total: 100.00
   When: Shipping the order
   Then: Order should emit shipped event
   PASS: Status=shipped, TrackingNo=TRACK-456

4. Table-Driven Tests (State Transitions):
   PASS: can place new order                    (emitted OrderPlacedEvent)
   PASS: cannot place already placed order      (error expected)
   PASS: can pay placed order                   (emitted OrderPaidEvent)
   PASS: cannot pay new order                   (error expected)
   PASS: cannot ship unpaid order               (error expected)
   PASS: can ship paid order                    (emitted OrderShippedEvent)
   PASS: cannot ship already shipped order      (error expected)
   Summary: 7 passed, 0 failed

5. Event Replay Test (State Reconstruction):
   Initial state: Status=''
   After event 1 (OrderPlacedEvent): Status='placed'
   After event 2 (OrderPaidEvent): Status='paid'
   After event 3 (OrderShippedEvent): Status='shipped'
   PASS: Final state matches expected state after replay

6. Using Test Fixtures:
   Tenant fixtures:
     TENANT_000_UUID: 3f58cd86-0d14-4c2f-bcea-be60b6e32c62
     TENANT_001_UUID: 52ff913d-a4a9-424d-90ca-4f049ec86eb6
   Order fixtures:
     ORDER_000_UUID: order-000-uuid
   State helper functions:
     GivenNewOrder():    Status=''
     GivenPlacedOrder(): Status='placed', Total=100.00
     GivenPaidOrder():   Status='paid', PaymentID='payment-123'
     GivenShippedOrder(): Status='shipped', TrackingNo='TRACK-123'

7. Simulated Benchmark (Command Processing):
   Place order (no repository): 100000 iterations in 8.1ms
     -> 12318810 ops/sec, 81.18 ns/op
   Full order lifecycle: 100000 iterations in 19.2ms
     -> 5194311 ops/sec, 192.52 ns/op
   Event replay (3 events): 100000 iterations in 496µs
     -> 201545451 ops/sec, 4.96 ns/op

=== Demo Complete ===
```

## Summary

Testing event-sourced systems leverages the architecture's inherent strengths:

1. **Aggregate tests**: Verify events emitted for business intentions and ensure business rules reject invalid operations
2. **Business rule tests**: Dedicated tests for each invariant the system must maintain
3. **Handler tests**: Test command and query coordination between components
4. **Projection tests**: Feed events and verify the resulting read model state
5. **Integration tests**: Full stack testing with in-memory stores for fast, isolated execution
6. **Table-driven tests**: Comprehensive coverage of state transitions with minimal code duplication
7. **Test suite setup**: Consistent environment configuration and test data population
8. **Test helpers**: Reusable utilities for creating commands, queries, and test data
9. **Performance benchmarks**: Measure throughput and identify bottlenecks before they become production issues
10. **Concurrency tests**: Verify thread safety under parallel execution
11. **Store tests**: Ensure persistence layer behavior including encryption

The event-based architecture makes testing natural — you control state precisely by providing events and verify behavior by checking emitted events. This results in tests that are deterministic, fast, and closely aligned with how the system actually operates.

---

## What's Next

In the next post, we'll explore **REST API** — building HTTP endpoints to expose our event-sourced system to clients. We'll see how to design clean, efficient APIs that leverage our command and query handlers for a seamless experience.
