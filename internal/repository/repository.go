package repository

import (
	"context"

	"netdiagram/internal/domain"
)

// Repository defines the interface for infrastructure data access
type Repository interface {
	// Read operations
	GetInfrastructure(ctx context.Context) (*domain.Infrastructure, error)
	GetHost(ctx context.Context, id string) (*domain.Host, error)
	GetConnection(ctx context.Context, id string) (*domain.Connection, error)

	// Write operations
	UpsertHost(ctx context.Context, host *domain.Host) error
	DeleteHost(ctx context.Context, id string) error
	UpsertConnection(ctx context.Context, conn *domain.Connection) error
	DeleteConnection(ctx context.Context, id string) error

	// Layout persistence
	SavePositions(ctx context.Context, positions map[string]domain.Position) error

	// Bulk operations
	ImportInfrastructure(ctx context.Context, infra *domain.Infrastructure) error

	// Close releases resources
	Close() error
}
