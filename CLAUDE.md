# CLAUDE.md - netdiagram-go

Instructions for Claude Code when working on this project.

## Project Overview

`netdiagram-go` is the network topology visualization backend for the Vanderlyn homelab. It serves both the API and the web UI as a single Go binary.

**Production URL**: https://netdiagram.vanderlyn.house
**Docker Image**: `cwilson613/netdiagram-go:latest`

## Development Commands

```bash
# Build (requires CGO for SQLite)
make build

# Run locally with file watching
make dev

# Run tests
make test

# Format code
make fmt

# Build Docker image
make docker

# Push to Docker Hub (after docker login)
docker push cwilson613/netdiagram-go:latest
```

## Key Files

| File | Purpose |
|------|---------|
| `cmd/server/main.go` | Entry point, component wiring |
| `cmd/server/web/` | Embedded static assets (HTML, CSS, JS) |
| `internal/domain/` | Core types (Host, Connection, Graph) |
| `internal/repository/sqlite/sqlite.go` | Database operations |
| `internal/service/infrastructure.go` | Business logic |
| `internal/handler/api.go` | HTTP API handlers |
| `internal/hub/hub.go` | SSE connection manager |
| `internal/watcher/watcher.go` | File system watcher |
| `internal/loader/yaml.go` | Infrastructure YAML parser |

## Architecture Decisions

1. **Single binary**: All assets embedded via `//go:embed` for simple deployment
2. **SQLite**: Chosen for simplicity; single-replica K8s deployment with Recreate strategy
3. **SSE over WebSocket**: Simpler implementation, works through proxies
4. **File watcher**: Monitors ConfigMap-mounted infrastructure.yml for changes
5. **Debouncing**: 500ms delay before processing file changes to handle multiple writes

## Deployment

Deployed to K3s cluster (brutus) via:

```bash
# From vanderlyn repo root
ansible-playbook ansible/playbooks/update-network-diagram.yml
```

This:
1. Generates infrastructure.yml from Ansible inventory
2. Updates K8s ConfigMap
3. Restarts deployment

**Manual image rebuild:**
```bash
cd netdiagram-go
docker build -t cwilson613/netdiagram-go:latest .
docker push cwilson613/netdiagram-go:latest
kubectl rollout restart deployment/netdiagram-go -n default --kubeconfig ~/.kube/config-brutus
```

## API Quick Reference

- `GET /api/graph` - Graph data for visualization
- `GET /events` - SSE stream
- `POST /api/positions` - Save node positions
- `GET /api/hosts/{id}` - Get host
- `POST /api/hosts` - Create host
- `PUT /api/hosts/{id}` - Update host
- `DELETE /api/hosts/{id}` - Delete host

## Common Tasks

### Add a new API endpoint

1. Add handler method in `internal/handler/api.go`
2. Register route in `cmd/server/main.go` (mux setup)
3. Add service method if business logic needed

### Modify the frontend

1. Edit files in `cmd/server/web/` (index.html, style.css, app.js)
2. Rebuild: `make build` (assets are embedded at compile time)
3. Test locally: `./netdiagram -addr :3000`

### Change domain types

1. Modify types in `internal/domain/`
2. Update SQLite schema in `internal/repository/sqlite/sqlite.go`
3. Update YAML loader in `internal/loader/yaml.go`
4. Run `make test` to catch issues

### Debug production

```bash
# Check pod status
kubectl get pods -n default --kubeconfig ~/.kube/config-brutus | grep netdiagram

# View logs
kubectl logs -f deployment/netdiagram-go -n default --kubeconfig ~/.kube/config-brutus

# Check API
curl -sk https://netdiagram.vanderlyn.house/api/graph | jq '.nodes | length'

# Exec into pod
kubectl exec -it deployment/netdiagram-go -n default --kubeconfig ~/.kube/config-brutus -- sh
```

## Testing

```bash
# Run all tests
make test

# Run specific package
go test -v ./internal/domain/...

# Test with coverage
go test -cover ./...
```

## Dependencies

- `github.com/fsnotify/fsnotify` - File watching
- `github.com/mattn/go-sqlite3` - SQLite driver (CGO)
- `gopkg.in/yaml.v3` - YAML parsing

## Notes

- CGO is required for SQLite; Docker build uses Alpine with musl-dev
- The frontend uses vis-network loaded from CDN
- Position data persists in SQLite, survives pod restarts via PVC
- SSE events trigger full graph reload on clients (simple but effective)
