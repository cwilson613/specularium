// Package handler implements HTTP request handlers for the Specularium API.
//
// This package provides the HTTP layer for the Specularium REST API, handling
// requests for graph operations, truth assertions, discovery triggers, and
// secret management.
//
// # Handlers
//
// GraphHandler handles graph-related operations including nodes, edges,
// positions, and import/export functionality.
//
// TruthHandler manages operator truth assertions and discrepancy tracking.
//
// Middleware provides request logging, error handling, and CORS support.
//
// # API Design
//
// All handlers follow REST conventions:
// - GET for retrieval
// - POST for creation
// - PUT for updates
// - DELETE for removal
//
// Errors are returned as JSON with appropriate HTTP status codes.
// Request bodies are validated before processing.
//
// # Response Format
//
// Success responses return JSON data with appropriate status codes (200, 201).
// Error responses return JSON with {error, details} structure.
//
// # Server-Sent Events
//
// The /events endpoint provides real-time graph updates via SSE, allowing
// clients to receive live notifications of topology changes.
//
// # Discovery Integration
//
// Handlers can trigger network discovery operations via the adapter registry,
// including subnet scanning, verification runs, and bootstrap self-discovery.
package handler
