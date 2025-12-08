package service

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"specularium/internal/codec"
	"specularium/internal/domain"
	"specularium/internal/repository/sqlite"
)

// GraphService provides business logic for graph operations
type GraphService struct {
	repo     *sqlite.Repository
	eventBus *EventBus
}

// NewGraphService creates a new graph service
func NewGraphService(repo *sqlite.Repository, eventBus *EventBus) *GraphService {
	return &GraphService{
		repo:     repo,
		eventBus: eventBus,
	}
}

// GetGraph returns the complete graph with nodes, edges, and positions
func (s *GraphService) GetGraph(ctx context.Context) (*domain.Graph, error) {
	return s.repo.GetGraph(ctx)
}

// GetNode retrieves a single node by ID
func (s *GraphService) GetNode(ctx context.Context, id string) (*domain.Node, error) {
	node, err := s.repo.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, fmt.Errorf("node %s not found", id)
	}
	return node, nil
}

// ListNodes returns all nodes, optionally filtered
func (s *GraphService) ListNodes(ctx context.Context, nodeType, source string) ([]domain.Node, error) {
	return s.repo.ListNodes(ctx, nodeType, source)
}

// CreateNode creates a new node
func (s *GraphService) CreateNode(ctx context.Context, node *domain.Node) error {
	if err := s.validateNode(node); err != nil {
		return err
	}

	if err := s.repo.CreateNode(ctx, node); err != nil {
		return err
	}

	s.eventBus.Publish(Event{
		Type:    EventNodeCreated,
		Payload: map[string]string{"node_id": node.ID, "type": string(node.Type)},
	})

	return nil
}

// UpdateNode updates an existing node
func (s *GraphService) UpdateNode(ctx context.Context, id string, updates map[string]interface{}) error {
	if err := s.repo.UpdateNode(ctx, id, updates); err != nil {
		return err
	}

	s.eventBus.Publish(Event{
		Type:    EventNodeUpdated,
		Payload: map[string]string{"node_id": id},
	})

	return nil
}

// DeleteNode removes a node and its connections
func (s *GraphService) DeleteNode(ctx context.Context, id string) error {
	if err := s.repo.DeleteNode(ctx, id); err != nil {
		return err
	}

	s.eventBus.Publish(Event{
		Type:    EventNodeDeleted,
		Payload: map[string]string{"node_id": id},
	})

	return nil
}

// GetEdge retrieves a single edge by ID
func (s *GraphService) GetEdge(ctx context.Context, id string) (*domain.Edge, error) {
	edge, err := s.repo.GetEdge(ctx, id)
	if err != nil {
		return nil, err
	}
	if edge == nil {
		return nil, fmt.Errorf("edge %s not found", id)
	}
	return edge, nil
}

// ListEdges returns all edges, optionally filtered
func (s *GraphService) ListEdges(ctx context.Context, edgeType, fromID, toID string) ([]domain.Edge, error) {
	return s.repo.ListEdges(ctx, edgeType, fromID, toID)
}

// CreateEdge creates a new edge
func (s *GraphService) CreateEdge(ctx context.Context, edge *domain.Edge) error {
	if err := s.validateEdge(edge); err != nil {
		return err
	}

	if err := s.repo.CreateEdge(ctx, edge); err != nil {
		return err
	}

	s.eventBus.Publish(Event{
		Type:    EventEdgeCreated,
		Payload: map[string]string{"edge_id": edge.ID},
	})

	return nil
}

// UpdateEdge updates an existing edge
func (s *GraphService) UpdateEdge(ctx context.Context, id string, updates map[string]interface{}) error {
	if err := s.repo.UpdateEdge(ctx, id, updates); err != nil {
		return err
	}

	s.eventBus.Publish(Event{
		Type:    EventEdgeUpdated,
		Payload: map[string]string{"edge_id": id},
	})

	return nil
}

// DeleteEdge removes an edge
func (s *GraphService) DeleteEdge(ctx context.Context, id string) error {
	if err := s.repo.DeleteEdge(ctx, id); err != nil {
		return err
	}

	s.eventBus.Publish(Event{
		Type:    EventEdgeDeleted,
		Payload: map[string]string{"edge_id": id},
	})

	return nil
}

// GetAllPositions returns all node positions
func (s *GraphService) GetAllPositions(ctx context.Context) (map[string]domain.NodePosition, error) {
	return s.repo.GetAllPositions(ctx)
}

// GetPosition retrieves a single node position
func (s *GraphService) GetPosition(ctx context.Context, nodeID string) (*domain.NodePosition, error) {
	return s.repo.GetPosition(ctx, nodeID)
}

// SavePosition saves a single node position
func (s *GraphService) SavePosition(ctx context.Context, pos domain.NodePosition) error {
	if err := s.repo.SavePosition(ctx, pos); err != nil {
		return err
	}

	s.eventBus.Publish(Event{
		Type:    EventPositionsUpdated,
		Payload: map[string]string{"node_id": pos.NodeID},
	})

	return nil
}

// SavePositions saves multiple node positions
func (s *GraphService) SavePositions(ctx context.Context, positions []domain.NodePosition) error {
	if len(positions) == 0 {
		return nil
	}

	if err := s.repo.SavePositions(ctx, positions); err != nil {
		return err
	}

	s.eventBus.Publish(Event{
		Type:    EventPositionsUpdated,
		Payload: map[string]int{"count": len(positions)},
	})

	return nil
}

// ImportResult represents the result of an import operation
type ImportResult struct {
	NodesCreated int    `json:"nodes_created"`
	NodesUpdated int    `json:"nodes_updated"`
	EdgesCreated int    `json:"edges_created"`
	EdgesUpdated int    `json:"edges_updated"`
	Strategy     string `json:"strategy"`
}

// ImportYAML imports graph data from YAML
func (s *GraphService) ImportYAML(ctx context.Context, data []byte, strategy string) (*ImportResult, error) {
	codec := codec.NewYAMLCodec()
	fragment, err := codec.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return s.importFragment(ctx, fragment, strategy)
}

// ImportAnsibleInventory imports graph data from Ansible inventory
func (s *GraphService) ImportAnsibleInventory(ctx context.Context, data []byte, strategy string) (*ImportResult, error) {
	codec := codec.NewAnsibleCodec()
	fragment, err := codec.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse Ansible inventory: %w", err)
	}

	return s.importFragment(ctx, fragment, strategy)
}

// importFragment imports a graph fragment with the specified strategy
func (s *GraphService) importFragment(ctx context.Context, fragment *domain.GraphFragment, strategy string) (*ImportResult, error) {
	if strategy == "" {
		strategy = "merge"
	}

	if strategy != "merge" && strategy != "replace" {
		return nil, fmt.Errorf("invalid strategy %s, must be 'merge' or 'replace'", strategy)
	}

	counts, err := s.repo.ImportFragment(ctx, fragment, strategy)
	if err != nil {
		return nil, err
	}

	result := &ImportResult{
		NodesCreated: counts["nodes_created"],
		NodesUpdated: counts["nodes_updated"],
		EdgesCreated: counts["edges_created"],
		EdgesUpdated: counts["edges_updated"],
		Strategy:     strategy,
	}

	s.eventBus.Publish(Event{
		Type:    EventGraphUpdated,
		Payload: result,
	})

	return result, nil
}

// ExportJSON exports the graph as JSON
func (s *GraphService) ExportJSON(ctx context.Context) ([]byte, error) {
	fragment, err := s.repo.ExportFragment(ctx)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	codec := codec.NewJSONCodec()
	if err := codec.Export(fragment, &buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// ExportYAML exports the graph as YAML
func (s *GraphService) ExportYAML(ctx context.Context, w io.Writer) error {
	fragment, err := s.repo.ExportFragment(ctx)
	if err != nil {
		return err
	}

	codec := codec.NewYAMLCodec()
	return codec.Export(fragment, w)
}

// ExportAnsibleInventory exports the graph as Ansible inventory
func (s *GraphService) ExportAnsibleInventory(ctx context.Context, w io.Writer) error {
	fragment, err := s.repo.ExportFragment(ctx)
	if err != nil {
		return err
	}

	codec := codec.NewAnsibleCodec()
	return codec.Export(fragment, w)
}

// ClearGraph removes all nodes, edges, and positions
func (s *GraphService) ClearGraph(ctx context.Context) error {
	if err := s.repo.ClearGraph(ctx); err != nil {
		return err
	}

	s.eventBus.Publish(Event{
		Type:    EventGraphUpdated,
		Payload: map[string]string{"action": "cleared"},
	})

	return nil
}

// Validation helpers

func (s *GraphService) validateNode(node *domain.Node) error {
	if node.ID == "" {
		return fmt.Errorf("node ID required")
	}
	if node.Type == "" {
		return fmt.Errorf("node type required")
	}
	if node.Label == "" {
		return fmt.Errorf("node label required")
	}
	return nil
}

func (s *GraphService) validateEdge(edge *domain.Edge) error {
	if edge.FromID == "" {
		return fmt.Errorf("edge from_id required")
	}
	if edge.ToID == "" {
		return fmt.Errorf("edge to_id required")
	}
	if edge.Type == "" {
		return fmt.Errorf("edge type required")
	}
	if edge.FromID == edge.ToID {
		return fmt.Errorf("edge from_id and to_id cannot be the same")
	}
	return nil
}
