# Specularium Testing Documentation

This document describes the testing strategy and coverage for the Specularium codebase.

## Test Coverage Summary

| Package | Coverage | Test Files | Description |
|---------|----------|------------|-------------|
| `internal/domain` | 55.6% | 6 test files | Core domain types and business logic |
| `internal/repository/sqlite` | 67.4% | 1 test file | Database operations (existing) |
| `internal/service` | 3.9% | 1 test file | Business logic validation |
| `internal/codec` | 0% | - | Import/export codecs (not yet tested) |
| `internal/adapter` | 0% | - | Network discovery adapters (not yet tested) |
| `internal/handler` | 0% | - | HTTP API handlers (not yet tested) |
| `internal/hub` | 0% | - | SSE event hub (not yet tested) |

## Domain Package Tests

### Files Created
- `node_test.go` - Tests for Node type, properties, hostname inference, confidence scoring
- `edge_test.go` - Tests for Edge type, ID generation, properties
- `graph_test.go` - Tests for Graph operations, node/edge/position management
- `truth_test.go` - Tests for truth assertions, discrepancies, value comparison
- `secret_test.go` - Tests for secret management, types, summaries
- `position_test.go` - Tests for node positioning
- `doc.go` - Package documentation

### Key Test Coverage
- **Node Operations**: Creation, property management, interface detection
- **Hostname Inference**: Confidence-weighted hostname discovery from multiple sources
- **Edge Operations**: ID generation, deterministic hashing, property management
- **Graph Management**: Add/retrieve nodes, edges, positions
- **Truth System**: Property assertions, discrepancy detection, value comparison
- **Secrets**: Secret types, summaries, field definitions

## Repository Package Tests

### Existing Coverage
The `internal/repository/sqlite` package has comprehensive existing tests (`sqlite_test.go`) covering:
- Helper functions (null value conversion, JSON marshaling)
- Row scanner tests
- CRUD operations for nodes, edges, positions
- Truth and discrepancy management
- Import/export fragments
- Verification operations
- Concurrent access patterns

### Test Highlights
- Uses in-memory SQLite (`:memory:`) for fast, isolated tests
- Tests cascade deletes, foreign key constraints
- Validates JSON round-tripping for complex properties
- Tests transaction isolation
- ~1900 lines of comprehensive test coverage

## Service Package Tests

### Files Created
- `service_test.go` - Tests for GraphService validation logic
- `doc.go` - Package documentation

### Test Coverage
- **Node Validation**: ID, type, label requirements
- **Edge Validation**: From/to IDs, type, self-loop prevention
- **Import Results**: Data structure validation

### Design Note
Service tests focus on validation logic that doesn't require database integration.
Full integration tests would require mocking the repository layer, which is complex
due to the concrete SQLite dependency. The existing repository tests provide
comprehensive coverage of the integrated stack.

## Package Documentation

All internal packages now have `doc.go` files with comprehensive descriptions:

- **domain**: Core domain types, truth system, hostname inference, secrets
- **service**: Business logic, event system, validation
- **repository**: Data access layer, SQLite implementation, schema migration
- **adapter**: Network discovery, verification, adapter registry, capabilities
- **handler**: HTTP API, REST conventions, SSE events

## Running Tests

### Run All Tests
```bash
make test
# or
go test ./internal/...
```

### Run Specific Package
```bash
go test ./internal/domain/... -v
go test ./internal/repository/sqlite/... -v
go test ./internal/service/... -v
```

### Run with Coverage
```bash
go test ./internal/... -cover
```

### Run with Coverage Report
```bash
go test ./internal/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Test Patterns

### Domain Tests
- Pure unit tests with no external dependencies
- Table-driven tests for multiple scenarios
- Helper assertions for readability

### Repository Tests
- In-memory SQLite for isolation
- Test helpers for common operations
- Comprehensive CRUD and edge case coverage
- Concurrent access validation

### Service Tests
- Validation logic without database integration
- Simple struct assertions
- Focus on business rule enforcement

## Future Testing Work

### Adapter Tests
Priority areas for future test coverage:
- Scanner discovery logic
- Verifier reachability checks
- Bootstrap environment detection
- Adapter registry coordination

### Handler Tests
Priority areas for future test coverage:
- HTTP endpoint validation
- Request/response serialization
- Error handling
- Authentication/authorization (if added)

### Integration Tests
Recommended additions:
- End-to-end API tests
- Discovery workflow tests
- Truth/discrepancy workflow tests
- Import/export round-trip tests

### Codec Tests
Recommended additions:
- YAML parsing and export
- JSON parsing and export
- Ansible inventory import/export
- Error handling for malformed input

## Testing Best Practices

1. **Isolation**: Each test should be independent
2. **Coverage**: Focus on critical paths first
3. **Readability**: Use descriptive test names and subtests
4. **Speed**: Keep tests fast with in-memory databases
5. **Reliability**: Avoid flaky tests, use deterministic test data
6. **Documentation**: Include test purpose in comments

## Known Issues

### TestConcurrentNodeCreation
One test in `sqlite_test.go` is currently flaky due to SQLite concurrent write
behavior. This is a known issue that doesn't affect production usage (WAL mode
handles concurrency correctly).

## Test Metrics

As of the latest test run:
- **Total test files**: 8
- **Total test functions**: 100+
- **Domain package**: 23 test functions, 55.6% coverage
- **Repository package**: 50+ test functions, 67.4% coverage
- **Service package**: 3 test functions, 3.9% coverage (validation only)

## Contributing Tests

When adding new tests:
1. Follow the existing patterns in each package
2. Use table-driven tests for multiple similar scenarios
3. Include both success and failure cases
4. Add documentation comments for complex test logic
5. Run `go test -race` to check for race conditions
6. Ensure all tests pass before committing
