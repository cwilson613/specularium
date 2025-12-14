package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"specularium/internal/domain"
)

// ============================================================================
// Null Type Conversion Helpers
// ============================================================================

// nullToString safely converts sql.NullString to string
func nullToString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// nullToTimePtr safely converts sql.NullTime to *time.Time
func nullToTimePtr(nt sql.NullTime) *time.Time {
	if nt.Valid {
		return &nt.Time
	}
	return nil
}

// nullToBool converts sql.NullInt64 to bool (0 = false, non-zero = true)
func nullToBool(ni sql.NullInt64) bool {
	return ni.Valid && ni.Int64 != 0
}

// stringToNull safely converts string to sql.NullString
func stringToNull(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// timePtrToNull safely converts *time.Time to sql.NullTime
func timePtrToNull(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// ============================================================================
// JSON Marshaling Helpers
// ============================================================================

// unmarshalJSONField safely unmarshals JSON from nullable string into target
func unmarshalJSONField(ns sql.NullString, target interface{}) error {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	return json.Unmarshal([]byte(ns.String), target)
}

// marshalToNull marshals interface to nullable JSON string
// Returns empty NullString for nil or empty maps
func marshalToNull(v interface{}) (sql.NullString, error) {
	if v == nil {
		return sql.NullString{}, nil
	}

	// Handle empty maps - don't store "{}"
	if m, ok := v.(map[string]any); ok && len(m) == 0 {
		return sql.NullString{}, nil
	}

	data, err := json.Marshal(v)
	if err != nil {
		return sql.NullString{}, err
	}
	return sql.NullString{String: string(data), Valid: true}, nil
}

// ============================================================================
// Schema Evolution Guide
// ============================================================================
//
// To add a new column to nodes table:
// 1. Add field to nodeRow struct (below)
// 2. Update scanArgs() - APPEND to end to match column order
// 3. Update nodeColumns constant - APPEND to end
// 4. Update toDomain() to map new field to domain.Node
// 5. Update nodeInsertArgs() if column should be writable
// 6. Add migration in sqlite.go migrate() using addColumnIfNotExists()
// 7. Update relevant tests
//
// CRITICAL: Column order must match between:
// - nodeColumns constant
// - scanArgs() return slice
// - All SELECT queries using nodeColumns
//
// Same pattern applies to edges and discrepancies.

// ============================================================================
// Node Row Scanner
// ============================================================================

// nodeRow holds all columns from a node query for scanning
type nodeRow struct {
	ID               string
	Type             string
	Label            string
	ParentID         sql.NullString
	PropertiesJSON   sql.NullString
	Source           sql.NullString
	Status           sql.NullString
	LastVerified     sql.NullTime
	LastSeen         sql.NullTime
	DiscoveredJSON   sql.NullString
	TruthJSON        sql.NullString
	TruthStatus      sql.NullString
	HasDiscrepancy   sql.NullInt64
	CapabilitiesJSON sql.NullString
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// scanArgs returns pointers to all fields for sql.Scan()
// MUST match nodeColumns order exactly:
// id, type, label, parent_id, properties, source, status,
// last_verified, last_seen, discovered, truth, truth_status,
// has_discrepancy, capabilities, created_at, updated_at
func (r *nodeRow) scanArgs() []interface{} {
	return []interface{}{
		&r.ID,               // 1
		&r.Type,             // 2
		&r.Label,            // 3
		&r.ParentID,         // 4
		&r.PropertiesJSON,   // 5
		&r.Source,           // 6
		&r.Status,           // 7
		&r.LastVerified,     // 8
		&r.LastSeen,         // 9
		&r.DiscoveredJSON,   // 10
		&r.TruthJSON,        // 11
		&r.TruthStatus,      // 12
		&r.HasDiscrepancy,   // 13
		&r.CapabilitiesJSON, // 14
		&r.CreatedAt,        // 15
		&r.UpdatedAt,        // 16
	}
}

// toDomain converts the scanned row to a domain.Node
func (r *nodeRow) toDomain() (*domain.Node, error) {
	node := &domain.Node{
		ID:             r.ID,
		Type:           domain.NodeType(r.Type),
		Label:          r.Label,
		ParentID:       nullToString(r.ParentID),
		Source:         nullToString(r.Source),
		Status:         domain.NodeStatus(nullToString(r.Status)),
		TruthStatus:    domain.TruthStatus(nullToString(r.TruthStatus)),
		HasDiscrepancy: nullToBool(r.HasDiscrepancy),
		LastVerified:   nullToTimePtr(r.LastVerified),
		LastSeen:       nullToTimePtr(r.LastSeen),
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
	}

	// Default status if empty
	if node.Status == "" {
		node.Status = domain.NodeStatusUnverified
	}

	// Unmarshal JSON fields
	if err := unmarshalJSONField(r.PropertiesJSON, &node.Properties); err != nil {
		return nil, fmt.Errorf("unmarshal properties: %w", err)
	}

	if err := unmarshalJSONField(r.DiscoveredJSON, &node.Discovered); err != nil {
		return nil, fmt.Errorf("unmarshal discovered: %w", err)
	}

	if r.TruthJSON.Valid && r.TruthJSON.String != "" {
		node.Truth = &domain.NodeTruth{}
		if err := json.Unmarshal([]byte(r.TruthJSON.String), node.Truth); err != nil {
			return nil, fmt.Errorf("unmarshal truth: %w", err)
		}
	}

	if err := unmarshalJSONField(r.CapabilitiesJSON, &node.Capabilities); err != nil {
		return nil, fmt.Errorf("unmarshal capabilities: %w", err)
	}

	return node, nil
}

// nodeColumns returns the SELECT column list for node queries
const nodeColumns = `id, type, label, parent_id, properties, source, status,
	last_verified, last_seen, discovered, truth, truth_status,
	has_discrepancy, capabilities, created_at, updated_at`

// ============================================================================
// Edge Row Scanner
// ============================================================================

// edgeRow holds all columns from an edge query for scanning
type edgeRow struct {
	ID             string
	FromID         string
	ToID           string
	Type           string
	PropertiesJSON sql.NullString
}

// scanArgs returns pointers to all fields for sql.Scan()
// MUST match edgeColumns order exactly:
// id, from_id, to_id, type, properties
func (r *edgeRow) scanArgs() []interface{} {
	return []interface{}{
		&r.ID,             // 1
		&r.FromID,         // 2
		&r.ToID,           // 3
		&r.Type,           // 4
		&r.PropertiesJSON, // 5
	}
}

// toDomain converts the scanned row to a domain.Edge
func (r *edgeRow) toDomain() (*domain.Edge, error) {
	edge := &domain.Edge{
		ID:     r.ID,
		FromID: r.FromID,
		ToID:   r.ToID,
		Type:   domain.EdgeType(r.Type),
	}

	if err := unmarshalJSONField(r.PropertiesJSON, &edge.Properties); err != nil {
		return nil, fmt.Errorf("unmarshal properties: %w", err)
	}

	return edge, nil
}

// edgeColumns returns the SELECT column list for edge queries
const edgeColumns = `id, from_id, to_id, type, properties`

// ============================================================================
// Discrepancy Row Scanner
// ============================================================================

// discrepancyRow holds all columns from a discrepancy query for scanning
type discrepancyRow struct {
	ID              string
	NodeID          string
	PropertyKey     string
	TruthValueJSON  sql.NullString
	ActualValueJSON sql.NullString
	Source          sql.NullString
	DetectedAt      time.Time
	ResolvedAt      sql.NullTime
	Resolution      sql.NullString
}

// scanArgs returns pointers to all fields for sql.Scan()
// MUST match discrepancyColumns order exactly:
// id, node_id, property_key, truth_value, actual_value, source, detected_at, resolved_at, resolution
func (r *discrepancyRow) scanArgs() []interface{} {
	return []interface{}{
		&r.ID,              // 1
		&r.NodeID,          // 2
		&r.PropertyKey,     // 3
		&r.TruthValueJSON,  // 4
		&r.ActualValueJSON, // 5
		&r.Source,          // 6
		&r.DetectedAt,      // 7
		&r.ResolvedAt,      // 8
		&r.Resolution,      // 9
	}
}

// toDomain converts the scanned row to a domain.Discrepancy
func (r *discrepancyRow) toDomain() *domain.Discrepancy {
	d := &domain.Discrepancy{
		ID:          r.ID,
		NodeID:      r.NodeID,
		PropertyKey: r.PropertyKey,
		Source:      nullToString(r.Source),
		DetectedAt:  r.DetectedAt,
		ResolvedAt:  nullToTimePtr(r.ResolvedAt),
		Resolution:  nullToString(r.Resolution),
	}

	// Unmarshal JSON values
	if r.TruthValueJSON.Valid {
		json.Unmarshal([]byte(r.TruthValueJSON.String), &d.TruthValue)
	}
	if r.ActualValueJSON.Valid {
		json.Unmarshal([]byte(r.ActualValueJSON.String), &d.ActualValue)
	}

	return d
}

// discrepancyColumns returns the SELECT column list for discrepancy queries
const discrepancyColumns = `id, node_id, property_key, truth_value, actual_value, source, detected_at, resolved_at, resolution`

// ============================================================================
// Node Write Helpers
// ============================================================================

// nodeInsertArgs prepares arguments for node INSERT/UPSERT
// Returns: id, type, label, parent_id, properties, source, status,
//          last_verified, last_seen, discovered, capabilities, created_at, updated_at
func nodeInsertArgs(node *domain.Node) ([]interface{}, error) {
	propsJSON, err := marshalToNull(node.Properties)
	if err != nil {
		return nil, fmt.Errorf("marshal properties: %w", err)
	}

	discoveredJSON, err := marshalToNull(node.Discovered)
	if err != nil {
		return nil, fmt.Errorf("marshal discovered: %w", err)
	}

	capabilitiesJSON, err := marshalToNull(node.Capabilities)
	if err != nil {
		return nil, fmt.Errorf("marshal capabilities: %w", err)
	}

	return []interface{}{
		node.ID,
		string(node.Type),
		node.Label,
		stringToNull(node.ParentID),
		propsJSON,
		node.Source,
		string(node.Status),
		timePtrToNull(node.LastVerified),
		timePtrToNull(node.LastSeen),
		discoveredJSON,
		capabilitiesJSON,
		node.CreatedAt,
		node.UpdatedAt,
	}, nil
}

// ============================================================================
// Edge Write Helpers
// ============================================================================

// edgeInsertArgs prepares arguments for edge INSERT/UPSERT
// Returns: id, from_id, to_id, type, properties
func edgeInsertArgs(edge *domain.Edge) ([]interface{}, error) {
	propsJSON, err := marshalToNull(edge.Properties)
	if err != nil {
		return nil, fmt.Errorf("marshal properties: %w", err)
	}

	return []interface{}{
		edge.ID,
		edge.FromID,
		edge.ToID,
		string(edge.Type),
		propsJSON,
	}, nil
}
