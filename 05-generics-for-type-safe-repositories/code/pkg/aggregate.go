package pkg

import "fmt"

// Aggregate is the interface that all aggregates must implement.
// This constraint enables the generic repository to work with any aggregate type.
type Aggregate interface {
	GetID() string
	SetID(id string)
	GetVersion() int64
	GetDomain() string
	GetUncommittedEvents() []Event
	ClearUncommittedEvents()
	ApplyEvent(event Event) error
}

// BaseAggregate provides common functionality for all aggregates.
type BaseAggregate struct {
	ID                string
	Version           int64
	Domain            string
	uncommittedEvents []Event
	eventHandlers     map[string]func(Event) error
}

// NewBaseAggregate creates a new base aggregate.
func NewBaseAggregate() *BaseAggregate {
	return &BaseAggregate{
		uncommittedEvents: make([]Event, 0),
		eventHandlers:     make(map[string]func(Event) error),
	}
}

func (a *BaseAggregate) GetID() string              { return a.ID }
func (a *BaseAggregate) SetID(id string)            { a.ID = id }
func (a *BaseAggregate) GetVersion() int64          { return a.Version }
func (a *BaseAggregate) GetDomain() string          { return a.Domain }
func (a *BaseAggregate) GetUncommittedEvents() []Event { return a.uncommittedEvents }
func (a *BaseAggregate) ClearUncommittedEvents()    { a.uncommittedEvents = nil }

// RegisterEventHandler registers a handler for a specific event type.
func (a *BaseAggregate) RegisterEventHandler(eventType string, handler func(Event) error) {
	a.eventHandlers[eventType] = handler
}

// ApplyEvent applies an event to the aggregate using registered handlers.
func (a *BaseAggregate) ApplyEvent(event Event) error {
	handler, ok := a.eventHandlers[event.EventType]
	if !ok {
		return fmt.Errorf("no handler registered for event type: %s", event.EventType)
	}
	if err := handler(event); err != nil {
		return err
	}
	a.Version = event.Version
	return nil
}

// RaiseEvent creates a new event and adds it to uncommitted events.
func (a *BaseAggregate) RaiseEvent(eventType string, data any) {
	a.Version++
	event := NewEvent(a.ID, a.Domain, eventType, data, a.Version)
	a.uncommittedEvents = append(a.uncommittedEvents, event)
	// Apply to self
	_ = a.ApplyEvent(event)
}

// --- Order Aggregate (Example) ---

// OrderStatus represents the state of an order.
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusShipped   OrderStatus = "shipped"
	OrderStatusDelivered OrderStatus = "delivered"
	OrderStatusCancelled OrderStatus = "cancelled"
)

// Order is an example aggregate representing a customer order.
type Order struct {
	*BaseAggregate
	CustomerID   string
	Items        []OrderItem
	Status       OrderStatus
	ShippingInfo string
	Total        float64
}

// OrderItem represents an item in an order.
type OrderItem struct {
	ProductID string
	Name      string
	Quantity  int
	Price     float64
}

// NewOrder is the factory function that creates a properly initialized Order.
// This is the NewAggregateFunc that gets passed to the repository.
func NewOrder() *Order {
	order := &Order{}
	order.BaseAggregate = NewBaseAggregate()
	order.Domain = "Order"
	order.Items = make([]OrderItem, 0)

	// Register domain event handlers - essential for event replay!
	order.RegisterEventHandler("OrderCreated", order.onOrderCreated)
	order.RegisterEventHandler("ItemAdded", order.onItemAdded)
	order.RegisterEventHandler("OrderConfirmed", order.onOrderConfirmed)
	order.RegisterEventHandler("OrderShipped", order.onOrderShipped)
	order.RegisterEventHandler("OrderCancelled", order.onOrderCancelled)

	return order
}

// --- Order Domain Events ---

type OrderCreatedData struct {
	CustomerID string
}

type ItemAddedData struct {
	ProductID string
	Name      string
	Quantity  int
	Price     float64
}

type OrderShippedData struct {
	TrackingNumber string
}

// --- Order Event Handlers ---

func (o *Order) onOrderCreated(event Event) error {
	data := event.Data.(OrderCreatedData)
	o.CustomerID = data.CustomerID
	o.Status = OrderStatusPending
	return nil
}

func (o *Order) onItemAdded(event Event) error {
	data := event.Data.(ItemAddedData)
	o.Items = append(o.Items, OrderItem{
		ProductID: data.ProductID,
		Name:      data.Name,
		Quantity:  data.Quantity,
		Price:     data.Price,
	})
	o.Total += data.Price * float64(data.Quantity)
	return nil
}

func (o *Order) onOrderConfirmed(event Event) error {
	o.Status = OrderStatusConfirmed
	return nil
}

func (o *Order) onOrderShipped(event Event) error {
	data := event.Data.(OrderShippedData)
	o.Status = OrderStatusShipped
	o.ShippingInfo = data.TrackingNumber
	return nil
}

func (o *Order) onOrderCancelled(event Event) error {
	o.Status = OrderStatusCancelled
	return nil
}

// --- Order Commands (business operations) ---

// Create initializes a new order with the given ID and customer.
func (o *Order) Create(orderID, customerID string) {
	o.ID = orderID
	o.RaiseEvent("OrderCreated", OrderCreatedData{CustomerID: customerID})
}

// AddItem adds an item to the order.
func (o *Order) AddItem(productID, name string, quantity int, price float64) error {
	if o.Status != OrderStatusPending {
		return fmt.Errorf("cannot add items to order in status: %s", o.Status)
	}
	o.RaiseEvent("ItemAdded", ItemAddedData{
		ProductID: productID,
		Name:      name,
		Quantity:  quantity,
		Price:     price,
	})
	return nil
}

// Confirm confirms the order.
func (o *Order) Confirm() error {
	if o.Status != OrderStatusPending {
		return fmt.Errorf("can only confirm pending orders, current status: %s", o.Status)
	}
	if len(o.Items) == 0 {
		return fmt.Errorf("cannot confirm order with no items")
	}
	o.RaiseEvent("OrderConfirmed", nil)
	return nil
}

// Ship marks the order as shipped.
func (o *Order) Ship(trackingNumber string) error {
	if o.Status != OrderStatusConfirmed {
		return fmt.Errorf("can only ship confirmed orders, current status: %s", o.Status)
	}
	o.RaiseEvent("OrderShipped", OrderShippedData{TrackingNumber: trackingNumber})
	return nil
}

// Cancel cancels the order.
func (o *Order) Cancel() error {
	if o.Status == OrderStatusShipped || o.Status == OrderStatusDelivered {
		return fmt.Errorf("cannot cancel order that has been shipped or delivered")
	}
	o.RaiseEvent("OrderCancelled", nil)
	return nil
}

// --- Customer Aggregate (Second Example) ---

// Customer is another example aggregate to demonstrate type safety.
type Customer struct {
	*BaseAggregate
	Email     string
	Name      string
	VIP       bool
	OrderIDs  []string
}

// NewCustomer is the factory function for Customer aggregates.
func NewCustomer() *Customer {
	customer := &Customer{}
	customer.BaseAggregate = NewBaseAggregate()
	customer.Domain = "Customer"
	customer.OrderIDs = make([]string, 0)

	// Register event handlers
	customer.RegisterEventHandler("CustomerRegistered", customer.onCustomerRegistered)
	customer.RegisterEventHandler("CustomerUpgradedToVIP", customer.onCustomerUpgradedToVIP)
	customer.RegisterEventHandler("OrderLinked", customer.onOrderLinked)

	return customer
}

type CustomerRegisteredData struct {
	Email string
	Name  string
}

func (c *Customer) onCustomerRegistered(event Event) error {
	data := event.Data.(CustomerRegisteredData)
	c.Email = data.Email
	c.Name = data.Name
	return nil
}

func (c *Customer) onCustomerUpgradedToVIP(event Event) error {
	c.VIP = true
	return nil
}

func (c *Customer) onOrderLinked(event Event) error {
	orderID := event.Data.(string)
	c.OrderIDs = append(c.OrderIDs, orderID)
	return nil
}

// Register creates a new customer.
func (c *Customer) Register(customerID, email, name string) {
	c.ID = customerID
	c.RaiseEvent("CustomerRegistered", CustomerRegisteredData{Email: email, Name: name})
}

// UpgradeToVIP upgrades the customer to VIP status.
func (c *Customer) UpgradeToVIP() {
	if !c.VIP {
		c.RaiseEvent("CustomerUpgradedToVIP", nil)
	}
}

// LinkOrder links an order to this customer.
func (c *Customer) LinkOrder(orderID string) {
	c.RaiseEvent("OrderLinked", orderID)
}
