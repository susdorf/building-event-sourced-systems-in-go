package pkg

import (
	"context"
	"fmt"
	"time"
)

// CommandHandlerFunc is the function signature for command handlers.
type CommandHandlerFunc func(ctx context.Context, cmd Command) ([]Event, error)

// CommandMiddlewareFunc wraps a command handler with additional behavior.
type CommandMiddlewareFunc func(ctx context.Context, next CommandHandlerFunc) CommandHandlerFunc

// WithCommandMiddlewares applies middlewares to a command handler.
func WithCommandMiddlewares(ctx context.Context, fn CommandHandlerFunc, middlewares ...CommandMiddlewareFunc) CommandHandlerFunc {
	if len(middlewares) == 0 {
		return fn
	}
	wrapped := fn
	// Apply in reverse order so first middleware executes first
	for i := len(middlewares) - 1; i >= 0; i-- {
		wrapped = middlewares[i](ctx, wrapped)
	}
	return wrapped
}

// EventHandlerFunc is the function signature for event handlers.
type EventHandlerFunc func(ctx context.Context, evt Event) error

// EventMiddlewareFunc wraps an event handler with additional behavior.
type EventMiddlewareFunc func(ctx context.Context, next EventHandlerFunc) EventHandlerFunc

// WithEventMiddlewares applies middlewares to an event handler.
func WithEventMiddlewares(ctx context.Context, fn EventHandlerFunc, middlewares ...EventMiddlewareFunc) EventHandlerFunc {
	if len(middlewares) == 0 {
		return fn
	}
	wrapped := fn
	for i := len(middlewares) - 1; i >= 0; i-- {
		wrapped = middlewares[i](ctx, wrapped)
	}
	return wrapped
}

// --- Example Middlewares ---

// LoggingCommandMiddleware logs command execution.
func LoggingCommandMiddleware() CommandMiddlewareFunc {
	return func(ctx context.Context, next CommandHandlerFunc) CommandHandlerFunc {
		return func(ctx context.Context, cmd Command) ([]Event, error) {
			start := time.Now()
			fmt.Printf("    [Middleware:Logging] Executing command: %s.%s\n", cmd.Domain, cmd.CommandType)

			events, err := next(ctx, cmd)

			duration := time.Since(start)
			if err != nil {
				fmt.Printf("    [Middleware:Logging] Command failed after %v: %v\n", duration, err)
			} else {
				fmt.Printf("    [Middleware:Logging] Command succeeded in %v, produced %d events\n", duration, len(events))
			}

			return events, err
		}
	}
}

// AuthorizationCommandMiddleware checks authorization before command execution.
func AuthorizationCommandMiddleware(authorizedTenants map[string]bool) CommandMiddlewareFunc {
	return func(ctx context.Context, next CommandHandlerFunc) CommandHandlerFunc {
		return func(ctx context.Context, cmd Command) ([]Event, error) {
			// Check if tenant is authorized
			if !authorizedTenants[cmd.TenantUuid] {
				fmt.Printf("    [Middleware:Auth] DENIED - Tenant %s not authorized\n", cmd.TenantUuid)
				return nil, fmt.Errorf("unauthorized: tenant %s", cmd.TenantUuid)
			}

			fmt.Printf("    [Middleware:Auth] ALLOWED - Tenant %s authorized\n", cmd.TenantUuid)
			return next(ctx, cmd)
		}
	}
}

// MetricsCommandMiddleware collects metrics about command execution.
type CommandMetrics struct {
	TotalCommands    int
	SuccessfulCount  int
	FailedCount      int
	TotalEventsCount int
}

func MetricsCommandMiddleware(metrics *CommandMetrics) CommandMiddlewareFunc {
	return func(ctx context.Context, next CommandHandlerFunc) CommandHandlerFunc {
		return func(ctx context.Context, cmd Command) ([]Event, error) {
			metrics.TotalCommands++

			events, err := next(ctx, cmd)

			if err != nil {
				metrics.FailedCount++
			} else {
				metrics.SuccessfulCount++
				metrics.TotalEventsCount += len(events)
			}

			return events, err
		}
	}
}

// LoggingEventMiddleware logs event handling.
func LoggingEventMiddleware() EventMiddlewareFunc {
	return func(ctx context.Context, next EventHandlerFunc) EventHandlerFunc {
		return func(ctx context.Context, evt Event) error {
			fmt.Printf("    [Middleware:EventLog] Processing event: %s.%s (aggregate: %s)\n",
				evt.Domain, evt.EventType, evt.AggregateUuid)

			err := next(ctx, evt)

			if err != nil {
				fmt.Printf("    [Middleware:EventLog] Event handler failed: %v\n", err)
			}

			return err
		}
	}
}

// WebhookMiddleware sends webhooks after events are processed.
type WebhookConfig struct {
	URL        string
	EventTypes []string
}

func WebhookEventMiddleware(config WebhookConfig) EventMiddlewareFunc {
	return func(ctx context.Context, next EventHandlerFunc) EventHandlerFunc {
		return func(ctx context.Context, evt Event) error {
			// First, let the handler process the event
			err := next(ctx, evt)
			if err != nil {
				return err
			}

			// Post-processing: check if we should send a webhook
			eventKey := fmt.Sprintf("%s.%s", evt.Domain, evt.EventType)
			for _, subscribedEvent := range config.EventTypes {
				if eventKey == subscribedEvent {
					fmt.Printf("    [Middleware:Webhook] Would POST to %s for event %s\n",
						config.URL, eventKey)
					// In real implementation: go postWebhook(config.URL, evt)
					break
				}
			}

			return nil
		}
	}
}
