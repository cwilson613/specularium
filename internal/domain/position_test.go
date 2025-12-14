package domain

import (
	"testing"
)

func TestNewNodePosition(t *testing.T) {
	t.Run("creates position with defaults", func(t *testing.T) {
		pos := NewNodePosition("node1", 100.5, 200.5)

		if pos.NodeID != "node1" {
			t.Errorf("expected NodeID 'node1', got %s", pos.NodeID)
		}
		if pos.X != 100.5 {
			t.Errorf("expected X=100.5, got %f", pos.X)
		}
		if pos.Y != 200.5 {
			t.Errorf("expected Y=200.5, got %f", pos.Y)
		}
		if pos.Pinned {
			t.Error("expected Pinned to be false by default")
		}
	})

	t.Run("creates position at origin", func(t *testing.T) {
		pos := NewNodePosition("origin", 0, 0)

		if pos.X != 0 {
			t.Errorf("expected X=0, got %f", pos.X)
		}
		if pos.Y != 0 {
			t.Errorf("expected Y=0, got %f", pos.Y)
		}
	})

	t.Run("creates position with negative coordinates", func(t *testing.T) {
		pos := NewNodePosition("negative", -50.5, -100.5)

		if pos.X != -50.5 {
			t.Errorf("expected X=-50.5, got %f", pos.X)
		}
		if pos.Y != -100.5 {
			t.Errorf("expected Y=-100.5, got %f", pos.Y)
		}
	})

	t.Run("creates position with large coordinates", func(t *testing.T) {
		pos := NewNodePosition("large", 9999.999, 8888.888)

		if pos.X != 9999.999 {
			t.Errorf("expected X=9999.999, got %f", pos.X)
		}
		if pos.Y != 8888.888 {
			t.Errorf("expected Y=8888.888, got %f", pos.Y)
		}
	})
}

func TestNodePositionPinned(t *testing.T) {
	t.Run("can set pinned flag", func(t *testing.T) {
		pos := NewNodePosition("test", 100, 100)
		pos.Pinned = true

		if !pos.Pinned {
			t.Error("expected Pinned to be true")
		}
	})

	t.Run("can unset pinned flag", func(t *testing.T) {
		pos := &NodePosition{
			NodeID: "test",
			X:      100,
			Y:      100,
			Pinned: true,
		}
		pos.Pinned = false

		if pos.Pinned {
			t.Error("expected Pinned to be false")
		}
	})
}
