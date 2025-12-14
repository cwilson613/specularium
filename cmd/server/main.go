package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"specularium/internal/adapter"
	"specularium/internal/config"
	"specularium/internal/domain"
	"specularium/internal/handler"
	"specularium/internal/hub"
	"specularium/internal/repository/sqlite"
	"specularium/internal/service"
)

//go:embed web/*
var webFS embed.FS

func main() {
	// Command line flags (override config file settings)
	addrFlag := flag.String("addr", "", "HTTP listen address (overrides config)")
	dbPathFlag := flag.String("db", "", "SQLite database path (overrides config)")
	forceBootstrap := flag.Bool("bootstrap", false, "Force re-run bootstrap")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting Specularium server...")

	// Load configuration
	cfg, configPath, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if configPath != "" {
		log.Printf("Loaded config from: %s", configPath)
	} else {
		log.Println("No config file found, using defaults")
		configPath = config.DefaultConfigPath()
	}

	// Determine effective settings (flags override config)
	addr := cfg.Database.Path // placeholder, replaced below
	if *addrFlag != "" {
		addr = *addrFlag
	} else {
		addr = ":3000" // Default listen address
	}

	dbPath := cfg.Database.Path
	if *dbPathFlag != "" {
		dbPath = *dbPathFlag
	}

	// Get effective mode and behavior
	effectiveMode := cfg.EffectiveMode()
	behavior := cfg.EffectiveBehavior()

	// Log operational mode
	log.Printf("Mode: %s, Posture: %s", effectiveMode, cfg.Posture)
	log.Printf("Behavior: verify=%s, scan=%s, concurrency=%d",
		behavior.VerifyInterval, behavior.ScanInterval, behavior.MaxConcurrentProbes)

	// Warn if mode override exceeds recommendation
	if cfg.ModeExceedsRecommendation() {
		log.Printf("WARNING: Mode override (%s) exceeds bootstrap recommendation (%s)",
			*cfg.Mode, cfg.Bootstrap.Recommendation.Mode)
		log.Println("         Failures may occur on resource-constrained hardware")
	}

	// Log enabled capabilities
	enabledCaps := cfg.GetEnabledCapabilities()
	capNames := make([]string, len(enabledCaps))
	for i, c := range enabledCaps {
		capNames[i] = c.Name
	}
	log.Printf("Enabled capabilities: %s", strings.Join(capNames, ", "))

	// Check if bootstrap needed (handled later by bootstrap adapter)
	_ = forceBootstrap // Will be used when Phase 3 is implemented

	// Initialize SQLite repository
	repo, err := sqlite.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer repo.Close()
	log.Printf("Database opened: %s", dbPath)

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
	secretsSvc := service.NewSecretsService(repo, eventBus)

	// Load mounted secrets at startup
	if err := secretsSvc.LoadMountedSecrets(); err != nil {
		log.Printf("Warning: Failed to load mounted secrets: %v", err)
	}

	// Initialize capability manager for adapter access to secrets
	capabilityMgr := adapter.NewCapabilityManager(secretsSvc)

	// Initialize reconcile service for adapter discoveries
	reconcileSvc := service.NewReconcileService(repo, truthSvc, eventBus)

	// Initialize adapter registry with reconcile function
	adapterRegistry := adapter.NewRegistry(reconcileSvc.ReconcileFragment)

	// Set up discovery event handler to broadcast to SSE
	adapterRegistry.SetDiscoveryEventHandler(func(eventType string, payload interface{}) {
		eventBus.Publish(service.Event{
			Type:    service.EventType(eventType),
			Payload: payload,
		})
	})

	// Register verifier adapter (if basic_verification enabled and mode >= monitor)
	if cfg.Capabilities.IsEnabled("basic_verification", effectiveMode) {
		verifierConfig := adapter.DefaultVerifierConfig()
		verifierConfig.Capabilities = capabilityMgr
		verifierConfig.PingTimeout = behavior.ProbeTimeout
		verifierConfig.MaxConcurrent = behavior.MaxConcurrentProbes
		// Use custom DNS server for PTR lookups if configured
		if cfg.Secrets.DNSServer != nil {
			verifierConfig.DNSServer = *cfg.Secrets.DNSServer
		} else if dnsServer := os.Getenv("DNS_SERVER"); dnsServer != "" {
			verifierConfig.DNSServer = dnsServer
		}
		verifierAdapter := adapter.NewVerifierAdapter(repo, verifierConfig)
		adapterRegistry.Register(verifierAdapter, adapter.AdapterConfig{
			Enabled:      true,
			Priority:     50,
			PollInterval: behavior.VerifyInterval.String(),
		})
		log.Println("Verifier adapter enabled")
	}

	// Register SSH probe adapter (if enabled in config and mode >= discovery)
	if cfg.Capabilities.IsEnabled("ssh_probe", effectiveMode) || os.Getenv("ENABLE_SSH_PROBE") == "true" {
		sshProbeConfig := adapter.DefaultSSHProbeConfig()
		sshProbeAdapter := adapter.NewSSHProbeAdapter(secretsSvc, sshProbeConfig)
		sshProbeAdapter.SetEventPublisher(adapterRegistry)
		adapterRegistry.Register(sshProbeAdapter, adapter.AdapterConfig{
			Enabled:      true,
			Priority:     60,
			PollInterval: "10m",
		})
		log.Println("SSH probe adapter enabled")
	}

	// Register nmap adapter (if enabled in config and mode >= discovery)
	nmapEnabled := cfg.Capabilities.IsEnabled("nmap", effectiveMode)
	nmapTargets := cfg.Targets.Primary
	// Fall back to env var for backwards compatibility
	if len(nmapTargets) == 0 {
		if scanSubnets := os.Getenv("SCAN_SUBNETS"); scanSubnets != "" {
			nmapTargets = strings.Split(scanSubnets, ",")
		}
	}
	if nmapEnabled && len(nmapTargets) > 0 {
		nmapAdapter := adapter.NewNmapAdapter(
			nmapTargets,
			adapter.WithCommonPorts(),
			adapter.WithServiceDetection(true),
		)
		nmapAdapter.SetEventPublisher(adapterRegistry)
		adapterRegistry.Register(nmapAdapter, adapter.AdapterConfig{
			Enabled:      true,
			Priority:     80,
			PollInterval: behavior.ScanInterval.String(),
		})
		log.Printf("Nmap adapter registered for targets: %v", nmapTargets)
	} else if !nmapEnabled {
		log.Println("Nmap adapter: disabled in config or mode insufficient")
	} else {
		log.Println("Nmap adapter: no targets configured")
	}

	// Create scanner adapter with service wrapper and capabilities
	scannerConfig := adapter.DefaultScannerConfig()
	scannerConfig.Capabilities = capabilityMgr
	// Use custom DNS server for PTR lookups if configured (e.g., Technitium)
	if dnsServer := os.Getenv("DNS_SERVER"); dnsServer != "" {
		scannerConfig.DNSServer = dnsServer
		log.Printf("Scanner using custom DNS server for PTR lookups: %s", dnsServer)
	}
	scannerAdapter := adapter.NewScannerAdapter(scannerConfig)

	// Create scanner service that saves discovered hosts
	scannerSvc := &scannerService{
		scanner:  scannerAdapter,
		repo:     repo,
		eventBus: eventBus,
	}
	// Connect scanner to event bus for progress updates
	scannerAdapter.SetEventPublisher(adapterRegistry)

	// Create bootstrap adapter for self-discovery
	bootstrapAdapter := adapter.NewBootstrapAdapter()
	bootstrapAdapter.SetEventPublisher(adapterRegistry)

	// Start bootstrap adapter to detect environment
	if err := bootstrapAdapter.Start(context.Background()); err != nil {
		log.Printf("Warning: Bootstrap environment detection failed: %v", err)
	}

	// Create bootstrap service that saves discovered nodes
	bootstrapSvc := &bootstrapService{
		bootstrap: bootstrapAdapter,
		repo:      repo,
		eventBus:  eventBus,
	}

	// Run bootstrap to discover initial infrastructure (K8s, gateway, DNS, etc.)
	log.Printf("Running bootstrap to discover infrastructure...")
	if err := bootstrapSvc.Bootstrap(context.Background()); err != nil {
		log.Printf("Warning: Bootstrap discovery failed: %v", err)
	}

	// Persist bootstrap results to config if needed
	if cfg.NeedsBootstrap() || *forceBootstrap {
		env := bootstrapAdapter.GetEnvironment()
		bootstrapResult := config.BuildBootstrapResult(
			env.Hostname,
			env.InKubernetes,
			env.InDocker,
			env.DefaultGateway,
			env.DNSServers,
			env.LocalSubnet,
		)
		cfg.SetBootstrapResult(bootstrapResult)

		// Save updated config
		if configPath != "" {
			if err := cfg.Save(configPath); err != nil {
				log.Printf("Warning: Failed to save bootstrap results to config: %v", err)
			} else {
				log.Printf("Bootstrap results saved to: %s", configPath)
				log.Printf("Recommended mode: %s (confidence: %.0f%%)",
					bootstrapResult.Recommendation.Mode,
					bootstrapResult.Recommendation.Confidence*100)
			}
		}

		// Update effective mode now that we have bootstrap recommendation
		effectiveMode = cfg.EffectiveMode()
		log.Printf("Effective mode after bootstrap: %s", effectiveMode)
	}

	// Start adapter registry
	adapterCtx, adapterCancel := context.WithCancel(context.Background())
	if err := adapterRegistry.Start(adapterCtx); err != nil {
		log.Printf("Warning: Failed to start adapter registry: %v", err)
	}

	// Initialize HTTP handlers
	graphHandler := handler.NewGraphHandler(graphSvc)
	graphHandler.SetDiscoveryTrigger(adapterRegistry)
	graphHandler.SetSubnetScanner(scannerSvc)
	graphHandler.SetBootstrapper(bootstrapSvc)
	truthHandler := handler.NewTruthHandler(truthSvc)
	secretsHandler := handler.NewSecretsHandler(secretsSvc)
	secretsHandler.SetCapabilityChecker(capabilityMgr)

	// Setup routes
	mux := http.NewServeMux()

	// Graph endpoint (complete graph with positions)
	mux.HandleFunc("GET /api/graph", graphHandler.GetGraph)
	mux.HandleFunc("DELETE /api/graph", graphHandler.ClearGraph)
	mux.HandleFunc("POST /api/discover", graphHandler.TriggerDiscovery)

	// Bootstrap / environment endpoints
	mux.HandleFunc("POST /api/bootstrap", graphHandler.Bootstrap)
	mux.HandleFunc("GET /api/environment", graphHandler.GetEnvironment)
	mux.HandleFunc("POST /api/client", graphHandler.RegisterClient)

	// Node endpoints
	mux.HandleFunc("GET /api/nodes", graphHandler.ListNodes)
	mux.HandleFunc("POST /api/nodes", graphHandler.CreateNode)
	mux.HandleFunc("POST /api/nodes/merge", graphHandler.MergeNodes)
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

	// Secrets endpoints
	mux.HandleFunc("GET /api/secrets/types", secretsHandler.GetSecretTypes)
	mux.HandleFunc("POST /api/secrets/refresh", secretsHandler.RefreshMountedSecrets)
	mux.HandleFunc("GET /api/secrets", secretsHandler.ListSecrets)
	mux.HandleFunc("POST /api/secrets", secretsHandler.CreateSecret)
	mux.HandleFunc("GET /api/secrets/{id}", secretsHandler.GetSecret)
	mux.HandleFunc("PUT /api/secrets/{id}", secretsHandler.UpdateSecret)
	mux.HandleFunc("DELETE /api/secrets/{id}", secretsHandler.DeleteSecret)

	// Capabilities endpoint
	mux.HandleFunc("GET /api/capabilities", secretsHandler.GetCapabilities)

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
		Addr:         addr,
		Handler:      finalHandler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server listening on %s", addr)
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

// bootstrapService wraps the bootstrap adapter and saves discovered nodes
type bootstrapService struct {
	bootstrap *adapter.BootstrapAdapter
	repo      *sqlite.Repository
	eventBus  *service.EventBus
}

// Bootstrap performs self-discovery and saves nodes
func (b *bootstrapService) Bootstrap(ctx context.Context) error {
	log.Printf("bootstrapService: Starting self-discovery")
	fragment, err := b.bootstrap.Bootstrap(ctx)
	if err != nil {
		log.Printf("bootstrapService: Bootstrap error: %v", err)
		return err
	}

	if fragment == nil {
		log.Printf("bootstrapService: Bootstrap returned nil fragment")
		return nil
	}

	log.Printf("bootstrapService: Received fragment with %d nodes, %d edges",
		len(fragment.Nodes), len(fragment.Edges))

	// Save discovered nodes to repository
	created := 0
	updated := 0
	for _, node := range fragment.Nodes {
		// Check if node already exists
		existing, _ := b.repo.GetNode(ctx, node.ID)
		if existing != nil {
			// Update existing node with discovered data
			if err := b.repo.UpdateNodeVerification(ctx, node.ID, node.Status, node.LastVerified, node.LastSeen, node.Discovered); err != nil {
				log.Printf("Failed to update bootstrap node %s: %v", node.ID, err)
			} else {
				updated++
			}
		} else {
			// Create new node
			if err := b.repo.CreateNode(ctx, &node); err != nil {
				log.Printf("Failed to create bootstrap node %s: %v", node.ID, err)
			} else {
				created++
			}
		}
	}

	// Save edges
	edgesCreated := 0
	for _, edge := range fragment.Edges {
		if err := b.repo.CreateEdge(ctx, &edge); err != nil {
			log.Printf("Failed to create bootstrap edge %s: %v", edge.ID, err)
		} else {
			edgesCreated++
		}
	}

	log.Printf("bootstrapService: Created %d nodes, updated %d nodes, created %d edges",
		created, updated, edgesCreated)

	// Broadcast graph update
	b.eventBus.Publish(service.Event{
		Type:    service.EventGraphUpdated,
		Payload: map[string]int{"nodes_bootstrapped": len(fragment.Nodes)},
	})

	return nil
}

// GetEnvironment returns the detected environment info
func (b *bootstrapService) GetEnvironment() domain.EnvironmentInfo {
	return b.bootstrap.GetEnvironment()
}

// GetSuggestedScanTargets returns networks to scan
func (b *bootstrapService) GetSuggestedScanTargets() []string {
	return b.bootstrap.GetSuggestedScanTargets()
}

// GetScanTargets returns categorized scan targets (primary and discovery)
func (b *bootstrapService) GetScanTargets() domain.ScanTargets {
	return b.bootstrap.GetScanTargets()
}
