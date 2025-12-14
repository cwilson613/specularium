package service

import (
	"testing"

	"specularium/internal/domain"
)

func TestGraphServiceValidateNode(t *testing.T) {
	svc := &GraphService{}

	t.Run("valid node passes validation", func(t *testing.T) {
		node := domain.NewNode("test", domain.NodeTypeServer, "Test Server")
		err := svc.validateNode(node)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("empty ID fails validation", func(t *testing.T) {
		node := &domain.Node{
			Type:  domain.NodeTypeServer,
			Label: "Test",
		}
		err := svc.validateNode(node)
		if err == nil {
			t.Error("expected error for empty ID")
		}
	})

	t.Run("empty type fails validation", func(t *testing.T) {
		node := &domain.Node{
			ID:    "test",
			Label: "Test",
		}
		err := svc.validateNode(node)
		if err == nil {
			t.Error("expected error for empty type")
		}
	})

	t.Run("empty label fails validation", func(t *testing.T) {
		node := &domain.Node{
			ID:   "test",
			Type: domain.NodeTypeServer,
		}
		err := svc.validateNode(node)
		if err == nil {
			t.Error("expected error for empty label")
		}
	})
}

func TestGraphServiceValidateEdge(t *testing.T) {
	svc := &GraphService{}

	t.Run("valid edge passes validation", func(t *testing.T) {
		edge := domain.NewEdge("node1", "node2", domain.EdgeTypeEthernet)
		err := svc.validateEdge(edge)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("empty from_id fails validation", func(t *testing.T) {
		edge := &domain.Edge{
			ToID: "node2",
			Type: domain.EdgeTypeEthernet,
		}
		err := svc.validateEdge(edge)
		if err == nil {
			t.Error("expected error for empty from_id")
		}
	})

	t.Run("empty to_id fails validation", func(t *testing.T) {
		edge := &domain.Edge{
			FromID: "node1",
			Type:   domain.EdgeTypeEthernet,
		}
		err := svc.validateEdge(edge)
		if err == nil {
			t.Error("expected error for empty to_id")
		}
	})

	t.Run("empty type fails validation", func(t *testing.T) {
		edge := &domain.Edge{
			FromID: "node1",
			ToID:   "node2",
		}
		err := svc.validateEdge(edge)
		if err == nil {
			t.Error("expected error for empty type")
		}
	})

	t.Run("self-loop fails validation", func(t *testing.T) {
		edge := &domain.Edge{
			FromID: "node1",
			ToID:   "node1",
			Type:   domain.EdgeTypeEthernet,
		}
		err := svc.validateEdge(edge)
		if err == nil {
			t.Error("expected error for self-loop")
		}
	})
}


func TestImportResult(t *testing.T) {
	t.Run("import result structure", func(t *testing.T) {
		result := &ImportResult{
			NodesCreated: 5,
			NodesUpdated: 2,
			EdgesCreated: 3,
			EdgesUpdated: 1,
			Strategy:     "merge",
		}

		if result.NodesCreated != 5 {
			t.Errorf("expected NodesCreated=5, got %d", result.NodesCreated)
		}
		if result.Strategy != "merge" {
			t.Errorf("expected Strategy='merge', got %s", result.Strategy)
		}
	})
}

