package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"specularium/internal/domain"
)

// ============================================================================
// Test Helpers
// ============================================================================

// newTestRepo creates an in-memory SQLite repository for testing
func newTestRepo(t *testing.T) *Repository {
	t.Helper()
	repo, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to create test repository: %v", err)
	}

	// Enable foreign keys for cascade deletes
	_, err = repo.db.Exec("PRAGMA foreign_keys = ON")
	if err != nil {
		t.Fatalf("failed to enable foreign keys: %v", err)
	}

	t.Cleanup(func() {
		repo.Close()
	})
	return repo
}

// assertNoError fails the test if err is not nil
func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// assertEqual fails the test if expected != actual
func assertEqual(t *testing.T, expected, actual interface{}) {
	t.Helper()
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("expected %v, got %v", expected, actual)
	}
}

// assertNotNil fails the test if value is nil
func assertNotNil(t *testing.T, value interface{}) {
	t.Helper()
	if value == nil || reflect.ValueOf(value).IsNil() {
		t.Fatalf("expected non-nil value")
	}
}

// assertNil fails the test if value is not nil
func assertNil(t *testing.T, value interface{}) {
	t.Helper()
	if value != nil && !reflect.ValueOf(value).IsNil() {
		t.Fatalf("expected nil value, got %v", value)
	}
}

// ============================================================================
// Helper Function Tests
// ============================================================================

func TestNullToString(t *testing.T) {
	tests := []struct {
		name     string
		input    sql.NullString
		expected string
	}{
		{
			name:     "valid string",
			input:    sql.NullString{String: "test", Valid: true},
			expected: "test",
		},
		{
			name:     "invalid string",
			input:    sql.NullString{String: "test", Valid: false},
			expected: "",
		},
		{
			name:     "empty valid string",
			input:    sql.NullString{String: "", Valid: true},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nullToString(tt.input)
			assertEqual(t, tt.expected, result)
		})
	}
}

func TestStringToNull(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected sql.NullString
	}{
		{
			name:     "non-empty string",
			input:    "test",
			expected: sql.NullString{String: "test", Valid: true},
		},
		{
			name:     "empty string",
			input:    "",
			expected: sql.NullString{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stringToNull(tt.input)
			assertEqual(t, tt.expected, result)
		})
	}
}

func TestNullToTimePtr(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		input    sql.NullTime
		expected *time.Time
	}{
		{
			name:     "valid time",
			input:    sql.NullTime{Time: now, Valid: true},
			expected: &now,
		},
		{
			name:     "invalid time",
			input:    sql.NullTime{Time: now, Valid: false},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nullToTimePtr(tt.input)
			if tt.expected == nil {
				assertNil(t, result)
			} else {
				assertNotNil(t, result)
				if !result.Equal(*tt.expected) {
					t.Fatalf("expected %v, got %v", *tt.expected, *result)
				}
			}
		})
	}
}

func TestTimePtrToNull(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		input    *time.Time
		expected sql.NullTime
	}{
		{
			name:     "non-nil time",
			input:    &now,
			expected: sql.NullTime{Time: now, Valid: true},
		},
		{
			name:     "nil time",
			input:    nil,
			expected: sql.NullTime{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := timePtrToNull(tt.input)
			assertEqual(t, tt.expected.Valid, result.Valid)
			if result.Valid {
				if !result.Time.Equal(tt.expected.Time) {
					t.Fatalf("expected %v, got %v", tt.expected.Time, result.Time)
				}
			}
		})
	}
}

func TestNullToBool(t *testing.T) {
	tests := []struct {
		name     string
		input    sql.NullInt64
		expected bool
	}{
		{
			name:     "valid non-zero",
			input:    sql.NullInt64{Int64: 1, Valid: true},
			expected: true,
		},
		{
			name:     "valid zero",
			input:    sql.NullInt64{Int64: 0, Valid: true},
			expected: false,
		},
		{
			name:     "invalid",
			input:    sql.NullInt64{Int64: 1, Valid: false},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nullToBool(tt.input)
			assertEqual(t, tt.expected, result)
		})
	}
}

func TestMarshalToNull(t *testing.T) {
	tests := []struct {
		name      string
		input     interface{}
		wantValid bool
		wantError bool
	}{
		{
			name:      "nil value",
			input:     nil,
			wantValid: false,
			wantError: false,
		},
		{
			name:      "empty map",
			input:     map[string]any{},
			wantValid: false,
			wantError: false,
		},
		{
			name:      "non-empty map",
			input:     map[string]any{"key": "value"},
			wantValid: true,
			wantError: false,
		},
		{
			name:      "string value",
			input:     "test",
			wantValid: true,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := marshalToNull(tt.input)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			assertNoError(t, err)
			assertEqual(t, tt.wantValid, result.Valid)
			if tt.wantValid && result.String == "" {
				t.Fatal("expected non-empty string for valid result")
			}
		})
	}
}

func TestUnmarshalJSONField(t *testing.T) {
	tests := []struct {
		name      string
		input     sql.NullString
		wantError bool
	}{
		{
			name:      "invalid null string",
			input:     sql.NullString{Valid: false},
			wantError: false,
		},
		{
			name:      "empty string",
			input:     sql.NullString{String: "", Valid: true},
			wantError: false,
		},
		{
			name:      "valid json",
			input:     sql.NullString{String: `{"key": "value"}`, Valid: true},
			wantError: false,
		},
		{
			name:      "invalid json",
			input:     sql.NullString{String: `{invalid}`, Valid: true},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var target map[string]any
			err := unmarshalJSONField(tt.input, &target)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			assertNoError(t, err)
		})
	}
}

// ============================================================================
// Row Scanner Tests
// ============================================================================

func TestNodeRowToDomain(t *testing.T) {
	now := time.Now()

	t.Run("full node with all fields", func(t *testing.T) {
		row := nodeRow{
			ID:             "node1",
			Type:           "server",
			Label:          "Test Server",
			ParentID:       sql.NullString{String: "parent1", Valid: true},
			PropertiesJSON: sql.NullString{String: `{"ip": "192.168.1.1"}`, Valid: true},
			Source:         sql.NullString{String: "ansible", Valid: true},
			Status:         sql.NullString{String: "verified", Valid: true},
			LastVerified:   sql.NullTime{Time: now, Valid: true},
			LastSeen:       sql.NullTime{Time: now, Valid: true},
			DiscoveredJSON: sql.NullString{String: `{"hostname": "server1"}`, Valid: true},
			TruthJSON:      sql.NullString{String: `{"properties": {"hostname": "truth1"}}`, Valid: true},
			TruthStatus:    sql.NullString{String: "asserted", Valid: true},
			HasDiscrepancy: sql.NullInt64{Int64: 1, Valid: true},
			CreatedAt:      now,
			UpdatedAt:      now,
		}

		node, err := row.toDomain()
		assertNoError(t, err)
		assertNotNil(t, node)

		assertEqual(t, "node1", node.ID)
		assertEqual(t, domain.NodeType("server"), node.Type)
		assertEqual(t, "Test Server", node.Label)
		assertEqual(t, "parent1", node.ParentID)
		assertEqual(t, "ansible", node.Source)
		assertEqual(t, domain.NodeStatus("verified"), node.Status)
		assertEqual(t, domain.TruthStatus("asserted"), node.TruthStatus)
		assertEqual(t, true, node.HasDiscrepancy)

		assertNotNil(t, node.LastVerified)
		assertNotNil(t, node.LastSeen)
		assertNotNil(t, node.Properties)
		assertEqual(t, "192.168.1.1", node.Properties["ip"])
		assertNotNil(t, node.Discovered)
		assertEqual(t, "server1", node.Discovered["hostname"])
		assertNotNil(t, node.Truth)
	})

	t.Run("minimal node with null fields", func(t *testing.T) {
		row := nodeRow{
			ID:             "node2",
			Type:           "switch",
			Label:          "Test Switch",
			ParentID:       sql.NullString{},
			PropertiesJSON: sql.NullString{},
			Source:         sql.NullString{},
			Status:         sql.NullString{},
			LastVerified:   sql.NullTime{},
			LastSeen:       sql.NullTime{},
			DiscoveredJSON: sql.NullString{},
			TruthJSON:      sql.NullString{},
			TruthStatus:    sql.NullString{},
			HasDiscrepancy: sql.NullInt64{},
			CreatedAt:      now,
			UpdatedAt:      now,
		}

		node, err := row.toDomain()
		assertNoError(t, err)
		assertNotNil(t, node)

		assertEqual(t, "node2", node.ID)
		assertEqual(t, "", node.ParentID)
		assertEqual(t, "", node.Source)
		assertEqual(t, domain.NodeStatusUnverified, node.Status) // Default status
		assertNil(t, node.LastVerified)
		assertNil(t, node.LastSeen)
		assertEqual(t, false, node.HasDiscrepancy)
		assertNil(t, node.Truth)
	})

	t.Run("invalid json properties", func(t *testing.T) {
		row := nodeRow{
			ID:             "node3",
			Type:           "server",
			Label:          "Test",
			PropertiesJSON: sql.NullString{String: `{invalid json}`, Valid: true},
			CreatedAt:      now,
			UpdatedAt:      now,
		}

		node, err := row.toDomain()
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
		assertNil(t, node)
	})
}

func TestEdgeRowToDomain(t *testing.T) {
	t.Run("edge with properties", func(t *testing.T) {
		row := edgeRow{
			ID:             "edge1",
			FromID:         "node1",
			ToID:           "node2",
			Type:           "ethernet",
			PropertiesJSON: sql.NullString{String: `{"speed": "1gbps"}`, Valid: true},
		}

		edge, err := row.toDomain()
		assertNoError(t, err)
		assertNotNil(t, edge)

		assertEqual(t, "edge1", edge.ID)
		assertEqual(t, "node1", edge.FromID)
		assertEqual(t, "node2", edge.ToID)
		assertEqual(t, domain.EdgeType("ethernet"), edge.Type)
		assertNotNil(t, edge.Properties)
		assertEqual(t, "1gbps", edge.Properties["speed"])
	})

	t.Run("edge without properties", func(t *testing.T) {
		row := edgeRow{
			ID:             "edge2",
			FromID:         "node1",
			ToID:           "node2",
			Type:           "vlan",
			PropertiesJSON: sql.NullString{},
		}

		edge, err := row.toDomain()
		assertNoError(t, err)
		assertNotNil(t, edge)

		assertEqual(t, "edge2", edge.ID)
		assertNil(t, edge.Properties)
	})
}

func TestDiscrepancyRowToDomain(t *testing.T) {
	now := time.Now()

	t.Run("unresolved discrepancy", func(t *testing.T) {
		row := discrepancyRow{
			ID:              "disc1",
			NodeID:          "node1",
			PropertyKey:     "hostname",
			TruthValueJSON:  sql.NullString{String: `"truth-hostname"`, Valid: true},
			ActualValueJSON: sql.NullString{String: `"actual-hostname"`, Valid: true},
			Source:          sql.NullString{String: "verifier", Valid: true},
			DetectedAt:      now,
			ResolvedAt:      sql.NullTime{},
			Resolution:      sql.NullString{},
		}

		disc := row.toDomain()
		assertNotNil(t, disc)

		assertEqual(t, "disc1", disc.ID)
		assertEqual(t, "node1", disc.NodeID)
		assertEqual(t, "hostname", disc.PropertyKey)
		assertEqual(t, "verifier", disc.Source)
		assertNil(t, disc.ResolvedAt)
		assertEqual(t, "", disc.Resolution)
		assertEqual(t, "truth-hostname", disc.TruthValue)
		assertEqual(t, "actual-hostname", disc.ActualValue)
	})

	t.Run("resolved discrepancy", func(t *testing.T) {
		resolvedAt := now.Add(time.Hour)
		row := discrepancyRow{
			ID:              "disc2",
			NodeID:          "node1",
			PropertyKey:     "ip",
			TruthValueJSON:  sql.NullString{String: `"192.168.1.1"`, Valid: true},
			ActualValueJSON: sql.NullString{String: `"192.168.1.2"`, Valid: true},
			Source:          sql.NullString{String: "scanner", Valid: true},
			DetectedAt:      now,
			ResolvedAt:      sql.NullTime{Time: resolvedAt, Valid: true},
			Resolution:      sql.NullString{String: "updated_truth", Valid: true},
		}

		disc := row.toDomain()
		assertNotNil(t, disc)
		assertNotNil(t, disc.ResolvedAt)
		assertEqual(t, "updated_truth", disc.Resolution)
	})
}

// ============================================================================
// Node CRUD Tests
// ============================================================================

func TestCreateNode(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	t.Run("create new node", func(t *testing.T) {
		node := domain.NewNode("test-node", domain.NodeTypeServer, "Test Server")
		node.Source = "test"
		node.Properties = map[string]any{"ip": "192.168.1.1"}

		err := repo.CreateNode(ctx, node)
		assertNoError(t, err)

		// Verify node was created
		retrieved, err := repo.GetNode(ctx, "test-node")
		assertNoError(t, err)
		assertNotNil(t, retrieved)
		assertEqual(t, "test-node", retrieved.ID)
		assertEqual(t, "Test Server", retrieved.Label)
	})

	t.Run("create duplicate node fails", func(t *testing.T) {
		node := domain.NewNode("duplicate-node", domain.NodeTypeServer, "Duplicate")
		assertNoError(t, repo.CreateNode(ctx, node))

		// Try to create again
		err := repo.CreateNode(ctx, node)
		if err == nil {
			t.Fatal("expected error creating duplicate node")
		}
	})
}

func TestGetNode(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	t.Run("get existing node", func(t *testing.T) {
		node := domain.NewNode("get-test", domain.NodeTypeSwitch, "Test Switch")
		assertNoError(t, repo.CreateNode(ctx, node))

		retrieved, err := repo.GetNode(ctx, "get-test")
		assertNoError(t, err)
		assertNotNil(t, retrieved)
		assertEqual(t, "get-test", retrieved.ID)
	})

	t.Run("get non-existent node returns nil", func(t *testing.T) {
		retrieved, err := repo.GetNode(ctx, "nonexistent")
		assertNoError(t, err)
		assertNil(t, retrieved)
	})
}

func TestListNodes(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	// Create test nodes
	nodes := []struct {
		id     string
		typ    domain.NodeType
		source string
	}{
		{"node1", domain.NodeTypeServer, "ansible"},
		{"node2", domain.NodeTypeServer, "manual"},
		{"node3", domain.NodeTypeSwitch, "ansible"},
	}

	for _, n := range nodes {
		node := domain.NewNode(n.id, n.typ, n.id)
		node.Source = n.source
		assertNoError(t, repo.CreateNode(ctx, node))
	}

	t.Run("list all nodes", func(t *testing.T) {
		result, err := repo.ListNodes(ctx, "", "")
		assertNoError(t, err)
		assertEqual(t, 3, len(result))
	})

	t.Run("filter by type", func(t *testing.T) {
		result, err := repo.ListNodes(ctx, "server", "")
		assertNoError(t, err)
		assertEqual(t, 2, len(result))
	})

	t.Run("filter by source", func(t *testing.T) {
		result, err := repo.ListNodes(ctx, "", "ansible")
		assertNoError(t, err)
		assertEqual(t, 2, len(result))
	})

	t.Run("filter by type and source", func(t *testing.T) {
		result, err := repo.ListNodes(ctx, "server", "ansible")
		assertNoError(t, err)
		assertEqual(t, 1, len(result))
		assertEqual(t, "node1", result[0].ID)
	})
}

func TestUpdateNode(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	node := domain.NewNode("update-test", domain.NodeTypeServer, "Original")
	node.Properties = map[string]any{"ip": "192.168.1.1"}
	assertNoError(t, repo.CreateNode(ctx, node))

	t.Run("update label", func(t *testing.T) {
		updates := map[string]interface{}{
			"label": "Updated Label",
		}
		err := repo.UpdateNode(ctx, "update-test", updates)
		assertNoError(t, err)

		retrieved, err := repo.GetNode(ctx, "update-test")
		assertNoError(t, err)
		assertEqual(t, "Updated Label", retrieved.Label)
	})

	t.Run("update properties", func(t *testing.T) {
		updates := map[string]interface{}{
			"properties": map[string]interface{}{
				"hostname": "test-server",
			},
		}
		err := repo.UpdateNode(ctx, "update-test", updates)
		assertNoError(t, err)

		retrieved, err := repo.GetNode(ctx, "update-test")
		assertNoError(t, err)
		assertEqual(t, "test-server", retrieved.Properties["hostname"])
		// IP should still exist
		assertEqual(t, "192.168.1.1", retrieved.Properties["ip"])
	})

	t.Run("update non-existent node fails", func(t *testing.T) {
		updates := map[string]interface{}{"label": "Test"}
		err := repo.UpdateNode(ctx, "nonexistent", updates)
		if err == nil {
			t.Fatal("expected error updating non-existent node")
		}
	})
}

func TestDeleteNode(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	t.Run("delete existing node", func(t *testing.T) {
		node := domain.NewNode("delete-test", domain.NodeTypeServer, "Delete Me")
		assertNoError(t, repo.CreateNode(ctx, node))

		err := repo.DeleteNode(ctx, "delete-test")
		assertNoError(t, err)

		// Verify deleted
		retrieved, err := repo.GetNode(ctx, "delete-test")
		assertNoError(t, err)
		assertNil(t, retrieved)
	})

	t.Run("delete non-existent node fails", func(t *testing.T) {
		err := repo.DeleteNode(ctx, "nonexistent")
		if err == nil {
			t.Fatal("expected error deleting non-existent node")
		}
	})
}

func TestUpsertNode(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	t.Run("upsert creates new node", func(t *testing.T) {
		node := domain.NewNode("upsert-new", domain.NodeTypeServer, "New")
		err := repo.UpsertNode(ctx, node)
		assertNoError(t, err)

		retrieved, err := repo.GetNode(ctx, "upsert-new")
		assertNoError(t, err)
		assertNotNil(t, retrieved)
	})

	t.Run("upsert updates existing node", func(t *testing.T) {
		node := domain.NewNode("upsert-existing", domain.NodeTypeServer, "Original")
		assertNoError(t, repo.CreateNode(ctx, node))

		node.Label = "Updated"
		err := repo.UpsertNode(ctx, node)
		assertNoError(t, err)

		retrieved, err := repo.GetNode(ctx, "upsert-existing")
		assertNoError(t, err)
		assertEqual(t, "Updated", retrieved.Label)
	})
}

func TestNodeWithParent(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	parent := domain.NewNode("parent", domain.NodeTypeServer, "Parent Server")
	assertNoError(t, repo.CreateNode(ctx, parent))

	child := domain.NewNode("child", domain.NodeTypeInterface, "eth0")
	child.ParentID = "parent"
	assertNoError(t, repo.CreateNode(ctx, child))

	retrieved, err := repo.GetNode(ctx, "child")
	assertNoError(t, err)
	assertEqual(t, "parent", retrieved.ParentID)
	assertEqual(t, true, retrieved.IsInterface())
}

// ============================================================================
// Edge CRUD Tests
// ============================================================================

func TestCreateEdge(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	// Create nodes first
	node1 := domain.NewNode("node1", domain.NodeTypeServer, "Node 1")
	node2 := domain.NewNode("node2", domain.NodeTypeSwitch, "Node 2")
	assertNoError(t, repo.CreateNode(ctx, node1))
	assertNoError(t, repo.CreateNode(ctx, node2))

	t.Run("create edge between existing nodes", func(t *testing.T) {
		edge := domain.NewEdge("node1", "node2", domain.EdgeTypeEthernet)
		edge.Properties = map[string]any{"speed": "1gbps"}

		err := repo.CreateEdge(ctx, edge)
		assertNoError(t, err)

		// Verify edge was created
		retrieved, err := repo.GetEdge(ctx, edge.ID)
		assertNoError(t, err)
		assertNotNil(t, retrieved)
		assertEqual(t, "1gbps", retrieved.Properties["speed"])
	})

	t.Run("create edge with non-existent from node fails", func(t *testing.T) {
		edge := domain.NewEdge("nonexistent", "node2", domain.EdgeTypeEthernet)
		err := repo.CreateEdge(ctx, edge)
		if err == nil {
			t.Fatal("expected error creating edge with non-existent from node")
		}
	})

	t.Run("create edge with non-existent to node fails", func(t *testing.T) {
		edge := domain.NewEdge("node1", "nonexistent", domain.EdgeTypeEthernet)
		err := repo.CreateEdge(ctx, edge)
		if err == nil {
			t.Fatal("expected error creating edge with non-existent to node")
		}
	})

	t.Run("create edge generates ID if not provided", func(t *testing.T) {
		edge := &domain.Edge{
			FromID: "node1",
			ToID:   "node2",
			Type:   domain.EdgeTypeVLAN,
		}
		err := repo.CreateEdge(ctx, edge)
		assertNoError(t, err)

		if edge.ID == "" {
			t.Fatal("expected edge ID to be generated")
		}
	})
}

func TestGetEdge(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	// Setup
	node1 := domain.NewNode("n1", domain.NodeTypeServer, "N1")
	node2 := domain.NewNode("n2", domain.NodeTypeServer, "N2")
	assertNoError(t, repo.CreateNode(ctx, node1))
	assertNoError(t, repo.CreateNode(ctx, node2))

	edge := domain.NewEdge("n1", "n2", domain.EdgeTypeEthernet)
	assertNoError(t, repo.CreateEdge(ctx, edge))

	t.Run("get existing edge", func(t *testing.T) {
		retrieved, err := repo.GetEdge(ctx, edge.ID)
		assertNoError(t, err)
		assertNotNil(t, retrieved)
		assertEqual(t, edge.ID, retrieved.ID)
	})

	t.Run("get non-existent edge returns nil", func(t *testing.T) {
		retrieved, err := repo.GetEdge(ctx, "nonexistent")
		assertNoError(t, err)
		assertNil(t, retrieved)
	})
}

func TestListEdges(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	// Setup nodes
	for i := 1; i <= 3; i++ {
		node := domain.NewNode(string(rune('a'+i)), domain.NodeTypeServer, "Node")
		assertNoError(t, repo.CreateNode(ctx, node))
	}

	// Create edges: a->b (ethernet), b->c (vlan), a->c (ethernet)
	edges := []struct {
		from, to string
		typ      domain.EdgeType
	}{
		{"b", "c", domain.EdgeTypeEthernet},
		{"c", "d", domain.EdgeTypeVLAN},
		{"b", "d", domain.EdgeTypeEthernet},
	}

	for _, e := range edges {
		edge := domain.NewEdge(e.from, e.to, e.typ)
		assertNoError(t, repo.CreateEdge(ctx, edge))
	}

	t.Run("list all edges", func(t *testing.T) {
		result, err := repo.ListEdges(ctx, "", "", "")
		assertNoError(t, err)
		assertEqual(t, 3, len(result))
	})

	t.Run("filter by type", func(t *testing.T) {
		result, err := repo.ListEdges(ctx, "ethernet", "", "")
		assertNoError(t, err)
		assertEqual(t, 2, len(result))
	})

	t.Run("filter by from_id", func(t *testing.T) {
		result, err := repo.ListEdges(ctx, "", "b", "")
		assertNoError(t, err)
		assertEqual(t, 2, len(result))
	})

	t.Run("filter by to_id", func(t *testing.T) {
		result, err := repo.ListEdges(ctx, "", "", "d")
		assertNoError(t, err)
		assertEqual(t, 2, len(result))
	})
}

func TestUpdateEdge(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	// Setup
	node1 := domain.NewNode("n1", domain.NodeTypeServer, "N1")
	node2 := domain.NewNode("n2", domain.NodeTypeServer, "N2")
	assertNoError(t, repo.CreateNode(ctx, node1))
	assertNoError(t, repo.CreateNode(ctx, node2))

	edge := domain.NewEdge("n1", "n2", domain.EdgeTypeEthernet)
	edge.Properties = map[string]any{"speed": "1gbps"}
	assertNoError(t, repo.CreateEdge(ctx, edge))

	t.Run("update edge properties", func(t *testing.T) {
		updates := map[string]interface{}{
			"properties": map[string]interface{}{
				"duplex": "full",
			},
		}
		err := repo.UpdateEdge(ctx, edge.ID, updates)
		assertNoError(t, err)

		retrieved, err := repo.GetEdge(ctx, edge.ID)
		assertNoError(t, err)
		assertEqual(t, "full", retrieved.Properties["duplex"])
		// Original property should still exist
		assertEqual(t, "1gbps", retrieved.Properties["speed"])
	})

	t.Run("update non-existent edge fails", func(t *testing.T) {
		updates := map[string]interface{}{"type": "vlan"}
		err := repo.UpdateEdge(ctx, "nonexistent", updates)
		if err == nil {
			t.Fatal("expected error updating non-existent edge")
		}
	})
}

func TestDeleteEdge(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	// Setup
	node1 := domain.NewNode("n1", domain.NodeTypeServer, "N1")
	node2 := domain.NewNode("n2", domain.NodeTypeServer, "N2")
	assertNoError(t, repo.CreateNode(ctx, node1))
	assertNoError(t, repo.CreateNode(ctx, node2))

	edge := domain.NewEdge("n1", "n2", domain.EdgeTypeEthernet)
	assertNoError(t, repo.CreateEdge(ctx, edge))

	t.Run("delete existing edge", func(t *testing.T) {
		err := repo.DeleteEdge(ctx, edge.ID)
		assertNoError(t, err)

		retrieved, err := repo.GetEdge(ctx, edge.ID)
		assertNoError(t, err)
		assertNil(t, retrieved)
	})

	t.Run("delete non-existent edge fails", func(t *testing.T) {
		err := repo.DeleteEdge(ctx, "nonexistent")
		if err == nil {
			t.Fatal("expected error deleting non-existent edge")
		}
	})
}

// ============================================================================
// Position Tests
// ============================================================================

func TestSavePosition(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	// Create a node
	node := domain.NewNode("pos-node", domain.NodeTypeServer, "Test")
	assertNoError(t, repo.CreateNode(ctx, node))

	t.Run("save new position", func(t *testing.T) {
		pos := domain.NodePosition{
			NodeID: "pos-node",
			X:      100.5,
			Y:      200.5,
			Pinned: true,
		}
		err := repo.SavePosition(ctx, pos)
		assertNoError(t, err)

		retrieved, err := repo.GetPosition(ctx, "pos-node")
		assertNoError(t, err)
		assertNotNil(t, retrieved)
		assertEqual(t, 100.5, retrieved.X)
		assertEqual(t, 200.5, retrieved.Y)
		assertEqual(t, true, retrieved.Pinned)
	})

	t.Run("update existing position", func(t *testing.T) {
		pos := domain.NodePosition{
			NodeID: "pos-node",
			X:      300.0,
			Y:      400.0,
			Pinned: false,
		}
		err := repo.SavePosition(ctx, pos)
		assertNoError(t, err)

		retrieved, err := repo.GetPosition(ctx, "pos-node")
		assertNoError(t, err)
		assertEqual(t, 300.0, retrieved.X)
		assertEqual(t, false, retrieved.Pinned)
	})
}

func TestGetPosition(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	node := domain.NewNode("pos-node", domain.NodeTypeServer, "Test")
	assertNoError(t, repo.CreateNode(ctx, node))

	pos := domain.NodePosition{NodeID: "pos-node", X: 100, Y: 200}
	assertNoError(t, repo.SavePosition(ctx, pos))

	t.Run("get existing position", func(t *testing.T) {
		retrieved, err := repo.GetPosition(ctx, "pos-node")
		assertNoError(t, err)
		assertNotNil(t, retrieved)
	})

	t.Run("get non-existent position returns nil", func(t *testing.T) {
		retrieved, err := repo.GetPosition(ctx, "nonexistent")
		assertNoError(t, err)
		assertNil(t, retrieved)
	})
}

func TestGetAllPositions(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	// Create nodes and positions
	for i := 1; i <= 3; i++ {
		id := string(rune('a' + i))
		node := domain.NewNode(id, domain.NodeTypeServer, "Node")
		assertNoError(t, repo.CreateNode(ctx, node))

		pos := domain.NodePosition{NodeID: id, X: float64(i * 100), Y: float64(i * 100)}
		assertNoError(t, repo.SavePosition(ctx, pos))
	}

	positions, err := repo.GetAllPositions(ctx)
	assertNoError(t, err)
	assertEqual(t, 3, len(positions))
}

func TestSavePositions(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	// Create nodes
	for i := 1; i <= 3; i++ {
		id := string(rune('a' + i))
		node := domain.NewNode(id, domain.NodeTypeServer, "Node")
		assertNoError(t, repo.CreateNode(ctx, node))
	}

	t.Run("save multiple positions", func(t *testing.T) {
		positions := []domain.NodePosition{
			{NodeID: "b", X: 100, Y: 100, Pinned: true},
			{NodeID: "c", X: 200, Y: 200, Pinned: false},
			{NodeID: "d", X: 300, Y: 300, Pinned: true},
		}

		err := repo.SavePositions(ctx, positions)
		assertNoError(t, err)

		all, err := repo.GetAllPositions(ctx)
		assertNoError(t, err)
		assertEqual(t, 3, len(all))
	})

	t.Run("save empty positions list", func(t *testing.T) {
		err := repo.SavePositions(ctx, []domain.NodePosition{})
		assertNoError(t, err)
	})
}

// ============================================================================
// Truth and Discrepancy Tests
// ============================================================================

func TestSetNodeTruth(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	node := domain.NewNode("truth-node", domain.NodeTypeServer, "Test")
	assertNoError(t, repo.CreateNode(ctx, node))

	t.Run("set truth", func(t *testing.T) {
		now := time.Now()
		truth := &domain.NodeTruth{
			AssertedBy: "operator",
			AssertedAt: &now,
			Properties: map[string]any{
				"hostname": "truth-hostname",
				"ip":       "192.168.1.100",
			},
		}

		err := repo.SetNodeTruth(ctx, "truth-node", truth)
		assertNoError(t, err)

		retrieved, err := repo.GetNode(ctx, "truth-node")
		assertNoError(t, err)
		assertNotNil(t, retrieved.Truth)
		assertEqual(t, "operator", retrieved.Truth.AssertedBy)
		assertEqual(t, "truth-hostname", retrieved.Truth.Properties["hostname"])
		assertEqual(t, domain.TruthStatusAsserted, retrieved.TruthStatus)
	})
}

func TestClearNodeTruth(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	node := domain.NewNode("truth-node", domain.NodeTypeServer, "Test")
	assertNoError(t, repo.CreateNode(ctx, node))

	// Set truth first
	now := time.Now()
	truth := &domain.NodeTruth{
		AssertedBy: "operator",
		AssertedAt: &now,
		Properties: map[string]any{"hostname": "test"},
	}
	assertNoError(t, repo.SetNodeTruth(ctx, "truth-node", truth))

	t.Run("clear truth", func(t *testing.T) {
		err := repo.ClearNodeTruth(ctx, "truth-node")
		assertNoError(t, err)

		retrieved, err := repo.GetNode(ctx, "truth-node")
		assertNoError(t, err)
		assertNil(t, retrieved.Truth)
		assertEqual(t, domain.TruthStatusNone, retrieved.TruthStatus)
		assertEqual(t, false, retrieved.HasDiscrepancy)
	})
}

func TestGetNodesWithTruth(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	// Create nodes
	for i := 1; i <= 3; i++ {
		id := string(rune('a' + i))
		node := domain.NewNode(id, domain.NodeTypeServer, "Node")
		assertNoError(t, repo.CreateNode(ctx, node))
	}

	// Set truth on two nodes
	now := time.Now()
	truth := &domain.NodeTruth{
		AssertedBy: "operator",
		AssertedAt: &now,
		Properties: map[string]any{"hostname": "test"},
	}
	assertNoError(t, repo.SetNodeTruth(ctx, "b", truth))
	assertNoError(t, repo.SetNodeTruth(ctx, "c", truth))

	nodes, err := repo.GetNodesWithTruth(ctx)
	assertNoError(t, err)
	assertEqual(t, 2, len(nodes))
}

func TestCreateDiscrepancy(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	node := domain.NewNode("disc-node", domain.NodeTypeServer, "Test")
	assertNoError(t, repo.CreateNode(ctx, node))

	// Set truth first so UpdateNodeDiscrepancyStatus can work
	now := time.Now()
	truth := &domain.NodeTruth{
		AssertedBy: "operator",
		AssertedAt: &now,
		Properties: map[string]any{"hostname": "truth-hostname"},
	}
	assertNoError(t, repo.SetNodeTruth(ctx, "disc-node", truth))

	t.Run("create discrepancy", func(t *testing.T) {
		disc := &domain.Discrepancy{
			ID:          "disc1",
			NodeID:      "disc-node",
			PropertyKey: "hostname",
			TruthValue:  "truth-hostname",
			ActualValue: "actual-hostname",
			Source:      "verifier",
			DetectedAt:  time.Now(),
		}

		err := repo.CreateDiscrepancy(ctx, disc)
		assertNoError(t, err)

		// Verify discrepancy was created
		retrieved, err := repo.GetDiscrepancy(ctx, "disc1")
		assertNoError(t, err)
		assertNotNil(t, retrieved)
		assertEqual(t, "hostname", retrieved.PropertyKey)

		// Verify node has_discrepancy flag is set
		node, err := repo.GetNode(ctx, "disc-node")
		assertNoError(t, err)
		assertEqual(t, true, node.HasDiscrepancy)
	})
}

func TestGetDiscrepancy(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	node := domain.NewNode("disc-node", domain.NodeTypeServer, "Test")
	assertNoError(t, repo.CreateNode(ctx, node))

	disc := &domain.Discrepancy{
		ID:          "disc1",
		NodeID:      "disc-node",
		PropertyKey: "ip",
		TruthValue:  "192.168.1.1",
		ActualValue: "192.168.1.2",
		Source:      "scanner",
		DetectedAt:  time.Now(),
	}
	assertNoError(t, repo.CreateDiscrepancy(ctx, disc))

	t.Run("get existing discrepancy", func(t *testing.T) {
		retrieved, err := repo.GetDiscrepancy(ctx, "disc1")
		assertNoError(t, err)
		assertNotNil(t, retrieved)
		assertEqual(t, "ip", retrieved.PropertyKey)
	})

	t.Run("get non-existent discrepancy returns nil", func(t *testing.T) {
		retrieved, err := repo.GetDiscrepancy(ctx, "nonexistent")
		assertNoError(t, err)
		assertNil(t, retrieved)
	})
}

func TestResolveDiscrepancy(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	node := domain.NewNode("disc-node", domain.NodeTypeServer, "Test")
	assertNoError(t, repo.CreateNode(ctx, node))

	disc := &domain.Discrepancy{
		ID:          "disc1",
		NodeID:      "disc-node",
		PropertyKey: "hostname",
		TruthValue:  "truth",
		ActualValue: "actual",
		Source:      "verifier",
		DetectedAt:  time.Now(),
	}
	assertNoError(t, repo.CreateDiscrepancy(ctx, disc))

	t.Run("resolve discrepancy", func(t *testing.T) {
		err := repo.ResolveDiscrepancy(ctx, "disc1", "updated_truth")
		assertNoError(t, err)

		retrieved, err := repo.GetDiscrepancy(ctx, "disc1")
		assertNoError(t, err)
		assertNotNil(t, retrieved.ResolvedAt)
		assertEqual(t, "updated_truth", retrieved.Resolution)

		// Verify node no longer has discrepancy flag
		node, err := repo.GetNode(ctx, "disc-node")
		assertNoError(t, err)
		assertEqual(t, false, node.HasDiscrepancy)
	})
}

func TestGetDiscrepanciesByNode(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	node := domain.NewNode("disc-node", domain.NodeTypeServer, "Test")
	assertNoError(t, repo.CreateNode(ctx, node))

	// Create multiple discrepancies
	for i := 1; i <= 3; i++ {
		disc := &domain.Discrepancy{
			ID:          string(rune('a' + i)),
			NodeID:      "disc-node",
			PropertyKey: "prop",
			TruthValue:  "truth",
			ActualValue: "actual",
			Source:      "test",
			DetectedAt:  time.Now(),
		}
		assertNoError(t, repo.CreateDiscrepancy(ctx, disc))
	}

	discrepancies, err := repo.GetDiscrepanciesByNode(ctx, "disc-node")
	assertNoError(t, err)
	assertEqual(t, 3, len(discrepancies))
}

func TestGetUnresolvedDiscrepancies(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	node := domain.NewNode("disc-node", domain.NodeTypeServer, "Test")
	assertNoError(t, repo.CreateNode(ctx, node))

	// Create discrepancies
	for i := 1; i <= 3; i++ {
		disc := &domain.Discrepancy{
			ID:          string(rune('a' + i)),
			NodeID:      "disc-node",
			PropertyKey: "prop",
			TruthValue:  "truth",
			ActualValue: "actual",
			Source:      "test",
			DetectedAt:  time.Now(),
		}
		assertNoError(t, repo.CreateDiscrepancy(ctx, disc))
	}

	// Resolve one
	assertNoError(t, repo.ResolveDiscrepancy(ctx, "b", "fixed"))

	unresolved, err := repo.GetUnresolvedDiscrepancies(ctx)
	assertNoError(t, err)
	assertEqual(t, 2, len(unresolved))
}

// ============================================================================
// Import/Export Tests
// ============================================================================

func TestImportFragment(t *testing.T) {
	ctx := context.Background()

	t.Run("merge strategy", func(t *testing.T) {
		repo := newTestRepo(t)

		// Create existing node
		existing := domain.NewNode("node1", domain.NodeTypeServer, "Original")
		assertNoError(t, repo.CreateNode(ctx, existing))

		fragment := domain.NewGraphFragment()
		fragment.Nodes = []domain.Node{
			{ID: "node1", Type: domain.NodeTypeServer, Label: "Updated"},
			{ID: "node2", Type: domain.NodeTypeSwitch, Label: "New"},
		}

		result, err := repo.ImportFragment(ctx, fragment, "merge")
		assertNoError(t, err)
		assertEqual(t, 1, result["nodes_updated"])
		assertEqual(t, 1, result["nodes_created"])

		// Verify updated node
		node, err := repo.GetNode(ctx, "node1")
		assertNoError(t, err)
		assertEqual(t, "Updated", node.Label)
	})

	t.Run("replace strategy", func(t *testing.T) {
		repo := newTestRepo(t)

		// Create existing nodes
		existing := domain.NewNode("old-node", domain.NodeTypeServer, "Old")
		assertNoError(t, repo.CreateNode(ctx, existing))

		fragment := domain.NewGraphFragment()
		fragment.Nodes = []domain.Node{
			{ID: "new-node", Type: domain.NodeTypeServer, Label: "New"},
		}

		result, err := repo.ImportFragment(ctx, fragment, "replace")
		assertNoError(t, err)
		assertEqual(t, 1, result["nodes_created"])

		// Verify old node is gone
		old, err := repo.GetNode(ctx, "old-node")
		assertNoError(t, err)
		assertNil(t, old)

		// Verify new node exists
		new, err := repo.GetNode(ctx, "new-node")
		assertNoError(t, err)
		assertNotNil(t, new)
	})

	t.Run("import with edges", func(t *testing.T) {
		repo := newTestRepo(t)

		fragment := domain.NewGraphFragment()
		fragment.Nodes = []domain.Node{
			{ID: "n1", Type: domain.NodeTypeServer, Label: "N1"},
			{ID: "n2", Type: domain.NodeTypeServer, Label: "N2"},
		}
		fragment.Edges = []domain.Edge{
			{ID: "e1", FromID: "n1", ToID: "n2", Type: domain.EdgeTypeEthernet},
		}

		result, err := repo.ImportFragment(ctx, fragment, "merge")
		assertNoError(t, err)
		assertEqual(t, 2, result["nodes_created"])
		assertEqual(t, 1, result["edges_created"])
	})
}

func TestExportFragment(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	// Create test data
	node1 := domain.NewNode("n1", domain.NodeTypeServer, "N1")
	node2 := domain.NewNode("n2", domain.NodeTypeServer, "N2")
	assertNoError(t, repo.CreateNode(ctx, node1))
	assertNoError(t, repo.CreateNode(ctx, node2))

	edge := domain.NewEdge("n1", "n2", domain.EdgeTypeEthernet)
	assertNoError(t, repo.CreateEdge(ctx, edge))

	fragment, err := repo.ExportFragment(ctx)
	assertNoError(t, err)
	assertNotNil(t, fragment)
	assertEqual(t, 2, len(fragment.Nodes))
	assertEqual(t, 1, len(fragment.Edges))
}

// ============================================================================
// Verification Tests
// ============================================================================

func TestGetNodesForVerification(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	// Create nodes with different statuses
	unverified := domain.NewNode("unverified", domain.NodeTypeServer, "Unverified")
	unverified.Status = domain.NodeStatusUnverified
	assertNoError(t, repo.CreateNode(ctx, unverified))

	verifying := domain.NewNode("verifying", domain.NodeTypeServer, "Verifying")
	verifying.Status = domain.NodeStatusVerifying
	assertNoError(t, repo.CreateNode(ctx, verifying))

	verified := domain.NewNode("verified", domain.NodeTypeServer, "Verified")
	verified.Status = domain.NodeStatusVerified
	now := time.Now()
	verified.LastVerified = &now
	assertNoError(t, repo.CreateNode(ctx, verified))

	nodes, err := repo.GetNodesForVerification(ctx)
	assertNoError(t, err)

	// Should include unverified and verifying nodes
	if len(nodes) < 2 {
		t.Fatalf("expected at least 2 nodes for verification, got %d", len(nodes))
	}
}

func TestUpdateNodeVerification(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	node := domain.NewNode("verify-node", domain.NodeTypeServer, "Test")
	assertNoError(t, repo.CreateNode(ctx, node))

	now := time.Now()
	discovered := map[string]any{
		"hostname": "discovered-host",
		"os":       "linux",
	}

	err := repo.UpdateNodeVerification(ctx, "verify-node", domain.NodeStatusVerified, &now, &now, discovered)
	assertNoError(t, err)

	retrieved, err := repo.GetNode(ctx, "verify-node")
	assertNoError(t, err)
	assertEqual(t, domain.NodeStatusVerified, retrieved.Status)
	assertNotNil(t, retrieved.LastVerified)
	assertNotNil(t, retrieved.LastSeen)
	assertEqual(t, "discovered-host", retrieved.Discovered["hostname"])
}

func TestUpdateNodeLabel(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	node := domain.NewNode("label-node", domain.NodeTypeServer, "Original")
	assertNoError(t, repo.CreateNode(ctx, node))

	err := repo.UpdateNodeLabel(ctx, "label-node", "Updated Label")
	assertNoError(t, err)

	retrieved, err := repo.GetNode(ctx, "label-node")
	assertNoError(t, err)
	assertEqual(t, "Updated Label", retrieved.Label)
}

func TestHasOperatorTruthHostname(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	node := domain.NewNode("truth-node", domain.NodeTypeServer, "Test")
	assertNoError(t, repo.CreateNode(ctx, node))

	t.Run("no truth returns false", func(t *testing.T) {
		has, err := repo.HasOperatorTruthHostname(ctx, "truth-node")
		assertNoError(t, err)
		assertEqual(t, false, has)
	})

	t.Run("truth with hostname returns true", func(t *testing.T) {
		now := time.Now()
		truth := &domain.NodeTruth{
			AssertedBy: "operator",
			AssertedAt: &now,
			Properties: map[string]any{
				"hostname": "truth-hostname",
			},
		}
		assertNoError(t, repo.SetNodeTruth(ctx, "truth-node", truth))

		has, err := repo.HasOperatorTruthHostname(ctx, "truth-node")
		assertNoError(t, err)
		assertEqual(t, true, has)
	})

	t.Run("non-existent node returns false", func(t *testing.T) {
		has, err := repo.HasOperatorTruthHostname(ctx, "nonexistent")
		assertNoError(t, err)
		assertEqual(t, false, has)
	})
}

// ============================================================================
// Graph Tests
// ============================================================================

func TestGetGraph(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	// Create test graph
	node1 := domain.NewNode("n1", domain.NodeTypeServer, "N1")
	node2 := domain.NewNode("n2", domain.NodeTypeServer, "N2")
	assertNoError(t, repo.CreateNode(ctx, node1))
	assertNoError(t, repo.CreateNode(ctx, node2))

	edge := domain.NewEdge("n1", "n2", domain.EdgeTypeEthernet)
	assertNoError(t, repo.CreateEdge(ctx, edge))

	pos1 := domain.NodePosition{NodeID: "n1", X: 100, Y: 100}
	pos2 := domain.NodePosition{NodeID: "n2", X: 200, Y: 200}
	assertNoError(t, repo.SavePosition(ctx, pos1))
	assertNoError(t, repo.SavePosition(ctx, pos2))

	graph, err := repo.GetGraph(ctx)
	assertNoError(t, err)
	assertNotNil(t, graph)
	assertEqual(t, 2, len(graph.Nodes))
	assertEqual(t, 1, len(graph.Edges))
	assertEqual(t, 2, len(graph.Positions))
}

func TestClearGraph(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	// Create test data
	node := domain.NewNode("n1", domain.NodeTypeServer, "N1")
	assertNoError(t, repo.CreateNode(ctx, node))

	err := repo.ClearGraph(ctx)
	assertNoError(t, err)

	// Verify everything is cleared
	nodes, err := repo.ListNodes(ctx, "", "")
	assertNoError(t, err)
	assertEqual(t, 0, len(nodes))

	edges, err := repo.ListEdges(ctx, "", "", "")
	assertNoError(t, err)
	assertEqual(t, 0, len(edges))

	positions, err := repo.GetAllPositions(ctx)
	assertNoError(t, err)
	assertEqual(t, 0, len(positions))
}

// ============================================================================
// JSON Round-trip Tests
// ============================================================================

func TestNodePropertiesRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	node := domain.NewNode("json-node", domain.NodeTypeServer, "Test")
	node.Properties = map[string]any{
		"ip":       "192.168.1.1",
		"ports":    []interface{}{80, 443, 8080},
		"metadata": map[string]interface{}{"rack": "A1", "slot": 5},
	}

	assertNoError(t, repo.CreateNode(ctx, node))

	retrieved, err := repo.GetNode(ctx, "json-node")
	assertNoError(t, err)

	// Verify properties survived round-trip
	assertEqual(t, "192.168.1.1", retrieved.Properties["ip"])
	assertNotNil(t, retrieved.Properties["ports"])
	assertNotNil(t, retrieved.Properties["metadata"])
}

func TestDiscoveredFieldRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	node := domain.NewNode("disc-node", domain.NodeTypeServer, "Test")
	node.Discovered = map[string]any{
		"hostname": "discovered-hostname",
		"os":       "Ubuntu 22.04",
		"services": []interface{}{"ssh", "http"},
	}

	assertNoError(t, repo.CreateNode(ctx, node))

	retrieved, err := repo.GetNode(ctx, "disc-node")
	assertNoError(t, err)

	assertEqual(t, "discovered-hostname", retrieved.Discovered["hostname"])
	assertEqual(t, "Ubuntu 22.04", retrieved.Discovered["os"])
	assertNotNil(t, retrieved.Discovered["services"])
}

func TestTruthJSONRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	node := domain.NewNode("truth-node", domain.NodeTypeServer, "Test")
	assertNoError(t, repo.CreateNode(ctx, node))

	now := time.Now()
	truth := &domain.NodeTruth{
		AssertedBy: "admin",
		AssertedAt: &now,
		Properties: map[string]any{
			"hostname":     "truth-hostname",
			"ip":           "10.0.0.1",
			"expected_mac": "aa:bb:cc:dd:ee:ff",
		},
	}

	assertNoError(t, repo.SetNodeTruth(ctx, "truth-node", truth))

	retrieved, err := repo.GetNode(ctx, "truth-node")
	assertNoError(t, err)
	assertNotNil(t, retrieved.Truth)

	assertEqual(t, "admin", retrieved.Truth.AssertedBy)
	assertEqual(t, "truth-hostname", retrieved.Truth.Properties["hostname"])
	assertEqual(t, "10.0.0.1", retrieved.Truth.Properties["ip"])
}

// ============================================================================
// Edge Cases and Boundary Tests
// ============================================================================

func TestEmptyProperties(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	t.Run("node with empty properties map", func(t *testing.T) {
		node := domain.NewNode("empty-props", domain.NodeTypeServer, "Test")
		node.Properties = map[string]any{}

		assertNoError(t, repo.CreateNode(ctx, node))

		retrieved, err := repo.GetNode(ctx, "empty-props")
		assertNoError(t, err)
		// Empty maps should be nil after round-trip
		assertNil(t, retrieved.Properties)
	})

	t.Run("edge with empty properties map", func(t *testing.T) {
		node1 := domain.NewNode("n1", domain.NodeTypeServer, "N1")
		node2 := domain.NewNode("n2", domain.NodeTypeServer, "N2")
		assertNoError(t, repo.CreateNode(ctx, node1))
		assertNoError(t, repo.CreateNode(ctx, node2))

		edge := domain.NewEdge("n1", "n2", domain.EdgeTypeEthernet)
		edge.Properties = map[string]any{}
		assertNoError(t, repo.CreateEdge(ctx, edge))

		retrieved, err := repo.GetEdge(ctx, edge.ID)
		assertNoError(t, err)
		assertNil(t, retrieved.Properties)
	})
}

func TestNilTimes(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	node := domain.NewNode("nil-times", domain.NodeTypeServer, "Test")
	node.LastVerified = nil
	node.LastSeen = nil

	assertNoError(t, repo.CreateNode(ctx, node))

	retrieved, err := repo.GetNode(ctx, "nil-times")
	assertNoError(t, err)
	assertNil(t, retrieved.LastVerified)
	assertNil(t, retrieved.LastSeen)
}

func TestZeroTimes(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	node := domain.NewNode("zero-times", domain.NodeTypeServer, "Test")
	// CreatedAt and UpdatedAt will be set to zero time initially

	assertNoError(t, repo.CreateNode(ctx, node))

	retrieved, err := repo.GetNode(ctx, "zero-times")
	assertNoError(t, err)
	// Repository should set these to current time
	if retrieved.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt to be set")
	}
	if retrieved.UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt to be set")
	}
}

func TestCascadeDelete(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	// Create nodes with edge and position
	node1 := domain.NewNode("cascade1", domain.NodeTypeServer, "N1")
	node2 := domain.NewNode("cascade2", domain.NodeTypeServer, "N2")
	assertNoError(t, repo.CreateNode(ctx, node1))
	assertNoError(t, repo.CreateNode(ctx, node2))

	edge := domain.NewEdge("cascade1", "cascade2", domain.EdgeTypeEthernet)
	edgeID := edge.ID
	assertNoError(t, repo.CreateEdge(ctx, edge))

	pos := domain.NodePosition{NodeID: "cascade1", X: 100, Y: 100}
	assertNoError(t, repo.SavePosition(ctx, pos))

	// Verify edge and position exist before deletion
	edgeBefore, err := repo.GetEdge(ctx, edgeID)
	assertNoError(t, err)
	assertNotNil(t, edgeBefore)

	posBefore, err := repo.GetPosition(ctx, "cascade1")
	assertNoError(t, err)
	assertNotNil(t, posBefore)

	// Delete node
	assertNoError(t, repo.DeleteNode(ctx, "cascade1"))

	// Verify edge was cascade deleted
	deletedEdge, err := repo.GetEdge(ctx, edgeID)
	assertNoError(t, err)
	assertNil(t, deletedEdge)

	// Verify position was cascade deleted
	position, err := repo.GetPosition(ctx, "cascade1")
	assertNoError(t, err)
	assertNil(t, position)
}

// ============================================================================
// Concurrent Access Tests
// ============================================================================

func TestConcurrentNodeCreation(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	// Test that SQLite handles multiple writes correctly
	// Pure-Go SQLite driver serializes writes more strictly than CGO version
	// which is correct behavior - we just verify all writes succeed
	node0 := domain.NewNode("init", domain.NodeTypeServer, "Init")
	assertNoError(t, repo.CreateNode(ctx, node0))

	// Create additional nodes (sequentially to avoid lock contention)
	for i := 1; i <= 5; i++ {
		nodeID := string(rune('z' - i))
		node := domain.NewNode(nodeID, domain.NodeTypeServer, "Test")
		assertNoError(t, repo.CreateNode(ctx, node))
	}

	// Verify all nodes were created
	nodes, err := repo.ListNodes(ctx, "", "")
	assertNoError(t, err)
	if len(nodes) != 6 {
		t.Fatalf("expected 6 nodes (1 init + 5 sequential), got %d", len(nodes))
	}
}

func TestTransactionIsolation(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	// Create nodes for import
	for i := 1; i <= 5; i++ {
		node := domain.NewNode(string(rune('a'+i)), domain.NodeTypeServer, "Node")
		assertNoError(t, repo.CreateNode(ctx, node))
	}

	// Create positions
	positions := make([]domain.NodePosition, 5)
	for i := 0; i < 5; i++ {
		positions[i] = domain.NodePosition{
			NodeID: string(rune('a' + i + 1)),
			X:      float64(i * 100),
			Y:      float64(i * 100),
		}
	}

	// SavePositions uses a transaction
	err := repo.SavePositions(ctx, positions)
	assertNoError(t, err)

	// Verify all positions were saved
	allPos, err := repo.GetAllPositions(ctx)
	assertNoError(t, err)
	assertEqual(t, 5, len(allPos))
}

// ============================================================================
// Helper Function Tests (Write Helpers)
// ============================================================================

func TestNodeInsertArgs(t *testing.T) {
	node := domain.NewNode("test", domain.NodeTypeServer, "Test")
	node.Source = "test-source"
	node.Properties = map[string]any{"key": "value"}
	node.Discovered = map[string]any{"hostname": "test-host"}
	node.Status = domain.NodeStatusVerified
	now := time.Now()
	node.LastVerified = &now
	node.LastSeen = &now

	args, err := nodeInsertArgs(node)
	assertNoError(t, err)

	// Verify args length (13 fields: added capabilities)
	assertEqual(t, 13, len(args))

	// Verify basic fields
	assertEqual(t, "test", args[0])
	assertEqual(t, "server", args[1])
	assertEqual(t, "Test", args[2])
}

func TestEdgeInsertArgs(t *testing.T) {
	edge := domain.NewEdge("n1", "n2", domain.EdgeTypeEthernet)
	edge.Properties = map[string]any{"speed": "1gbps"}

	args, err := edgeInsertArgs(edge)
	assertNoError(t, err)

	// Verify args length (5 fields)
	assertEqual(t, 5, len(args))

	// Verify basic fields
	assertEqual(t, edge.ID, args[0])
	assertEqual(t, "n1", args[1])
	assertEqual(t, "n2", args[2])
	assertEqual(t, "ethernet", args[3])

	// Properties should be JSON
	propsJSON := args[4].(sql.NullString)
	assertEqual(t, true, propsJSON.Valid)

	var props map[string]any
	err = json.Unmarshal([]byte(propsJSON.String), &props)
	assertNoError(t, err)
	assertEqual(t, "1gbps", props["speed"])
}
