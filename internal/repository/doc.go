// Package repository defines the data access interfaces for Specularium.
//
// This package provides the repository abstraction layer for persisting
// and retrieving domain entities. The actual implementation is in the
// sqlite subpackage.
//
// # Repository Interface
//
// The Repository interface defines all data access methods for nodes,
// edges, positions, truth assertions, discrepancies, and secrets.
//
// # SQLite Implementation
//
// The sqlite implementation provides a complete repository using SQLite
// with WAL mode for concurrency. It handles:
//
// - CRUD operations for all entity types
// - JSON serialization of complex properties
// - Foreign key constraints and cascade deletes
// - Transactional imports for bulk operations
// - Position persistence for graph visualization
//
// # Schema Migration
//
// The sqlite repository automatically migrates the schema on startup,
// adding new columns and indexes as needed while preserving existing data.
//
// # Testing
//
// The sqlite repository is extensively tested with in-memory databases
// to ensure data integrity and proper handling of edge cases.
package repository
