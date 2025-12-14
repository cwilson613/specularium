# PLAN-02: Config System

**Branch**: `refactor/tiered-architecture`
**Status**: Pending
**Depends On**: PLAN-01 (Pure-Go SQLite) - optional but recommended
**Goal**: Implement a persistent configuration system separate from the database, enabling deterministic rebuilds and operator overrides.

---

## Rationale

Current state:
- All configuration via environment variables (ephemeral)
- No persistence of bootstrap results
- DB wipe loses everything

Target state:
- Config file persists "who I am" (survives DB wipe)
- DB stores "what I know" (can be reset)
- Same config + same environment = deterministic knowledge graph

---

## Design

### Config File Location

Priority order:
1. `$SPECULARIUM_CONFIG` (explicit path)
2. `./specularium.yaml` (working directory)
3. `$XDG_CONFIG_HOME/specularium/config.yaml` (~/.config/specularium/)
4. `/etc/specularium/config.yaml` (system-wide)

### Config Schema

```yaml
# Schema version for future migrations
version: 1

# Bootstrap results (written by bootstrap, read-only for operators)
bootstrap:
  timestamp: "2025-12-14T10:30:00Z"
  environment:
    type: container
    runtime: kubernetes
    confidence: 0.95
  resources:
    cpu_cores: 4
    memory_mb: 2048
    architecture: amd64
  permissions:
    can_icmp_ping: false
    effective_uid: 1000
  network:
    hostname: specularium-xyz
    interfaces: [...]
  recommendation:
    mode: monitor
    confidence: 0.85
    reasons: [...]

# Operator settings (editable)
mode: null              # null = use recommendation, or: passive|monitor|discovery
posture: balanced       # stealth|cautious|balanced|aggressive

# Behavior overrides (optional, posture provides defaults)
behavior:
  verify_interval: 5m
  max_concurrent_probes: 10

# Database path
database:
  path: ./specularium.db

# Plugin configuration
plugins:
  directory: ./plugins
  enabled: []
  disabled: []
  config: {}

# Discovery targets
targets:
  primary: []
  discovery: []

# Secrets references (paths, not values)
secrets:
  ssh_key_path: null
  dns_server: null
```

---

## Implementation

### Step 1: Create Config Package

```
internal/
└── config/
    ├── config.go       # Core types and loading
    ├── mode.go         # Mode/Posture types and profiles
    ├── paths.go        # Config file discovery
    ├── schema.go       # YAML schema and validation
    └── config_test.go  # Unit tests
```

### Step 2: Define Core Types

**File: `internal/config/mode.go`**

```go
package config

import "time"

// Mode defines the capability ceiling based on resources
type Mode string

const (
    ModePassive   Mode = "passive"   // HTTP server only, no probing
    ModeMonitor   Mode = "monitor"   // + light verification
    ModeDiscovery Mode = "discovery" // + full scanning
)

// Posture defines behavioral aggressiveness
type Posture string

const (
    PostureStealth    Posture = "stealth"    // Minimal, slow, quiet
    PostureCautious   Posture = "cautious"   // Conservative
    PostureBalanced   Posture = "balanced"   // Default
    PostureAggressive Posture = "aggressive" // Fast, thorough
)

// BehaviorProfile defines timing and concurrency settings
type BehaviorProfile struct {
    VerifyInterval      time.Duration
    ScanInterval        time.Duration
    ProbeTimeout        time.Duration
    MaxConcurrentProbes int
    MaxConcurrentScans  int
    MaxRetries          int
    RateLimitPerHost    int
    JitterPercent       int
}

// PostureProfiles maps postures to their default behaviors
var PostureProfiles = map[Posture]BehaviorProfile{
    PostureStealth: {
        VerifyInterval:      4 * time.Hour,
        ScanInterval:        24 * time.Hour,
        ProbeTimeout:        5 * time.Second,
        MaxConcurrentProbes: 2,
        MaxConcurrentScans:  1,
        MaxRetries:          0,
        RateLimitPerHost:    1,
        JitterPercent:       30,
    },
    PostureBalanced: {
        VerifyInterval:      5 * time.Minute,
        ScanInterval:        15 * time.Minute,
        ProbeTimeout:        2 * time.Second,
        MaxConcurrentProbes: 10,
        MaxConcurrentScans:  3,
        MaxRetries:          2,
        RateLimitPerHost:    10,
        JitterPercent:       10,
    },
    PostureAggressive: {
        VerifyInterval:      30 * time.Second,
        ScanInterval:        5 * time.Minute,
        ProbeTimeout:        1 * time.Second,
        MaxConcurrentProbes: 100,
        MaxConcurrentScans:  10,
        MaxRetries:          3,
        RateLimitPerHost:    60,
        JitterPercent:       0,
    },
}
```

### Step 3: Define Config Schema

**File: `internal/config/schema.go`**

```go
package config

import "time"

// Config is the root configuration structure
type Config struct {
    Version   int              `yaml:"version"`
    Bootstrap *BootstrapResult `yaml:"bootstrap,omitempty"`
    Mode      *Mode            `yaml:"mode"`      // nil = use recommendation
    Posture   Posture          `yaml:"posture"`
    Behavior  *BehaviorOverride `yaml:"behavior,omitempty"`
    Database  DatabaseConfig   `yaml:"database"`
    Plugins   PluginConfig     `yaml:"plugins"`
    Targets   TargetConfig     `yaml:"targets"`
    Secrets   SecretsConfig    `yaml:"secrets"`
}

// BootstrapResult stores self-discovery findings
type BootstrapResult struct {
    Timestamp      time.Time           `yaml:"timestamp"`
    Environment    EnvironmentInfo     `yaml:"environment"`
    Resources      ResourceInfo        `yaml:"resources"`
    Permissions    PermissionInfo      `yaml:"permissions"`
    Network        NetworkInfo         `yaml:"network"`
    Recommendation ModeRecommendation  `yaml:"recommendation"`
}

type EnvironmentInfo struct {
    Type       string  `yaml:"type"`       // bare_metal, vm, container
    Runtime    string  `yaml:"runtime"`    // none, docker, kubernetes, podman
    Confidence float64 `yaml:"confidence"`
}

type ResourceInfo struct {
    CPUCores     int    `yaml:"cpu_cores"`
    MemoryMB     int    `yaml:"memory_mb"`
    Architecture string `yaml:"architecture"`
}

type PermissionInfo struct {
    CanICMPPing    bool   `yaml:"can_icmp_ping"`
    CanRawSocket   bool   `yaml:"can_raw_socket"`
    CanReadProcFS  bool   `yaml:"can_read_procfs"`
    EffectiveUser  string `yaml:"effective_user"`
    EffectiveUID   int    `yaml:"effective_uid"`
}

type NetworkInfo struct {
    Hostname   string          `yaml:"hostname"`
    Interfaces []InterfaceInfo `yaml:"interfaces"`
    Gateway    string          `yaml:"gateway"`
    DNSServers []string        `yaml:"dns_servers"`
}

type InterfaceInfo struct {
    Name   string `yaml:"name"`
    IP     string `yaml:"ip"`
    Subnet string `yaml:"subnet"`
}

type ModeRecommendation struct {
    Mode       Mode     `yaml:"mode"`
    Confidence float64  `yaml:"confidence"`
    Reasons    []string `yaml:"reasons"`
}

type BehaviorOverride struct {
    VerifyInterval      *Duration `yaml:"verify_interval,omitempty"`
    ScanInterval        *Duration `yaml:"scan_interval,omitempty"`
    MaxConcurrentProbes *int      `yaml:"max_concurrent_probes,omitempty"`
}

type DatabaseConfig struct {
    Path string `yaml:"path"`
}

type PluginConfig struct {
    Directory string            `yaml:"directory"`
    Enabled   []string          `yaml:"enabled"`
    Disabled  []string          `yaml:"disabled"`
    Config    map[string]any    `yaml:"config"`
}

type TargetConfig struct {
    Primary   []string `yaml:"primary"`
    Discovery []string `yaml:"discovery"`
}

type SecretsConfig struct {
    SSHKeyPath *string `yaml:"ssh_key_path,omitempty"`
    DNSServer  *string `yaml:"dns_server,omitempty"`
}

// Duration wraps time.Duration for YAML unmarshaling
type Duration time.Duration

func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
    var s string
    if err := unmarshal(&s); err != nil {
        return err
    }
    parsed, err := time.ParseDuration(s)
    if err != nil {
        return err
    }
    *d = Duration(parsed)
    return nil
}
```

### Step 4: Implement Config Loading

**File: `internal/config/config.go`**

```go
package config

import (
    "fmt"
    "os"
    "path/filepath"

    "gopkg.in/yaml.v3"
)

// Load finds and loads the config file
func Load() (*Config, string, error) {
    path, err := findConfigPath()
    if err != nil {
        return nil, "", err
    }

    if path == "" {
        // No config found - return defaults
        return DefaultConfig(), "", nil
    }

    return LoadFromPath(path)
}

// LoadFromPath loads config from a specific path
func LoadFromPath(path string) (*Config, string, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, path, fmt.Errorf("read config: %w", err)
    }

    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, path, fmt.Errorf("parse config: %w", err)
    }

    // Apply defaults
    cfg.applyDefaults()

    return &cfg, path, nil
}

// Save writes config to the specified path
func (c *Config) Save(path string) error {
    data, err := yaml.Marshal(c)
    if err != nil {
        return fmt.Errorf("marshal config: %w", err)
    }

    // Ensure directory exists
    dir := filepath.Dir(path)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return fmt.Errorf("create config dir: %w", err)
    }

    return os.WriteFile(path, data, 0644)
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
    return &Config{
        Version: 1,
        Posture: PostureBalanced,
        Database: DatabaseConfig{
            Path: "./specularium.db",
        },
        Plugins: PluginConfig{
            Directory: "./plugins",
        },
    }
}

func (c *Config) applyDefaults() {
    if c.Version == 0 {
        c.Version = 1
    }
    if c.Posture == "" {
        c.Posture = PostureBalanced
    }
    if c.Database.Path == "" {
        c.Database.Path = "./specularium.db"
    }
}

// EffectiveMode returns the mode to use (override or recommendation)
func (c *Config) EffectiveMode() Mode {
    if c.Mode != nil {
        return *c.Mode
    }
    if c.Bootstrap != nil {
        return c.Bootstrap.Recommendation.Mode
    }
    return ModeMonitor // fallback default
}

// EffectiveBehavior returns behavior profile with overrides applied
func (c *Config) EffectiveBehavior() BehaviorProfile {
    base := PostureProfiles[c.Posture]

    if c.Behavior == nil {
        return base
    }

    // Apply overrides
    if c.Behavior.VerifyInterval != nil {
        base.VerifyInterval = time.Duration(*c.Behavior.VerifyInterval)
    }
    if c.Behavior.ScanInterval != nil {
        base.ScanInterval = time.Duration(*c.Behavior.ScanInterval)
    }
    if c.Behavior.MaxConcurrentProbes != nil {
        base.MaxConcurrentProbes = *c.Behavior.MaxConcurrentProbes
    }

    return base
}

// NeedsBootstrap returns true if bootstrap should run
func (c *Config) NeedsBootstrap() bool {
    return c.Bootstrap == nil
}
```

### Step 5: Implement Path Discovery

**File: `internal/config/paths.go`**

```go
package config

import (
    "os"
    "path/filepath"
)

const (
    EnvConfigPath = "SPECULARIUM_CONFIG"
    ConfigFileName = "specularium.yaml"
)

// findConfigPath searches for config file in priority order
func findConfigPath() (string, error) {
    // 1. Explicit environment variable
    if path := os.Getenv(EnvConfigPath); path != "" {
        if _, err := os.Stat(path); err == nil {
            return path, nil
        }
    }

    // 2. Working directory
    if _, err := os.Stat(ConfigFileName); err == nil {
        return ConfigFileName, nil
    }

    // 3. XDG config home
    if xdgHome := os.Getenv("XDG_CONFIG_HOME"); xdgHome != "" {
        path := filepath.Join(xdgHome, "specularium", "config.yaml")
        if _, err := os.Stat(path); err == nil {
            return path, nil
        }
    }

    // 4. Default XDG location
    if home := os.Getenv("HOME"); home != "" {
        path := filepath.Join(home, ".config", "specularium", "config.yaml")
        if _, err := os.Stat(path); err == nil {
            return path, nil
        }
    }

    // 5. System-wide
    systemPath := "/etc/specularium/config.yaml"
    if _, err := os.Stat(systemPath); err == nil {
        return systemPath, nil
    }

    // No config found
    return "", nil
}

// DefaultConfigPath returns where to create a new config
func DefaultConfigPath() string {
    // Prefer XDG config home
    if xdgHome := os.Getenv("XDG_CONFIG_HOME"); xdgHome != "" {
        return filepath.Join(xdgHome, "specularium", "config.yaml")
    }
    if home := os.Getenv("HOME"); home != "" {
        return filepath.Join(home, ".config", "specularium", "config.yaml")
    }
    // Fallback to working directory
    return ConfigFileName
}
```

### Step 6: Update Main Entry Point

**File: `cmd/server/main.go`** (additions)

```go
import (
    "specularium/internal/config"
)

func main() {
    // Parse flags
    addr := flag.String("addr", "", "HTTP listen address (overrides config)")
    dbPath := flag.String("db", "", "SQLite database path (overrides config)")
    forceBootstrap := flag.Bool("bootstrap", false, "Force re-run bootstrap")
    flag.Parse()

    // Load config
    cfg, configPath, err := config.Load()
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    if configPath != "" {
        log.Printf("Loaded config from: %s", configPath)
    } else {
        log.Println("No config file found, using defaults")
        configPath = config.DefaultConfigPath()
    }

    // Check if bootstrap needed
    if cfg.NeedsBootstrap() || *forceBootstrap {
        log.Println("Running bootstrap...")
        // Bootstrap will be implemented in Phase 3
        // For now, just log
        log.Println("Bootstrap not yet implemented, using defaults")
    }

    // Apply flag overrides
    effectiveAddr := *addr
    if effectiveAddr == "" {
        effectiveAddr = ":3000" // TODO: add to config
    }

    effectiveDBPath := *dbPath
    if effectiveDBPath == "" {
        effectiveDBPath = cfg.Database.Path
    }

    // Get effective mode and behavior
    mode := cfg.EffectiveMode()
    behavior := cfg.EffectiveBehavior()

    log.Printf("Operating in %s mode with %s posture", mode, cfg.Posture)
    log.Printf("Behavior: verify=%s, scan=%s, concurrency=%d",
        behavior.VerifyInterval, behavior.ScanInterval, behavior.MaxConcurrentProbes)

    // Continue with existing initialization...
}
```

---

## Integration

### Environment Variable Migration

Current env vars become config equivalents:

| Env Var | Config Field |
|---------|--------------|
| `ENABLE_SSH_PROBE` | `plugins.enabled: [ssh-probe]` |
| `SCAN_SUBNETS` | `targets.primary` |
| `DNS_SERVER` | `secrets.dns_server` |

Env vars still work as overrides (for container deployments).

### Adapter Registry Integration

Pass behavior profile to adapters:

```go
verifierConfig := adapter.VerifierConfig{
    PingTimeout:   behavior.ProbeTimeout,
    MaxConcurrent: behavior.MaxConcurrentProbes,
    // ...
}
```

---

## Verification & Testing

### Unit Tests

```go
// internal/config/config_test.go

func TestLoadDefaults(t *testing.T) {
    cfg := config.DefaultConfig()
    assert.Equal(t, config.PostureBalanced, cfg.Posture)
    assert.Equal(t, "./specularium.db", cfg.Database.Path)
}

func TestEffectiveMode(t *testing.T) {
    // No override, no bootstrap - use fallback
    cfg := config.DefaultConfig()
    assert.Equal(t, config.ModeMonitor, cfg.EffectiveMode())

    // With override
    mode := config.ModePassive
    cfg.Mode = &mode
    assert.Equal(t, config.ModePassive, cfg.EffectiveMode())
}

func TestBehaviorOverride(t *testing.T) {
    cfg := config.DefaultConfig()
    cfg.Posture = config.PostureBalanced

    override := 10 * time.Minute
    cfg.Behavior = &config.BehaviorOverride{
        VerifyInterval: (*config.Duration)(&override),
    }

    behavior := cfg.EffectiveBehavior()
    assert.Equal(t, 10*time.Minute, behavior.VerifyInterval)
    // Other fields should be posture defaults
    assert.Equal(t, 10, behavior.MaxConcurrentProbes)
}
```

### Integration Test

```bash
# Test config file loading
cat > /tmp/specularium-test.yaml << 'EOF'
version: 1
posture: aggressive
database:
  path: /tmp/test.db
targets:
  primary:
    - 192.168.1.0/24
EOF

SPECULARIUM_CONFIG=/tmp/specularium-test.yaml ./specularium &
# Should log "Operating in monitor mode with aggressive posture"

# Test config persistence
curl -X POST http://localhost:3000/api/bootstrap  # When implemented
cat /tmp/specularium-test.yaml
# Should now have bootstrap section
```

---

## Completion Checklist

- [ ] Create `internal/config/` package
- [ ] Implement `mode.go` (Mode, Posture, BehaviorProfile)
- [ ] Implement `schema.go` (Config struct, YAML tags)
- [ ] Implement `config.go` (Load, Save, defaults)
- [ ] Implement `paths.go` (config file discovery)
- [ ] Write unit tests
- [ ] Update `cmd/server/main.go` to use config
- [ ] Verify env var overrides still work
- [ ] Test config file persistence
- [ ] Test posture/behavior profiles

---

## Knowledge Capture (for CLAUDE.md)

After completion, update CLAUDE.md with:

```markdown
## Configuration

Config file location (priority order):
1. `$SPECULARIUM_CONFIG`
2. `./specularium.yaml`
3. `~/.config/specularium/config.yaml`
4. `/etc/specularium/config.yaml`

Key settings:
- `mode`: passive | monitor | discovery (or null for auto)
- `posture`: stealth | cautious | balanced | aggressive
- `behavior`: override specific timing/concurrency settings

Config survives DB wipes. Wiping the DB resets knowledge; config defines identity.

CLI flags:
- `-bootstrap`: Force re-run bootstrap
- `-db`: Override database path
- `-addr`: Override listen address
```

---

## Plan File Removal

After successful merge:

```bash
rm PLAN-02-config-system.md
git add -A && git commit -m "Complete Phase 2: Config system"
```
