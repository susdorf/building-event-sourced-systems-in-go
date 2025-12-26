package inventory

import (
	"context"
	"fmt"

	"domain-registration-demo/domain/inventory/aggregate"
	"domain-registration-demo/domain/inventory/reactor"
	"domain-registration-demo/pkg"
)

// OrderEventsReactor is exported for access in demo.
var OrderEventsReactor *reactor.OrderEventsReactor

// Register registers all Inventory domain components with the facade.
func Register(ctx context.Context, fc *pkg.Facade) error {
	fmt.Println("\n[Inventory Domain] Registering components...")

	// 1. Register aggregate
	if err := pkg.RegisterAggregate(fc, aggregate.NewAggregate); err != nil {
		return fmt.Errorf("inventory aggregate: %w", err)
	}

	// 2. Register reactor for Order events (cross-domain event handling)
	OrderEventsReactor = reactor.NewOrderEventsReactor()
	if err := pkg.RegisterEventHandler(fc, OrderEventsReactor); err != nil {
		return fmt.Errorf("inventory order events reactor: %w", err)
	}

	fmt.Println("[Inventory Domain] Registration complete")
	return nil
}
