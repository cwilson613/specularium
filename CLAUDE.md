# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Specularium is a network topology visualization backend for the Vanderlyn homelab. It serves both the API and web UI as a single Go binary. The name comes from Latin "place of observation/watching" - fitting for a passive visualization tool.

**Production URL**: https://specularium.vanderlyn.house
**Docker Image**: `cwilson613/specularium:latest`

## Build and Development

Go 1.23.4 is installed at `/usr/local/go`. The Makefile uses the full path so it works regardless of shell PATH configuration.

```bash
make build      # Build binary (pure-Go, CGO_ENABLED=0)
make build-arm64  # Cross-compile for ARM64
make build-armv7  # Cross-compile for ARMv7
make build-all  # Build for all architectures
make run        # Build and run on :3000
make test       # Run all tests
make tidy       # go mod tidy
make clean      # Remove binary and db files
make docker     # Build Docker image
make push       # Build and push to Docker Hub
```

### Running Single Tests

```bash
# Run a specific test
/usr/local/go/bin/go test -v ./internal/domain -run TestCapability

# Run tests in a specific package
/usr/local/go/bin/go test -v ./internal/adapter/...
```

### CLI Flags

```
./specularium -addr :3000 -db ./specularium.db -bootstrap
```

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:3000` | HTTP listen address (overrides config) |
| `-db` | `./specularium.db` | SQLite database path (overrides config) |
| `-bootstrap` | `false` | Force re-run bootstrap even if already done |

## Development Workflow

### Plan-Driven Development

For significant features or refactors, use plan files:

1. **Create plan**: `PLAN-NN-feature-name.md` with:
   - Rationale and design
   - Implementation steps
   - Integration notes
   - Verification/testing procedures
   - Completion checklist

2. **Implement**: Execute the plan, checking off items

3. **Verify**: Run tests, validate behavior

4. **Capture knowledge**: Update CLAUDE.md with permanent learnings

5. **Clean up**: Remove plan file, commit

```bash
# Example workflow
git checkout -b feature/new-capability
# Create PLAN-01-new-capability.md
# ... implement ...
# ... test ...
# Update CLAUDE.md with new knowledge
rm PLAN-01-new-capability.md
git add -A && git commit -m "Complete: new capability"
```

### Design Principles

- **Determinism**: Same config + same environment = same knowledge graph
- **Epistemic humility**: Track confidence in discoveries, acknowledge uncertainty
- **Evidence chains**: Every finding has source, method, and confidence
- **Separation of concerns**: Config (identity) survives DB (knowledge) wipes

## Architecture

### Layered Design

```
cmd/server/main.go          # Entry point, component wiring
    ├── internal/config/    # Configuration system (identity, persists across DB wipes)
    ├── internal/handler/   # HTTP API handlers
    ├── internal/service/   # Business logic, event publishing
    ├── internal/repository/sqlite/  # Data persistence
    ├── internal/domain/    # Core types (Node, Edge, Graph, Truth, Secret, Capability)
    ├── internal/adapter/   # Network discovery adapters
    ├── internal/hub/       # SSE connection manager
    └── internal/codec/     # Import/export codecs (YAML, JSON, Ansible)
```

### Key Patterns

1. **Single binary**: All assets embedded via `//go:embed`
2. **SQLite**: Pure-Go (modernc.org/sqlite), single-replica K8s deployment with Recreate strategy
3. **SSE**: Server-Sent Events for real-time updates (simpler than WebSocket)
4. **Codec pattern**: Pluggable import/export formats
5. **Adapter pattern**: Pluggable network discovery adapters (scanner, verifier, nmap, ssh probe)
6. **Truth system**: Operator assertions vs discovered reality with discrepancy tracking
7. **Capability system**: Evidence-based capability detection with confidence scoring
8. **Config/DB separation**: Config (identity) survives DB (knowledge) wipes
9. **Mode/Posture**: Capability ceiling (mode) vs behavioral aggressiveness (posture)

### Adapter System

Adapters are pluggable discovery components in `internal/adapter/`:

| Adapter | Type | Purpose |
|---------|------|---------|
| Scanner | OneShot | Subnet scanning via TCP probes, DNS lookups |
| Verifier | Continuous | Validates node reachability (ping, TCP, SSH) |
| Bootstrap | OneShot | Self-discovery of runtime environment |
| Nmap | Continuous | Service fingerprinting via nmap |
| SSHProbe | Continuous | SSH-based fact gathering |

Adapters publish discovery events and return `GraphFragment` results for reconciliation.

### Truth vs Discovery

- **Operator Truth**: Authoritative values asserted by operators (`/api/nodes/{id}/truth`)
- **Discovered**: Values found by adapters (stored in `node.Discovered` map)
- **Discrepancies**: Conflicts between truth and discovery, tracked for resolution

## Configuration

Specularium uses a YAML config file to persist identity (survives DB wipes). The database stores knowledge (can be reset).

### Config File Location

Priority order (first found wins):
1. `$SPECULARIUM_CONFIG` (explicit path)
2. `./specularium.yaml` (working directory)
3. `$XDG_CONFIG_HOME/specularium/config.yaml` (~/.config/specularium/)
4. `/etc/specularium/config.yaml` (system-wide)

### Mode vs Posture

**Mode** defines the capability ceiling based on resources:
| Mode | Description |
|------|-------------|
| `passive` | HTTP server only, no network probing |
| `monitor` | + light verification (TCP/DNS checks) |
| `discovery` | + full scanning (nmap, SSH probes) |

**Posture** defines behavioral aggressiveness:
| Posture | Verify Interval | Concurrent Probes |
|---------|-----------------|-------------------|
| `stealth` | 4 hours | 2 |
| `cautious` | 1 hour | 5 |
| `balanced` | 5 minutes | 10 |
| `aggressive` | 30 seconds | 100 |

### Capabilities

**Core** (always available, cannot be disabled):
- `http_server` - Web UI and API
- `sse_events` - Real-time updates
- `import_export` - YAML/JSON/Ansible codecs
- `basic_verification` - TCP ping, DNS lookup

**Plugins** (optional, may require external dependencies):
- `scanner` - Subnet discovery (requires mode >= monitor)
- `nmap` - Service fingerprinting (requires nmap binary, mode >= discovery)
- `ssh_probe` - SSH fact gathering (requires mode >= discovery)
- `snmp` - SNMP discovery (future, requires mode >= discovery)

### Example Config

```yaml
version: 1
posture: balanced
mode: null  # null = use bootstrap recommendation

# Behavior overrides (optional)
behavior:
  verify_interval: 5m
  max_concurrent_probes: 10

database:
  path: ./specularium.db

capabilities:
  core:
    http_server: { enabled: true }
    sse_events: { enabled: true }
  plugins:
    scanner: { enabled: true, min_mode: monitor }
    nmap: { enabled: false }  # Disabled by default

targets:
  primary:
    - 192.168.0.0/24
```

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `SPECULARIUM_CONFIG` | Explicit config file path |
| `DNS_SERVER` | Custom DNS for PTR lookups (e.g., Technitium) |
| `SCAN_SUBNETS` | Comma-separated CIDRs for nmap scanning |
| `ENABLE_SSH_PROBE` | Set to `true` to enable SSH fact gathering |

## API Endpoints

See `api/openapi.yaml` for full specification. Key endpoint groups:

- **Graph**: `GET /api/graph`, `DELETE /api/graph`, `POST /api/discover`
- **Nodes**: CRUD at `/api/nodes`, plus `POST /api/nodes/merge`
- **Edges**: CRUD at `/api/edges`
- **Positions**: `/api/positions` for layout persistence
- **Truth**: `/api/nodes/{id}/truth`, `/api/nodes/{id}/discrepancies`
- **Discrepancies**: `/api/discrepancies`, `/api/discrepancies/{id}/resolve`
- **Secrets**: CRUD at `/api/secrets`, plus `/api/secrets/types`, `/api/capabilities`
- **Import**: `/api/import/yaml`, `/api/import/ansible-inventory`, `/api/import/scan`
- **Export**: `/api/export/json`, `/api/export/yaml`, `/api/export/ansible-inventory`
- **SSE**: `GET /events`
- **Bootstrap**: `POST /api/bootstrap`, `GET /api/environment`

## Common Tasks

### Add a new node type icon

1. Create SVG in `cmd/server/web/icons/` (use `currentColor` for tinting)
2. Add entry to `nodeTypes` in `cmd/server/web/app.js`
3. Rebuild

### Add a new adapter

1. Create adapter file in `internal/adapter/`
2. Implement the `Adapter` interface (Name, Type, Start, Stop, Discover)
3. Register in `cmd/server/main.go` with `adapterRegistry.Register()`
4. Add event publisher for progress updates

### Debug production

```bash
kubectl get pods -n default --kubeconfig ~/.kube/config-brutus | grep specularium
kubectl logs -f deployment/specularium -n default --kubeconfig ~/.kube/config-brutus
curl -sk https://specularium.vanderlyn.house/api/graph | jq '.nodes | length'
```

## Deployment

```bash
# Build and push
make push

# Apply K8s manifests
kubectl apply -f ../k8s/manifests/specularium/ --kubeconfig ~/.kube/config-brutus

# Or restart to pull latest
kubectl rollout restart deployment/specularium -n default --kubeconfig ~/.kube/config-brutus
```

## SQLite

Specularium uses `modernc.org/sqlite`, a pure-Go SQLite implementation:
- No CGO required, single static binary
- Cross-compiles to ARM64, ARMv7, etc. (`make build-all`)
- DSN uses `_pragma=name(value)` syntax for pragmas
- Driver name is `"sqlite"` (not `"sqlite3"`)

Build with `CGO_ENABLED=0` for all targets.

## Notes

- Frontend uses vis-network from CDN with circularImage nodes
- 40K Mechanicus theming throughout UI ("Heresy Detected" for discrepancies)
- Position data persists in SQLite via PVC
