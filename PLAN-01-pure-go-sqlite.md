# PLAN-01: Pure-Go SQLite Migration

**Branch**: `refactor/tiered-architecture`
**Status**: Pending
**Goal**: Replace CGO-dependent `mattn/go-sqlite3` with pure-Go `modernc.org/sqlite` to enable cross-compilation and IoT deployment.

---

## Rationale

The current SQLite driver requires CGO, which:
- Complicates cross-compilation (ARM, MIPS)
- Requires C toolchain in Docker builds
- Adds ~5MB to binary size from C library

`modernc.org/sqlite` is a pure-Go SQLite implementation (~10% slower, acceptable for our workload).

---

## Implementation

### Step 1: Update Dependencies

```bash
# Remove CGO driver
go get -u github.com/mattn/go-sqlite3@none

# Add pure-Go driver
go get modernc.org/sqlite
go mod tidy
```

### Step 2: Update Import Path

In `internal/repository/sqlite/sqlite.go`:

```go
// BEFORE
import (
    "database/sql"
    _ "github.com/mattn/go-sqlite3"
)

// AFTER
import (
    "database/sql"
    _ "modernc.org/sqlite"
)
```

The driver registers itself as `"sqlite"` (not `"sqlite3"`), so update the `sql.Open()` call:

```go
// BEFORE
db, err := sql.Open("sqlite3", dsn)

// AFTER
db, err := sql.Open("sqlite", dsn)
```

### Step 3: Update DSN Parameters

The pure-Go driver uses slightly different pragma syntax:

```go
// BEFORE (mattn style)
dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000", path)

// AFTER (modernc style - uses _pragma prefix)
dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", path)
```

### Step 4: Update Makefile

Remove CGO requirement:

```makefile
# BEFORE
build:
	CGO_ENABLED=1 $(GO) build -o $(BINARY) ./cmd/server

# AFTER
build:
	CGO_ENABLED=0 $(GO) build -o $(BINARY) ./cmd/server
```

### Step 5: Update Dockerfile

Simplify build stage (no more gcc/musl-dev):

```dockerfile
# BEFORE
FROM golang:1.22-alpine AS builder
RUN apk add --no-cache gcc musl-dev

# AFTER
FROM golang:1.22-alpine AS builder
# No C toolchain needed
```

Simplify runtime stage:

```dockerfile
# BEFORE
RUN apk add --no-cache ca-certificates sqlite-libs tzdata nmap nmap-scripts

# AFTER
RUN apk add --no-cache ca-certificates tzdata nmap nmap-scripts
# sqlite-libs not needed - pure Go
```

---

## Integration

### Database Compatibility

The on-disk format is identical (both use SQLite 3.x format). Existing databases will work without migration.

### Query Compatibility

All standard SQL queries work identically. The driver implements `database/sql` interface.

### Known Differences

1. **Performance**: ~10% slower for write-heavy workloads (acceptable)
2. **Error messages**: Slightly different wording in some errors
3. **Pragma syntax**: Different DSN format (handled in Step 3)

---

## Verification & Testing

### Unit Tests

```bash
# Run existing tests - should all pass
make test

# Specifically test repository
/usr/local/go/bin/go test -v ./internal/repository/sqlite/...
```

### Cross-Compilation Test

```bash
# Test ARM64 build (Raspberry Pi)
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o specularium-arm64 ./cmd/server
file specularium-arm64
# Should show: "ELF 64-bit LSB executable, ARM aarch64"

# Test ARMv7 build (Pi Zero 2, older Pis)
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -o specularium-armv7 ./cmd/server
file specularium-armv7
# Should show: "ELF 32-bit LSB executable, ARM"
```

### Functional Test

```bash
# Build and run
make build
./specularium -db ./test.db &

# Test API
curl -s http://localhost:3000/api/graph | jq '.nodes | length'

# Create a node
curl -X POST http://localhost:3000/api/nodes \
  -H "Content-Type: application/json" \
  -d '{"id":"test","type":"server","label":"Test Node"}'

# Verify persistence
pkill specularium
./specularium -db ./test.db &
curl -s http://localhost:3000/api/nodes/test | jq .

# Cleanup
pkill specularium
rm test.db*
```

### Docker Build Test

```bash
# Build image without CGO
docker build -t specularium:pure-go .

# Verify it runs
docker run --rm -p 3000:3000 specularium:pure-go &
curl -s http://localhost:3000/api/graph
docker stop $(docker ps -q --filter ancestor=specularium:pure-go)
```

---

## Completion Checklist

- [ ] Dependencies updated in `go.mod`
- [ ] Import path changed in `sqlite.go`
- [ ] DSN format updated for pragmas
- [ ] `sql.Open()` driver name changed to `"sqlite"`
- [ ] Makefile updated (`CGO_ENABLED=0`)
- [ ] Dockerfile simplified (no gcc/musl-dev)
- [ ] All existing tests pass
- [ ] Cross-compilation verified (ARM64, ARMv7)
- [ ] Docker build works
- [ ] Manual functional test passes

---

## Knowledge Capture (for CLAUDE.md)

After completion, update CLAUDE.md with:

```markdown
## SQLite

Specularium uses `modernc.org/sqlite`, a pure-Go SQLite implementation:
- No CGO required, single static binary
- Cross-compiles to ARM, MIPS, etc.
- DSN uses `_pragma=name(value)` syntax for pragmas
- Driver name is `"sqlite"` (not `"sqlite3"`)

Build with `CGO_ENABLED=0` for all targets.
```

---

## Rollback

If issues arise:

```bash
go get github.com/mattn/go-sqlite3
# Revert changes to sqlite.go, Makefile, Dockerfile
```

---

## Plan File Removal

After successful merge to main:

```bash
rm PLAN-01-pure-go-sqlite.md
git add -A && git commit -m "Complete Phase 1: Pure-Go SQLite migration"
```
