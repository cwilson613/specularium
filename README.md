# netdiagram-go

A Go-based network topology visualization tool for the Vanderlyn homelab infrastructure.

**Live**: https://netdiagram.vanderlyn.house

## Features

- **SQLite persistence** - Hosts, connections, and layout positions stored locally
- **YAML import** - Reads infrastructure.yml generated from Ansible inventory
- **Real-time updates** - Server-Sent Events (SSE) for live topology changes
- **File watching** - Auto-reload when infrastructure.yml changes (with debouncing)
- **Interactive graph** - vis-network visualization with drag-and-drop positioning
- **Position persistence** - Save node positions to maintain consistent layouts
- **Single binary** - Embedded static assets via `//go:embed`, no external dependencies
- **CRT terminal theme** - Retro green phosphor aesthetic

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    netdiagram-go                            │
├─────────────────────────────────────────────────────────────┤
│  cmd/server/                                                │
│  ├── main.go          Entry point, wires components        │
│  └── web/             Embedded static assets (HTML/CSS/JS) │
├─────────────────────────────────────────────────────────────┤
│  internal/                                                  │
│  ├── domain/          Core types: Host, Connection, Graph  │
│  ├── repository/      Data access interface                │
│  │   └── sqlite/      SQLite implementation                │
│  ├── loader/          YAML file parser                     │
│  ├── service/         Business logic, caching              │
│  ├── handler/         HTTP API handlers                    │
│  ├── hub/             SSE connection manager               │
│  └── watcher/         fsnotify file watcher                │
└─────────────────────────────────────────────────────────────┘
```

## Quick Start

### Local Development

```bash
# Build (requires CGO for SQLite)
make build

# Run with sample data
./netdiagram -addr :3000 -db ./netdiagram.db -yaml examples/infrastructure.yml

# Or use make target
make dev
```

### Docker

```bash
# Build image
docker build -t netdiagram-go:latest .

# Run container
docker run -d \
  --name netdiagram \
  -p 3000:3000 \
  -v $(pwd)/data:/data \
  -v $(pwd)/infrastructure.yml:/config/infrastructure.yml \
  netdiagram-go:latest \
  -yaml /config/infrastructure.yml
```

## Deployment (K8s)

The application is deployed to the Vanderlyn K3s cluster:

```bash
# Update infrastructure data from Ansible inventory
ansible-playbook ansible/playbooks/update-network-diagram.yml

# Manual deployment (if needed)
kubectl apply -f ../k8s/manifests/netdiagram-go/ --kubeconfig ~/.kube/config-brutus
```

**K8s Resources:**
- Deployment: `netdiagram-go` (1 replica, Recreate strategy for SQLite)
- Service: `netdiagram-go` (ClusterIP, port 80 -> 3000)
- Ingress: `netdiagram.vanderlyn.house` (Traefik, TLS via cert-manager)
- ConfigMap: `netdiagram-go-config` (infrastructure.yml)
- PVC: `netdiagram-go-data` (1Gi, SQLite database)

## API Reference

### Graph Visualization

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/graph` | Graph data for vis-network (nodes + edges) |
| `GET` | `/events` | SSE stream for real-time updates |

### Infrastructure CRUD

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/infrastructure` | Full infrastructure data |
| `GET` | `/api/hosts/{id}` | Get single host |
| `POST` | `/api/hosts` | Create host |
| `PUT` | `/api/hosts/{id}` | Update host |
| `DELETE` | `/api/hosts/{id}` | Delete host |
| `POST` | `/api/connections` | Create connection |
| `DELETE` | `/api/connections/{id}` | Delete connection |

### Persistence & Import

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/positions` | Save node positions (JSON map) |
| `GET` | `/api/export` | Export as YAML |
| `POST` | `/api/import?path=...` | Import from YAML file path |
| `POST` | `/api/reload` | Force reload from database |

## Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:3000` | HTTP listen address |
| `-db` | `./netdiagram.db` | SQLite database path |
| `-yaml` | (none) | Infrastructure YAML to import and watch |

## Data Flow

```
Ansible Inventory (hosts.yml)
        │
        ▼
generate-infrastructure-yaml.py
        │
        ▼
infrastructure.yml (ConfigMap)
        │
        ▼
netdiagram-go (file watcher)
        │
        ├──► SQLite (hosts, connections, positions)
        │
        └──► SSE Hub ──► Browser clients
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

- **domain/**: Pure Go types, no dependencies
- **repository/**: Interface + SQLite implementation
- **service/**: Business logic, event publishing
- **handler/**: HTTP routing, JSON encoding
- **hub/**: SSE broadcast to connected clients
- **watcher/**: fsnotify with 500ms debounce
- **loader/**: YAML parsing to domain types

## Infrastructure YAML Schema

```yaml
version: "1.0"
metadata:
  network:
    cidr: "192.168.0.0/24"
    gateway: "192.168.0.1"
    domains:
      internal: "vanderlyn.local"
      external: "vanderlyn.house"
  description: "Network Infrastructure"

groups:
  k8s_workers:
    members: [node1, node2]

hosts:
  router:
    ip: "192.168.0.1"
    role: "Router"
    platform: "edgeos"
    classification: "device"
    network:
      ports:
        - name: "wan"
          speed_gbps: 1
        - name: "lan1"
          speed_gbps: 2
          connected_to: "switch:eth0"

  server1:
    ip: "192.168.0.10"
    role: "Application Server"
    platform: "debian"
    network:
      ports:
        - name: "eth0"
          speed_gbps: 1
          connected_to: "router:lan1"
```
