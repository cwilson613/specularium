# Specularium

A Go-based network topology visualization tool for the Vanderlyn homelab infrastructure.

**Live**: https://specularium.vanderlyn.house

## Features

- **SQLite persistence** - Nodes, edges, and layout positions stored locally
- **Import/Export** - Ansible inventory, YAML, and JSON formats
- **Real-time updates** - Server-Sent Events (SSE) for live topology changes
- **Network scanning** - Subnet discovery with port scanning and service detection
- **Truth assertions** - Lock expected values and detect discrepancies
- **Verification adapters** - Automated reachability checking and status tracking
- **Interactive graph** - vis-network visualization with drag-and-drop positioning
- **Position persistence** - Save node positions to maintain consistent layouts
- **Single binary** - Embedded static assets via `//go:embed`, no external dependencies
- **40K Mechanicus theme** - Dark gothic industrial CRT terminal aesthetic

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Specularium                              │
├─────────────────────────────────────────────────────────────┤
│  cmd/server/                                                │
│  ├── main.go          Entry point, wires components        │
│  └── web/             Embedded static assets (HTML/CSS/JS) │
├─────────────────────────────────────────────────────────────┤
│  internal/                                                  │
│  ├── domain/          Core types: Node, Edge, Graph        │
│  ├── repository/      Data access interface                │
│  │   └── sqlite/      SQLite implementation                │
│  ├── adapter/         Discovery and verification adapters  │
│  ├── codec/           Import/export format codecs          │
│  ├── service/         Business logic, event bus            │
│  ├── handler/         HTTP API handlers                    │
│  └── hub/             SSE connection manager               │
└─────────────────────────────────────────────────────────────┘
```

## Quick Start

### Local Development

```bash
# Build (requires CGO for SQLite)
make build

# Run locally
./specularium -addr :3000 -db ./specularium.db

# Or use make target
make run
```

### Docker

```bash
# Build and push image
make docker-push

# Run container
docker run -d \
  --name specularium \
  -p 3000:3000 \
  -v $(pwd)/data:/data \
  cwilson613/specularium:latest
```

## Deployment (K8s)

The application is deployed to the Vanderlyn K3s cluster:

```bash
# Build and push new image
make docker-push

# Deploy to cluster
kubectl apply -f ../k8s/manifests/specularium/ --kubeconfig ~/.kube/config-brutus

# Or restart to pull latest
kubectl rollout restart deployment/specularium -n default --kubeconfig ~/.kube/config-brutus
```

**K8s Resources:**
- Deployment: `specularium` (1 replica, Recreate strategy for SQLite)
- Service: `specularium` (ClusterIP, port 80 -> 3000)
- Ingress: `specularium.vanderlyn.house` (Traefik, TLS via cert-manager)
- PVC: `specularium-data` (1Gi, SQLite database)

## API Reference

### Graph Visualization

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/graph` | Graph data for vis-network (nodes + edges) |
| `GET` | `/events` | SSE stream for real-time updates |

### Node CRUD

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/nodes` | List all nodes (filter by type/source) |
| `POST` | `/api/nodes` | Create node |
| `GET` | `/api/nodes/{id}` | Get single node |
| `PUT` | `/api/nodes/{id}` | Update node |
| `DELETE` | `/api/nodes/{id}` | Delete node |

### Edge CRUD

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/edges` | List all edges |
| `POST` | `/api/edges` | Create edge |
| `GET` | `/api/edges/{id}` | Get single edge |
| `PUT` | `/api/edges/{id}` | Update edge |
| `DELETE` | `/api/edges/{id}` | Delete edge |

### Positions

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/positions` | Get all positions |
| `POST` | `/api/positions` | Bulk save positions |
| `PUT` | `/api/positions/{node_id}` | Update single position |

### Import/Export

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/import/yaml` | Import generic YAML |
| `POST` | `/api/import/ansible-inventory` | Import Ansible inventory |
| `POST` | `/api/import/scan` | Network scan (CIDR) |
| `GET` | `/api/export/json` | Export as JSON |
| `GET` | `/api/export/yaml` | Export as YAML |
| `GET` | `/api/export/ansible-inventory` | Export as Ansible inventory |

### Truth & Verification

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/discover` | Trigger verification of all nodes |
| `GET` | `/api/nodes/{id}/truth` | Get truth assertions |
| `PUT` | `/api/nodes/{id}/truth` | Set truth assertions |
| `DELETE` | `/api/nodes/{id}/truth` | Clear truth assertions |
| `GET` | `/api/nodes/{id}/discrepancies` | Get node discrepancies |
| `GET` | `/api/discrepancies` | List all discrepancies |
| `POST` | `/api/discrepancies/{id}/resolve` | Resolve discrepancy |

## Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:3000` | HTTP listen address |
| `-db` | `./specularium.db` | SQLite database path |

## Data Flow

```
Ansible Inventory (hosts.yml)
        │
        ▼
Import via API (/api/import/ansible-inventory)
        │
        ▼
SQLite Database (source of truth)
        │
        ├──► Verification Adapter (polls nodes)
        │        │
        │        └──► Updates status, detects discrepancies
        │
        ├──► Scanner Adapter (subnet scans)
        │        │
        │        └──► Discovers new nodes
        │
        └──► SSE Hub ──► Browser clients (real-time updates)
```

## Development

```bash
# Install dependencies
go mod download

# Run tests
make test

# Format code
make fmt

# Tidy modules
make tidy

# Build for current platform
make build

# Build Docker image
make docker
```

### Code Structure

- **domain/**: Pure Go types (Node, Edge, Graph, Truth)
- **repository/**: SQLite implementation with full CRUD
- **adapter/**: Discovery and verification adapters
- **codec/**: Import/export format handlers
- **service/**: Business logic, event publishing
- **handler/**: HTTP routing, JSON encoding
- **hub/**: SSE broadcast to connected clients

## Key Features

### Truth Assertions
Lock expected values for nodes to detect drift and configuration changes:
- Define canonical properties (IP, MAC, hostname, etc.)
- Automatic discrepancy detection when actual values differ
- Visual indicators in UI (gold borders for asserted, amber for conflicts)
- Operator-defined "source of truth" for critical infrastructure

### Verification Adapters
Automated reachability checking and status tracking:
- Periodic ICMP ping checks
- TCP port availability verification
- Latency measurement and trending
- Automatic status updates (verified, unreachable, degraded)

### Network Scanning
Subnet discovery with detailed host profiling:
- CIDR range scanning with configurable ports
- Service detection with banner grabbing
- MAC address and reverse DNS lookup
- Integration with truth system for conflict detection

### Import/Export
Multiple format support for integration:
- **Ansible Inventory**: Direct import from Ansible infrastructure
- **YAML**: Generic structured format
- **JSON**: API-friendly format
- All formats support bidirectional conversion
