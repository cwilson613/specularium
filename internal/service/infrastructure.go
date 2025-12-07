package service

import (
	"context"
	"fmt"
	"sync"

	"netdiagram/internal/domain"
	"netdiagram/internal/loader"
	"netdiagram/internal/repository"
)

// InfrastructureService provides business logic for infrastructure operations
type InfrastructureService struct {
	repo     repository.Repository
	eventBus *EventBus
	mu       sync.RWMutex
	cache    *domain.Infrastructure
}

// NewInfrastructureService creates a new infrastructure service
func NewInfrastructureService(repo repository.Repository, eventBus *EventBus) *InfrastructureService {
	return &InfrastructureService{
		repo:     repo,
		eventBus: eventBus,
	}
}

// GetInfrastructure returns the current infrastructure state
func (s *InfrastructureService) GetInfrastructure(ctx context.Context) (*domain.Infrastructure, error) {
	s.mu.RLock()
	if s.cache != nil {
		defer s.mu.RUnlock()
		return s.cache, nil
	}
	s.mu.RUnlock()

	// Load from repository
	infra, err := s.repo.GetInfrastructure(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get infrastructure: %w", err)
	}

	s.mu.Lock()
	s.cache = infra
	s.mu.Unlock()

	return infra, nil
}

// GetGraph returns the derived graph for visualization
func (s *InfrastructureService) GetGraph(ctx context.Context) (*domain.Graph, error) {
	infra, err := s.GetInfrastructure(ctx)
	if err != nil {
		return nil, err
	}

	return domain.DeriveGraph(infra), nil
}

// GetHost retrieves a single host by ID
func (s *InfrastructureService) GetHost(ctx context.Context, id string) (*domain.Host, error) {
	return s.repo.GetHost(ctx, id)
}

// CreateHost creates a new host
func (s *InfrastructureService) CreateHost(ctx context.Context, host *domain.Host) error {
	if err := s.validateHost(host); err != nil {
		return err
	}

	// Check if host already exists
	existing, err := s.repo.GetHost(ctx, host.ID)
	if err != nil {
		return fmt.Errorf("failed to check existing host: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("host %s already exists", host.ID)
	}

	if err := s.repo.UpsertHost(ctx, host); err != nil {
		return fmt.Errorf("failed to create host: %w", err)
	}

	s.invalidateCache()
	s.eventBus.Publish(Event{Type: EventHostCreated, Payload: host})

	return nil
}

// UpdateHost updates an existing host
func (s *InfrastructureService) UpdateHost(ctx context.Context, host *domain.Host) error {
	if err := s.validateHost(host); err != nil {
		return err
	}

	// Check if host exists
	existing, err := s.repo.GetHost(ctx, host.ID)
	if err != nil {
		return fmt.Errorf("failed to check existing host: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("host %s not found", host.ID)
	}

	// Preserve position if not provided
	if host.Position == nil && existing.Position != nil {
		host.Position = existing.Position
	}

	if err := s.repo.UpsertHost(ctx, host); err != nil {
		return fmt.Errorf("failed to update host: %w", err)
	}

	s.invalidateCache()
	s.eventBus.Publish(Event{Type: EventHostUpdated, Payload: host})

	return nil
}

// DeleteHost removes a host
func (s *InfrastructureService) DeleteHost(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("host ID required")
	}

	// Check if host exists
	existing, err := s.repo.GetHost(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to check existing host: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("host %s not found", id)
	}

	if err := s.repo.DeleteHost(ctx, id); err != nil {
		return fmt.Errorf("failed to delete host: %w", err)
	}

	s.invalidateCache()
	s.eventBus.Publish(Event{Type: EventHostDeleted, Payload: map[string]string{"id": id}})

	return nil
}

// CreateConnection creates a new connection
func (s *InfrastructureService) CreateConnection(ctx context.Context, conn *domain.Connection) error {
	if err := s.validateConnection(ctx, conn); err != nil {
		return err
	}

	if err := s.repo.UpsertConnection(ctx, conn); err != nil {
		return fmt.Errorf("failed to create connection: %w", err)
	}

	s.invalidateCache()
	s.eventBus.Publish(Event{Type: EventConnectionCreated, Payload: conn})

	return nil
}

// DeleteConnection removes a connection
func (s *InfrastructureService) DeleteConnection(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("connection ID required")
	}

	if err := s.repo.DeleteConnection(ctx, id); err != nil {
		return fmt.Errorf("failed to delete connection: %w", err)
	}

	s.invalidateCache()
	s.eventBus.Publish(Event{Type: EventConnectionDeleted, Payload: map[string]string{"id": id}})

	return nil
}

// SavePositions saves node positions
func (s *InfrastructureService) SavePositions(ctx context.Context, positions map[string]domain.Position) error {
	if len(positions) == 0 {
		return nil
	}

	if err := s.repo.SavePositions(ctx, positions); err != nil {
		return fmt.Errorf("failed to save positions: %w", err)
	}

	s.invalidateCache()
	s.eventBus.Publish(Event{Type: EventPositionsUpdated, Payload: positions})

	return nil
}

// ImportFromYAML imports infrastructure from a YAML file
func (s *InfrastructureService) ImportFromYAML(ctx context.Context, path string) error {
	infra, err := loader.LoadYAML(path)
	if err != nil {
		return fmt.Errorf("failed to load YAML: %w", err)
	}

	if err := s.repo.ImportInfrastructure(ctx, infra); err != nil {
		return fmt.Errorf("failed to import infrastructure: %w", err)
	}

	s.invalidateCache()
	s.eventBus.Publish(Event{Type: EventInfraReloaded})

	return nil
}

// ExportToYAML exports infrastructure to YAML format
func (s *InfrastructureService) ExportToYAML(ctx context.Context) ([]byte, error) {
	infra, err := s.GetInfrastructure(ctx)
	if err != nil {
		return nil, err
	}

	return loader.ExportYAML(infra)
}

// Reload reloads infrastructure from the repository
func (s *InfrastructureService) Reload(ctx context.Context) error {
	s.invalidateCache()

	_, err := s.GetInfrastructure(ctx)
	if err != nil {
		return err
	}

	s.eventBus.Publish(Event{Type: EventInfraReloaded})
	return nil
}

func (s *InfrastructureService) validateHost(host *domain.Host) error {
	if host.ID == "" {
		return fmt.Errorf("host ID required")
	}
	if host.IP == "" {
		return fmt.Errorf("host IP required")
	}
	if host.Role == "" {
		return fmt.Errorf("host role required")
	}
	if host.Platform == "" {
		return fmt.Errorf("host platform required")
	}
	return nil
}

func (s *InfrastructureService) validateConnection(ctx context.Context, conn *domain.Connection) error {
	if conn.SourceID == "" {
		return fmt.Errorf("source ID required")
	}
	if conn.TargetID == "" {
		return fmt.Errorf("target ID required")
	}
	if conn.SourceID == conn.TargetID {
		return fmt.Errorf("source and target cannot be the same")
	}

	// Verify endpoints exist
	source, err := s.repo.GetHost(ctx, conn.SourceID)
	if err != nil {
		return fmt.Errorf("failed to verify source: %w", err)
	}
	if source == nil {
		return fmt.Errorf("source host %s not found", conn.SourceID)
	}

	target, err := s.repo.GetHost(ctx, conn.TargetID)
	if err != nil {
		return fmt.Errorf("failed to verify target: %w", err)
	}
	if target == nil {
		return fmt.Errorf("target host %s not found", conn.TargetID)
	}

	return nil
}

func (s *InfrastructureService) invalidateCache() {
	s.mu.Lock()
	s.cache = nil
	s.mu.Unlock()
}
