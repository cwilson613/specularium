// Package service implements business logic for the Specularium application.
//
// This package provides service layers that coordinate between the HTTP handlers
// and the repository layer, implementing business rules, validation, and event
// publishing.
//
// # Services
//
// GraphService manages graph operations (nodes, edges, positions) and handles
// import/export functionality via codec adapters.
//
// TruthService manages operator truth assertions and discrepancy tracking,
// allowing operators to assert authoritative values for node properties and
// detect conflicts with discovered reality.
//
// SecretsService provides unified access to secrets from multiple sources
// (mounted files, environment variables, operator-created) for use in network
// discovery adapters.
//
// # Event System
//
// All services publish events via EventBus for real-time updates to connected
// clients via Server-Sent Events (SSE). Event types include node/edge creation,
// truth assertions, discrepancy detection, and more.
//
// # Design Principles
//
// - Services own business logic and validation
// - Repository pattern for data access
// - Event-driven for real-time updates
// - Context-aware for cancellation and timeouts
package service
