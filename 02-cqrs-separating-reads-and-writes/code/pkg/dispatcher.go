package pkg

import (
	"context"
	"fmt"
	"reflect"
)

// --- Command Dispatcher ---

// CommandHandlerFunc is a typed handler for a specific command
type CommandHandlerFunc[T any] func(ctx context.Context, cmd T) error

// CommandDispatcher routes commands to their registered handlers
type CommandDispatcher struct {
	handlers map[reflect.Type]any
}

func NewCommandDispatcher() *CommandDispatcher {
	return &CommandDispatcher{
		handlers: make(map[reflect.Type]any),
	}
}

// RegisterCommand adds a typed handler for a specific command type
func RegisterCommand[T any](d *CommandDispatcher, handler CommandHandlerFunc[T]) {
	var zero T
	d.handlers[reflect.TypeOf(zero)] = handler
}

// Dispatch routes a command to its registered handler
func (d *CommandDispatcher) Dispatch(ctx context.Context, cmd any) error {
	handler, ok := d.handlers[reflect.TypeOf(cmd)]
	if !ok {
		return fmt.Errorf("no handler for command: %T", cmd)
	}

	// Use reflection to invoke the typed handler
	handlerValue := reflect.ValueOf(handler)
	ctxValue := reflect.ValueOf(ctx)
	cmdValue := reflect.ValueOf(cmd)

	results := handlerValue.Call([]reflect.Value{ctxValue, cmdValue})

	if !results[0].IsNil() {
		return results[0].Interface().(error)
	}
	return nil
}

// --- Query Dispatcher ---

// QueryHandlerFunc is a typed handler returning a specific result type
type QueryHandlerFunc[Q any, R any] func(ctx context.Context, query Q) (R, error)

// QueryDispatcher routes queries to their registered handlers
type QueryDispatcher struct {
	handlers map[reflect.Type]any
}

func NewQueryDispatcher() *QueryDispatcher {
	return &QueryDispatcher{
		handlers: make(map[reflect.Type]any),
	}
}

// RegisterQuery adds a typed handler for a specific query type
func RegisterQuery[Q any, R any](d *QueryDispatcher, handler QueryHandlerFunc[Q, R]) {
	var zero Q
	d.handlers[reflect.TypeOf(zero)] = handler
}

// Query dispatches a query and returns the result
func Query[R any](d *QueryDispatcher, ctx context.Context, query any) (R, error) {
	var zero R
	handler, ok := d.handlers[reflect.TypeOf(query)]
	if !ok {
		return zero, fmt.Errorf("no handler for query: %T", query)
	}

	// Use reflection to invoke the typed handler
	handlerValue := reflect.ValueOf(handler)
	ctxValue := reflect.ValueOf(ctx)
	queryValue := reflect.ValueOf(query)

	results := handlerValue.Call([]reflect.Value{ctxValue, queryValue})

	result := results[0].Interface()
	errInterface := results[1].Interface()

	if errInterface != nil {
		return zero, errInterface.(error)
	}

	if result == nil {
		return zero, nil
	}

	return result.(R), nil
}

// --- Event Dispatcher ---

// EventHandlerFunc is a typed handler for a specific event
type EventHandlerFunc[T any] func(event T) error

// EventDispatcher routes events to registered handlers (supports multiple handlers per event)
type EventDispatcher struct {
	handlers map[reflect.Type][]any
}

func NewEventDispatcher() *EventDispatcher {
	return &EventDispatcher{
		handlers: make(map[reflect.Type][]any),
	}
}

// RegisterEventHandler adds a handler for a specific event type
func RegisterEventHandler[T any](d *EventDispatcher, handler EventHandlerFunc[T]) {
	var zero T
	eventType := reflect.TypeOf(zero)
	d.handlers[eventType] = append(d.handlers[eventType], handler)
}

// Dispatch sends an event to all registered handlers
func (d *EventDispatcher) Dispatch(event any) error {
	handlers, ok := d.handlers[reflect.TypeOf(event)]
	if !ok {
		return nil // No handlers for this event is valid
	}

	for _, handler := range handlers {
		handlerValue := reflect.ValueOf(handler)
		eventValue := reflect.ValueOf(event)

		results := handlerValue.Call([]reflect.Value{eventValue})

		if !results[0].IsNil() {
			return results[0].Interface().(error)
		}
	}
	return nil
}
