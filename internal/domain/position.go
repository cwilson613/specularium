package domain

// NodePosition represents the position and pinning state of a node in the visualization
type NodePosition struct {
	NodeID string  `json:"node_id"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Pinned bool    `json:"pinned"`
}

// NewNodePosition creates a new node position
func NewNodePosition(nodeID string, x, y float64) *NodePosition {
	return &NodePosition{
		NodeID: nodeID,
		X:      x,
		Y:      y,
		Pinned: false,
	}
}
