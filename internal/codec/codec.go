package codec

import (
	"io"

	"specularium/internal/domain"
)

// Importer interface for importing graph data from various formats
type Importer interface {
	Parse(r io.Reader) (*domain.GraphFragment, error)
	Format() string
}

// Exporter interface for exporting graph data to various formats
type Exporter interface {
	Export(fragment *domain.GraphFragment, w io.Writer) error
	Format() string
}
