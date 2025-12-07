package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"netdiagram/internal/domain"

	_ "github.com/mattn/go-sqlite3"
)

// Repository implements repository.Repository using SQLite
type Repository struct {
	db *sql.DB
}

// New creates a new SQLite repository
func New(dbPath string) (*Repository, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
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
	schema := `
	CREATE TABLE IF NOT EXISTS hosts (
		id TEXT PRIMARY KEY,
		ip TEXT NOT NULL,
		role TEXT NOT NULL,
		platform TEXT NOT NULL,
		classification TEXT NOT NULL DEFAULT 'machine',
		data JSON NOT NULL,
		position_x REAL,
		position_y REAL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS connections (
		id TEXT PRIMARY KEY,
		source_id TEXT NOT NULL,
		source_port TEXT,
		target_id TEXT NOT NULL,
		target_port TEXT,
		speed_gbps INTEGER NOT NULL DEFAULT 1,
		type TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (source_id) REFERENCES hosts(id) ON DELETE CASCADE,
		FOREIGN KEY (target_id) REFERENCES hosts(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS host_groups (
		host_id TEXT NOT NULL,
		group_name TEXT NOT NULL,
		PRIMARY KEY (host_id, group_name),
		FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS metadata (
		key TEXT PRIMARY KEY,
		value JSON NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_connections_source ON connections(source_id);
	CREATE INDEX IF NOT EXISTS idx_connections_target ON connections(target_id);
	CREATE INDEX IF NOT EXISTS idx_host_groups_group ON host_groups(group_name);
	`

	_, err := r.db.Exec(schema)
	return err
}

// GetInfrastructure loads the complete infrastructure from the database
func (r *Repository) GetInfrastructure(ctx context.Context) (*domain.Infrastructure, error) {
	infra := domain.NewInfrastructure()

	// Load hosts
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, ip, role, platform, classification, data, position_x, position_y
		FROM hosts
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query hosts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id, ip, role, platform, classification string
			data                                   []byte
			posX, posY                             sql.NullFloat64
		)

		if err := rows.Scan(&id, &ip, &role, &platform, &classification, &data, &posX, &posY); err != nil {
			return nil, fmt.Errorf("failed to scan host: %w", err)
		}

		host := &domain.Host{}
		if err := json.Unmarshal(data, host); err != nil {
			return nil, fmt.Errorf("failed to unmarshal host data: %w", err)
		}

		// Override with indexed columns (source of truth)
		host.ID = id
		host.IP = ip
		host.Role = role
		host.Platform = platform
		host.Classification = domain.Classification(classification)

		if posX.Valid && posY.Valid {
			host.Position = &domain.Position{X: posX.Float64, Y: posY.Float64}
		}

		infra.Hosts[id] = host
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating hosts: %w", err)
	}

	// Load connections
	connRows, err := r.db.QueryContext(ctx, `
		SELECT id, source_id, source_port, target_id, target_port, speed_gbps, type
		FROM connections
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query connections: %w", err)
	}
	defer connRows.Close()

	for connRows.Next() {
		var (
			id, sourceID, targetID           string
			sourcePort, targetPort, connType sql.NullString
			speedGbps                        int
		)

		if err := connRows.Scan(&id, &sourceID, &sourcePort, &targetID, &targetPort, &speedGbps, &connType); err != nil {
			return nil, fmt.Errorf("failed to scan connection: %w", err)
		}

		conn := &domain.Connection{
			ID:        id,
			SourceID:  sourceID,
			TargetID:  targetID,
			SpeedGbps: speedGbps,
		}

		if sourcePort.Valid {
			conn.SourcePort = sourcePort.String
		}
		if targetPort.Valid {
			conn.TargetPort = targetPort.String
		}
		if connType.Valid {
			conn.Type = domain.ConnectionType(connType.String)
		}

		infra.Connections[id] = conn
	}

	if err := connRows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating connections: %w", err)
	}

	// Load group memberships
	groupRows, err := r.db.QueryContext(ctx, `
		SELECT host_id, group_name FROM host_groups
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query host groups: %w", err)
	}
	defer groupRows.Close()

	for groupRows.Next() {
		var hostID, groupName string
		if err := groupRows.Scan(&hostID, &groupName); err != nil {
			return nil, fmt.Errorf("failed to scan host group: %w", err)
		}

		if host := infra.Hosts[hostID]; host != nil {
			host.Groups = append(host.Groups, groupName)
		}
	}

	return infra, nil
}

// GetHost retrieves a single host by ID
func (r *Repository) GetHost(ctx context.Context, id string) (*domain.Host, error) {
	var (
		ip, role, platform, classification string
		data                               []byte
		posX, posY                         sql.NullFloat64
	)

	err := r.db.QueryRowContext(ctx, `
		SELECT ip, role, platform, classification, data, position_x, position_y
		FROM hosts WHERE id = ?
	`, id).Scan(&ip, &role, &platform, &classification, &data, &posX, &posY)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query host: %w", err)
	}

	host := &domain.Host{}
	if err := json.Unmarshal(data, host); err != nil {
		return nil, fmt.Errorf("failed to unmarshal host data: %w", err)
	}

	host.ID = id
	host.IP = ip
	host.Role = role
	host.Platform = platform
	host.Classification = domain.Classification(classification)

	if posX.Valid && posY.Valid {
		host.Position = &domain.Position{X: posX.Float64, Y: posY.Float64}
	}

	// Load groups
	rows, err := r.db.QueryContext(ctx, `
		SELECT group_name FROM host_groups WHERE host_id = ?
	`, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query host groups: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var groupName string
		if err := rows.Scan(&groupName); err != nil {
			return nil, fmt.Errorf("failed to scan group: %w", err)
		}
		host.Groups = append(host.Groups, groupName)
	}

	return host, nil
}

// GetConnection retrieves a single connection by ID
func (r *Repository) GetConnection(ctx context.Context, id string) (*domain.Connection, error) {
	var (
		sourceID, targetID           string
		sourcePort, targetPort, connType sql.NullString
		speedGbps                        int
	)

	err := r.db.QueryRowContext(ctx, `
		SELECT source_id, source_port, target_id, target_port, speed_gbps, type
		FROM connections WHERE id = ?
	`, id).Scan(&sourceID, &sourcePort, &targetID, &targetPort, &speedGbps, &connType)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query connection: %w", err)
	}

	conn := &domain.Connection{
		ID:        id,
		SourceID:  sourceID,
		TargetID:  targetID,
		SpeedGbps: speedGbps,
	}

	if sourcePort.Valid {
		conn.SourcePort = sourcePort.String
	}
	if targetPort.Valid {
		conn.TargetPort = targetPort.String
	}
	if connType.Valid {
		conn.Type = domain.ConnectionType(connType.String)
	}

	return conn, nil
}

// UpsertHost inserts or updates a host
func (r *Repository) UpsertHost(ctx context.Context, host *domain.Host) error {
	data, err := json.Marshal(host)
	if err != nil {
		return fmt.Errorf("failed to marshal host: %w", err)
	}

	classification := host.InferClassification()

	var posX, posY sql.NullFloat64
	if host.Position != nil {
		posX = sql.NullFloat64{Float64: host.Position.X, Valid: true}
		posY = sql.NullFloat64{Float64: host.Position.Y, Valid: true}
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO hosts (id, ip, role, platform, classification, data, position_x, position_y, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			ip = excluded.ip,
			role = excluded.role,
			platform = excluded.platform,
			classification = excluded.classification,
			data = excluded.data,
			position_x = COALESCE(excluded.position_x, hosts.position_x),
			position_y = COALESCE(excluded.position_y, hosts.position_y),
			updated_at = CURRENT_TIMESTAMP
	`, host.ID, host.IP, host.Role, host.Platform, classification, data, posX, posY)

	if err != nil {
		return fmt.Errorf("failed to upsert host: %w", err)
	}

	// Update groups
	if err := r.updateHostGroups(ctx, host.ID, host.Groups); err != nil {
		return fmt.Errorf("failed to update host groups: %w", err)
	}

	return nil
}

func (r *Repository) updateHostGroups(ctx context.Context, hostID string, groups []string) error {
	// Delete existing groups
	if _, err := r.db.ExecContext(ctx, `DELETE FROM host_groups WHERE host_id = ?`, hostID); err != nil {
		return err
	}

	// Insert new groups
	if len(groups) == 0 {
		return nil
	}

	stmt, err := r.db.PrepareContext(ctx, `INSERT INTO host_groups (host_id, group_name) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, group := range groups {
		if _, err := stmt.ExecContext(ctx, hostID, group); err != nil {
			return err
		}
	}

	return nil
}

// DeleteHost removes a host and its associated connections
func (r *Repository) DeleteHost(ctx context.Context, id string) error {
	// Connections will be deleted by CASCADE
	_, err := r.db.ExecContext(ctx, `DELETE FROM hosts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete host: %w", err)
	}
	return nil
}

// UpsertConnection inserts or updates a connection
func (r *Repository) UpsertConnection(ctx context.Context, conn *domain.Connection) error {
	conn.Normalize()
	if conn.ID == "" {
		conn.ID = conn.GenerateID()
	}

	var sourcePort, targetPort, connType sql.NullString
	if conn.SourcePort != "" {
		sourcePort = sql.NullString{String: conn.SourcePort, Valid: true}
	}
	if conn.TargetPort != "" {
		targetPort = sql.NullString{String: conn.TargetPort, Valid: true}
	}
	if conn.Type != "" {
		connType = sql.NullString{String: string(conn.Type), Valid: true}
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO connections (id, source_id, source_port, target_id, target_port, speed_gbps, type, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			source_id = excluded.source_id,
			source_port = excluded.source_port,
			target_id = excluded.target_id,
			target_port = excluded.target_port,
			speed_gbps = excluded.speed_gbps,
			type = excluded.type,
			updated_at = CURRENT_TIMESTAMP
	`, conn.ID, conn.SourceID, sourcePort, conn.TargetID, targetPort, conn.SpeedGbps, connType)

	if err != nil {
		return fmt.Errorf("failed to upsert connection: %w", err)
	}
	return nil
}

// DeleteConnection removes a connection
func (r *Repository) DeleteConnection(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM connections WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete connection: %w", err)
	}
	return nil
}

// SavePositions updates position data for multiple hosts
func (r *Repository) SavePositions(ctx context.Context, positions map[string]domain.Position) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		UPDATE hosts SET position_x = ?, position_y = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for id, pos := range positions {
		if _, err := stmt.ExecContext(ctx, pos.X, pos.Y, id); err != nil {
			return fmt.Errorf("failed to update position for %s: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// ImportInfrastructure replaces all data with the provided infrastructure
func (r *Repository) ImportInfrastructure(ctx context.Context, infra *domain.Infrastructure) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Clear existing data (order matters due to foreign keys)
	if _, err := tx.ExecContext(ctx, `DELETE FROM host_groups`); err != nil {
		return fmt.Errorf("failed to clear host_groups: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM connections`); err != nil {
		return fmt.Errorf("failed to clear connections: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM hosts`); err != nil {
		return fmt.Errorf("failed to clear hosts: %w", err)
	}

	// Insert hosts
	hostStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO hosts (id, ip, role, platform, classification, data, position_x, position_y)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare host statement: %w", err)
	}
	defer hostStmt.Close()

	groupStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO host_groups (host_id, group_name) VALUES (?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare group statement: %w", err)
	}
	defer groupStmt.Close()

	for id, host := range infra.Hosts {
		host.ID = id
		data, err := json.Marshal(host)
		if err != nil {
			return fmt.Errorf("failed to marshal host %s: %w", id, err)
		}

		classification := host.InferClassification()

		var posX, posY sql.NullFloat64
		if host.Position != nil {
			posX = sql.NullFloat64{Float64: host.Position.X, Valid: true}
			posY = sql.NullFloat64{Float64: host.Position.Y, Valid: true}
		}

		if _, err := hostStmt.ExecContext(ctx, id, host.IP, host.Role, host.Platform, classification, data, posX, posY); err != nil {
			return fmt.Errorf("failed to insert host %s: %w", id, err)
		}

		for _, group := range host.Groups {
			if _, err := groupStmt.ExecContext(ctx, id, group); err != nil {
				return fmt.Errorf("failed to insert group for %s: %w", id, err)
			}
		}
	}

	// Insert connections
	connStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO connections (id, source_id, source_port, target_id, target_port, speed_gbps, type)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare connection statement: %w", err)
	}
	defer connStmt.Close()

	for id, conn := range infra.Connections {
		var sourcePort, targetPort, connType sql.NullString
		if conn.SourcePort != "" {
			sourcePort = sql.NullString{String: conn.SourcePort, Valid: true}
		}
		if conn.TargetPort != "" {
			targetPort = sql.NullString{String: conn.TargetPort, Valid: true}
		}
		if conn.Type != "" {
			connType = sql.NullString{String: string(conn.Type), Valid: true}
		}

		if _, err := connStmt.ExecContext(ctx, id, conn.SourceID, sourcePort, conn.TargetID, targetPort, conn.SpeedGbps, connType); err != nil {
			return fmt.Errorf("failed to insert connection %s: %w", id, err)
		}
	}

	// Store metadata
	if infra.Metadata != nil {
		metaData, _ := json.Marshal(infra.Metadata)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO metadata (key, value, updated_at) VALUES ('infrastructure', ?, CURRENT_TIMESTAMP)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
		`, metaData); err != nil {
			return fmt.Errorf("failed to store metadata: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO metadata (key, value, updated_at) VALUES ('last_import', ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
	`, fmt.Sprintf(`"%s"`, time.Now().Format(time.RFC3339))); err != nil {
		return fmt.Errorf("failed to store import timestamp: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Close closes the database connection
func (r *Repository) Close() error {
	return r.db.Close()
}
