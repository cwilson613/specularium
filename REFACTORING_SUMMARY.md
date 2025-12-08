# netdiagram-go Backend Refactoring Summary

## Overview

This refactoring transforms netdiagram-go from a file-watching YAML loader to a full graph database system with import/export capabilities, aligning with the OpenAPI v1.0.0 specification.

## Key Changes

### Architecture Shift

**Before**: YAML file as source of truth → file watcher → in-memory cache → API
**After**: SQLite database as source of truth → API with CRUD operations → import/export codecs

### Database is Now Source of Truth

- The SQLite database stores all graph data persistently
- YAML files become import/export formats, not the canonical data source
- File watcher removed - no longer needed since DB is source of truth

## New Domain Model

### Previous Types (Host/Connection based)
- `Host` - Complex type with hardware, network specs, VMs
- `Connection` - Network link between hosts
- `Infrastructure` - Container for hosts and connections

### New Types (Node/Edge graph model)
- `Node` - Generic network entity with flexible property bag
- `Edge` - Connection between nodes with flexible properties
- `NodePosition` - Visualization position with pinning support
- `Graph` - Complete topology (nodes + edges + positions)
- `GraphFragment` - Partial graph for import/export

### Node Types
```go
NodeTypeServer, NodeTypeSwitch, NodeTypeRouter,
NodeTypeVM, NodeTypeVIP, NodeTypeContainer, NodeTypeUnknown
```

### Edge Types
```go
EdgeTypeEthernet, EdgeTypeVLAN, EdgeTypeVirtual, EdgeTypeAggregation
```

## New Database Schema

### Tables

**nodes**
- `id` (PK) - Node identifier
- `type` - Node type (server, switch, router, etc.)
- `label` - Display label
- `properties` - JSON property bag
- `source` - Data source (ansible, manual, scan)
- `created_at`, `updated_at` - Timestamps

**edges**
- `id` (PK) - Edge identifier (auto-generated)
- `from_id`, `to_id` (FK → nodes) - Endpoints
- `type` - Edge type (ethernet, vlan, etc.)
- `properties` - JSON property bag

**node_positions**
- `node_id` (PK, FK → nodes) - Node reference
- `x`, `y` - Coordinates
- `pinned` - Lock position flag

### Indexes
- `idx_nodes_type`, `idx_nodes_source`
- `idx_edges_from`, `idx_edges_to`

## New Codec System

Location: `internal/codec/`

### Codec Interface
```go
type Importer interface {
    Parse(r io.Reader) (*domain.GraphFragment, error)
    Format() string
}

type Exporter interface {
    Export(fragment *domain.GraphFragment, w io.Writer) error
    Format() string
}
```

### Implemented Codecs

1. **YAML Codec** (`yaml.go`)
   - Generic YAML import/export
   - Direct node/edge representation

2. **Ansible Inventory Codec** (`ansible.go`)
   - Parses Ansible inventory YAML format
   - Infers node types from group names
   - Automatically creates edges to router/gateway nodes
   - Converts host vars to node properties

3. **JSON Codec** (`json.go`)
   - JSON import/export for API compatibility

## API Endpoints (OpenAPI Spec Compliant)

### Graph Operations
- `GET /api/graph` - Get complete graph (nodes + edges + positions)

### Node CRUD
- `GET /api/nodes` - List nodes (filter by type, source)
- `POST /api/nodes` - Create node
- `GET /api/nodes/{id}` - Get node
- `PUT /api/nodes/{id}` - Update node (partial)
- `DELETE /api/nodes/{id}` - Delete node

### Edge CRUD
- `GET /api/edges` - List edges (filter by type, from_id, to_id)
- `POST /api/edges` - Create edge
- `GET /api/edges/{id}` - Get edge
- `PUT /api/edges/{id}` - Update edge (partial)
- `DELETE /api/edges/{id}` - Delete edge

### Positions
- `GET /api/positions` - Get all positions
- `POST /api/positions` - Bulk save positions
- `PUT /api/positions/{node_id}` - Update single position

### Import
- `POST /api/import/yaml` - Import generic YAML
- `POST /api/import/ansible-inventory` - Import Ansible inventory
- `POST /api/import/scan` - Network scan (stub/not implemented)

Query parameter: `?strategy=merge|replace`
- `merge` (default) - Upsert by ID, preserve existing data
- `replace` - Clear all data before importing

### Export
- `GET /api/export/json` - Export as JSON
- `GET /api/export/yaml` - Export as YAML
- `GET /api/export/ansible-inventory` - Export as Ansible inventory

### Events
- `GET /events` - Server-Sent Events stream

## Import Strategy

### Merge Strategy (Default)
- Upserts nodes/edges by ID
- Preserves existing data not in import
- Updates only provided fields
- Useful for incremental updates

### Replace Strategy
- Clears all existing data
- Imports fresh dataset
- Useful for full infrastructure reload

### Import Result
```json
{
  "nodes_created": 5,
  "nodes_updated": 2,
  "edges_created": 8,
  "edges_updated": 1,
  "strategy": "merge"
}
```

## Service Layer Changes

Location: `internal/service/service.go`

### New GraphService
Replaces `InfrastructureService` with:
- Full CRUD operations for nodes and edges
- Import/export methods using codecs
- Validation logic
- Event publishing for SSE

### Removed
- File loading logic (moved to codec layer)
- File-based import/export (now uses codecs)
- Infrastructure caching (DB is now source)

## Repository Layer

Location: `internal/repository/sqlite/sqlite.go`

### New Methods
- `GetGraph()` - Returns complete graph
- `GetNode()`, `ListNodes()`, `CreateNode()`, `UpsertNode()`, `UpdateNode()`, `DeleteNode()`
- `GetEdge()`, `ListEdges()`, `CreateEdge()`, `UpsertEdge()`, `UpdateEdge()`, `DeleteEdge()`
- `GetAllPositions()`, `GetPosition()`, `SavePosition()`, `SavePositions()`
- `ImportFragment()` - Import with strategy support
- `ExportFragment()` - Export as fragment

### Migration
Auto-creates new schema on startup. Old schema tables (hosts, connections, host_groups) can coexist but are not used.

## Removed Components

### File Watcher (`internal/watcher/`)
No longer needed since database is source of truth. Users interact via API or import operations.

### Command-line YAML Import
The `-yaml` flag has been removed from main.go. To import data:
1. Use API: `POST /api/import/yaml` or `POST /api/import/ansible-inventory`
2. Or manually import via service layer in custom code

## Event System

### New Event Types
```go
EventNodeCreated, EventNodeUpdated, EventNodeDeleted
EventEdgeCreated, EventEdgeUpdated, EventEdgeDeleted
EventPositionsUpdated
EventGraphUpdated (on import)
```

Legacy events (host/connection) remain for backwards compatibility but may be deprecated.

## Migration Path

### For Existing Deployments

1. **Backup existing data**: Export current YAML
2. **Deploy new version**: Will create new schema alongside old
3. **Import data**: Use `POST /api/import/ansible-inventory` with your existing infrastructure.yml
4. **Verify**: Check `GET /api/graph` returns expected data
5. **Switch**: Update K8s ConfigMap or deployment to use DB-first approach

### For New Deployments

1. Deploy with empty database
2. Import initial topology via API
3. Manage via CRUD operations
4. Export as needed for backup/versioning

## Kubernetes Deployment Notes

### ConfigMap Changes
Previously: ConfigMap contained infrastructure.yml mounted and watched
Now: ConfigMap can be removed OR used only for initial import

### Recommended Approach
- Use PersistentVolumeClaim for SQLite database
- Import infrastructure.yml once via API during initialization
- Manage topology via API/UI going forward
- Optionally export to YAML for Git versioning

### Ansible Playbook Update

The `update-network-diagram.yml` playbook should be updated:

**Before**: Update ConfigMap → trigger file watcher reload
**After**: Generate YAML → POST to import API endpoint

Example:
```yaml
- name: Import infrastructure data
  uri:
    url: "https://netdiagram.vanderlyn.house/api/import/ansible-inventory"
    method: POST
    body: "{{ lookup('file', 'infrastructure.yml') }}"
    headers:
      Content-Type: "application/x-yaml"
    body_format: raw
```

## Property Bag Flexibility

### Node Properties (Examples)
```json
{
  "ip": "192.168.0.10",
  "hostname": "brutus.vanderlyn.local",
  "os": "Debian 12",
  "role": "k3s-server",
  "ram_gb": 64,
  "cpu": "AMD Ryzen 9 5950X"
}
```

### Edge Properties (Examples)
```json
{
  "speed": "10GbE",
  "interface": "eth0",
  "vlan_id": 100,
  "duplex": "full"
}
```

Properties are stored as JSON in SQLite, allowing arbitrary key-value pairs.

## Frontend Compatibility

The frontend (`cmd/server/web/`) should mostly work with minimal changes:

### API Changes
- Old: `GET /api/graph` returned derived graph from Infrastructure
- New: `GET /api/graph` returns `{ nodes, edges, positions }`

### Node Format Change
```javascript
// Old format
{ id: "brutus", label: "brutus", group: "machine", title: "..." }

// New format
{ id: "brutus", type: "server", label: "brutus", properties: {...} }
```

### Recommended Frontend Updates
- Map `node.type` to visualization `group` (server → machine, switch → device)
- Build tooltip from `node.properties` instead of structured fields
- Use `node.properties.ip` instead of separate IP field

## Testing

To test the refactored system:

1. **Build**: `make build`
2. **Run**: `./netdiagram -addr :3000`
3. **Test Import**:
   ```bash
   curl -X POST http://localhost:3000/api/import/yaml \
     -H "Content-Type: application/x-yaml" \
     --data-binary @infrastructure.yml
   ```
4. **Test CRUD**:
   ```bash
   # Create node
   curl -X POST http://localhost:3000/api/nodes \
     -H "Content-Type: application/json" \
     -d '{"id":"test1","type":"server","label":"Test Server"}'

   # List nodes
   curl http://localhost:3000/api/nodes

   # Get graph
   curl http://localhost:3000/api/graph
   ```

## Files Changed/Created

### New Files
- `internal/domain/node.go` - Node type
- `internal/domain/edge.go` - Edge type
- `internal/domain/position.go` - NodePosition type
- `internal/domain/fragment.go` - GraphFragment type
- `internal/domain/graph.go` - New Graph type
- `internal/codec/codec.go` - Codec interfaces
- `internal/codec/yaml.go` - YAML codec
- `internal/codec/ansible.go` - Ansible inventory codec
- `internal/codec/json.go` - JSON codec
- `internal/repository/sqlite/sqlite.go` - Rewritten repository
- `internal/service/service.go` - New GraphService
- `internal/handler/handler.go` - New GraphHandler
- `cmd/server/main.go` - Simplified main

### Modified Files
- `internal/service/events.go` - Added new event types

### Preserved (Legacy)
- `internal/domain/host.go` - Old Host type (unused)
- `internal/domain/connection.go` - Old Connection type (unused)
- `internal/domain/infrastructure.go` - Old Infrastructure type (unused)
- `internal/loader/yaml.go` - Old YAML loader (can be removed)
- `internal/watcher/watcher.go` - File watcher (unused)

### Backed Up (*.old files)
- `cmd/server/main.go.old`
- `internal/repository/sqlite/sqlite.go.old`
- `internal/service/infrastructure.go.old`
- `internal/handler/api.go.old`
- `internal/domain/graph.go.old`

## Next Steps

1. **Test Compilation**: Ensure `make build` succeeds (requires Go installation)
2. **Update Frontend**: Adapt vis-network integration to new node/edge format
3. **Update Ansible Playbook**: Change from ConfigMap update to API import
4. **Test Import**: Verify Ansible inventory import works correctly
5. **Documentation**: Update README with new API usage
6. **Clean Up**: Remove .old files and unused legacy code after verification

## Benefits of Refactoring

1. **True CRUD Operations**: Create, read, update, delete individual nodes/edges
2. **Persistent Storage**: Database survives pod restarts without ConfigMap
3. **Flexible Schema**: Property bags allow arbitrary metadata
4. **Multiple Import Formats**: YAML, Ansible inventory, future: network scans
5. **API-First**: Can be managed programmatically without file manipulation
6. **Scalability**: Database queries more efficient than in-memory filtering
7. **Version Control**: Export to YAML for Git storage when needed
8. **Selective Updates**: Update individual components without full reload

## Backwards Compatibility Notes

- SSE events maintain legacy names for compatibility
- Old domain types preserved but unused
- API responses changed format (breaking change for consumers)
- ConfigMap-based workflow no longer supported

For production migration, coordinate with frontend/client updates.
