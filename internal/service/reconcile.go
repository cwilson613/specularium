package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"specularium/internal/domain"
)

// ReconcileRepository defines the repository interface for reconciliation
type ReconcileRepository interface {
	GetNode(ctx context.Context, id string) (*domain.Node, error)
	UpdateNodeVerification(ctx context.Context, id string, status domain.NodeStatus, lastVerified, lastSeen *time.Time, discovered map[string]any) error
	UpdateNodeLabel(ctx context.Context, id string, label string) error
	HasOperatorTruthHostname(ctx context.Context, nodeID string) (bool, error)
}

// ReconcileService handles reconciliation of adapter discoveries
type ReconcileService struct {
	repo     ReconcileRepository
	truthSvc *TruthService
	eventBus *EventBus
}

// NewReconcileService creates a new reconcile service
func NewReconcileService(repo ReconcileRepository, truthSvc *TruthService, eventBus *EventBus) *ReconcileService {
	return &ReconcileService{
		repo:     repo,
		truthSvc: truthSvc,
		eventBus: eventBus,
	}
}

// ReconcileFragment reconciles adapter discoveries with existing nodes
// Updates node status/discovered fields and checks for discrepancies
func (r *ReconcileService) ReconcileFragment(ctx context.Context, source string, fragment *domain.GraphFragment) error {
	changedCount := 0

	for _, node := range fragment.Nodes {
		changed, err := r.reconcileNode(ctx, source, node)
		if err != nil {
			log.Printf("Failed to reconcile node %s: %v", node.ID, err)
			continue
		}
		if changed {
			changedCount++
		}
	}

	if changedCount > 0 {
		log.Printf("Reconciled %d changed nodes from %s", changedCount, source)
	}

	return nil
}

// reconcileNode handles reconciliation of a single node
func (r *ReconcileService) reconcileNode(ctx context.Context, source string, node domain.Node) (bool, error) {
	// Get existing node to compare
	existing, err := r.repo.GetNode(ctx, node.ID)
	if err != nil {
		return false, fmt.Errorf("get node: %w", err)
	}
	if existing == nil {
		// Node doesn't exist (shouldn't happen for verifier, but handle it)
		log.Printf("Node %s not found during verification reconcile", node.ID)
		return false, nil
	}

	// Check if verification data actually changed
	statusChanged := existing.Status != node.Status
	discoveredChanged := !discoveredEqual(existing.Discovered, node.Discovered)

	if !statusChanged && !discoveredChanged {
		// No changes, skip update and event
		return false, nil
	}

	// Update verification status
	if err := r.repo.UpdateNodeVerification(ctx, node.ID, node.Status, node.LastVerified, node.LastSeen, node.Discovered); err != nil {
		return false, fmt.Errorf("update verification: %w", err)
	}

	// Check for discrepancies against operator truth
	discrepancies, err := r.truthSvc.CheckDiscrepancies(ctx, node.ID, node.Discovered, source)
	if err != nil {
		log.Printf("Failed to check discrepancies for %s: %v", node.ID, err)
	} else if len(discrepancies) > 0 {
		log.Printf("Node %s has %d new discrepancies with operator truth", node.ID, len(discrepancies))
	}

	// Auto-update label from hostname inference if no operator truth
	if inference := extractHostnameInference(node.Discovered); inference != nil && inference.Best != nil {
		hasOperatorHostname, _ := r.repo.HasOperatorTruthHostname(ctx, node.ID)
		if !hasOperatorHostname {
			newLabel := domain.ExtractShortName(inference.Best.Hostname)
			if newLabel != "" && newLabel != existing.Label {
				if err := r.repo.UpdateNodeLabel(ctx, node.ID, newLabel); err != nil {
					log.Printf("Failed to update label for %s: %v", node.ID, err)
				} else {
					log.Printf("Auto-updated label for %s: %s -> %s (confidence: %.0f%%, source: %s)",
						node.ID, existing.Label, newLabel,
						inference.Best.Confidence*100, inference.Best.Source)
				}
			}
		}
	}

	// Fetch the updated node with all fields for the event payload
	updatedNode, err := r.repo.GetNode(ctx, node.ID)
	if err != nil {
		return false, fmt.Errorf("fetch updated node: %w", err)
	}

	// Emit node-updated event with full node data for incremental UI update
	r.eventBus.Publish(Event{
		Type:    EventNodeUpdated,
		Payload: updatedNode,
	})

	return true, nil
}

// discoveredEqual compares two discovered maps for equality
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
