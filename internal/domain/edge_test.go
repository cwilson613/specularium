package domain

import (
	"testing"
)

func TestNewEdge(t *testing.T) {
	t.Run("creates edge with generated ID", func(t *testing.T) {
		edge := NewEdge("node1", "node2", EdgeTypeEthernet)

		if edge.FromID != "node1" {
			t.Errorf("expected FromID 'node1', got %s", edge.FromID)
		}
		if edge.ToID != "node2" {
			t.Errorf("expected ToID 'node2', got %s", edge.ToID)
		}
		if edge.Type != EdgeTypeEthernet {
			t.Errorf("expected Type %s, got %s", EdgeTypeEthernet, edge.Type)
		}
		if edge.ID == "" {
			t.Error("expected ID to be generated")
		}
		if edge.Properties == nil {
			t.Error("expected Properties to be initialized")
		}
	})
}

func TestEdgeGenerateID(t *testing.T) {
	t.Run("generates consistent ID", func(t *testing.T) {
		edge1 := NewEdge("node1", "node2", EdgeTypeEthernet)
		edge2 := NewEdge("node1", "node2", EdgeTypeEthernet)

		if edge1.ID != edge2.ID {
			t.Error("expected same endpoints to generate same ID")
		}
	})

	t.Run("normalizes endpoints for consistent ID", func(t *testing.T) {
		edge1 := NewEdge("node1", "node2", EdgeTypeEthernet)
		edge2 := NewEdge("node2", "node1", EdgeTypeEthernet)

		if edge1.ID != edge2.ID {
			t.Error("expected reversed endpoints to generate same ID")
		}
	})

	t.Run("different edge types generate different IDs", func(t *testing.T) {
		edge1 := NewEdge("node1", "node2", EdgeTypeEthernet)
		edge2 := NewEdge("node1", "node2", EdgeTypeVLAN)

		if edge1.ID == edge2.ID {
			t.Error("expected different edge types to generate different IDs")
		}
	})

	t.Run("different endpoints generate different IDs", func(t *testing.T) {
		edge1 := NewEdge("node1", "node2", EdgeTypeEthernet)
		edge2 := NewEdge("node1", "node3", EdgeTypeEthernet)

		if edge1.ID == edge2.ID {
			t.Error("expected different endpoints to generate different IDs")
		}
	})

	t.Run("generates deterministic hash", func(t *testing.T) {
		edge := &Edge{
			FromID: "a",
			ToID:   "b",
			Type:   EdgeTypeEthernet,
		}
		id1 := edge.GenerateID()
		id2 := edge.GenerateID()

		if id1 != id2 {
			t.Error("expected GenerateID to be deterministic")
		}
	})

	t.Run("generates short hash", func(t *testing.T) {
		edge := NewEdge("node1", "node2", EdgeTypeEthernet)
		// Hash should be 16 hex characters (8 bytes * 2)
		if len(edge.ID) != 16 {
			t.Errorf("expected ID length 16, got %d", len(edge.ID))
		}
	})
}

func TestEdgeSetGetProperty(t *testing.T) {
	edge := NewEdge("node1", "node2", EdgeTypeEthernet)

	t.Run("set and get property", func(t *testing.T) {
		edge.SetProperty("speed", "1gbps")
		val, ok := edge.GetProperty("speed")
		if !ok {
			t.Error("expected property to exist")
		}
		if val != "1gbps" {
			t.Errorf("expected '1gbps', got %v", val)
		}
	})

	t.Run("get non-existent property", func(t *testing.T) {
		_, ok := edge.GetProperty("nonexistent")
		if ok {
			t.Error("expected property not to exist")
		}
	})

	t.Run("set property on nil map initializes map", func(t *testing.T) {
		edge := &Edge{}
		edge.SetProperty("key", "value")
		if edge.Properties == nil {
			t.Error("expected Properties to be initialized")
		}
	})

	t.Run("set complex property types", func(t *testing.T) {
		edge.SetProperty("vlan_ids", []int{10, 20, 30})
		edge.SetProperty("metadata", map[string]string{"rack": "A1"})

		vlanIDs, ok := edge.GetProperty("vlan_ids")
		if !ok {
			t.Error("expected vlan_ids property to exist")
		}
		if vlanIDs == nil {
			t.Error("expected vlan_ids to be non-nil")
		}

		metadata, ok := edge.GetProperty("metadata")
		if !ok {
			t.Error("expected metadata property to exist")
		}
		if metadata == nil {
			t.Error("expected metadata to be non-nil")
		}
	})
}

func TestEdgeTypes(t *testing.T) {
	types := []EdgeType{
		EdgeTypeEthernet,
		EdgeTypeVLAN,
		EdgeTypeVirtual,
		EdgeTypeAggregation,
	}

	t.Run("all edge types are valid", func(t *testing.T) {
		for _, edgeType := range types {
			edge := NewEdge("n1", "n2", edgeType)
			if edge.Type != edgeType {
				t.Errorf("expected type %s, got %s", edgeType, edge.Type)
			}
		}
	})
}
