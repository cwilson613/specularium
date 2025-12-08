package adapter

import (
	"context"

	"specularium/internal/domain"
)

// AdapterType defines how an adapter interacts with its data source
type AdapterType string

const (
	// AdapterTypePolling - adapter pulls data on a schedule
	AdapterTypePolling AdapterType = "polling"
	// AdapterTypeWebhook - external source pushes data to us
	AdapterTypeWebhook AdapterType = "webhook"
	// AdapterTypeBidirectional - both push and pull
	AdapterTypeBidirectional AdapterType = "bidirectional"
	// AdapterTypeOneShot - manual trigger only (e.g., file import)
	AdapterTypeOneShot AdapterType = "oneshot"
)

// AdapterConfig holds configuration for an adapter instance
type AdapterConfig struct {
	// Enabled determines if the adapter should run
	Enabled bool `json:"enabled"`
	// Priority determines which adapter wins in conflicts (higher = more authoritative)
	Priority int `json:"priority"`
	// PollInterval for polling adapters (e.g., "30s", "5m")
	PollInterval string `json:"poll_interval,omitempty"`
	// Settings holds adapter-specific configuration
	Settings map[string]any `json:"settings,omitempty"`
}

// SyncResult represents the outcome of an adapter sync operation
type SyncResult struct {
	// NodesAffected is the count of nodes created/updated/deleted
	NodesAffected int `json:"nodes_affected"`
	// EdgesAffected is the count of edges created/updated/deleted
	EdgesAffected int `json:"edges_affected"`
	// Errors encountered during sync (non-fatal)
	Errors []string `json:"errors,omitempty"`
}

// Adapter defines the interface for data source integrations
type Adapter interface {
	// Name returns the unique identifier for this adapter
	Name() string

	// Type returns how this adapter interacts with its source
	Type() AdapterType

	// Priority returns the authority level (higher = more authoritative)
	Priority() int

	// Start initializes the adapter (called once on startup)
	Start(ctx context.Context) error

	// Stop gracefully shuts down the adapter
	Stop() error

	// Sync pulls data from the source and returns a graph fragment
	// This is called on schedule for polling adapters, or manually for oneshot
	Sync(ctx context.Context) (*domain.GraphFragment, error)
}

// PushAdapter extends Adapter for bidirectional sync
type PushAdapter interface {
	Adapter

	// Push sends local changes to the external source
	Push(ctx context.Context, fragment *domain.GraphFragment) error
}

// WebhookAdapter extends Adapter for webhook-based sources
type WebhookAdapter interface {
	Adapter

	// HandleWebhook processes incoming webhook data
	HandleWebhook(ctx context.Context, payload []byte) (*domain.GraphFragment, error)
}

// EventPublisher allows adapters to publish progress events
type EventPublisher interface {
	PublishDiscoveryEvent(eventType string, payload interface{})
}

// ProgressAdapter extends Adapter with progress reporting
type ProgressAdapter interface {
	Adapter

	// SetEventPublisher sets the event publisher for progress updates
	SetEventPublisher(pub EventPublisher)
}
