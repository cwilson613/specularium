# Nmap Adapter

The nmap adapter provides network scanning and service fingerprinting capabilities for Specularium using the nmap tool.

## Overview

- **Type**: Polling adapter
- **Priority**: 80 (higher than basic scanner, lower than bootstrap)
- **Dependencies**: Requires `nmap` binary installed on the system
- **Evidence Types**: Generates `EvidenceSourcePortScan` and `EvidenceSourceBanner` evidence

## Features

- **Service Detection**: Uses nmap's `-sV` flag to identify services and versions
- **OS Detection**: Optional `-O` flag (requires root privileges)
- **Port Scanning**: Configurable port ranges (individual ports, ranges, or presets)
- **MAC Discovery**: Extracts MAC addresses and vendor information
- **Evidence Generation**: Creates structured evidence for each discovered service

## Usage

### Basic Configuration

```go
// Scan a single subnet
adapter := adapter.NewNmapAdapter(
    []string{"192.168.0.0/24"},
)
```

### With Options

```go
// Custom configuration
adapter := adapter.NewNmapAdapter(
    []string{"192.168.0.0/24", "10.0.0.0/24"},
    adapter.WithInterval(10 * time.Minute),
    adapter.WithPortRange("22,80,443,8080"),
    adapter.WithServiceDetection(true),
    adapter.WithSkipHostDiscovery(true), // Useful for ICMP-blocked networks
)
```

### Preset Scan Modes

```go
// Fast scan (minimal ports, no service detection)
adapter := adapter.NewNmapAdapter(
    []string{"192.168.0.0/24"},
    adapter.WithFastScan(),
)

// Aggressive scan (all ports, service + OS detection)
adapter := adapter.NewNmapAdapter(
    []string{"192.168.0.0/24"},
    adapter.WithAggressiveScan(), // Requires root
)

// Common homelab ports
adapter := adapter.NewNmapAdapter(
    []string{"192.168.0.0/24"},
    adapter.WithCommonPorts(), // SSH, HTTP, HTTPS, K8s, Docker, etc.
)

// Top N ports
adapter := adapter.NewNmapAdapter(
    []string{"192.168.0.0/24"},
    adapter.WithTopPorts(100), // Scan top 100 most common ports
)
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `WithInterval(d)` | Polling interval for scans | 5 minutes |
| `WithTimeout(d)` | Timeout for entire scan | 10 minutes |
| `WithPortRange(ports)` | Ports to scan (e.g., "22,80,443" or "1-1000") | Common services |
| `WithServiceDetection(bool)` | Enable service version detection | true |
| `WithOSDetection(bool)` | Enable OS detection (requires root) | false |
| `WithSkipHostDiscovery(bool)` | Skip ping, treat all hosts as online | false |
| `WithCommonPorts()` | Scan common homelab service ports | - |
| `WithTopPorts(n)` | Scan top N ports (10, 100, 1000) | - |
| `WithFastScan()` | Fast scan mode (22,80,443 only) | - |
| `WithAggressiveScan()` | Aggressive scan (all ports, OS detection) | - |

## Evidence Generation

The adapter generates structured evidence for discovered services:

### Port Scan Evidence
- **Source**: `EvidenceSourcePortScan`
- **Property**: `service:{port}` (e.g., "service:22")
- **Value**: Port state ("open")
- **Confidence**: 0.5

### Service Name Evidence
- **Source**: `EvidenceSourceBanner`
- **Property**: `service:{port}:name` (e.g., "service:22:name")
- **Value**: Service name (e.g., "ssh")
- **Confidence**: 0.7

### Product/Version Evidence
- **Source**: `EvidenceSourceBanner`
- **Property**: `service:{port}:product` (e.g., "service:22:product")
- **Value**: Product and version (e.g., "OpenSSH 8.9p1")
- **Confidence**: 0.8

## Node Discovery

The adapter creates nodes with the following discovered data:

- **IP Address**: Primary IPv4 address
- **Hostname**: Reverse DNS if available
- **MAC Address**: Hardware address (uppercase)
- **MAC Vendor**: Manufacturer from MAC OUI
- **Open Ports**: List of open port numbers
- **Services**: Detailed service information with banners
- **OS Detection**: Operating system family and version (if enabled)

## Integration

### Register with Adapter Registry

```go
// In main.go or service initialization
registry := adapter.NewRegistry(reconcileFunc)

nmapAdapter := adapter.NewNmapAdapter(
    []string{"192.168.0.0/24"},
    adapter.WithCommonPorts(),
)

err := registry.Register(nmapAdapter, adapter.AdapterConfig{
    Enabled:      true,
    Priority:     80,
    PollInterval: "15m",
})
```

### Manual Trigger

```go
// Trigger a scan manually
ctx := context.Background()
fragment, err := adapter.Sync(ctx)
if err != nil {
    log.Printf("Scan failed: %v", err)
}

// Process discovered nodes
for _, node := range fragment.Nodes {
    log.Printf("Discovered: %s (%s)", node.Label, node.GetPropertyString("ip"))
}
```

## Requirements

- **nmap binary**: Must be installed and in PATH
- **Root privileges**: Required for OS detection (`-O`) and some advanced features
- **Network access**: Adapter must have network access to target ranges

## Error Handling

The adapter gracefully handles common errors:

- **nmap not installed**: Start() returns error immediately
- **Scan failures**: Logged but don't crash the adapter
- **Invalid targets**: Validated and logged
- **Timeouts**: Configurable per scan

## Performance Considerations

- **CIDR ranges**: Nmap handles expansion internally (more efficient)
- **Concurrency**: Nmap parallelizes port scans automatically
- **Timeout**: Set appropriate timeouts for large networks
- **Polling interval**: Balance freshness vs. network load

## Comparison with Scanner Adapter

| Feature | Scanner Adapter | Nmap Adapter |
|---------|----------------|--------------|
| Speed | Fast (parallel TCP connect) | Slower (comprehensive) |
| Service Detection | Basic banner grab | Full version detection |
| OS Detection | No | Yes (with root) |
| Accuracy | Moderate | High |
| Dependencies | None | nmap binary |
| Root Required | No | Only for OS detection |

**Recommendation**: Use Scanner for fast discovery, Nmap for detailed fingerprinting.

## Example Output

```json
{
  "id": "192-168-0-100",
  "type": "server",
  "label": "brutus",
  "properties": {
    "ip": "192.168.0.100"
  },
  "discovered": {
    "reverse_dns": "brutus.vanderlyn.local",
    "mac_address": "AA:BB:CC:DD:EE:FF",
    "mac_vendor": "Dell Inc.",
    "open_ports": [22, 80, 443, 6443],
    "services": [
      {
        "port": 22,
        "service": "ssh",
        "banner": "OpenSSH 8.9p1 Ubuntu-3ubuntu0.13"
      },
      {
        "port": 6443,
        "service": "kubernetes-api",
        "banner": "Kubernetes API Server"
      }
    ],
    "nmap_evidence": [
      {
        "source": "port_scan",
        "property": "service:22",
        "value": "open",
        "confidence": 0.5
      },
      {
        "source": "banner",
        "property": "service:22:product",
        "value": "OpenSSH 8.9p1",
        "confidence": 0.8
      }
    ]
  }
}
```
