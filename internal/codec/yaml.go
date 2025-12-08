package codec

import (
	"fmt"
	"io"

	"specularium/internal/domain"

	"gopkg.in/yaml.v3"
)

// YAMLCodec handles generic YAML import/export
type YAMLCodec struct{}

// NewYAMLCodec creates a new YAML codec
func NewYAMLCodec() *YAMLCodec {
	return &YAMLCodec{}
}

// Format returns the codec format identifier
func (c *YAMLCodec) Format() string {
	return "yaml"
}

// yamlFragment represents the YAML structure for graph data
type yamlFragment struct {
	Nodes []yamlNode `yaml:"nodes"`
	Edges []yamlEdge `yaml:"edges"`
}

type yamlNode struct {
	ID         string         `yaml:"id"`
	Type       string         `yaml:"type"`
	Label      string         `yaml:"label"`
	Properties map[string]any `yaml:"properties,omitempty"`
	Source     string         `yaml:"source,omitempty"`
}

type yamlEdge struct {
	ID         string         `yaml:"id,omitempty"`
	FromID     string         `yaml:"from_id"`
	ToID       string         `yaml:"to_id"`
	Type       string         `yaml:"type"`
	Properties map[string]any `yaml:"properties,omitempty"`
}

// Parse imports graph data from YAML
func (c *YAMLCodec) Parse(r io.Reader) (*domain.GraphFragment, error) {
	var yf yamlFragment
	decoder := yaml.NewDecoder(r)
	if err := decoder.Decode(&yf); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	fragment := domain.NewGraphFragment()

	// Convert nodes
	for _, yn := range yf.Nodes {
		node := domain.Node{
			ID:         yn.ID,
			Type:       domain.NodeType(yn.Type),
			Label:      yn.Label,
			Properties: yn.Properties,
			Source:     yn.Source,
		}
		if node.Properties == nil {
			node.Properties = make(map[string]any)
		}
		fragment.AddNode(node)
	}

	// Convert edges
	for _, ye := range yf.Edges {
		edge := domain.Edge{
			ID:         ye.ID,
			FromID:     ye.FromID,
			ToID:       ye.ToID,
			Type:       domain.EdgeType(ye.Type),
			Properties: ye.Properties,
		}
		if edge.Properties == nil {
			edge.Properties = make(map[string]any)
		}
		if edge.ID == "" {
			edge.ID = edge.GenerateID()
		}
		fragment.AddEdge(edge)
	}

	return fragment, nil
}

// Export exports graph data to YAML
func (c *YAMLCodec) Export(fragment *domain.GraphFragment, w io.Writer) error {
	yf := yamlFragment{
		Nodes: make([]yamlNode, 0, len(fragment.Nodes)),
		Edges: make([]yamlEdge, 0, len(fragment.Edges)),
	}

	// Convert nodes
	for _, node := range fragment.Nodes {
		yn := yamlNode{
			ID:         node.ID,
			Type:       string(node.Type),
			Label:      node.Label,
			Properties: node.Properties,
			Source:     node.Source,
		}
		yf.Nodes = append(yf.Nodes, yn)
	}

	// Convert edges
	for _, edge := range fragment.Edges {
		ye := yamlEdge{
			ID:         edge.ID,
			FromID:     edge.FromID,
			ToID:       edge.ToID,
			Type:       string(edge.Type),
			Properties: edge.Properties,
		}
		yf.Edges = append(yf.Edges, ye)
	}

	encoder := yaml.NewEncoder(w)
	encoder.SetIndent(2)
	defer encoder.Close()

	if err := encoder.Encode(&yf); err != nil {
		return fmt.Errorf("failed to encode YAML: %w", err)
	}

	return nil
}
