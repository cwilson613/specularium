// Package domain defines the core domain types for the Specularium network topology visualization system.
//
// This package contains the fundamental entities and value objects that represent
// network topology concepts, including nodes, edges, graphs, and operator truth assertions.
//
// # Core Types
//
// Node represents a network entity (server, switch, router, etc.) with properties,
// discovery status, and optional operator truth assertions.
//
// Edge represents a connection between two nodes with typed relationships
// (ethernet, vlan, virtual, aggregation).
//
// Graph represents the complete network topology with nodes, edges, and their
// visual positions.
//
// # Truth System
//
// The truth system allows operators to assert authoritative values for node properties.
// When discovered values conflict with operator truth, discrepancies are tracked
// for investigation.
//
// NodeTruth holds operator-asserted property values with metadata about who asserted
// them and when.
//
// Discrepancy tracks conflicts between operator truth and discovered reality,
// supporting resolution workflows.
//
// # Hostname Inference
//
// HostnameInference provides confidence-weighted hostname discovery from multiple
// sources (PTR, DNS, SMTP, SSH, etc.), automatically selecting the most reliable
// candidate based on source confidence scores.
//
// # Secrets
//
// Secret represents credentials and sensitive configuration (SSH keys, SNMP
// community strings, API tokens) used for network discovery. Secrets can be
// operator-created or mounted from external sources (Kubernetes, Docker).
//
// # Design Principles
//
// - Immutable value objects where possible
// - No database or external dependencies
// - Pure domain logic without infrastructure concerns
// - Rich type system with meaningful constants and enumerations
package domain
