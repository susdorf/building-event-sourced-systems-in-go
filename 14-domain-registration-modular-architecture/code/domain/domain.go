package domain

import (
	"context"
	"fmt"

	"domain-registration-demo/domain/inventory"
	"domain-registration-demo/domain/order"
	"domain-registration-demo/pkg"
)

// RegisterDomains registers all application domains with the facade.
// This is the central entry point for domain registration.
func RegisterDomains(ctx context.Context, fc *pkg.Facade) error {
	fmt.Println("\n========================================")
	fmt.Println("Starting Domain Registration")
	fmt.Println("========================================")

	// Register domains in dependency order.
	// Order domain produces events that Inventory domain consumes,
	// so Order should be registered first.

	// 1. Order domain (produces OrderPlaced, OrderCancelled events)
	if err := order.Register(ctx, fc); err != nil {
		return fmt.Errorf("order domain: %w", err)
	}

	// 2. Inventory domain (consumes Order events)
	if err := inventory.Register(ctx, fc); err != nil {
		return fmt.Errorf("inventory domain: %w", err)
	}

	fmt.Println("\n========================================")
	fmt.Println("Domain Registration Complete")
	fmt.Printf("  Registered domains: %v\n", fc.GetRegisteredDomains())
	fmt.Printf("  Command handlers: %d\n", fc.GetCommandHandlerCount())
	fmt.Printf("  Event handlers: %d\n", fc.GetEventHandlerCount())
	fmt.Println("========================================")

	return nil
}
