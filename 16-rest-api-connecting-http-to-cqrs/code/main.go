package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"rest-api-cqrs-demo/api"
	"rest-api-cqrs-demo/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

const baseURL = "http://localhost:8080"

func main() {
	fmt.Println("=== REST API + CQRS Demo ===")
	fmt.Println()

	// --- 1. Create the facade (domain layer) ---
	fmt.Println("1. Creating Facade (domain orchestrator)...")
	fc := domain.NewFacade()
	fmt.Println("   -> Facade created with in-memory storage")

	// --- 2. Create HTTP router and Huma API ---
	fmt.Println()
	fmt.Println("2. Setting up HTTP server with Huma...")
	router := chi.NewRouter()
	humaAPI := humachi.New(router, huma.DefaultConfig("Order API", "1.0.0"))
	fmt.Println("   -> Router: Chi")
	fmt.Println("   -> Framework: Huma v2")
	fmt.Println("   -> OpenAPI: Auto-generated at /docs")

	// --- 3. Register endpoints and middleware ---
	fmt.Println()
	fmt.Println("3. Registering middleware and endpoints...")
	api.RegisterEndpoints(fc, humaAPI)
	fmt.Println("   -> HTTP Middleware: AuthAnonymous -> AuthBearer -> AuthTarget")
	fmt.Println("   -> Domain Middleware: CommandLogging, QueryLogging")
	fmt.Println("   -> Endpoints registered:")
	fmt.Println("      POST   /api/tenants/{tenantUuid}/orders")
	fmt.Println("      GET    /api/tenants/{tenantUuid}/orders")
	fmt.Println("      GET    /api/tenants/{tenantUuid}/orders/{orderUuid}")
	fmt.Println("      PATCH  /api/tenants/{tenantUuid}/orders/{orderUuid}")
	fmt.Println("      POST   /api/tenants/{tenantUuid}/orders/{orderUuid}/ship")
	fmt.Println("      POST   /api/tenants/{tenantUuid}/orders/{orderUuid}/cancel")

	// --- 4. Start server in background ---
	fmt.Println()
	fmt.Println("4. Starting HTTP server on :8080...")
	go func() {
		if err := http.ListenAndServe(":8080", router); err != nil {
			log.Fatal(err)
		}
	}()
	time.Sleep(100 * time.Millisecond) // Wait for server to start
	fmt.Println("   -> Server running!")

	// --- 5. Run demo requests ---
	fmt.Println()
	fmt.Println("=== Running Demo Requests ===")
	fmt.Println()
	runDemoRequests()

	// --- 6. Show metrics ---
	fmt.Println()
	fmt.Println("=== Metrics ===")
	commands, queries := fc.GetMetrics()
	fmt.Printf("Commands executed: %d\n", commands)
	fmt.Printf("Queries executed: %d\n", queries)

	fmt.Println()
	fmt.Println("=== Demo Complete ===")
	fmt.Println()
	fmt.Println("The server is still running. You can test with curl:")
	fmt.Println()
	fmt.Println("  # Create an order")
	fmt.Println(`  curl -X POST http://localhost:8080/api/tenants/tenant-demo/orders \`)
	fmt.Println(`    -H "Content-Type: application/json" \`)
	fmt.Println(`    -H "Authorization: Bearer user:alice" \`)
	fmt.Println(`    -d '{"orderUuid":"my-order","customerName":"Bob","items":[{"name":"Widget","quantity":1,"price":9.99}]}'`)
	fmt.Println()
	fmt.Println("  # List orders")
	fmt.Println(`  curl http://localhost:8080/api/tenants/tenant-demo/orders`)
	fmt.Println()
	fmt.Println("  # View OpenAPI docs")
	fmt.Println(`  curl http://localhost:8080/docs`)
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop the server.")

	// Keep server running
	select {}
}

func runDemoRequests() {
	tenantUuid := "tenant-A"
	orderUuid := "order-001"

	// --- Demo 1: Create an order ---
	fmt.Println("--- Demo 1: Create Order (POST) ---")
	createReq := map[string]any{
		"orderUuid":    orderUuid,
		"customerName": "Alice Smith",
		"items": []map[string]any{
			{"name": "Laptop", "quantity": 1, "price": 999.99},
			{"name": "Mouse", "quantity": 2, "price": 29.99},
		},
	}
	resp := doRequest("POST", fmt.Sprintf("/api/tenants/%s/orders", tenantUuid), createReq, "user:alice")
	printResponse("Created order", resp)

	// --- Demo 2: Get the order ---
	fmt.Println()
	fmt.Println("--- Demo 2: Get Order (GET) ---")
	resp = doRequest("GET", fmt.Sprintf("/api/tenants/%s/orders/%s", tenantUuid, orderUuid), nil, "user:alice")
	printResponse("Retrieved order", resp)

	// --- Demo 3: List orders ---
	fmt.Println()
	fmt.Println("--- Demo 3: List Orders (GET with pagination) ---")
	resp = doRequest("GET", fmt.Sprintf("/api/tenants/%s/orders?page=1&pageSize=10", tenantUuid), nil, "user:alice")
	printResponse("Order list", resp)

	// --- Demo 4: Update order ---
	fmt.Println()
	fmt.Println("--- Demo 4: Update Order (PATCH) ---")
	updateReq := map[string]any{
		"customerName": "Alice Smith-Jones",
		"items": []map[string]any{
			{"name": "Laptop Pro", "quantity": 1, "price": 1299.99},
			{"name": "Mouse", "quantity": 2, "price": 29.99},
			{"name": "Keyboard", "quantity": 1, "price": 79.99},
		},
	}
	resp = doRequest("PATCH", fmt.Sprintf("/api/tenants/%s/orders/%s", tenantUuid, orderUuid), updateReq, "user:alice")
	printResponse("Updated order", resp)

	// --- Demo 5: Ship order (custom action) ---
	fmt.Println()
	fmt.Println("--- Demo 5: Ship Order (POST custom action) ---")
	resp = doRequest("POST", fmt.Sprintf("/api/tenants/%s/orders/%s/ship", tenantUuid, orderUuid), nil, "user:warehouse")
	printResponse("Shipped order", resp)

	// --- Demo 6: Try to cancel shipped order (should fail) ---
	fmt.Println()
	fmt.Println("--- Demo 6: Cancel Shipped Order (should fail) ---")
	resp = doRequest("POST", fmt.Sprintf("/api/tenants/%s/orders/%s/cancel", tenantUuid, orderUuid), nil, "user:alice")
	printResponse("Cancel attempt", resp)

	// --- Demo 7: Create and cancel another order ---
	fmt.Println()
	fmt.Println("--- Demo 7: Create and Cancel Order ---")
	order2Uuid := "order-002"
	createReq2 := map[string]any{
		"orderUuid":    order2Uuid,
		"customerName": "Bob Wilson",
		"items": []map[string]any{
			{"name": "Book", "quantity": 3, "price": 19.99},
		},
	}
	resp = doRequest("POST", fmt.Sprintf("/api/tenants/%s/orders", tenantUuid), createReq2, "user:bob")
	printResponse("Created order-002", resp)

	resp = doRequest("POST", fmt.Sprintf("/api/tenants/%s/orders/%s/cancel", tenantUuid, order2Uuid), nil, "user:bob")
	printResponse("Cancelled order-002", resp)

	// --- Demo 8: List orders with status filter ---
	fmt.Println()
	fmt.Println("--- Demo 8: List Orders by Status ---")
	resp = doRequest("GET", fmt.Sprintf("/api/tenants/%s/orders?status=shipped", tenantUuid), nil, "")
	printResponse("Shipped orders", resp)

	resp = doRequest("GET", fmt.Sprintf("/api/tenants/%s/orders?status=cancelled", tenantUuid), nil, "")
	printResponse("Cancelled orders", resp)

	// --- Demo 9: Multi-tenant isolation ---
	fmt.Println()
	fmt.Println("--- Demo 9: Multi-Tenant Isolation ---")
	tenantB := "tenant-B"
	createReqB := map[string]any{
		"orderUuid":    "order-B-001",
		"customerName": "Charlie Brown",
		"items": []map[string]any{
			{"name": "Coffee", "quantity": 10, "price": 12.99},
		},
	}
	resp = doRequest("POST", fmt.Sprintf("/api/tenants/%s/orders", tenantB), createReqB, "user:charlie")
	printResponse("Created order in tenant-B", resp)

	resp = doRequest("GET", fmt.Sprintf("/api/tenants/%s/orders", tenantUuid), nil, "")
	printResponse("Orders in tenant-A (should not include tenant-B orders)", resp)

	resp = doRequest("GET", fmt.Sprintf("/api/tenants/%s/orders", tenantB), nil, "")
	printResponse("Orders in tenant-B", resp)

	// --- Demo 10: Error handling ---
	fmt.Println()
	fmt.Println("--- Demo 10: Error Handling ---")

	// Not found
	resp = doRequest("GET", fmt.Sprintf("/api/tenants/%s/orders/nonexistent", tenantUuid), nil, "")
	printResponse("Get non-existent order (404)", resp)

	// Conflict - duplicate order
	resp = doRequest("POST", fmt.Sprintf("/api/tenants/%s/orders", tenantUuid), createReq, "user:alice")
	printResponse("Create duplicate order (409)", resp)
}

func doRequest(method, path string, body any, authToken string) map[string]any {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, _ := http.NewRequest(method, baseURL+path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	result := map[string]any{
		"status": resp.StatusCode,
	}

	if len(respBody) > 0 {
		var data any
		if err := json.Unmarshal(respBody, &data); err == nil {
			result["body"] = data
		} else {
			result["body"] = string(respBody)
		}
	}

	return result
}

func printResponse(label string, resp map[string]any) {
	status := resp["status"]
	fmt.Printf("   %s: HTTP %v\n", label, status)

	if body, ok := resp["body"]; ok {
		// Pretty print a subset of the response
		switch v := body.(type) {
		case map[string]any:
			if orderUuid, ok := v["orderUuid"]; ok {
				fmt.Printf("   -> Order: %v, Status: %v, Total: %v\n",
					orderUuid, v["status"], v["total"])
			} else if data, ok := v["data"].([]any); ok {
				fmt.Printf("   -> Found %d items (total: %v)\n", len(data), v["total"])
			} else if title, ok := v["title"]; ok {
				// Error response
				fmt.Printf("   -> Error: %v - %v\n", title, v["detail"])
			}
		}
	}
}
