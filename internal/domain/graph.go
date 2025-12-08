package domain

// Graph represents the complete network topology with positions
type Graph struct {
	Nodes     []Node                  `json:"nodes"`
	Edges     []Edge                  `json:"edges"`
	Positions map[string]NodePosition `json:"positions,omitempty"`
}

// NewGraph creates an empty graph
func NewGraph() *Graph {
	return &Graph{
		Nodes:     make([]Node, 0),
		Edges:     make([]Edge, 0),
		Positions: make(map[string]NodePosition),
	}
}

// AddNode adds a node to the graph
func (g *Graph) AddNode(node Node) {
	g.Nodes = append(g.Nodes, node)
}

// AddEdge adds an edge to the graph
func (g *Graph) AddEdge(edge Edge) {
	g.Edges = append(g.Edges, edge)
}

// SetPosition sets the position for a node
func (g *Graph) SetPosition(pos NodePosition) {
	if g.Positions == nil {
		g.Positions = make(map[string]NodePosition)
	}
	g.Positions[pos.NodeID] = pos
}

// GetPosition retrieves a node's position
func (g *Graph) GetPosition(nodeID string) (NodePosition, bool) {
	if g.Positions == nil {
		return NodePosition{}, false
	}
	pos, ok := g.Positions[nodeID]
	return pos, ok
}
