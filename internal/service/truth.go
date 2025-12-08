package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"specularium/internal/domain"
	"specularium/internal/repository/sqlite"
)

// TruthService provides business logic for operator truth operations
type TruthService struct {
	repo     *sqlite.Repository
	eventBus *EventBus
}

// NewTruthService creates a new truth service
func NewTruthService(repo *sqlite.Repository, eventBus *EventBus) *TruthService {
	return &TruthService{
		repo:     repo,
		eventBus: eventBus,
	}
}

// SetTruth locks specific properties as operator truth for a node
func (s *TruthService) SetTruth(ctx context.Context, nodeID string, properties map[string]any, operator string) error {
	// Verify node exists
	node, err := s.repo.GetNode(ctx, nodeID)
	if err != nil {
		return err
	}
	if node == nil {
		return fmt.Errorf("node %s not found", nodeID)
	}

	// Validate properties are truthable
	for key := range properties {
		if !domain.IsTruthable(key) {
			return fmt.Errorf("property %q cannot be set as truth", key)
		}
	}

	// Create truth assertion
	now := time.Now()
	truth := &domain.NodeTruth{
		AssertedBy: operator,
		AssertedAt: &now,
		Properties: properties,
	}

	if err := s.repo.SetNodeTruth(ctx, nodeID, truth); err != nil {
		return err
	}

	// Resolve any existing discrepancies for properties that now match
	s.reconcileDiscrepancies(ctx, nodeID, properties)

	s.eventBus.Publish(Event{
		Type: EventTruthSet,
		Payload: map[string]interface{}{
			"node_id":    nodeID,
			"operator":   operator,
			"properties": properties,
		},
	})

	return nil
}

// ClearTruth removes truth assertion from a node
func (s *TruthService) ClearTruth(ctx context.Context, nodeID string) error {
	// Verify node exists
	node, err := s.repo.GetNode(ctx, nodeID)
	if err != nil {
		return err
	}
	if node == nil {
		return fmt.Errorf("node %s not found", nodeID)
	}

	if err := s.repo.ClearNodeTruth(ctx, nodeID); err != nil {
		return err
	}

	s.eventBus.Publish(Event{
		Type:    EventTruthCleared,
		Payload: map[string]string{"node_id": nodeID},
	})

	return nil
}

// GetTruth returns the truth assertion for a node
func (s *TruthService) GetTruth(ctx context.Context, nodeID string) (*domain.NodeTruth, error) {
	node, err := s.repo.GetNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	return node.Truth, nil
}

// CheckDiscrepancies compares discovered values against truth and creates discrepancy records
// Returns the list of new discrepancies created
func (s *TruthService) CheckDiscrepancies(ctx context.Context, nodeID string, discovered map[string]any, source string) ([]domain.Discrepancy, error) {
	node, err := s.repo.GetNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, nil // Node doesn't exist, no discrepancies to check
	}

	// No truth set, nothing to check against
	if node.Truth == nil || node.Truth.Properties == nil {
		return nil, nil
	}

	var newDiscrepancies []domain.Discrepancy
	now := time.Now()

	// Check each truth property against discovered values
	for key, truthValue := range node.Truth.Properties {
		actualValue, exists := discovered[key]

		// Also check node properties for things like IP
		if !exists {
			if propValue, ok := node.Properties[key]; ok {
				actualValue = propValue
				exists = true
			}
		}

		if !exists {
			// Property not in discovered data - skip (might just not be discovered yet)
			continue
		}

		// Compare values
		if !domain.CompareValues(truthValue, actualValue) {
			// Check if an unresolved discrepancy already exists for this property
			existing, _ := s.findUnresolvedDiscrepancy(ctx, nodeID, key)
			if existing != nil {
				// Update the actual value in the existing discrepancy
				continue
			}

			// Create new discrepancy
			d := domain.Discrepancy{
				ID:          generateID(),
				NodeID:      nodeID,
				PropertyKey: key,
				TruthValue:  truthValue,
				ActualValue: actualValue,
				Source:      source,
				DetectedAt:  now,
			}

			if err := s.repo.CreateDiscrepancy(ctx, &d); err != nil {
				return nil, fmt.Errorf("failed to create discrepancy: %w", err)
			}

			newDiscrepancies = append(newDiscrepancies, d)

			s.eventBus.Publish(Event{
				Type: EventDiscrepancyCreated,
				Payload: map[string]interface{}{
					"discrepancy_id": d.ID,
					"node_id":        nodeID,
					"property":       key,
					"truth":          truthValue,
					"actual":         actualValue,
					"source":         source,
				},
			})
		}
	}

	return newDiscrepancies, nil
}

// findUnresolvedDiscrepancy finds an existing unresolved discrepancy for a node/property
func (s *TruthService) findUnresolvedDiscrepancy(ctx context.Context, nodeID, propertyKey string) (*domain.Discrepancy, error) {
	discrepancies, err := s.repo.GetDiscrepanciesByNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}

	for _, d := range discrepancies {
		if d.PropertyKey == propertyKey && !d.IsResolved() {
			return &d, nil
		}
	}

	return nil, nil
}

// reconcileDiscrepancies resolves discrepancies when truth is updated to match actual values
func (s *TruthService) reconcileDiscrepancies(ctx context.Context, nodeID string, newTruthProperties map[string]any) {
	discrepancies, err := s.repo.GetDiscrepanciesByNode(ctx, nodeID)
	if err != nil {
		return
	}

	for _, d := range discrepancies {
		if d.IsResolved() {
			continue
		}

		// Check if the new truth value matches the actual value
		if newTruth, ok := newTruthProperties[d.PropertyKey]; ok {
			if domain.CompareValues(newTruth, d.ActualValue) {
				// Truth now matches actual - auto-resolve
				s.repo.ResolveDiscrepancy(ctx, d.ID, string(domain.ResolutionUpdatedTruth))
			}
		}
	}
}

// ResolveDiscrepancy marks a discrepancy as resolved
func (s *TruthService) ResolveDiscrepancy(ctx context.Context, discrepancyID string, resolution domain.DiscrepancyResolution) error {
	d, err := s.repo.GetDiscrepancy(ctx, discrepancyID)
	if err != nil {
		return err
	}
	if d == nil {
		return fmt.Errorf("discrepancy %s not found", discrepancyID)
	}

	if err := s.repo.ResolveDiscrepancy(ctx, discrepancyID, string(resolution)); err != nil {
		return err
	}

	s.eventBus.Publish(Event{
		Type: EventDiscrepancyResolved,
		Payload: map[string]interface{}{
			"discrepancy_id": discrepancyID,
			"node_id":        d.NodeID,
			"property":       d.PropertyKey,
			"resolution":     resolution,
		},
	})

	return nil
}

// GetDiscrepanciesByNode returns all discrepancies for a node
func (s *TruthService) GetDiscrepanciesByNode(ctx context.Context, nodeID string) ([]domain.Discrepancy, error) {
	return s.repo.GetDiscrepanciesByNode(ctx, nodeID)
}

// GetUnresolvedDiscrepancies returns all unresolved discrepancies
func (s *TruthService) GetUnresolvedDiscrepancies(ctx context.Context) ([]domain.Discrepancy, error) {
	return s.repo.GetUnresolvedDiscrepancies(ctx)
}

// GetDiscrepancy retrieves a single discrepancy by ID
func (s *TruthService) GetDiscrepancy(ctx context.Context, id string) (*domain.Discrepancy, error) {
	return s.repo.GetDiscrepancy(ctx, id)
}

// UpdateTruthProperty updates a single property in the truth assertion
func (s *TruthService) UpdateTruthProperty(ctx context.Context, nodeID, key string, value any, operator string) error {
	node, err := s.repo.GetNode(ctx, nodeID)
	if err != nil {
		return err
	}
	if node == nil {
		return fmt.Errorf("node %s not found", nodeID)
	}

	if !domain.IsTruthable(key) {
		return fmt.Errorf("property %q cannot be set as truth", key)
	}

	// Get existing truth or create new
	truth := node.Truth
	if truth == nil {
		now := time.Now()
		truth = &domain.NodeTruth{
			AssertedBy: operator,
			AssertedAt: &now,
			Properties: make(map[string]any),
		}
	}

	truth.Properties[key] = value

	return s.repo.SetNodeTruth(ctx, nodeID, truth)
}

// generateID creates a random ID for discrepancies
func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
