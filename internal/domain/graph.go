package domain

import "fmt"

// Graph is the derived view for vis-network visualization
type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// GraphNode represents a node in the visualization
type GraphNode struct {
	ID       string    `json:"id"`
	Label    string    `json:"label"`
	Group    string    `json:"group"` // "machine" or "device"
	Title    string    `json:"title"` // Tooltip content
	Position *Position `json:"position,omitempty"`
}

// GraphEdge represents an edge in the visualization
type GraphEdge struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Label     string `json:"label"` // "1GbE", "10GbE", etc.
	SpeedGbps int    `json:"speed_gbps"`
}

// DeriveGraph converts Infrastructure to a vis-network compatible Graph
func DeriveGraph(infra *Infrastructure) *Graph {
	graph := &Graph{
		Nodes: make([]GraphNode, 0, len(infra.Hosts)),
		Edges: make([]GraphEdge, 0, len(infra.Connections)),
	}

	// Convert hosts to nodes
	for id, host := range infra.Hosts {
		classification := host.InferClassification()

		node := GraphNode{
			ID:       id,
			Label:    id,
			Group:    string(classification),
			Title:    buildTooltip(host),
			Position: host.Position,
		}
		graph.Nodes = append(graph.Nodes, node)
	}

	// Convert connections to edges
	for id, conn := range infra.Connections {
		edge := GraphEdge{
			ID:        id,
			From:      conn.SourceID,
			To:        conn.TargetID,
			Label:     speedLabel(conn.SpeedGbps),
			SpeedGbps: conn.SpeedGbps,
		}
		graph.Edges = append(graph.Edges, edge)
	}

	return graph
}

func buildTooltip(host *Host) string {
	tooltip := fmt.Sprintf("%s\n%s\n%s", host.ID, host.Role, host.IP)
	if host.Description != "" {
		tooltip += "\n" + host.Description
	}
	return tooltip
}

func speedLabel(gbps int) string {
	if gbps == 0 {
		gbps = 1 // Default to 1GbE
	}
	if gbps >= 1000 {
		return fmt.Sprintf("%dTbE", gbps/1000)
	}
	return fmt.Sprintf("%dGbE", gbps)
}
