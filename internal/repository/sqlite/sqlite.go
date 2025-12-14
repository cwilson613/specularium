package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"specularium/internal/domain"

	_ "modernc.org/sqlite" // Pure-Go SQLite driver (no CGO)
)

// Repository implements repository operations using SQLite
type Repository struct {
	db *sql.DB
}

// New creates a new SQLite repository
func New(dbPath string) (*Repository, error) {
	// Pure-Go driver uses "sqlite" and _pragma=name(value) syntax
	dsn := dbPath + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	repo := &Repository{db: db}
	if err := repo.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return repo, nil
}

func (r *Repository) migrate() error {
	// Create tables if they don't exist
	schema := `
	CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		label TEXT NOT NULL,
		properties TEXT,
		source TEXT,
		status TEXT DEFAULT 'unverified',
		last_verified DATETIME,
		last_seen DATETIME,
		discovered TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS edges (
		id TEXT PRIMARY KEY,
		from_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
		to_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
		type TEXT NOT NULL,
		properties TEXT
	);

	CREATE TABLE IF NOT EXISTS node_positions (
		node_id TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
		x REAL NOT NULL,
		y REAL NOT NULL,
		pinned INTEGER DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS discrepancies (
		id TEXT PRIMARY KEY,
		node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
		property_key TEXT NOT NULL,
		truth_value TEXT,
		actual_value TEXT,
		source TEXT NOT NULL,
		detected_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		resolved_at DATETIME,
		resolution TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_nodes_type ON nodes(type);
	CREATE INDEX IF NOT EXISTS idx_nodes_source ON nodes(source);
	CREATE INDEX IF NOT EXISTS idx_edges_from ON edges(from_id);
	CREATE INDEX IF NOT EXISTS idx_edges_to ON edges(to_id);
	CREATE INDEX IF NOT EXISTS idx_discrepancies_node ON discrepancies(node_id);
	`

	if _, err := r.db.Exec(schema); err != nil {
		return err
	}

	// Run migrations for existing databases - check if columns exist first
	r.addColumnIfNotExists("nodes", "status", "TEXT DEFAULT 'unverified'")
	r.addColumnIfNotExists("nodes", "last_verified", "DATETIME")
	r.addColumnIfNotExists("nodes", "last_seen", "DATETIME")
	r.addColumnIfNotExists("nodes", "discovered", "TEXT")

	// Truth columns
	r.addColumnIfNotExists("nodes", "truth", "TEXT")
	r.addColumnIfNotExists("nodes", "truth_status", "TEXT DEFAULT ''")
	r.addColumnIfNotExists("nodes", "has_discrepancy", "INTEGER DEFAULT 0")

	// Parent-child relationship for interface nodes
	r.addColumnIfNotExists("nodes", "parent_id", "TEXT")

	// Capabilities column for Evidence Model
	r.addColumnIfNotExists("nodes", "capabilities", "TEXT")

	// Create indexes if not exists
	r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_status ON nodes(status)`)
	r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_parent ON nodes(parent_id)`)
	r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_truth_status ON nodes(truth_status)`)
	r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_discrepancies_unresolved ON discrepancies(node_id) WHERE resolved_at IS NULL`)

	// Secrets table for operator-created secrets
	secretsSchema := `
	CREATE TABLE IF NOT EXISTS secrets (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		source TEXT NOT NULL DEFAULT 'operator',
		description TEXT,
		data TEXT,
		metadata TEXT,
		immutable INTEGER DEFAULT 0,
		status TEXT DEFAULT 'unknown',
		status_message TEXT,
		usage_count INTEGER DEFAULT 0,
		last_used_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_secrets_type ON secrets(type);
	CREATE INDEX IF NOT EXISTS idx_secrets_source ON secrets(source);
	`
	r.db.Exec(secretsSchema)

	return nil
}

// addColumnIfNotExists adds a column to a table if it doesn't already exist
func (r *Repository) addColumnIfNotExists(table, column, colType string) {
	// Check if column exists by querying table info
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&count)
	if err != nil || count > 0 {
		// Column exists or error checking - skip
		return
	}

	// Add the column
	query := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, colType)
	r.db.Exec(query)
}

// GetGraph returns the complete graph with nodes, edges, and positions
func (r *Repository) GetGraph(ctx context.Context) (*domain.Graph, error) {
	graph := domain.NewGraph()

	// Load nodes
	nodes, err := r.ListNodes(ctx, "", "")
	if err != nil {
		return nil, err
	}
	graph.Nodes = nodes

	// Load edges
	edges, err := r.ListEdges(ctx, "", "", "")
	if err != nil {
		return nil, err
	}
	graph.Edges = edges

	// Load positions
	positions, err := r.GetAllPositions(ctx)
	if err != nil {
		return nil, err
	}
	graph.Positions = positions

	return graph, nil
}

// GetNode retrieves a single node by ID
func (r *Repository) GetNode(ctx context.Context, id string) (*domain.Node, error) {
	var row nodeRow
	row.ID = id

	err := r.db.QueryRowContext(ctx,
		`SELECT `+nodeColumns+` FROM nodes WHERE id = ?`, id,
	).Scan(row.scanArgs()...)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query node: %w", err)
	}

	return row.toDomain()
}

// ListNodes returns all nodes, optionally filtered by type or source
func (r *Repository) ListNodes(ctx context.Context, nodeType, source string) ([]domain.Node, error) {
	query := "SELECT " + nodeColumns + " FROM nodes WHERE 1=1"
	args := make([]interface{}, 0)

	if nodeType != "" {
		query += " AND type = ?"
		args = append(args, nodeType)
	}
	if source != "" {
		query += " AND source = ?"
		args = append(args, source)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query nodes: %w", err)
	}
	defer rows.Close()

	return scanNodeRows(rows)
}

// scanNodeRows scans multiple node rows into a slice
func scanNodeRows(rows *sql.Rows) ([]domain.Node, error) {
	nodes := make([]domain.Node, 0)
	for rows.Next() {
		var row nodeRow
		if err := rows.Scan(row.scanArgs()...); err != nil {
			return nil, fmt.Errorf("scan node: %w", err)
		}
		node, err := row.toDomain()
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, *node)
	}
	return nodes, rows.Err()
}

// CreateNode creates a new node
func (r *Repository) CreateNode(ctx context.Context, node *domain.Node) error {
	// Check if node already exists
	existing, err := r.GetNode(ctx, node.ID)
	if err != nil {
		return err
	}
	if existing != nil {
		return fmt.Errorf("node %s already exists", node.ID)
	}

	return r.UpsertNode(ctx, node)
}

// UpsertNode inserts or updates a node
func (r *Repository) UpsertNode(ctx context.Context, node *domain.Node) error {
	now := time.Now()
	if node.CreatedAt.IsZero() {
		node.CreatedAt = now
	}
	node.UpdatedAt = now

	if node.Status == "" {
		node.Status = domain.NodeStatusUnverified
	}

	args, err := nodeInsertArgs(node)
	if err != nil {
		return fmt.Errorf("prepare node args: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO nodes (id, type, label, parent_id, properties, source, status, last_verified, last_seen, discovered, capabilities, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			type = excluded.type,
			label = excluded.label,
			parent_id = excluded.parent_id,
			properties = excluded.properties,
			source = excluded.source,
			status = excluded.status,
			last_verified = excluded.last_verified,
			last_seen = excluded.last_seen,
			discovered = excluded.discovered,
			capabilities = excluded.capabilities,
			updated_at = excluded.updated_at
	`, args...)

	if err != nil {
		return fmt.Errorf("upsert node: %w", err)
	}

	return nil
}

// UpdateNode updates an existing node (partial update)
func (r *Repository) UpdateNode(ctx context.Context, id string, updates map[string]interface{}) error {
	// Get existing node
	existing, err := r.GetNode(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("node %s not found", id)
	}

	// Apply updates
	if label, ok := updates["label"].(string); ok && label != "" {
		existing.Label = label
	}
	if nodeType, ok := updates["type"].(string); ok && nodeType != "" {
		existing.Type = domain.NodeType(nodeType)
	}
	if source, ok := updates["source"].(string); ok {
		existing.Source = source
	}
	if parentID, ok := updates["parent_id"].(string); ok {
		existing.ParentID = parentID
	}
	if props, ok := updates["properties"].(map[string]interface{}); ok {
		if existing.Properties == nil {
			existing.Properties = make(map[string]any)
		}
		for k, v := range props {
			if v == nil {
				delete(existing.Properties, k)
			} else {
				existing.Properties[k] = v
			}
		}
	}
	if discovered, ok := updates["discovered"].(map[string]any); ok {
		if existing.Discovered == nil {
			existing.Discovered = make(map[string]any)
		}
		for k, v := range discovered {
			if v == nil {
				delete(existing.Discovered, k)
			} else {
				existing.Discovered[k] = v
			}
		}
	}
	if capabilities, ok := updates["capabilities"].(map[string]interface{}); ok {
		// Convert to map[CapabilityType]*Capability
		if existing.Capabilities == nil {
			existing.Capabilities = make(map[domain.CapabilityType]*domain.Capability)
		}
		// This allows full replacement of capabilities map
		for k, v := range capabilities {
			if v == nil {
				delete(existing.Capabilities, domain.CapabilityType(k))
			} else {
				if cap, ok := v.(*domain.Capability); ok {
					existing.Capabilities[domain.CapabilityType(k)] = cap
				}
			}
		}
	}
	if lastSeen, ok := updates["last_seen"].(time.Time); ok {
		existing.LastSeen = &lastSeen
	}

	return r.UpsertNode(ctx, existing)
}

// DeleteNode removes a node and its associated edges
func (r *Repository) DeleteNode(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete node: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("node %s not found", id)
	}

	return nil
}

// GetEdge retrieves a single edge by ID
func (r *Repository) GetEdge(ctx context.Context, id string) (*domain.Edge, error) {
	var row edgeRow

	err := r.db.QueryRowContext(ctx,
		`SELECT `+edgeColumns+` FROM edges WHERE id = ?`, id,
	).Scan(row.scanArgs()...)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query edge: %w", err)
	}

	return row.toDomain()
}

// ListEdges returns all edges, optionally filtered
func (r *Repository) ListEdges(ctx context.Context, edgeType, fromID, toID string) ([]domain.Edge, error) {
	query := "SELECT " + edgeColumns + " FROM edges WHERE 1=1"
	args := make([]interface{}, 0)

	if edgeType != "" {
		query += " AND type = ?"
		args = append(args, edgeType)
	}
	if fromID != "" {
		query += " AND from_id = ?"
		args = append(args, fromID)
	}
	if toID != "" {
		query += " AND to_id = ?"
		args = append(args, toID)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query edges: %w", err)
	}
	defer rows.Close()

	return scanEdgeRows(rows)
}

// scanEdgeRows scans multiple edge rows into a slice
func scanEdgeRows(rows *sql.Rows) ([]domain.Edge, error) {
	edges := make([]domain.Edge, 0)
	for rows.Next() {
		var row edgeRow
		if err := rows.Scan(row.scanArgs()...); err != nil {
			return nil, fmt.Errorf("scan edge: %w", err)
		}
		edge, err := row.toDomain()
		if err != nil {
			return nil, err
		}
		edges = append(edges, *edge)
	}
	return edges, rows.Err()
}

// CreateEdge creates a new edge
func (r *Repository) CreateEdge(ctx context.Context, edge *domain.Edge) error {
	// Verify both endpoints exist
	from, err := r.GetNode(ctx, edge.FromID)
	if err != nil {
		return err
	}
	if from == nil {
		return fmt.Errorf("from node %s not found", edge.FromID)
	}

	to, err := r.GetNode(ctx, edge.ToID)
	if err != nil {
		return err
	}
	if to == nil {
		return fmt.Errorf("to node %s not found", edge.ToID)
	}

	// Generate ID if not provided
	if edge.ID == "" {
		edge.ID = edge.GenerateID()
	}

	return r.UpsertEdge(ctx, edge)
}

// UpsertEdge inserts or updates an edge
func (r *Repository) UpsertEdge(ctx context.Context, edge *domain.Edge) error {
	args, err := edgeInsertArgs(edge)
	if err != nil {
		return fmt.Errorf("prepare edge args: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO edges (id, from_id, to_id, type, properties)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			from_id = excluded.from_id,
			to_id = excluded.to_id,
			type = excluded.type,
			properties = excluded.properties
	`, args...)

	if err != nil {
		return fmt.Errorf("upsert edge: %w", err)
	}

	return nil
}

// UpdateEdge updates an existing edge (partial update)
func (r *Repository) UpdateEdge(ctx context.Context, id string, updates map[string]interface{}) error {
	// Get existing edge
	existing, err := r.GetEdge(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("edge %s not found", id)
	}

	// Apply updates
	if edgeType, ok := updates["type"].(string); ok && edgeType != "" {
		existing.Type = domain.EdgeType(edgeType)
	}
	if props, ok := updates["properties"].(map[string]interface{}); ok {
		if existing.Properties == nil {
			existing.Properties = make(map[string]any)
		}
		for k, v := range props {
			if v == nil {
				delete(existing.Properties, k)
			} else {
				existing.Properties[k] = v
			}
		}
	}

	return r.UpsertEdge(ctx, existing)
}

// DeleteEdge removes an edge
func (r *Repository) DeleteEdge(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM edges WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete edge: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("edge %s not found", id)
	}

	return nil
}

// GetAllPositions returns all node positions
func (r *Repository) GetAllPositions(ctx context.Context) (map[string]domain.NodePosition, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT node_id, x, y, pinned FROM node_positions
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query positions: %w", err)
	}
	defer rows.Close()

	positions := make(map[string]domain.NodePosition)
	for rows.Next() {
		var nodeID string
		var x, y float64
		var pinned int

		if err := rows.Scan(&nodeID, &x, &y, &pinned); err != nil {
			return nil, fmt.Errorf("failed to scan position: %w", err)
		}

		positions[nodeID] = domain.NodePosition{
			NodeID: nodeID,
			X:      x,
			Y:      y,
			Pinned: pinned != 0,
		}
	}

	return positions, rows.Err()
}

// GetPosition retrieves a single node position
func (r *Repository) GetPosition(ctx context.Context, nodeID string) (*domain.NodePosition, error) {
	var x, y float64
	var pinned int

	err := r.db.QueryRowContext(ctx, `
		SELECT x, y, pinned FROM node_positions WHERE node_id = ?
	`, nodeID).Scan(&x, &y, &pinned)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query position: %w", err)
	}

	return &domain.NodePosition{
		NodeID: nodeID,
		X:      x,
		Y:      y,
		Pinned: pinned != 0,
	}, nil
}

// SavePosition saves or updates a single node position
func (r *Repository) SavePosition(ctx context.Context, pos domain.NodePosition) error {
	pinnedInt := 0
	if pos.Pinned {
		pinnedInt = 1
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO node_positions (node_id, x, y, pinned)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(node_id) DO UPDATE SET
			x = excluded.x,
			y = excluded.y,
			pinned = excluded.pinned
	`, pos.NodeID, pos.X, pos.Y, pinnedInt)

	if err != nil {
		return fmt.Errorf("failed to save position: %w", err)
	}

	return nil
}

// SavePositions saves multiple node positions
func (r *Repository) SavePositions(ctx context.Context, positions []domain.NodePosition) error {
	if len(positions) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO node_positions (node_id, x, y, pinned)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(node_id) DO UPDATE SET
			x = excluded.x,
			y = excluded.y,
			pinned = excluded.pinned
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, pos := range positions {
		pinnedInt := 0
		if pos.Pinned {
			pinnedInt = 1
		}

		if _, err := stmt.ExecContext(ctx, pos.NodeID, pos.X, pos.Y, pinnedInt); err != nil {
			return fmt.Errorf("failed to save position for %s: %w", pos.NodeID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// ImportFragment imports a graph fragment with the specified strategy
func (r *Repository) ImportFragment(ctx context.Context, fragment *domain.GraphFragment, strategy string) (map[string]int, error) {
	result := map[string]int{
		"nodes_created": 0,
		"nodes_updated": 0,
		"edges_created": 0,
		"edges_updated": 0,
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// If replace strategy, clear all data first
	if strategy == "replace" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM node_positions`); err != nil {
			return nil, fmt.Errorf("failed to clear positions: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM edges`); err != nil {
			return nil, fmt.Errorf("failed to clear edges: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM nodes`); err != nil {
			return nil, fmt.Errorf("failed to clear nodes: %w", err)
		}
	}

	// Import nodes
	for _, node := range fragment.Nodes {
		// Check if node exists (for merge strategy)
		var exists bool
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM nodes WHERE id = ?`, node.ID).Scan(&exists)
		isUpdate := err == nil && exists

		var propertiesJSON sql.NullString
		if node.Properties != nil && len(node.Properties) > 0 {
			data, err := json.Marshal(node.Properties)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal node properties: %w", err)
			}
			propertiesJSON = sql.NullString{String: string(data), Valid: true}
		}

		now := time.Now()
		if node.CreatedAt.IsZero() {
			node.CreatedAt = now
		}
		node.UpdatedAt = now

		_, err = tx.ExecContext(ctx, `
			INSERT INTO nodes (id, type, label, properties, source, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				type = excluded.type,
				label = excluded.label,
				properties = excluded.properties,
				source = excluded.source,
				updated_at = excluded.updated_at
		`, node.ID, node.Type, node.Label, propertiesJSON, node.Source, node.CreatedAt, node.UpdatedAt)

		if err != nil {
			return nil, fmt.Errorf("failed to import node %s: %w", node.ID, err)
		}

		if isUpdate {
			result["nodes_updated"]++
		} else {
			result["nodes_created"]++
		}
	}

	// Import edges
	for _, edge := range fragment.Edges {
		// Generate ID if not provided
		if edge.ID == "" {
			edge.ID = edge.GenerateID()
		}

		// Check if edge exists (for merge strategy)
		var exists bool
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM edges WHERE id = ?`, edge.ID).Scan(&exists)
		isUpdate := err == nil && exists

		var propertiesJSON sql.NullString
		if edge.Properties != nil && len(edge.Properties) > 0 {
			data, err := json.Marshal(edge.Properties)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal edge properties: %w", err)
			}
			propertiesJSON = sql.NullString{String: string(data), Valid: true}
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO edges (id, from_id, to_id, type, properties)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				from_id = excluded.from_id,
				to_id = excluded.to_id,
				type = excluded.type,
				properties = excluded.properties
		`, edge.ID, edge.FromID, edge.ToID, edge.Type, propertiesJSON)

		if err != nil {
			return nil, fmt.Errorf("failed to import edge %s: %w", edge.ID, err)
		}

		if isUpdate {
			result["edges_updated"]++
		} else {
			result["edges_created"]++
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return result, nil
}

// ExportFragment exports all nodes and edges as a fragment
func (r *Repository) ExportFragment(ctx context.Context) (*domain.GraphFragment, error) {
	fragment := domain.NewGraphFragment()

	nodes, err := r.ListNodes(ctx, "", "")
	if err != nil {
		return nil, err
	}
	fragment.Nodes = nodes

	edges, err := r.ListEdges(ctx, "", "", "")
	if err != nil {
		return nil, err
	}
	fragment.Edges = edges

	return fragment, nil
}

// Close closes the database connection
func (r *Repository) Close() error {
	return r.db.Close()
}

// GetNodesForVerification returns nodes that need verification
// This includes unverified nodes and nodes that haven't been verified recently
func (r *Repository) GetNodesForVerification(ctx context.Context) ([]domain.Node, error) {
	query := `SELECT ` + nodeColumns + ` FROM nodes
		WHERE status = 'unverified'
		   OR status = 'verifying'
		   OR last_verified IS NULL
		   OR last_verified < datetime('now', '-5 minutes')`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query nodes for verification: %w", err)
	}
	defer rows.Close()

	return scanNodeRows(rows)
}

// UpdateNodeVerification updates only the verification-related fields of a node
func (r *Repository) UpdateNodeVerification(ctx context.Context, nodeID string, status domain.NodeStatus, lastVerified, lastSeen *time.Time, discovered map[string]any) error {
	var discoveredJSON sql.NullString
	if discovered != nil && len(discovered) > 0 {
		data, err := json.Marshal(discovered)
		if err != nil {
			return fmt.Errorf("failed to marshal discovered: %w", err)
		}
		discoveredJSON = sql.NullString{String: string(data), Valid: true}
	}

	var lastVerifiedSQL, lastSeenSQL sql.NullTime
	if lastVerified != nil {
		lastVerifiedSQL = sql.NullTime{Time: *lastVerified, Valid: true}
	}
	if lastSeen != nil {
		lastSeenSQL = sql.NullTime{Time: *lastSeen, Valid: true}
	}

	_, err := r.db.ExecContext(ctx, `
		UPDATE nodes
		SET status = ?, last_verified = ?, last_seen = ?, discovered = ?, updated_at = ?
		WHERE id = ?
	`, status, lastVerifiedSQL, lastSeenSQL, discoveredJSON, time.Now(), nodeID)

	if err != nil {
		return fmt.Errorf("failed to update node verification: %w", err)
	}

	return nil
}

// UpdateNodeLabel updates only the label of a node
func (r *Repository) UpdateNodeLabel(ctx context.Context, nodeID string, label string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE nodes
		SET label = ?, updated_at = ?
		WHERE id = ?
	`, label, time.Now(), nodeID)

	if err != nil {
		return fmt.Errorf("failed to update node label: %w", err)
	}

	return nil
}

// HasOperatorTruthHostname checks if the node has an operator-asserted hostname
func (r *Repository) HasOperatorTruthHostname(ctx context.Context, nodeID string) (bool, error) {
	var truthJSON sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT truth FROM nodes WHERE id = ?`, nodeID).Scan(&truthJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}

	if !truthJSON.Valid || truthJSON.String == "" {
		return false, nil
	}

	var truth domain.NodeTruth
	if err := json.Unmarshal([]byte(truthJSON.String), &truth); err != nil {
		return false, nil
	}

	// Check if hostname is in truth properties
	if truth.Properties != nil {
		if _, hasHostname := truth.Properties["hostname"]; hasHostname {
			return true, nil
		}
		// Also check for label truth
		if _, hasLabel := truth.Properties["label"]; hasLabel {
			return true, nil
		}
	}

	return false, nil
}

// ClearGraph removes all nodes, edges, and positions from the database
func (r *Repository) ClearGraph(ctx context.Context) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete in order due to foreign key constraints
	if _, err := tx.ExecContext(ctx, `DELETE FROM node_positions`); err != nil {
		return fmt.Errorf("failed to clear positions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM edges`); err != nil {
		return fmt.Errorf("failed to clear edges: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM nodes`); err != nil {
		return fmt.Errorf("failed to clear nodes: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM discrepancies`); err != nil {
		return fmt.Errorf("failed to clear discrepancies: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// SetNodeTruth sets or updates the operator truth for a node
func (r *Repository) SetNodeTruth(ctx context.Context, nodeID string, truth *domain.NodeTruth) error {
	var truthJSON sql.NullString
	if truth != nil {
		data, err := json.Marshal(truth)
		if err != nil {
			return fmt.Errorf("failed to marshal truth: %w", err)
		}
		truthJSON = sql.NullString{String: string(data), Valid: true}
	}

	_, err := r.db.ExecContext(ctx, `
		UPDATE nodes
		SET truth = ?, truth_status = ?, updated_at = ?
		WHERE id = ?
	`, truthJSON, domain.TruthStatusAsserted, time.Now(), nodeID)

	if err != nil {
		return fmt.Errorf("failed to set node truth: %w", err)
	}

	return nil
}

// ClearNodeTruth removes the operator truth from a node
func (r *Repository) ClearNodeTruth(ctx context.Context, nodeID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE nodes
		SET truth = NULL, truth_status = '', has_discrepancy = 0, updated_at = ?
		WHERE id = ?
	`, time.Now(), nodeID)

	if err != nil {
		return fmt.Errorf("failed to clear node truth: %w", err)
	}

	// Also resolve any open discrepancies for this node
	_, err = r.db.ExecContext(ctx, `
		UPDATE discrepancies
		SET resolved_at = ?, resolution = 'truth_cleared'
		WHERE node_id = ? AND resolved_at IS NULL
	`, time.Now(), nodeID)

	return err
}

// GetNodesWithTruth returns all nodes that have operator truth set
func (r *Repository) GetNodesWithTruth(ctx context.Context) ([]domain.Node, error) {
	query := `SELECT ` + nodeColumns + ` FROM nodes
		WHERE truth_status = 'asserted' OR truth_status = 'conflict'`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query nodes with truth: %w", err)
	}
	defer rows.Close()

	return scanNodeRows(rows)
}

// UpdateNodeDiscrepancyStatus updates the has_discrepancy flag and truth_status
func (r *Repository) UpdateNodeDiscrepancyStatus(ctx context.Context, nodeID string, hasDiscrepancy bool) error {
	truthStatus := domain.TruthStatusAsserted
	if hasDiscrepancy {
		truthStatus = domain.TruthStatusConflict
	}

	_, err := r.db.ExecContext(ctx, `
		UPDATE nodes
		SET has_discrepancy = ?, truth_status = ?, updated_at = ?
		WHERE id = ? AND truth IS NOT NULL
	`, hasDiscrepancy, truthStatus, time.Now(), nodeID)

	return err
}

// CreateDiscrepancy creates a new discrepancy record
func (r *Repository) CreateDiscrepancy(ctx context.Context, d *domain.Discrepancy) error {
	truthValueJSON, _ := json.Marshal(d.TruthValue)
	actualValueJSON, _ := json.Marshal(d.ActualValue)

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO discrepancies (id, node_id, property_key, truth_value, actual_value, source, detected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, d.ID, d.NodeID, d.PropertyKey, string(truthValueJSON), string(actualValueJSON), d.Source, d.DetectedAt)

	if err != nil {
		return fmt.Errorf("failed to create discrepancy: %w", err)
	}

	// Update node's has_discrepancy flag
	return r.UpdateNodeDiscrepancyStatus(ctx, d.NodeID, true)
}

// GetDiscrepancy retrieves a single discrepancy by ID
func (r *Repository) GetDiscrepancy(ctx context.Context, id string) (*domain.Discrepancy, error) {
	var (
		nodeID, propertyKey, source     string
		truthValueJSON, actualValueJSON sql.NullString
		detectedAt                      time.Time
		resolvedAt                      sql.NullTime
		resolution                      sql.NullString
	)

	err := r.db.QueryRowContext(ctx, `
		SELECT node_id, property_key, truth_value, actual_value, source, detected_at, resolved_at, resolution
		FROM discrepancies WHERE id = ?
	`, id).Scan(&nodeID, &propertyKey, &truthValueJSON, &actualValueJSON, &source, &detectedAt, &resolvedAt, &resolution)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query discrepancy: %w", err)
	}

	d := &domain.Discrepancy{
		ID:          id,
		NodeID:      nodeID,
		PropertyKey: propertyKey,
		Source:      source,
		DetectedAt:  detectedAt,
		Resolution:  resolution.String,
	}

	if resolvedAt.Valid {
		d.ResolvedAt = &resolvedAt.Time
	}

	if truthValueJSON.Valid {
		json.Unmarshal([]byte(truthValueJSON.String), &d.TruthValue)
	}
	if actualValueJSON.Valid {
		json.Unmarshal([]byte(actualValueJSON.String), &d.ActualValue)
	}

	return d, nil
}

// GetDiscrepanciesByNode returns all discrepancies for a specific node
func (r *Repository) GetDiscrepanciesByNode(ctx context.Context, nodeID string) ([]domain.Discrepancy, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, node_id, property_key, truth_value, actual_value, source, detected_at, resolved_at, resolution
		FROM discrepancies
		WHERE node_id = ?
		ORDER BY detected_at DESC
	`, nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to query discrepancies: %w", err)
	}
	defer rows.Close()

	return r.scanDiscrepancies(rows)
}

// GetUnresolvedDiscrepancies returns all unresolved discrepancies
func (r *Repository) GetUnresolvedDiscrepancies(ctx context.Context) ([]domain.Discrepancy, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, node_id, property_key, truth_value, actual_value, source, detected_at, resolved_at, resolution
		FROM discrepancies
		WHERE resolved_at IS NULL
		ORDER BY detected_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query unresolved discrepancies: %w", err)
	}
	defer rows.Close()

	return r.scanDiscrepancies(rows)
}

// ResolveDiscrepancy marks a discrepancy as resolved
func (r *Repository) ResolveDiscrepancy(ctx context.Context, id string, resolution string) error {
	// Get the discrepancy first to find the node
	d, err := r.GetDiscrepancy(ctx, id)
	if err != nil {
		return err
	}
	if d == nil {
		return fmt.Errorf("discrepancy not found: %s", id)
	}

	_, err = r.db.ExecContext(ctx, `
		UPDATE discrepancies
		SET resolved_at = ?, resolution = ?
		WHERE id = ?
	`, time.Now(), resolution, id)

	if err != nil {
		return fmt.Errorf("failed to resolve discrepancy: %w", err)
	}

	// Check if node has any remaining unresolved discrepancies
	var count int
	err = r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM discrepancies
		WHERE node_id = ? AND resolved_at IS NULL
	`, d.NodeID).Scan(&count)

	if err != nil {
		return err
	}

	// Update node's discrepancy status
	return r.UpdateNodeDiscrepancyStatus(ctx, d.NodeID, count > 0)
}

// scanDiscrepancies is a helper to scan rows into Discrepancy slice
func (r *Repository) scanDiscrepancies(rows *sql.Rows) ([]domain.Discrepancy, error) {
	discrepancies := make([]domain.Discrepancy, 0)
	for rows.Next() {
		var (
			id, nodeID, propertyKey, source string
			truthValueJSON, actualValueJSON sql.NullString
			detectedAt                      time.Time
			resolvedAt                      sql.NullTime
			resolution                      sql.NullString
		)

		if err := rows.Scan(&id, &nodeID, &propertyKey, &truthValueJSON, &actualValueJSON, &source, &detectedAt, &resolvedAt, &resolution); err != nil {
			return nil, fmt.Errorf("failed to scan discrepancy: %w", err)
		}

		d := domain.Discrepancy{
			ID:          id,
			NodeID:      nodeID,
			PropertyKey: propertyKey,
			Source:      source,
			DetectedAt:  detectedAt,
			Resolution:  resolution.String,
		}

		if resolvedAt.Valid {
			d.ResolvedAt = &resolvedAt.Time
		}

		if truthValueJSON.Valid {
			json.Unmarshal([]byte(truthValueJSON.String), &d.TruthValue)
		}
		if actualValueJSON.Valid {
			json.Unmarshal([]byte(actualValueJSON.String), &d.ActualValue)
		}

		discrepancies = append(discrepancies, d)
	}

	return discrepancies, rows.Err()
}

// ==================== Secrets Repository Methods ====================

// CreateSecret creates a new operator secret
func (r *Repository) CreateSecret(ctx context.Context, secret *domain.Secret) error {
	dataJSON, err := json.Marshal(secret.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal secret data: %w", err)
	}

	metadataJSON, err := json.Marshal(secret.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal secret metadata: %w", err)
	}

	now := time.Now()
	secret.CreatedAt = now
	secret.UpdatedAt = now

	query := `
		INSERT INTO secrets (id, name, type, source, description, data, metadata, immutable, status, status_message, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = r.db.ExecContext(ctx, query,
		secret.ID,
		secret.Name,
		string(secret.Type),
		string(secret.Source),
		secret.Description,
		string(dataJSON),
		string(metadataJSON),
		boolToInt(secret.Immutable),
		string(secret.Status),
		secret.StatusMessage,
		secret.CreatedAt,
		secret.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	return nil
}

// GetSecret retrieves a secret by ID
func (r *Repository) GetSecret(ctx context.Context, id string) (*domain.Secret, error) {
	query := `
		SELECT id, name, type, source, description, data, metadata, immutable, status, status_message, usage_count, last_used_at, created_at, updated_at
		FROM secrets WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, query, id)

	var secret domain.Secret
	var dataJSON, metadataJSON sql.NullString
	var immutable int
	var lastUsedAt sql.NullTime

	err := row.Scan(
		&secret.ID,
		&secret.Name,
		&secret.Type,
		&secret.Source,
		&secret.Description,
		&dataJSON,
		&metadataJSON,
		&immutable,
		&secret.Status,
		&secret.StatusMessage,
		&secret.UsageCount,
		&lastUsedAt,
		&secret.CreatedAt,
		&secret.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	secret.Immutable = immutable != 0
	if lastUsedAt.Valid {
		secret.LastUsedAt = &lastUsedAt.Time
	}

	if dataJSON.Valid {
		secret.Data = make(map[string]string)
		json.Unmarshal([]byte(dataJSON.String), &secret.Data)
	}
	if metadataJSON.Valid {
		secret.Metadata = make(map[string]string)
		json.Unmarshal([]byte(metadataJSON.String), &secret.Metadata)
	}

	return &secret, nil
}

// UpdateSecret updates an existing secret
func (r *Repository) UpdateSecret(ctx context.Context, secret *domain.Secret) error {
	dataJSON, err := json.Marshal(secret.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal secret data: %w", err)
	}

	metadataJSON, err := json.Marshal(secret.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal secret metadata: %w", err)
	}

	secret.UpdatedAt = time.Now()

	query := `
		UPDATE secrets SET
			name = ?, type = ?, description = ?, data = ?, metadata = ?,
			status = ?, status_message = ?, updated_at = ?
		WHERE id = ? AND immutable = 0
	`
	result, err := r.db.ExecContext(ctx, query,
		secret.Name,
		string(secret.Type),
		secret.Description,
		string(dataJSON),
		string(metadataJSON),
		string(secret.Status),
		secret.StatusMessage,
		secret.UpdatedAt,
		secret.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("secret not found or is immutable")
	}

	return nil
}

// DeleteSecret deletes a secret by ID (only operator secrets)
func (r *Repository) DeleteSecret(ctx context.Context, id string) error {
	query := `DELETE FROM secrets WHERE id = ? AND immutable = 0`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("secret not found or is immutable")
	}

	return nil
}

// ListSecrets lists all secrets, optionally filtered by type or source
func (r *Repository) ListSecrets(ctx context.Context, secretType string, source string) ([]domain.Secret, error) {
	query := `
		SELECT id, name, type, source, description, data, metadata, immutable, status, status_message, usage_count, last_used_at, created_at, updated_at
		FROM secrets WHERE 1=1
	`
	args := []interface{}{}

	if secretType != "" {
		query += " AND type = ?"
		args = append(args, secretType)
	}
	if source != "" {
		query += " AND source = ?"
		args = append(args, source)
	}

	query += " ORDER BY name ASC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}
	defer rows.Close()

	var secrets []domain.Secret
	for rows.Next() {
		var secret domain.Secret
		var dataJSON, metadataJSON sql.NullString
		var immutable int
		var lastUsedAt sql.NullTime

		err := rows.Scan(
			&secret.ID,
			&secret.Name,
			&secret.Type,
			&secret.Source,
			&secret.Description,
			&dataJSON,
			&metadataJSON,
			&immutable,
			&secret.Status,
			&secret.StatusMessage,
			&secret.UsageCount,
			&lastUsedAt,
			&secret.CreatedAt,
			&secret.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan secret: %w", err)
		}

		secret.Immutable = immutable != 0
		if lastUsedAt.Valid {
			secret.LastUsedAt = &lastUsedAt.Time
		}

		if dataJSON.Valid {
			secret.Data = make(map[string]string)
			json.Unmarshal([]byte(dataJSON.String), &secret.Data)
		}
		if metadataJSON.Valid {
			secret.Metadata = make(map[string]string)
			json.Unmarshal([]byte(metadataJSON.String), &secret.Metadata)
		}

		secrets = append(secrets, secret)
	}

	return secrets, rows.Err()
}

// UpdateSecretUsage updates the usage tracking for a secret
func (r *Repository) UpdateSecretUsage(ctx context.Context, id string) error {
	query := `
		UPDATE secrets SET
			usage_count = usage_count + 1,
			last_used_at = ?
		WHERE id = ?
	`
	_, err := r.db.ExecContext(ctx, query, time.Now(), id)
	return err
}

// UpdateSecretStatus updates the status of a secret
func (r *Repository) UpdateSecretStatus(ctx context.Context, id string, status domain.SecretStatus, message string) error {
	query := `UPDATE secrets SET status = ?, status_message = ?, updated_at = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, string(status), message, time.Now(), id)
	return err
}

// boolToInt converts bool to int for SQLite
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
