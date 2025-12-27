package pkg

// Test fixtures provide well-known test data that can be reused across tests

const (
	// Tenant fixtures
	TENANT_000_UUID = "3f58cd86-0d14-4c2f-bcea-be60b6e32c62"
	TENANT_000_NAME = "tenant-000"
	TENANT_001_UUID = "52ff913d-a4a9-424d-90ca-4f049ec86eb6"
	TENANT_001_NAME = "tenant-001"

	// Order fixtures
	ORDER_000_UUID = "order-000-uuid"
	ORDER_001_UUID = "order-001-uuid"
)

// SampleItems returns a slice of sample items for testing
func SampleItems() []Item {
	return []Item{
		{ItemUid: "item-1", Sku: "SKU001", Quantity: 2, Price: 29.99},
		{ItemUid: "item-2", Sku: "SKU002", Quantity: 1, Price: 49.99},
	}
}

// SingleItem returns a single item for simple tests
func SingleItem() []Item {
	return []Item{
		{ItemUid: "item-1", Sku: "SKU001", Quantity: 1, Price: 100.0},
	}
}

// GivenNewOrder creates a new empty order for testing
func GivenNewOrder(orderID string) *Order {
	order := NewOrder()
	order.AggregateUuid = orderID
	return order
}

// GivenPlacedOrder creates an order in "placed" status
func GivenPlacedOrder(orderID string) *Order {
	order := GivenNewOrder(orderID)
	order.Place(SingleItem())
	order.ClearUncommittedEvents()
	return order
}

// GivenPaidOrder creates an order in "paid" status
func GivenPaidOrder(orderID string) *Order {
	order := GivenPlacedOrder(orderID)
	order.Pay("payment-123", 100.0)
	order.ClearUncommittedEvents()
	return order
}

// GivenShippedOrder creates an order in "shipped" status
func GivenShippedOrder(orderID string) *Order {
	order := GivenPaidOrder(orderID)
	order.Ship("TRACK-123")
	order.ClearUncommittedEvents()
	return order
}

// TransitionTestCase defines a test case for state transition testing
type TransitionTestCase struct {
	Name          string
	InitialState  func(string) *Order
	Action        func(*Order) error
	ExpectError   bool
	ExpectedEvent string
	Description   string
}

// OrderTransitionTests returns all state transition test cases
func OrderTransitionTests() []TransitionTestCase {
	return []TransitionTestCase{
		{
			Name:         "can place new order",
			InitialState: GivenNewOrder,
			Action: func(o *Order) error {
				return o.Place(SingleItem())
			},
			ExpectError:   false,
			ExpectedEvent: "OrderPlacedEvent",
			Description:   "New orders can be placed",
		},
		{
			Name:         "cannot place already placed order",
			InitialState: GivenPlacedOrder,
			Action: func(o *Order) error {
				return o.Place(SingleItem())
			},
			ExpectError:   true,
			ExpectedEvent: "",
			Description:   "Placing an already placed order violates business rules",
		},
		{
			Name:         "can pay placed order",
			InitialState: GivenPlacedOrder,
			Action: func(o *Order) error {
				return o.Pay("pay-123", 100.0)
			},
			ExpectError:   false,
			ExpectedEvent: "OrderPaidEvent",
			Description:   "Orders in 'placed' status can be paid",
		},
		{
			Name:         "cannot pay new order",
			InitialState: GivenNewOrder,
			Action: func(o *Order) error {
				return o.Pay("pay-123", 100.0)
			},
			ExpectError:   true,
			ExpectedEvent: "",
			Description:   "Cannot pay an order that hasn't been placed",
		},
		{
			Name:         "cannot ship unpaid order",
			InitialState: GivenPlacedOrder,
			Action: func(o *Order) error {
				return o.Ship("TRACK-123")
			},
			ExpectError:   true,
			ExpectedEvent: "",
			Description:   "Orders must be paid before shipping",
		},
		{
			Name:         "can ship paid order",
			InitialState: GivenPaidOrder,
			Action: func(o *Order) error {
				return o.Ship("TRACK-123")
			},
			ExpectError:   false,
			ExpectedEvent: "OrderShippedEvent",
			Description:   "Paid orders can be shipped",
		},
		{
			Name:         "cannot ship already shipped order",
			InitialState: GivenShippedOrder,
			Action: func(o *Order) error {
				return o.Ship("TRACK-456")
			},
			ExpectError:   true,
			ExpectedEvent: "",
			Description:   "Already shipped orders cannot be shipped again",
		},
	}
}

// BusinessRuleTestCase defines a test case for business rule validation
type BusinessRuleTestCase struct {
	Name        string
	Items       []Item
	ExpectError bool
	ErrorMsg    string
	Description string
}

// OrderBusinessRuleTests returns business rule validation test cases
func OrderBusinessRuleTests() []BusinessRuleTestCase {
	return []BusinessRuleTestCase{
		{
			Name:        "rejects empty order",
			Items:       []Item{},
			ExpectError: true,
			ErrorMsg:    "at least one item",
			Description: "Orders must have at least one item",
		},
		{
			Name: "rejects zero quantity",
			Items: []Item{
				{ItemUid: "item-1", Sku: "SKU001", Quantity: 0, Price: 29.99},
			},
			ExpectError: true,
			ErrorMsg:    "quantity must be positive",
			Description: "Item quantity must be positive",
		},
		{
			Name: "rejects negative quantity",
			Items: []Item{
				{ItemUid: "item-1", Sku: "SKU001", Quantity: -1, Price: 29.99},
			},
			ExpectError: true,
			ErrorMsg:    "quantity must be positive",
			Description: "Item quantity cannot be negative",
		},
		{
			Name: "rejects negative price",
			Items: []Item{
				{ItemUid: "item-1", Sku: "SKU001", Quantity: 1, Price: -10.0},
			},
			ExpectError: true,
			ErrorMsg:    "price cannot be negative",
			Description: "Item price cannot be negative",
		},
		{
			Name: "accepts valid order",
			Items: []Item{
				{ItemUid: "item-1", Sku: "SKU001", Quantity: 2, Price: 29.99},
			},
			ExpectError: false,
			ErrorMsg:    "",
			Description: "Valid orders are accepted",
		},
	}
}
