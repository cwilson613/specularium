package codec

import (
	"encoding/json"
	"fmt"
	"io"

	"specularium/internal/domain"
)

// JSONCodec handles JSON import/export
type JSONCodec struct{}

// NewJSONCodec creates a new JSON codec
func NewJSONCodec() *JSONCodec {
	return &JSONCodec{}
}

// Format returns the codec format identifier
func (c *JSONCodec) Format() string {
	return "json"
}

// Parse imports graph data from JSON
func (c *JSONCodec) Parse(r io.Reader) (*domain.GraphFragment, error) {
	var fragment domain.GraphFragment
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&fragment); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return &fragment, nil
}

// Export exports graph data to JSON
func (c *JSONCodec) Export(fragment *domain.GraphFragment, w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(fragment); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}
