package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"specularium/internal/adapter"
	"specularium/internal/domain"
	"specularium/internal/handler"
	"specularium/internal/hub"
	"specularium/internal/repository/sqlite"
	"specularium/internal/service"
)

//go:embed web/*
var webFS embed.FS

func main() {
	// Command line flags
	addr := flag.String("addr", ":3000", "HTTP listen address")
	dbPath := flag.String("db", "./specularium.db", "SQLite database path")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting Specularium server...")

	// Initialize SQLite repository
	repo, err := sqlite.New(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer repo.Close()
	log.Printf("Database opened: %s", *dbPath)

	// Initialize event bus
	eventBus := service.NewEventBus()

	// Initialize SSE hub
	sseHub := hub.New()
	go sseHub.Run()

	// Connect event bus to SSE hub
	eventChan := make(chan service.Event, 100)
	eventBus.Subscribe(eventChan)
	go func() {
		for event := range eventChan {
			sseHub.Broadcast(event)
		}
	}()

	// Initialize services
	graphSvc := service.NewGraphService(repo, eventBus)
	truthSvc := service.NewTruthService(repo, eventBus)

	// Initialize adapter registry with reconcile function
	adapterRegistry := adapter.NewRegistry(func(ctx context.Context, source string, fragment *domain.GraphFragment) error {
		// Reconcile verification results - update node status/discovered fields
		// Only emit events for nodes that actually changed to avoid physics reset
		changedCount := 0
		for _, node := range fragment.Nodes {
			// Get existing node to compare
			existing, _ := repo.GetNode(ctx, node.ID)
			if existing == nil {
				// Node doesn't exist (shouldn't happen for verifier, but handle it)
				log.Printf("Node %s not found during verification reconcile", node.ID)
				continue
			}

			// Check if verification data actually changed
			statusChanged := existing.Status != node.Status
			discoveredChanged := !discoveredEqual(existing.Discovered, node.Discovered)

			if !statusChanged && !discoveredChanged {
				// No changes, skip update and event
				continue
			}

			// Update verification status
			if err := repo.UpdateNodeVerification(ctx, node.ID, node.Status, node.LastVerified, node.LastSeen, node.Discovered); err != nil {
				log.Printf("Failed to update verification for %s: %v", node.ID, err)
				continue
			}

			// Check for discrepancies against operator truth
			// This compares discovered values with truth assertions
			discrepancies, err := truthSvc.CheckDiscrepancies(ctx, node.ID, node.Discovered, source)
			if err != nil {
				log.Printf("Failed to check discrepancies for %s: %v", node.ID, err)
			} else if len(discrepancies) > 0 {
				log.Printf("Node %s has %d new discrepancies with operator truth", node.ID, len(discrepancies))
			}

			// Auto-update label from hostname inference if no operator truth
			labelUpdated := false
			if inference := extractHostnameInference(node.Discovered); inference != nil && inference.Best != nil {
				// Check if operator has set hostname/label truth
				hasOperatorHostname, _ := repo.HasOperatorTruthHostname(ctx, node.ID)
				if !hasOperatorHostname {
					// Use best inferred hostname as label (short name)
					newLabel := domain.ExtractShortName(inference.Best.Hostname)
					if newLabel != "" && newLabel != existing.Label {
						if err := repo.UpdateNodeLabel(ctx, node.ID, newLabel); err != nil {
							log.Printf("Failed to update label for %s: %v", node.ID, err)
						} else {
							log.Printf("Auto-updated label for %s: %s -> %s (confidence: %.0f%%, source: %s)",
								node.ID, existing.Label, newLabel,
								inference.Best.Confidence*100, inference.Best.Source)
							labelUpdated = true
						}
					}
				}
			}

			// Fetch the updated node with all fields for the event payload
			updatedNode, err := repo.GetNode(ctx, node.ID)
			if err != nil {
				log.Printf("Failed to fetch updated node %s: %v", node.ID, err)
				continue
			}

			// Emit node-updated event with full node data for incremental UI update
			eventBus.Publish(service.Event{
				Type:    service.EventNodeUpdated,
				Payload: updatedNode,
			})
			changedCount++
			_ = labelUpdated // Used for logging above
		}

		if changedCount > 0 {
			log.Printf("Reconciled %d changed nodes from %s", changedCount, source)
		}
		// Don't emit graph-updated - individual node-updated events handle UI updates
		return nil
	})

	// Set up discovery event handler to broadcast to SSE
	adapterRegistry.SetDiscoveryEventHandler(func(eventType string, payload interface{}) {
		eventBus.Publish(service.Event{
			Type:    service.EventType(eventType),
			Payload: payload,
		})
	})

	// Register verifier adapter
	verifierConfig := adapter.DefaultVerifierConfig()
	verifierAdapter := adapter.NewVerifierAdapter(repo, verifierConfig)
	adapterRegistry.Register(verifierAdapter, adapter.AdapterConfig{
		Enabled:      true,
		Priority:     50,
		PollInterval: "30s", // Verify nodes every 30 seconds
	})

	// Create scanner adapter with service wrapper
	scannerConfig := adapter.DefaultScannerConfig()
	scannerAdapter := adapter.NewScannerAdapter(scannerConfig)

	// Create scanner service that saves discovered hosts
	scannerSvc := &scannerService{
		scanner:  scannerAdapter,
		repo:     repo,
		eventBus: eventBus,
	}
	// Connect scanner to event bus for progress updates
	scannerAdapter.SetEventPublisher(adapterRegistry)

	// Start adapter registry
	adapterCtx, adapterCancel := context.WithCancel(context.Background())
	if err := adapterRegistry.Start(adapterCtx); err != nil {
		log.Printf("Warning: Failed to start adapter registry: %v", err)
	}

	// Initialize HTTP handlers
	graphHandler := handler.NewGraphHandler(graphSvc)
	graphHandler.SetDiscoveryTrigger(adapterRegistry)
	graphHandler.SetSubnetScanner(scannerSvc)
	truthHandler := handler.NewTruthHandler(truthSvc)

	// Setup routes
	mux := http.NewServeMux()

	// Graph endpoint (complete graph with positions)
	mux.HandleFunc("GET /api/graph", graphHandler.GetGraph)
	mux.HandleFunc("DELETE /api/graph", graphHandler.ClearGraph)
	mux.HandleFunc("POST /api/discover", graphHandler.TriggerDiscovery)

	// Node endpoints
	mux.HandleFunc("GET /api/nodes", graphHandler.ListNodes)
	mux.HandleFunc("POST /api/nodes", graphHandler.CreateNode)
	mux.HandleFunc("GET /api/nodes/{id}", graphHandler.GetNode)
	mux.HandleFunc("PUT /api/nodes/{id}", graphHandler.UpdateNode)
	mux.HandleFunc("DELETE /api/nodes/{id}", graphHandler.DeleteNode)

	// Edge endpoints
	mux.HandleFunc("GET /api/edges", graphHandler.ListEdges)
	mux.HandleFunc("POST /api/edges", graphHandler.CreateEdge)
	mux.HandleFunc("GET /api/edges/{id}", graphHandler.GetEdge)
	mux.HandleFunc("PUT /api/edges/{id}", graphHandler.UpdateEdge)
	mux.HandleFunc("DELETE /api/edges/{id}", graphHandler.DeleteEdge)

	// Position endpoints
	mux.HandleFunc("GET /api/positions", graphHandler.GetPositions)
	mux.HandleFunc("POST /api/positions", graphHandler.SavePositions)
	mux.HandleFunc("PUT /api/positions/{node_id}", graphHandler.UpdatePosition)

	// Import endpoints
	mux.HandleFunc("POST /api/import/yaml", graphHandler.ImportYAML)
	mux.HandleFunc("POST /api/import/ansible-inventory", graphHandler.ImportAnsibleInventory)
	mux.HandleFunc("POST /api/import/scan", graphHandler.ImportScan)

	// Export endpoints
	mux.HandleFunc("GET /api/export/json", graphHandler.ExportJSON)
	mux.HandleFunc("GET /api/export/yaml", graphHandler.ExportYAML)
	mux.HandleFunc("GET /api/export/ansible-inventory", graphHandler.ExportAnsibleInventory)

	// Truth endpoints
	mux.HandleFunc("GET /api/nodes/{id}/truth", truthHandler.GetNodeTruth)
	mux.HandleFunc("PUT /api/nodes/{id}/truth", truthHandler.SetNodeTruth)
	mux.HandleFunc("DELETE /api/nodes/{id}/truth", truthHandler.ClearNodeTruth)
	mux.HandleFunc("GET /api/nodes/{id}/discrepancies", truthHandler.GetNodeDiscrepancies)

	// Discrepancy endpoints
	mux.HandleFunc("GET /api/discrepancies", truthHandler.ListDiscrepancies)
	mux.HandleFunc("GET /api/discrepancies/{id}", truthHandler.GetDiscrepancy)
	mux.HandleFunc("POST /api/discrepancies/{id}/resolve", truthHandler.ResolveDiscrepancy)

	// SSE events endpoint
	mux.Handle("GET /events", sseHub)

	// Static files from embedded filesystem
	webContent, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("Failed to get embedded web content: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(webContent)))

	// Apply middleware
	finalHandler := handler.Chain(mux,
		handler.Recover,
		handler.CORS,
		handler.Logger,
	)

	// Create server
	server := &http.Server{
		Addr:         *addr,
		Handler:      finalHandler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server listening on %s", *addr)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Stop adapter registry
	adapterCancel()
	if err := adapterRegistry.Stop(); err != nil {
		log.Printf("Adapter registry shutdown error: %v", err)
	}

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}

// scannerService wraps the scanner adapter and saves discovered hosts
type scannerService struct {
	scanner  *adapter.ScannerAdapter
	repo     *sqlite.Repository
	eventBus *service.EventBus
}

// ScanSubnet scans a CIDR range and saves discovered hosts
func (s *scannerService) ScanSubnet(ctx context.Context, cidr string) error {
	log.Printf("scannerService: Starting scan of %s", cidr)
	fragment, err := s.scanner.ScanSubnet(ctx, cidr)
	if err != nil {
		log.Printf("scannerService: Scan error: %v", err)
		return err
	}

	if fragment == nil {
		log.Printf("scannerService: Scan returned nil fragment")
		return nil
	}

	log.Printf("scannerService: Received fragment with %d nodes", len(fragment.Nodes))

	// Save discovered nodes to repository
	created := 0
	updated := 0
	for _, node := range fragment.Nodes {
		// Check if node already exists
		existing, _ := s.repo.GetNode(ctx, node.ID)
		if existing != nil {
			// Update existing node with discovered data
			if err := s.repo.UpdateNodeVerification(ctx, node.ID, node.Status, node.LastVerified, node.LastSeen, node.Discovered); err != nil {
				log.Printf("Failed to update discovered node %s: %v", node.ID, err)
			} else {
				updated++
			}
		} else {
			// Create new node
			if err := s.repo.CreateNode(ctx, &node); err != nil {
				log.Printf("Failed to create discovered node %s: %v", node.ID, err)
			} else {
				created++
			}
		}
	}

	log.Printf("scannerService: Created %d nodes, updated %d nodes", created, updated)

	// Broadcast graph update
	s.eventBus.Publish(service.Event{
		Type:    service.EventGraphUpdated,
		Payload: map[string]int{"nodes_discovered": len(fragment.Nodes)},
	})

	return nil
}

// discoveredEqual compares two discovered maps for equality
// Returns true if both maps have the same keys and values
func discoveredEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok {
			return false
		}
		// Compare values - handle common types
		switch va := va.(type) {
		case int64:
			if vb, ok := vb.(int64); !ok || va != vb {
				return false
			}
		case float64:
			if vb, ok := vb.(float64); !ok || va != vb {
				return false
			}
		case string:
			if vb, ok := vb.(string); !ok || va != vb {
				return false
			}
		case bool:
			if vb, ok := vb.(bool); !ok || va != vb {
				return false
			}
		default:
			// For complex types (slices, maps), use fmt.Sprintf comparison
			if fmt.Sprintf("%v", va) != fmt.Sprintf("%v", vb) {
				return false
			}
		}
	}
	return true
}

// extractHostnameInference extracts HostnameInference from discovered map
func extractHostnameInference(discovered map[string]any) *domain.HostnameInference {
	if discovered == nil {
		return nil
	}
	raw, ok := discovered["hostname_inference"]
	if !ok {
		return nil
	}

	// Handle both direct struct and map[string]interface{} (from JSON)
	switch v := raw.(type) {
	case domain.HostnameInference:
		return &v
	case *domain.HostnameInference:
		return v
	case map[string]interface{}:
		// Reconstruct from map (when loaded from JSON/DB)
		inference := &domain.HostnameInference{}

		if candidates, ok := v["candidates"].([]interface{}); ok {
			for _, c := range candidates {
				if cm, ok := c.(map[string]interface{}); ok {
					candidate := domain.HostnameCandidate{
						Hostname:   getStringField(cm, "hostname"),
						Confidence: getFloatField(cm, "confidence"),
						Source:     domain.ConfidenceSource(getStringField(cm, "source")),
					}
					inference.Candidates = append(inference.Candidates, candidate)
				}
			}
		}

		if best, ok := v["best"].(map[string]interface{}); ok {
			inference.Best = &domain.HostnameCandidate{
				Hostname:   getStringField(best, "hostname"),
				Confidence: getFloatField(best, "confidence"),
				Source:     domain.ConfidenceSource(getStringField(best, "source")),
			}
		}

		return inference
	}
	return nil
}

// getStringField safely extracts a string field from a map
func getStringField(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// getFloatField safely extracts a float64 field from a map
func getFloatField(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}
