package pkg

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

// EventHandlerFunc is the function signature for event handlers
type EventHandlerFunc func(ctx context.Context, evt Event) error

// EventHandlerMiddleware wraps an event handler with additional behavior
type EventHandlerMiddleware func(next EventHandlerFunc) EventHandlerFunc

// WithLogging creates a middleware that logs event processing
func WithLogging() EventHandlerMiddleware {
	return func(next EventHandlerFunc) EventHandlerFunc {
		return func(ctx context.Context, evt Event) error {
			fmt.Printf("      [LOG] Processing event: %s (aggregate: %s)\n",
				evt.DataType, evt.AggregateUUID)

			err := next(ctx, evt)

			if err != nil {
				fmt.Printf("      [LOG] Event processing failed: %s - %v\n",
					evt.DataType, err)
			} else {
				fmt.Printf("      [LOG] Event processed successfully: %s\n",
					evt.DataType)
			}

			return err
		}
	}
}

// ProjectionMetrics tracks projection performance
type ProjectionMetrics struct {
	EventsProcessed int64
	EventsFailed    int64
	TotalDuration   int64 // nanoseconds
}

func (m *ProjectionMetrics) RecordEvent(duration time.Duration, err error) {
	atomic.AddInt64(&m.EventsProcessed, 1)
	atomic.AddInt64(&m.TotalDuration, int64(duration))
	if err != nil {
		atomic.AddInt64(&m.EventsFailed, 1)
	}
}

func (m *ProjectionMetrics) GetStats() (processed, failed int64, avgDuration time.Duration) {
	processed = atomic.LoadInt64(&m.EventsProcessed)
	failed = atomic.LoadInt64(&m.EventsFailed)
	total := atomic.LoadInt64(&m.TotalDuration)
	if processed > 0 {
		avgDuration = time.Duration(total / processed)
	}
	return
}

// WithMetrics creates a middleware that tracks event processing metrics
func WithMetrics(metrics *ProjectionMetrics) EventHandlerMiddleware {
	return func(next EventHandlerFunc) EventHandlerFunc {
		return func(ctx context.Context, evt Event) error {
			start := time.Now()

			err := next(ctx, evt)

			metrics.RecordEvent(time.Since(start), err)

			return err
		}
	}
}

// WithRecovery creates a middleware that recovers from panics
func WithRecovery() EventHandlerMiddleware {
	return func(next EventHandlerFunc) EventHandlerFunc {
		return func(ctx context.Context, evt Event) (err error) {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("      [RECOVERY] Panic recovered in event handler: %v\n", r)
					err = fmt.Errorf("panic recovered: %v", r)
				}
			}()

			return next(ctx, evt)
		}
	}
}

// WithEventTypeFilter creates a middleware that only processes specific event types
func WithEventTypeFilter(allowedTypes ...string) EventHandlerMiddleware {
	typeSet := make(map[string]bool)
	for _, t := range allowedTypes {
		typeSet[t] = true
	}

	return func(next EventHandlerFunc) EventHandlerFunc {
		return func(ctx context.Context, evt Event) error {
			if !typeSet[evt.DataType] {
				fmt.Printf("      [FILTER] Skipping event: %s (not in allowed types)\n", evt.DataType)
				return nil
			}
			return next(ctx, evt)
		}
	}
}

// ChainMiddleware chains multiple middleware together
func ChainMiddleware(middlewares ...EventHandlerMiddleware) EventHandlerMiddleware {
	return func(next EventHandlerFunc) EventHandlerFunc {
		// Apply in reverse order so the first middleware is the outermost
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}
