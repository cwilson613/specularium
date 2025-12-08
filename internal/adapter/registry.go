package adapter

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"specularium/internal/domain"
)

// ReconcileFunc is called when an adapter produces a fragment to be merged
type ReconcileFunc func(ctx context.Context, source string, fragment *domain.GraphFragment) error

// DiscoveryEventFunc is called when discovery events occur
type DiscoveryEventFunc func(eventType string, payload interface{})

// Registry manages all registered adapters and their lifecycle
type Registry struct {
	mu              sync.RWMutex
	adapters        map[string]Adapter
	configs         map[string]AdapterConfig
	reconcile       ReconcileFunc
	discoveryEvent  DiscoveryEventFunc
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

// NewRegistry creates a new adapter registry
func NewRegistry(reconcile ReconcileFunc) *Registry {
	return &Registry{
		adapters:  make(map[string]Adapter),
		configs:   make(map[string]AdapterConfig),
		reconcile: reconcile,
	}
}

// SetDiscoveryEventHandler sets the handler for discovery events
func (r *Registry) SetDiscoveryEventHandler(handler DiscoveryEventFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.discoveryEvent = handler
}

// PublishDiscoveryEvent implements EventPublisher interface
func (r *Registry) PublishDiscoveryEvent(eventType string, payload interface{}) {
	r.mu.RLock()
	handler := r.discoveryEvent
	r.mu.RUnlock()

	if handler != nil {
		handler(eventType, payload)
	}
}

// Register adds an adapter to the registry
func (r *Registry) Register(adapter Adapter, config AdapterConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := adapter.Name()
	if _, exists := r.adapters[name]; exists {
		return fmt.Errorf("adapter %s already registered", name)
	}

	// Set event publisher if adapter supports it
	if progressAdapter, ok := adapter.(ProgressAdapter); ok {
		progressAdapter.SetEventPublisher(r)
	}

	r.adapters[name] = adapter
	r.configs[name] = config
	log.Printf("Registered adapter: %s (type=%s, priority=%d, enabled=%v)",
		name, adapter.Type(), config.Priority, config.Enabled)

	return nil
}

// Start initializes all enabled adapters and begins their sync cycles
func (r *Registry) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.ctx, r.cancel = context.WithCancel(ctx)

	for name, adapter := range r.adapters {
		config := r.configs[name]
		if !config.Enabled {
			log.Printf("Adapter %s is disabled, skipping", name)
			continue
		}

		// Initialize adapter
		if err := adapter.Start(r.ctx); err != nil {
			log.Printf("Failed to start adapter %s: %v", name, err)
			continue
		}

		// Start polling loop for polling adapters
		if adapter.Type() == AdapterTypePolling {
			r.startPollingLoop(name, adapter, config)
		}
	}

	return nil
}

// Stop gracefully shuts down all adapters
func (r *Registry) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cancel != nil {
		r.cancel()
	}

	// Wait for all polling loops to finish
	r.wg.Wait()

	// Stop all adapters
	for name, adapter := range r.adapters {
		if err := adapter.Stop(); err != nil {
			log.Printf("Error stopping adapter %s: %v", name, err)
		}
	}

	return nil
}

// TriggerSync manually triggers a sync for a specific adapter
func (r *Registry) TriggerSync(ctx context.Context, name string) error {
	r.mu.RLock()
	adapter, exists := r.adapters[name]
	config := r.configs[name]
	r.mu.RUnlock()

	if !exists {
		return fmt.Errorf("adapter %s not found", name)
	}

	if !config.Enabled {
		return fmt.Errorf("adapter %s is disabled", name)
	}

	return r.runSync(ctx, name, adapter)
}

// TriggerSyncAll manually triggers sync for all enabled adapters
func (r *Registry) TriggerSyncAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var errs []error
	for name, adapter := range r.adapters {
		config := r.configs[name]
		if !config.Enabled {
			continue
		}

		if err := r.runSync(ctx, name, adapter); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("sync errors: %v", errs)
	}
	return nil
}

// ListAdapters returns information about registered adapters
func (r *Registry) ListAdapters() []AdapterInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var infos []AdapterInfo
	for name, adapter := range r.adapters {
		config := r.configs[name]
		infos = append(infos, AdapterInfo{
			Name:         name,
			Type:         adapter.Type(),
			Priority:     config.Priority,
			Enabled:      config.Enabled,
			PollInterval: config.PollInterval,
		})
	}
	return infos
}

// AdapterInfo provides read-only information about an adapter
type AdapterInfo struct {
	Name         string      `json:"name"`
	Type         AdapterType `json:"type"`
	Priority     int         `json:"priority"`
	Enabled      bool        `json:"enabled"`
	PollInterval string      `json:"poll_interval,omitempty"`
}

// startPollingLoop starts a goroutine that polls the adapter on schedule
func (r *Registry) startPollingLoop(name string, adapter Adapter, config AdapterConfig) {
	interval, err := time.ParseDuration(config.PollInterval)
	if err != nil {
		log.Printf("Invalid poll interval for %s: %v, using 1m default", name, err)
		interval = time.Minute
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		// Run initial sync
		if err := r.runSync(r.ctx, name, adapter); err != nil {
			log.Printf("Initial sync failed for %s: %v", name, err)
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-r.ctx.Done():
				log.Printf("Stopping polling loop for %s", name)
				return
			case <-ticker.C:
				if err := r.runSync(r.ctx, name, adapter); err != nil {
					log.Printf("Sync failed for %s: %v", name, err)
				}
			}
		}
	}()

	log.Printf("Started polling loop for %s (interval=%s)", name, interval)
}

// runSync executes a sync operation and reconciles the result
func (r *Registry) runSync(ctx context.Context, name string, adapter Adapter) error {
	log.Printf("Running sync for adapter: %s", name)

	fragment, err := adapter.Sync(ctx)
	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	if fragment == nil || (len(fragment.Nodes) == 0 && len(fragment.Edges) == 0) {
		log.Printf("Adapter %s returned empty fragment", name)
		return nil
	}

	// Reconcile the fragment with the main graph
	if err := r.reconcile(ctx, name, fragment); err != nil {
		return fmt.Errorf("reconcile failed: %w", err)
	}

	log.Printf("Adapter %s sync complete: %d nodes, %d edges",
		name, len(fragment.Nodes), len(fragment.Edges))

	return nil
}
