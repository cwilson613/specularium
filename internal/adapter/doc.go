// Package adapter implements network discovery adapters for Specularium.
//
// Adapters are pluggable components that discover and verify nodes in the network
// topology. Each adapter implements specific discovery capabilities and registers
// with the central adapter registry.
//
// # Adapter Types
//
// AdapterTypeOneShot runs once on-demand (subnet scanner, DNS importer)
// AdapterTypeContinuous runs continuously (SNMP poller, API watcher)
// AdapterTypeVerifier validates discovered nodes (ping, SSH, SNMP)
//
// # Core Adapters
//
// Scanner performs subnet scanning to discover live hosts via TCP port probes,
// DNS lookups, and banner grabbing. It can scan arbitrary CIDR ranges and
// reports progress via events.
//
// Verifier validates node reachability using ping, TCP probes, and SSH/SNMP
// checks. It updates node verification status and discovered properties.
//
// Bootstrap performs self-discovery to identify the runtime environment
// (Kubernetes, Docker, bare metal) and populates initial network context.
//
// # Adapter Registry
//
// Registry manages all registered adapters and coordinates discovery operations.
// It provides a unified interface for triggering discovery, managing adapter
// lifecycle, and publishing discovery events.
//
// # Capabilities System
//
// Adapters declare capabilities (ping, ssh, snmp, dns) that determine which
// discovery methods they can perform. Capabilities can require secrets for
// authentication.
//
// # Event System
//
// Adapters publish discovery progress events for real-time feedback in the UI,
// including host discovery, service detection, and verification status updates.
package adapter
