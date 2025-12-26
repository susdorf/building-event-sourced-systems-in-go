package pkg

import (
	"context"
	"fmt"
	"sync"
)

// Event represents a domain event.
type Event struct {
	Uuid          string
	TenantUuid    string
	AggregateUuid string
	Domain        string
	EventType     string
	Data          interface{}
	Version       int64
}

// Command represents a command to be executed.
type Command struct {
	Uuid          string
	TenantUuid    string
	AggregateUuid string
	Domain        string
	CommandType   string
	Data          interface{}
}

// Aggregate is the interface for domain aggregates.
type Aggregate interface {
	GetDomain() string
	GetName() string
	GetIdentifier() string
	GetUuid() string
	GetVersion() int64
	GetEvents() []interface{}
	Apply(event interface{}) error
	ClearEvents()
}

// CommandHandler handles commands for a domain.
type CommandHandler interface {
	GetDomain() string
	GetIdentifier() string
	GetCommands() []string
	Handle(ctx context.Context, cmd Command) ([]Event, error)
}

// EventHandler handles events (readmodels, reactors).
type EventHandler interface {
	GetDomain() string
	GetName() string
	GetIdentifier() string
	GetEvents() []string
	Handle(ctx context.Context, evt Event) error
}

// QueryHandler handles queries for a domain.
type QueryHandler interface {
	GetDomain() string
	GetIdentifier() string
}

// NewAggregateFunc is a factory function for creating aggregates.
type NewAggregateFunc func() Aggregate

// Facade is the central hub that wires all components together.
type Facade struct {
	AppName string

	// Handler registries
	aggregates      sync.Map // map[string]Aggregate
	commandHandlers sync.Map // map[string]CommandHandler
	queryHandlers   sync.Map // map[string]QueryHandler
	eventHandlers   sync.Map // map[string]EventHandler

	// Event storage (in-memory for demo)
	events []Event
	mu     sync.RWMutex

	// Middleware stacks
	commandMiddlewares []CommandMiddlewareFunc
	eventMiddlewares   []EventMiddlewareFunc

	// Event bus for dispatching to handlers
	eventSubscribers map[string][]EventHandler
}

// FacadeOption configures the facade.
type FacadeOption func(*Facade) error

// NewFacade creates a new facade with the given options.
func NewFacade(opts ...FacadeOption) (*Facade, error) {
	fc := &Facade{
		AppName:          "app",
		events:           make([]Event, 0),
		eventSubscribers: make(map[string][]EventHandler),
	}

	for _, opt := range opts {
		if err := opt(fc); err != nil {
			return nil, err
		}
	}

	return fc, nil
}

// FacadeWithAppName sets the application name.
func FacadeWithAppName(name string) FacadeOption {
	return func(fc *Facade) error {
		fc.AppName = name
		return nil
	}
}

// AddCommandMiddleware adds middleware to the command pipeline.
func (fc *Facade) AddCommandMiddleware(mw CommandMiddlewareFunc) {
	fc.commandMiddlewares = append(fc.commandMiddlewares, mw)
}

// AddEventMiddleware adds middleware to the event pipeline.
func (fc *Facade) AddEventMiddleware(mw EventMiddlewareFunc) {
	fc.eventMiddlewares = append(fc.eventMiddlewares, mw)
}

// RegisterAggregate registers an aggregate type.
func RegisterAggregate(fc *Facade, fn NewAggregateFunc) error {
	agg := fn()

	if agg.GetDomain() == "" {
		return fmt.Errorf("aggregate has no Domain set")
	}

	identifier := agg.GetIdentifier()
	if _, exists := fc.aggregates.Load(identifier); exists {
		return fmt.Errorf("aggregate already registered: %s", identifier)
	}

	fc.aggregates.Store(identifier, agg)
	fmt.Printf("  [Facade] Registered aggregate: %s\n", identifier)
	return nil
}

// RegisterCommandHandler registers a command handler.
func RegisterCommandHandler(fc *Facade, ch CommandHandler) error {
	if ch.GetDomain() == "" {
		return fmt.Errorf("command handler has no Domain set")
	}

	identifier := ch.GetIdentifier()
	if _, exists := fc.commandHandlers.Load(identifier); exists {
		return fmt.Errorf("command handler already registered: %s", identifier)
	}

	fc.commandHandlers.Store(identifier, ch)
	fmt.Printf("  [Facade] Registered command handler: %s (commands: %v)\n", identifier, ch.GetCommands())
	return nil
}

// RegisterQueryHandler registers a query handler.
func RegisterQueryHandler(fc *Facade, qh QueryHandler) error {
	if qh.GetDomain() == "" {
		return fmt.Errorf("query handler has no Domain set")
	}

	identifier := qh.GetIdentifier()
	fc.queryHandlers.Store(identifier, qh)
	fmt.Printf("  [Facade] Registered query handler: %s\n", identifier)
	return nil
}

// RegisterEventHandler registers an event handler (readmodel or reactor).
func RegisterEventHandler(fc *Facade, eh EventHandler) error {
	if eh.GetDomain() == "" {
		return fmt.Errorf("event handler has no Domain set")
	}

	identifier := eh.GetIdentifier()
	if _, exists := fc.eventHandlers.Load(identifier); exists {
		return fmt.Errorf("event handler already registered: %s", identifier)
	}

	fc.eventHandlers.Store(identifier, eh)

	// Subscribe to events
	for _, eventType := range eh.GetEvents() {
		fc.eventSubscribers[eventType] = append(fc.eventSubscribers[eventType], eh)
	}

	fmt.Printf("  [Facade] Registered event handler: %s (subscribes to: %v)\n", identifier, eh.GetEvents())
	return nil
}

// ExecuteCommand executes a command through the middleware pipeline.
func (fc *Facade) ExecuteCommand(ctx context.Context, cmd Command) ([]Event, error) {
	// Find the command handler
	var handler CommandHandler
	fc.commandHandlers.Range(func(key, value interface{}) bool {
		ch := value.(CommandHandler)
		if ch.GetDomain() == cmd.Domain {
			for _, cmdType := range ch.GetCommands() {
				if cmdType == cmd.CommandType {
					handler = ch
					return false
				}
			}
		}
		return true
	})

	if handler == nil {
		return nil, fmt.Errorf("no handler found for command: %s.%s", cmd.Domain, cmd.CommandType)
	}

	// Create the handler function
	handleFunc := func(ctx context.Context, cmd Command) ([]Event, error) {
		return handler.Handle(ctx, cmd)
	}

	// Apply middlewares
	wrapped := WithCommandMiddlewares(ctx, handleFunc, fc.commandMiddlewares...)

	// Execute
	events, err := wrapped(ctx, cmd)
	if err != nil {
		return nil, err
	}

	// Store events
	fc.mu.Lock()
	fc.events = append(fc.events, events...)
	fc.mu.Unlock()

	// Dispatch events to handlers
	for _, evt := range events {
		if err := fc.dispatchEvent(ctx, evt); err != nil {
			fmt.Printf("  [Warning] Event dispatch error: %v\n", err)
		}
	}

	return events, nil
}

// dispatchEvent dispatches an event to all subscribed handlers.
func (fc *Facade) dispatchEvent(ctx context.Context, evt Event) error {
	eventKey := fmt.Sprintf("%s.%s", evt.Domain, evt.EventType)
	handlers := fc.eventSubscribers[eventKey]

	for _, handler := range handlers {
		handleFunc := func(ctx context.Context, evt Event) error {
			return handler.Handle(ctx, evt)
		}

		// Apply event middlewares
		wrapped := WithEventMiddlewares(ctx, handleFunc, fc.eventMiddlewares...)

		if err := wrapped(ctx, evt); err != nil {
			return fmt.Errorf("handler %s failed: %w", handler.GetIdentifier(), err)
		}
	}

	return nil
}

// RestoreState replays all events to rebuild read models.
func (fc *Facade) RestoreState(ctx context.Context) error {
	fc.mu.RLock()
	events := make([]Event, len(fc.events))
	copy(events, fc.events)
	fc.mu.RUnlock()

	fmt.Printf("\n[Facade] Restoring state from %d events...\n", len(events))

	for _, evt := range events {
		eventKey := fmt.Sprintf("%s.%s", evt.Domain, evt.EventType)
		handlers := fc.eventSubscribers[eventKey]

		for _, handler := range handlers {
			if err := handler.Handle(ctx, evt); err != nil {
				return fmt.Errorf("restore failed for handler %s: %w", handler.GetIdentifier(), err)
			}
		}
	}

	fmt.Println("[Facade] State restoration complete")
	return nil
}

// GetEvents returns all stored events (for demo purposes).
func (fc *Facade) GetEvents() []Event {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	result := make([]Event, len(fc.events))
	copy(result, fc.events)
	return result
}

// GetRegisteredDomains returns all registered domain names.
func (fc *Facade) GetRegisteredDomains() []string {
	domains := make(map[string]bool)

	fc.aggregates.Range(func(key, value interface{}) bool {
		agg := value.(Aggregate)
		domains[agg.GetDomain()] = true
		return true
	})

	result := make([]string, 0, len(domains))
	for d := range domains {
		result = append(result, d)
	}
	return result
}

// GetCommandHandlerCount returns the number of registered command handlers.
func (fc *Facade) GetCommandHandlerCount() int {
	count := 0
	fc.commandHandlers.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// GetEventHandlerCount returns the number of registered event handlers.
func (fc *Facade) GetEventHandlerCount() int {
	count := 0
	fc.eventHandlers.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}
