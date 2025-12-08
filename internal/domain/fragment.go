package domain

// GraphFragment represents a partial graph for import/export operations
type GraphFragment struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// NewGraphFragment creates an empty graph fragment
func NewGraphFragment() *GraphFragment {
	return &GraphFragment{
		Nodes: make([]Node, 0),
		Edges: make([]Edge, 0),
	}
}

// AddNode adds a node to the fragment
func (g *GraphFragment) AddNode(node Node) {
	g.Nodes = append(g.Nodes, node)
}

// AddEdge adds an edge to the fragment
func (g *GraphFragment) AddEdge(edge Edge) {
	g.Edges = append(g.Edges, edge)
}
