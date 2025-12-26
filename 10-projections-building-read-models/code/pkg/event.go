package pkg

import (
	"encoding/json"
	"time"
)

// Event represents a domain event stored in the event store
type Event struct {
	UUID          string
	TenantUUID    string
	AggregateUUID string
	Domain        string
	DataType      string
	Data          string
	Version       int64
	CreatedAt     int64
}

func (e Event) GetEventUUID() string     { return e.UUID }
func (e Event) GetTenantUUID() string    { return e.TenantUUID }
func (e Event) GetAggregateUUID() string { return e.AggregateUUID }
func (e Event) GetDomain() string        { return e.Domain }
func (e Event) GetDataType() string      { return e.DataType }
func (e Event) GetVersion() int64        { return e.Version }
func (e Event) GetCreatedAt() int64      { return e.CreatedAt }

// Domain Events

type OrderPlacedEvent struct {
	CustomerID   string      `json:"customerId"`
	CustomerName string      `json:"customerName"`
	Items        []OrderItem `json:"items"`
	Total        float64     `json:"total"`
}

type OrderItem struct {
	ProductID   string  `json:"productId"`
	ProductName string  `json:"productName"`
	Quantity    int     `json:"quantity"`
	Price       float64 `json:"price"`
}

type OrderPaidEvent struct {
	PaymentID     string  `json:"paymentId"`
	PaymentMethod string  `json:"paymentMethod"`
	Amount        float64 `json:"amount"`
}

type OrderShippedEvent struct {
	ShipmentID      string `json:"shipmentId"`
	TrackingNumber  string `json:"trackingNumber"`
	EstimatedArrival string `json:"estimatedArrival"`
}

type OrderCancelledEvent struct {
	Reason string `json:"reason"`
}

type ItemAddedEvent struct {
	ProductID   string  `json:"productId"`
	ProductName string  `json:"productName"`
	Quantity    int     `json:"quantity"`
	Price       float64 `json:"price"`
}

// EventRepository simulates an event store for the demo
type EventRepository struct {
	events []Event
}

func NewEventRepository() *EventRepository {
	return &EventRepository{
		events: make([]Event, 0),
	}
}

func (r *EventRepository) Add(evt Event) {
	r.events = append(r.events, evt)
}

func (r *EventRepository) GetEventsForAggregate(tenantUUID, aggregateUUID string) []Event {
	var result []Event
	for _, evt := range r.events {
		if evt.TenantUUID == tenantUUID && evt.AggregateUUID == aggregateUUID {
			result = append(result, evt)
		}
	}
	return result
}

func (r *EventRepository) GetAllEventsForDomain(domain string) []Event {
	var result []Event
	for _, evt := range r.events {
		if evt.Domain == domain {
			result = append(result, evt)
		}
	}
	return result
}

func (r *EventRepository) GetEventsAfter(domain string, timestamp int64) []Event {
	var result []Event
	for _, evt := range r.events {
		if evt.Domain == domain && evt.CreatedAt > timestamp {
			result = append(result, evt)
		}
	}
	return result
}

func (r *EventRepository) Count() int {
	return len(r.events)
}

// Helper to create sample events
func CreateSampleEvents() *EventRepository {
	repo := NewEventRepository()
	baseTime := time.Now().Add(-10 * time.Minute)

	// Order 1 - Tenant A: Placed, Paid, Shipped
	order1Placed := OrderPlacedEvent{
		CustomerID:   "cust-1",
		CustomerName: "Alice Johnson",
		Items: []OrderItem{
			{ProductID: "prod-1", ProductName: "Laptop", Quantity: 1, Price: 999.99},
			{ProductID: "prod-2", ProductName: "Mouse", Quantity: 2, Price: 29.99},
		},
		Total: 1059.97,
	}
	data1, _ := json.Marshal(order1Placed)
	repo.Add(Event{
		UUID: "evt-1", TenantUUID: "tenant-A", AggregateUUID: "order-1",
		Domain: "Order", DataType: "OrderPlacedEvent", Data: string(data1),
		Version: 1, CreatedAt: baseTime.UnixNano(),
	})

	order1Paid := OrderPaidEvent{PaymentID: "pay-1", PaymentMethod: "credit_card", Amount: 1059.97}
	data2, _ := json.Marshal(order1Paid)
	repo.Add(Event{
		UUID: "evt-2", TenantUUID: "tenant-A", AggregateUUID: "order-1",
		Domain: "Order", DataType: "OrderPaidEvent", Data: string(data2),
		Version: 2, CreatedAt: baseTime.Add(1 * time.Minute).UnixNano(),
	})

	order1Shipped := OrderShippedEvent{ShipmentID: "ship-1", TrackingNumber: "TRK123456", EstimatedArrival: "2025-01-03"}
	data3, _ := json.Marshal(order1Shipped)
	repo.Add(Event{
		UUID: "evt-3", TenantUUID: "tenant-A", AggregateUUID: "order-1",
		Domain: "Order", DataType: "OrderShippedEvent", Data: string(data3),
		Version: 3, CreatedAt: baseTime.Add(2 * time.Minute).UnixNano(),
	})

	// Order 2 - Tenant A: Placed only
	order2Placed := OrderPlacedEvent{
		CustomerID:   "cust-2",
		CustomerName: "Bob Smith",
		Items: []OrderItem{
			{ProductID: "prod-3", ProductName: "Keyboard", Quantity: 1, Price: 149.99},
		},
		Total: 149.99,
	}
	data4, _ := json.Marshal(order2Placed)
	repo.Add(Event{
		UUID: "evt-4", TenantUUID: "tenant-A", AggregateUUID: "order-2",
		Domain: "Order", DataType: "OrderPlacedEvent", Data: string(data4),
		Version: 1, CreatedAt: baseTime.Add(3 * time.Minute).UnixNano(),
	})

	// Order 3 - Tenant B: Placed, Cancelled
	order3Placed := OrderPlacedEvent{
		CustomerID:   "cust-3",
		CustomerName: "Carol Davis",
		Items: []OrderItem{
			{ProductID: "prod-1", ProductName: "Laptop", Quantity: 1, Price: 999.99},
		},
		Total: 999.99,
	}
	data5, _ := json.Marshal(order3Placed)
	repo.Add(Event{
		UUID: "evt-5", TenantUUID: "tenant-B", AggregateUUID: "order-3",
		Domain: "Order", DataType: "OrderPlacedEvent", Data: string(data5),
		Version: 1, CreatedAt: baseTime.Add(4 * time.Minute).UnixNano(),
	})

	order3Cancelled := OrderCancelledEvent{Reason: "Customer changed mind"}
	data6, _ := json.Marshal(order3Cancelled)
	repo.Add(Event{
		UUID: "evt-6", TenantUUID: "tenant-B", AggregateUUID: "order-3",
		Domain: "Order", DataType: "OrderCancelledEvent", Data: string(data6),
		Version: 2, CreatedAt: baseTime.Add(5 * time.Minute).UnixNano(),
	})

	// Order 4 - Tenant B: Placed, Paid (item added after placed)
	order4Placed := OrderPlacedEvent{
		CustomerID:   "cust-4",
		CustomerName: "Dan Wilson",
		Items: []OrderItem{
			{ProductID: "prod-4", ProductName: "Monitor", Quantity: 1, Price: 299.99},
		},
		Total: 299.99,
	}
	data7, _ := json.Marshal(order4Placed)
	repo.Add(Event{
		UUID: "evt-7", TenantUUID: "tenant-B", AggregateUUID: "order-4",
		Domain: "Order", DataType: "OrderPlacedEvent", Data: string(data7),
		Version: 1, CreatedAt: baseTime.Add(6 * time.Minute).UnixNano(),
	})

	itemAdded := ItemAddedEvent{ProductID: "prod-5", ProductName: "Cable", Quantity: 1, Price: 19.99}
	data8, _ := json.Marshal(itemAdded)
	repo.Add(Event{
		UUID: "evt-8", TenantUUID: "tenant-B", AggregateUUID: "order-4",
		Domain: "Order", DataType: "ItemAddedEvent", Data: string(data8),
		Version: 2, CreatedAt: baseTime.Add(7 * time.Minute).UnixNano(),
	})

	order4Paid := OrderPaidEvent{PaymentID: "pay-2", PaymentMethod: "paypal", Amount: 319.98}
	data9, _ := json.Marshal(order4Paid)
	repo.Add(Event{
		UUID: "evt-9", TenantUUID: "tenant-B", AggregateUUID: "order-4",
		Domain: "Order", DataType: "OrderPaidEvent", Data: string(data9),
		Version: 3, CreatedAt: baseTime.Add(8 * time.Minute).UnixNano(),
	})

	return repo
}
