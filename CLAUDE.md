# CLAUDE.md - Specularium

Instructions for Claude Code when working on this project.

## Project Overview

`specularium` is the network topology visualization backend for the Vanderlyn homelab. It serves both the API and the web UI as a single Go binary. The name comes from Latin "place of observation/watching" - fitting for a passive visualization tool.

**Production URL**: https://specularium.vanderlyn.house
**Docker Image**: `cwilson613/specularium:latest`

## Go Toolchain

Go 1.23.4 is installed at `/usr/local/go`. The Makefile uses the full path (`/usr/local/go/bin/go`) so it works regardless of shell PATH configuration.

## Development Commands

```bash
make build    # Build binary (CGO_ENABLED=1 for SQLite)
make run      # Build and run on :3000
make test     # Run tests
make tidy     # go mod tidy
make clean    # Remove binary and db files
make docker   # Build Docker image
make push     # Build and push to Docker Hub
```

## CLI Flags

```
./specularium -addr :3000 -db ./specularium.db
```

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:3000` | HTTP listen address |
| `-db` | `./specularium.db` | SQLite database path |

## Key Files

| File | Purpose |
|------|---------|
| `cmd/server/main.go` | Entry point, component wiring |
| `cmd/server/web/` | Embedded static assets (HTML, CSS, JS, icons) |
| `cmd/server/web/icons/` | Gothic-industrial SVG icons |
| `internal/domain/` | Core types (Node, Edge, Graph) |
| `internal/repository/sqlite/sqlite.go` | Database operations |
| `internal/service/service.go` | Business logic |
| `internal/handler/handler.go` | HTTP API handlers |
| `internal/hub/hub.go` | SSE connection manager |
| `internal/codec/` | Import/export codecs (YAML, JSON, Ansible) |

## Architecture

1. **Single binary**: All assets embedded via `//go:embed`
2. **SQLite**: Single-replica K8s deployment with Recreate strategy
3. **SSE**: Server-Sent Events for real-time updates (simpler than WebSocket)
4. **Codec pattern**: Pluggable import/export formats
5. **circularImage nodes**: vis-network with tinted SVG icons

## Deployment

```bash
# Build and push
make push

# Apply K8s manifests
kubectl apply -f ../k8s/manifests/specularium/ --kubeconfig ~/.kube/config-brutus

# Or restart to pull latest
kubectl rollout restart deployment/specularium -n default --kubeconfig ~/.kube/config-brutus
```

## API Quick Reference

### Graph
- `GET /api/graph` - Complete graph with nodes, edges, positions
- `GET /events` - SSE stream

### Nodes
- `GET /api/nodes` - List (filter: `?type=`, `?source=`)
- `POST /api/nodes` - Create
- `GET /api/nodes/{id}` - Get
- `PUT /api/nodes/{id}` - Update
- `DELETE /api/nodes/{id}` - Delete

### Edges
- `GET /api/edges` - List
- `POST /api/edges` - Create
- `GET /api/edges/{id}` - Get
- `PUT /api/edges/{id}` - Update
- `DELETE /api/edges/{id}` - Delete

### Positions
- `GET /api/positions` - Get all
- `POST /api/positions` - Save multiple
- `PUT /api/positions/{node_id}` - Update single

### Import/Export
- `POST /api/import/yaml` - Import YAML
- `POST /api/import/ansible-inventory` - Import Ansible inventory
- `GET /api/export/json` - Export JSON
- `GET /api/export/yaml` - Export YAML
- `GET /api/export/ansible-inventory` - Export Ansible inventory

## Common Tasks

### Modify the frontend

1. Edit files in `cmd/server/web/`
2. `make build` (assets embedded at compile time)
3. `./specularium -addr :3000`

### Add a new node type icon

1. Create SVG in `cmd/server/web/icons/` (use `currentColor` for tinting)
2. Add entry to `nodeTypes` in `cmd/server/web/app.js`
3. Rebuild

### Debug production

```bash
kubectl get pods -n default --kubeconfig ~/.kube/config-brutus | grep specularium
kubectl logs -f deployment/specularium -n default --kubeconfig ~/.kube/config-brutus
curl -sk https://specularium.vanderlyn.house/api/graph | jq '.nodes | length'
```

## Dependencies

- `github.com/mattn/go-sqlite3` - SQLite (CGO required)
- `gopkg.in/yaml.v3` - YAML parsing

## Notes

- CGO required for SQLite; Docker uses Alpine with musl-dev
- Frontend uses vis-network from CDN
- Position data persists in SQLite via PVC
- 40K Mechanicus theming throughout UI
