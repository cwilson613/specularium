package domain

import (
	"crypto/sha256"
	"fmt"
)

// EdgeType represents the type of network connection
type EdgeType string

const (
	EdgeTypeEthernet    EdgeType = "ethernet"
	EdgeTypeVLAN        EdgeType = "vlan"
	EdgeTypeVirtual     EdgeType = "virtual"
	EdgeTypeAggregation EdgeType = "aggregation"
)

// Edge represents a connection between two nodes
type Edge struct {
	ID         string         `json:"id"`
	FromID     string         `json:"from_id"`
	ToID       string         `json:"to_id"`
	Type       EdgeType       `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
}

// NewEdge creates a new edge
func NewEdge(fromID, toID string, edgeType EdgeType) *Edge {
	edge := &Edge{
		FromID:     fromID,
		ToID:       toID,
		Type:       edgeType,
		Properties: make(map[string]any),
	}
	edge.ID = edge.GenerateID()
	return edge
}

// GenerateID creates a deterministic ID for the edge based on endpoints
func (e *Edge) GenerateID() string {
	// Normalize endpoints for consistent ID
	from, to := e.FromID, e.ToID
	if from > to {
		from, to = to, from
	}

	key := fmt.Sprintf("%s-%s-%s", from, to, e.Type)
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", hash[:8])
}

// SetProperty sets a property value
func (e *Edge) SetProperty(key string, value any) {
	if e.Properties == nil {
		e.Properties = make(map[string]any)
	}
	e.Properties[key] = value
}

// GetProperty gets a property value
func (e *Edge) GetProperty(key string) (any, bool) {
	if e.Properties == nil {
		return nil, false
	}
	val, ok := e.Properties[key]
	return val, ok
}
