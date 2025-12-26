package order

import (
	"context"
	"fmt"

	"domain-registration-demo/domain/order/aggregate"
	"domain-registration-demo/domain/order/command"
	"domain-registration-demo/domain/order/readmodel"
	"domain-registration-demo/pkg"
)

// Readmodel is exported for cross-domain access (e.g., queries).
var Readmodel *readmodel.OrderReadmodel

// Register registers all Order domain components with the facade.
func Register(ctx context.Context, fc *pkg.Facade) error {
	fmt.Println("\n[Order Domain] Registering components...")

	// 1. Register aggregate
	if err := pkg.RegisterAggregate(fc, aggregate.NewAggregate); err != nil {
		return fmt.Errorf("order aggregate: %w", err)
	}

	// 2. Register command handler
	ch := command.NewCommandHandler()
	if err := pkg.RegisterCommandHandler(fc, ch); err != nil {
		return fmt.Errorf("order command handler: %w", err)
	}

	// 3. Register readmodel as event handler
	Readmodel = readmodel.NewOrderReadmodel()
	if err := pkg.RegisterEventHandler(fc, Readmodel); err != nil {
		return fmt.Errorf("order readmodel: %w", err)
	}

	fmt.Println("[Order Domain] Registration complete")
	return nil
}
