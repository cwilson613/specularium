package domain

import (
	"testing"
)

func TestNewGraph(t *testing.T) {
	t.Run("creates empty graph with initialized collections", func(t *testing.T) {
		graph := NewGraph()

		if graph.Nodes == nil {
			t.Error("expected Nodes to be initialized")
		}
		if len(graph.Nodes) != 0 {
			t.Errorf("expected empty Nodes slice, got length %d", len(graph.Nodes))
		}

		if graph.Edges == nil {
			t.Error("expected Edges to be initialized")
		}
		if len(graph.Edges) != 0 {
			t.Errorf("expected empty Edges slice, got length %d", len(graph.Edges))
		}

		if graph.Positions == nil {
			t.Error("expected Positions to be initialized")
		}
		if len(graph.Positions) != 0 {
			t.Errorf("expected empty Positions map, got length %d", len(graph.Positions))
		}
	})
}

func TestGraphAddNode(t *testing.T) {
	graph := NewGraph()

	t.Run("adds single node", func(t *testing.T) {
		node := *NewNode("node1", NodeTypeServer, "Server 1")
		graph.AddNode(node)

		if len(graph.Nodes) != 1 {
			t.Errorf("expected 1 node, got %d", len(graph.Nodes))
		}
		if graph.Nodes[0].ID != "node1" {
			t.Errorf("expected node ID 'node1', got %s", graph.Nodes[0].ID)
		}
	})

	t.Run("adds multiple nodes", func(t *testing.T) {
		graph := NewGraph()
		for i := 1; i <= 5; i++ {
			node := *NewNode("node"+string(rune('0'+i)), NodeTypeServer, "Server")
			graph.AddNode(node)
		}

		if len(graph.Nodes) != 5 {
			t.Errorf("expected 5 nodes, got %d", len(graph.Nodes))
		}
	})

	t.Run("nodes are added by value", func(t *testing.T) {
		graph := NewGraph()
		node := *NewNode("test", NodeTypeServer, "Test")
		graph.AddNode(node)

		// Modify original node
		node.Label = "Modified"

		// Graph node should not be modified
		if graph.Nodes[0].Label == "Modified" {
			t.Error("expected graph node to be independent of original")
		}
	})
}

func TestGraphAddEdge(t *testing.T) {
	graph := NewGraph()

	t.Run("adds single edge", func(t *testing.T) {
		edge := *NewEdge("node1", "node2", EdgeTypeEthernet)
		graph.AddEdge(edge)

		if len(graph.Edges) != 1 {
			t.Errorf("expected 1 edge, got %d", len(graph.Edges))
		}
		if graph.Edges[0].FromID != "node1" {
			t.Errorf("expected FromID 'node1', got %s", graph.Edges[0].FromID)
		}
	})

	t.Run("adds multiple edges", func(t *testing.T) {
		graph := NewGraph()
		for i := 1; i <= 3; i++ {
			from := "node" + string(rune('0'+i))
			to := "node" + string(rune('0'+i+1))
			edge := *NewEdge(from, to, EdgeTypeEthernet)
			graph.AddEdge(edge)
		}

		if len(graph.Edges) != 3 {
			t.Errorf("expected 3 edges, got %d", len(graph.Edges))
		}
	})
}

func TestGraphSetPosition(t *testing.T) {
	graph := NewGraph()

	t.Run("sets position for node", func(t *testing.T) {
		pos := NodePosition{
			NodeID: "node1",
			X:      100.5,
			Y:      200.5,
			Pinned: true,
		}
		graph.SetPosition(pos)

		if len(graph.Positions) != 1 {
			t.Errorf("expected 1 position, got %d", len(graph.Positions))
		}

		storedPos, ok := graph.Positions["node1"]
		if !ok {
			t.Fatal("expected position to be stored")
		}
		if storedPos.X != 100.5 {
			t.Errorf("expected X=100.5, got %f", storedPos.X)
		}
		if storedPos.Y != 200.5 {
			t.Errorf("expected Y=200.5, got %f", storedPos.Y)
		}
		if !storedPos.Pinned {
			t.Error("expected Pinned=true")
		}
	})

	t.Run("updates existing position", func(t *testing.T) {
		graph := NewGraph()
		pos1 := NodePosition{NodeID: "node1", X: 100, Y: 100}
		graph.SetPosition(pos1)

		pos2 := NodePosition{NodeID: "node1", X: 200, Y: 200, Pinned: true}
		graph.SetPosition(pos2)

		if len(graph.Positions) != 1 {
			t.Errorf("expected 1 position, got %d", len(graph.Positions))
		}

		storedPos := graph.Positions["node1"]
		if storedPos.X != 200 {
			t.Errorf("expected updated X=200, got %f", storedPos.X)
		}
	})

	t.Run("initializes nil positions map", func(t *testing.T) {
		graph := &Graph{}
		pos := NodePosition{NodeID: "node1", X: 100, Y: 100}
		graph.SetPosition(pos)

		if graph.Positions == nil {
			t.Error("expected Positions to be initialized")
		}
	})
}

func TestGraphGetPosition(t *testing.T) {
	graph := NewGraph()

	t.Run("gets existing position", func(t *testing.T) {
		pos := NodePosition{NodeID: "node1", X: 100, Y: 100}
		graph.SetPosition(pos)

		retrieved, ok := graph.GetPosition("node1")
		if !ok {
			t.Fatal("expected position to be found")
		}
		if retrieved.NodeID != "node1" {
			t.Errorf("expected NodeID 'node1', got %s", retrieved.NodeID)
		}
		if retrieved.X != 100 {
			t.Errorf("expected X=100, got %f", retrieved.X)
		}
	})

	t.Run("returns false for non-existent position", func(t *testing.T) {
		_, ok := graph.GetPosition("nonexistent")
		if ok {
			t.Error("expected position not to be found")
		}
	})

	t.Run("returns false for nil positions map", func(t *testing.T) {
		graph := &Graph{}
		_, ok := graph.GetPosition("node1")
		if ok {
			t.Error("expected position not to be found in nil map")
		}
	})
}

func TestGraphOperations(t *testing.T) {
	t.Run("complete graph workflow", func(t *testing.T) {
		graph := NewGraph()

		// Add nodes
		node1 := *NewNode("server1", NodeTypeServer, "Server 1")
		node2 := *NewNode("server2", NodeTypeServer, "Server 2")
		node3 := *NewNode("switch1", NodeTypeSwitch, "Switch 1")

		graph.AddNode(node1)
		graph.AddNode(node2)
		graph.AddNode(node3)

		if len(graph.Nodes) != 3 {
			t.Errorf("expected 3 nodes, got %d", len(graph.Nodes))
		}

		// Add edges
		edge1 := *NewEdge("server1", "switch1", EdgeTypeEthernet)
		edge2 := *NewEdge("server2", "switch1", EdgeTypeEthernet)
		graph.AddEdge(edge1)
		graph.AddEdge(edge2)

		if len(graph.Edges) != 2 {
			t.Errorf("expected 2 edges, got %d", len(graph.Edges))
		}

		// Set positions
		graph.SetPosition(NodePosition{NodeID: "server1", X: 0, Y: 0})
		graph.SetPosition(NodePosition{NodeID: "server2", X: 200, Y: 0})
		graph.SetPosition(NodePosition{NodeID: "switch1", X: 100, Y: 100})

		if len(graph.Positions) != 3 {
			t.Errorf("expected 3 positions, got %d", len(graph.Positions))
		}

		// Verify positions can be retrieved
		pos, ok := graph.GetPosition("switch1")
		if !ok {
			t.Fatal("expected to find switch1 position")
		}
		if pos.X != 100 || pos.Y != 100 {
			t.Errorf("expected switch1 at (100, 100), got (%f, %f)", pos.X, pos.Y)
		}
	})
}
