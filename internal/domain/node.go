package domain

import "time"

// NodeType represents the type of network node
type NodeType string

const (
	NodeTypeServer      NodeType = "server"
	NodeTypeSwitch      NodeType = "switch"
	NodeTypeRouter      NodeType = "router"
	NodeTypeAccessPoint NodeType = "access_point"
	NodeTypeVM          NodeType = "vm"
	NodeTypeVIP         NodeType = "vip"
	NodeTypeContainer   NodeType = "container"
	NodeTypeUnknown     NodeType = "unknown"
)

// NodeStatus represents the verification status of a node
type NodeStatus string

const (
	NodeStatusUnverified  NodeStatus = "unverified"  // Imported but not yet checked
	NodeStatusVerifying   NodeStatus = "verifying"   // Currently being probed
	NodeStatusVerified    NodeStatus = "verified"    // Successfully contacted
	NodeStatusUnreachable NodeStatus = "unreachable" // Failed to contact
	NodeStatusDegraded    NodeStatus = "degraded"    // Partially reachable (some probes failed)
)

// Node represents a network entity in the graph
type Node struct {
	ID         string         `json:"id"`
	Type       NodeType       `json:"type"`
	Label      string         `json:"label"`
	Properties map[string]any `json:"properties,omitempty"`
	Source     string         `json:"source,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`

	// Verification fields
	Status       NodeStatus `json:"status"`
	LastVerified *time.Time `json:"last_verified,omitempty"`
	LastSeen     *time.Time `json:"last_seen,omitempty"`

	// Discovered properties (auto-populated by adapters)
	Discovered map[string]any `json:"discovered,omitempty"`

	// Operator Truth fields
	Truth          *NodeTruth  `json:"truth,omitempty"`
	TruthStatus    TruthStatus `json:"truth_status,omitempty"`
	HasDiscrepancy bool        `json:"has_discrepancy,omitempty"`
}

// NewNode creates a new node with initialized properties
func NewNode(id string, nodeType NodeType, label string) *Node {
	now := time.Now()
	return &Node{
		ID:         id,
		Type:       nodeType,
		Label:      label,
		Properties: make(map[string]any),
		Discovered: make(map[string]any),
		Status:     NodeStatusUnverified,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// SetDiscovered sets a discovered property value
func (n *Node) SetDiscovered(key string, value any) {
	if n.Discovered == nil {
		n.Discovered = make(map[string]any)
	}
	n.Discovered[key] = value
}

// GetDiscovered gets a discovered property value
func (n *Node) GetDiscovered(key string) (any, bool) {
	if n.Discovered == nil {
		return nil, false
	}
	val, ok := n.Discovered[key]
	return val, ok
}

// SetProperty sets a property value
func (n *Node) SetProperty(key string, value any) {
	if n.Properties == nil {
		n.Properties = make(map[string]any)
	}
	n.Properties[key] = value
}

// GetProperty gets a property value
func (n *Node) GetProperty(key string) (any, bool) {
	if n.Properties == nil {
		return nil, false
	}
	val, ok := n.Properties[key]
	return val, ok
}

// GetPropertyString gets a property as a string
func (n *Node) GetPropertyString(key string) string {
	val, ok := n.GetProperty(key)
	if !ok {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	return ""
}
