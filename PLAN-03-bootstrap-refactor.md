# PLAN-03: Bootstrap Refactor

**Branch**: `refactor/tiered-architecture`
**Status**: Pending
**Depends On**: PLAN-02 (Config System)
**Goal**: Transform bootstrap from an addon adapter into the core self-discovery system that drives mode selection and builds the system's understanding from first principles.

---

## Rationale

Current state:
- Bootstrap is one adapter among many
- Runs once, discovers K8s/Docker environment
- No confidence scoring on findings
- No impact on system behavior

Target state:
- Bootstrap is the **core cognitive process**
- Runs on first launch, persists findings to config
- Every finding has confidence and evidence chain
- Drives automatic mode recommendation
- System's self-knowledge is a node in its own graph

---

## Design Philosophy

### First Principles with Shortcuts

The system starts knowing nothing. It builds understanding through observation:

1. **Direct observation** (high confidence)
   - Read /proc/cpuinfo → know CPU count
   - Check file existence → know environment type

2. **Inference** (medium confidence)
   - No container indicators → probably bare metal
   - Port 6443 open locally → probably K8s node

3. **Assumption** (low confidence)
   - Can't detect VM → assume bare metal (but note uncertainty)

Every finding is tagged with its **source** and **method**, enabling:
- Audit trail ("why does it think I'm in Docker?")
- Confidence aggregation
- Future re-evaluation

---

## Architecture

### Package Structure

```
internal/
├── core/
│   └── bootstrap/
│       ├── bootstrap.go      # Orchestrator
│       ├── evidence.go       # Evidence types and aggregation
│       ├── environment.go    # Container/VM/bare-metal detection
│       ├── resources.go      # CPU/RAM/disk assessment
│       ├── permissions.go    # Capability probing
│       ├── network.go        # Interface/route/DNS discovery
│       ├── synthesis.go      # Mode recommendation logic
│       └── bootstrap_test.go
```

### Evidence-Based Knowledge

Every piece of self-knowledge is an `Evidence` struct:

```go
type Evidence struct {
    ID         string          `json:"id"`
    Category   Category        `json:"category"`
    Property   string          `json:"property"`
    Value      any             `json:"value"`
    Confidence float64         `json:"confidence"`
    Source     string          `json:"source"`
    Method     string          `json:"method"`
    Timestamp  time.Time       `json:"timestamp"`
    DependsOn  []string        `json:"depends_on,omitempty"`
    Raw        map[string]any  `json:"raw,omitempty"`
}

type Category string

const (
    CategoryEnvironment Category = "environment"
    CategoryResources   Category = "resources"
    CategoryPermissions Category = "permissions"
    CategoryNetwork     Category = "network"
    CategoryCapability  Category = "capability"
)
```

---

## Implementation

### Step 1: Create Evidence System

**File: `internal/core/bootstrap/evidence.go`**

```go
package bootstrap

import (
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "sort"
    "time"
)

// Evidence represents a single piece of discovered knowledge
type Evidence struct {
    ID         string         `json:"id"`
    Category   Category       `json:"category"`
    Property   string         `json:"property"`
    Value      any            `json:"value"`
    Confidence float64        `json:"confidence"`
    Source     string         `json:"source"`
    Method     string         `json:"method"`
    Timestamp  time.Time      `json:"timestamp"`
    DependsOn  []string       `json:"depends_on,omitempty"`
    Raw        map[string]any `json:"raw,omitempty"`
}

// NewEvidence creates evidence with auto-generated ID
func NewEvidence(cat Category, prop string, value any, conf float64, source, method string) Evidence {
    e := Evidence{
        Category:   cat,
        Property:   prop,
        Value:      value,
        Confidence: conf,
        Source:     source,
        Method:     method,
        Timestamp:  time.Now(),
    }
    e.ID = e.generateID()
    return e
}

func (e *Evidence) generateID() string {
    data := fmt.Sprintf("%s:%s:%v:%s", e.Category, e.Property, e.Value, e.Source)
    hash := sha256.Sum256([]byte(data))
    return hex.EncodeToString(hash[:8])
}

// EvidenceSet aggregates multiple pieces of evidence
type EvidenceSet struct {
    items []Evidence
}

func NewEvidenceSet() *EvidenceSet {
    return &EvidenceSet{}
}

func (es *EvidenceSet) Add(e Evidence) {
    es.items = append(es.items, e)
}

func (es *EvidenceSet) AddAll(items []Evidence) {
    es.items = append(es.items, items...)
}

func (es *EvidenceSet) All() []Evidence {
    return es.items
}

// ByCategory returns evidence filtered by category
func (es *EvidenceSet) ByCategory(cat Category) []Evidence {
    var result []Evidence
    for _, e := range es.items {
        if e.Category == cat {
            result = append(result, e)
        }
    }
    return result
}

// BestValue returns the highest-confidence value for a property
func (es *EvidenceSet) BestValue(cat Category, prop string) (any, float64, bool) {
    var best Evidence
    var found bool

    for _, e := range es.items {
        if e.Category == cat && e.Property == prop {
            if !found || e.Confidence > best.Confidence {
                best = e
                found = true
            }
        }
    }

    if !found {
        return nil, 0, false
    }
    return best.Value, best.Confidence, true
}

// AggregateConfidence combines evidence for the same property
// Uses: max(confidences) + diminishing bonus for corroboration
func (es *EvidenceSet) AggregateConfidence(cat Category, prop string) float64 {
    var confidences []float64
    for _, e := range es.items {
        if e.Category == cat && e.Property == prop {
            confidences = append(confidences, e.Confidence)
        }
    }

    if len(confidences) == 0 {
        return 0
    }

    // Sort descending
    sort.Sort(sort.Reverse(sort.Float64Slice(confidences)))

    // Start with max, add diminishing bonus for corroboration
    result := confidences[0]
    for i := 1; i < len(confidences); i++ {
        // Each additional source adds 10% of its confidence, decaying
        bonus := confidences[i] * 0.1 / float64(i)
        result += bonus
    }

    // Cap at 0.99 (never claim absolute certainty)
    if result > 0.99 {
        result = 0.99
    }

    return result
}
```

### Step 2: Environment Detection

**File: `internal/core/bootstrap/environment.go`**

```go
package bootstrap

import (
    "os"
    "strings"
)

// DetectEnvironment gathers evidence about the execution environment
func DetectEnvironment() []Evidence {
    var evidence []Evidence

    // Docker detection
    evidence = append(evidence, detectDocker()...)

    // Kubernetes detection
    evidence = append(evidence, detectKubernetes()...)

    // VM detection
    evidence = append(evidence, detectVM()...)

    // If no container/VM evidence, infer bare metal
    if !hasContainerEvidence(evidence) && !hasVMEvidence(evidence) {
        evidence = append(evidence, NewEvidence(
            CategoryEnvironment,
            "environment_type",
            "bare_metal",
            0.60, // Lower confidence - absence of evidence
            "inference",
            "no container or VM indicators detected",
        ))
    }

    return evidence
}

func detectDocker() []Evidence {
    var evidence []Evidence

    // Method 1: /.dockerenv file
    if _, err := os.Stat("/.dockerenv"); err == nil {
        evidence = append(evidence, NewEvidence(
            CategoryEnvironment,
            "container_runtime",
            "docker",
            0.95,
            "filesystem",
            "/.dockerenv exists",
        ))
    }

    // Method 2: /proc/1/cgroup contains "docker"
    if cgroup, err := os.ReadFile("/proc/1/cgroup"); err == nil {
        if strings.Contains(string(cgroup), "docker") {
            evidence = append(evidence, NewEvidence(
                CategoryEnvironment,
                "container_runtime",
                "docker",
                0.90,
                "procfs",
                "/proc/1/cgroup contains 'docker'",
            ))
        }
    }

    // Method 3: DOCKER_CONTAINER env var (some setups)
    if os.Getenv("DOCKER_CONTAINER") != "" {
        evidence = append(evidence, NewEvidence(
            CategoryEnvironment,
            "container_runtime",
            "docker",
            0.85,
            "environment",
            "DOCKER_CONTAINER env var set",
        ))
    }

    return evidence
}

func detectKubernetes() []Evidence {
    var evidence []Evidence

    // Method 1: KUBERNETES_SERVICE_HOST (very reliable)
    if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
        evidence = append(evidence, NewEvidence(
            CategoryEnvironment,
            "orchestrator",
            "kubernetes",
            0.98,
            "environment",
            "KUBERNETES_SERVICE_HOST set",
        ))
    }

    // Method 2: /proc/1/cgroup contains "kubepods"
    if cgroup, err := os.ReadFile("/proc/1/cgroup"); err == nil {
        if strings.Contains(string(cgroup), "kubepods") {
            evidence = append(evidence, NewEvidence(
                CategoryEnvironment,
                "container_runtime",
                "kubernetes",
                0.92,
                "procfs",
                "/proc/1/cgroup contains 'kubepods'",
            ))
        }
    }

    // Method 3: Kubernetes service account mounted
    if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
        evidence = append(evidence, NewEvidence(
            CategoryEnvironment,
            "orchestrator",
            "kubernetes",
            0.95,
            "filesystem",
            "Kubernetes service account token exists",
        ))
    }

    // Method 4: Downward API env vars
    if os.Getenv("POD_NAME") != "" || os.Getenv("POD_NAMESPACE") != "" {
        evidence = append(evidence, NewEvidence(
            CategoryEnvironment,
            "orchestrator",
            "kubernetes",
            0.90,
            "environment",
            "Kubernetes downward API env vars present",
        ))
    }

    return evidence
}

func detectVM() []Evidence {
    var evidence []Evidence

    // Method 1: Check /sys/class/dmi/id/product_name
    if product, err := os.ReadFile("/sys/class/dmi/id/product_name"); err == nil {
        name := strings.TrimSpace(string(product))
        vmIndicators := map[string]string{
            "VirtualBox":     "virtualbox",
            "VMware":         "vmware",
            "KVM":            "kvm",
            "QEMU":           "qemu",
            "Hyper-V":        "hyperv",
            "Virtual Machine": "unknown_hypervisor",
        }
        for indicator, vmType := range vmIndicators {
            if strings.Contains(name, indicator) {
                evidence = append(evidence, NewEvidence(
                    CategoryEnvironment,
                    "virtualization",
                    vmType,
                    0.90,
                    "dmi",
                    fmt.Sprintf("/sys/class/dmi/id/product_name contains '%s'", indicator),
                ))
            }
        }
    }

    // Method 2: systemd-detect-virt (if available)
    // This could be added but adds external dependency

    return evidence
}

func hasContainerEvidence(evidence []Evidence) bool {
    for _, e := range evidence {
        if e.Property == "container_runtime" {
            return true
        }
    }
    return false
}

func hasVMEvidence(evidence []Evidence) bool {
    for _, e := range evidence {
        if e.Property == "virtualization" {
            return true
        }
    }
    return false
}
```

### Step 3: Resource Assessment

**File: `internal/core/bootstrap/resources.go`**

```go
package bootstrap

import (
    "os"
    "runtime"
    "strconv"
    "strings"
)

// DetectResources gathers evidence about available resources
func DetectResources() []Evidence {
    var evidence []Evidence

    // CPU cores (Go runtime)
    evidence = append(evidence, NewEvidence(
        CategoryResources,
        "cpu_cores",
        runtime.NumCPU(),
        0.99, // Very reliable
        "runtime",
        "runtime.NumCPU()",
    ))

    // Architecture
    evidence = append(evidence, NewEvidence(
        CategoryResources,
        "architecture",
        runtime.GOARCH,
        1.0, // Certain
        "runtime",
        "runtime.GOARCH",
    ))

    // Memory - try /proc/meminfo
    if memInfo, err := os.ReadFile("/proc/meminfo"); err == nil {
        if memMB := parseMemTotal(string(memInfo)); memMB > 0 {
            evidence = append(evidence, NewEvidence(
                CategoryResources,
                "memory_mb",
                memMB,
                0.95,
                "procfs",
                "/proc/meminfo MemTotal",
            ))
        }
    }

    // Cgroup memory limit (for containers)
    if limit := getCgroupMemoryLimit(); limit > 0 {
        evidence = append(evidence, NewEvidence(
            CategoryResources,
            "memory_limit_mb",
            limit,
            0.90,
            "cgroup",
            "cgroup memory.limit_in_bytes",
        ))
    }

    return evidence
}

func parseMemTotal(meminfo string) int {
    for _, line := range strings.Split(meminfo, "\n") {
        if strings.HasPrefix(line, "MemTotal:") {
            fields := strings.Fields(line)
            if len(fields) >= 2 {
                kb, err := strconv.ParseInt(fields[1], 10, 64)
                if err == nil {
                    return int(kb / 1024) // Convert to MB
                }
            }
        }
    }
    return 0
}

func getCgroupMemoryLimit() int {
    // cgroup v2
    if limit, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
        s := strings.TrimSpace(string(limit))
        if s != "max" {
            if bytes, err := strconv.ParseInt(s, 10, 64); err == nil {
                return int(bytes / 1024 / 1024)
            }
        }
    }

    // cgroup v1
    if limit, err := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes"); err == nil {
        if bytes, err := strconv.ParseInt(strings.TrimSpace(string(limit)), 10, 64); err == nil {
            // Check for "unlimited" (very large number)
            if bytes < 1<<62 {
                return int(bytes / 1024 / 1024)
            }
        }
    }

    return 0
}
```

### Step 4: Permission Probing

**File: `internal/core/bootstrap/permissions.go`**

```go
package bootstrap

import (
    "context"
    "net"
    "os"
    "os/exec"
    "os/user"
    "syscall"
    "time"
)

// DetectPermissions probes what operations we're allowed to perform
func DetectPermissions() []Evidence {
    var evidence []Evidence

    // Current user info
    evidence = append(evidence, detectUserInfo()...)

    // Capability probes
    evidence = append(evidence, probeCapabilities()...)

    return evidence
}

func detectUserInfo() []Evidence {
    var evidence []Evidence

    // Effective UID
    evidence = append(evidence, NewEvidence(
        CategoryPermissions,
        "effective_uid",
        os.Geteuid(),
        1.0,
        "syscall",
        "os.Geteuid()",
    ))

    // Is root?
    isRoot := os.Geteuid() == 0
    evidence = append(evidence, NewEvidence(
        CategoryPermissions,
        "is_root",
        isRoot,
        1.0,
        "syscall",
        "os.Geteuid() == 0",
    ))

    // Username
    if u, err := user.Current(); err == nil {
        evidence = append(evidence, NewEvidence(
            CategoryPermissions,
            "username",
            u.Username,
            0.99,
            "user",
            "user.Current().Username",
        ))
    }

    return evidence
}

func probeCapabilities() []Evidence {
    var evidence []Evidence

    // Probe: Can read /proc filesystem?
    evidence = append(evidence, probeReadProc()...)

    // Probe: Can execute ping?
    evidence = append(evidence, probePing()...)

    // Probe: Can bind to low ports?
    evidence = append(evidence, probeLowPorts()...)

    // Probe: Can create raw sockets?
    evidence = append(evidence, probeRawSocket()...)

    // Probe: Is nmap available?
    evidence = append(evidence, probeNmap()...)

    return evidence
}

func probeReadProc() []Evidence {
    _, err := os.ReadFile("/proc/1/cmdline")
    success := err == nil

    conf := 0.95
    if !success {
        conf = 0.90
    }

    return []Evidence{NewEvidence(
        CategoryCapability,
        "can_read_procfs",
        success,
        conf,
        "probe",
        "read /proc/1/cmdline",
    )}
}

func probePing() []Evidence {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()

    // Check if ping exists
    path, err := exec.LookPath("ping")
    if err != nil {
        return []Evidence{NewEvidence(
            CategoryCapability,
            "can_icmp_ping",
            false,
            0.90,
            "probe",
            "ping binary not found",
        )}
    }

    // Try to execute (to 127.0.0.1, should always work if permitted)
    cmd := exec.CommandContext(ctx, path, "-c", "1", "-W", "1", "127.0.0.1")
    err = cmd.Run()
    success := err == nil

    return []Evidence{NewEvidence(
        CategoryCapability,
        "can_icmp_ping",
        success,
        0.95,
        "probe",
        "execute ping -c 1 127.0.0.1",
    )}
}

func probeLowPorts() []Evidence {
    // Try to bind to port 80 briefly
    ln, err := net.Listen("tcp", "127.0.0.1:80")
    if err == nil {
        ln.Close()
        return []Evidence{NewEvidence(
            CategoryCapability,
            "can_bind_low_ports",
            true,
            0.95,
            "probe",
            "successfully bound to port 80",
        )}
    }

    return []Evidence{NewEvidence(
        CategoryCapability,
        "can_bind_low_ports",
        false,
        0.90,
        "probe",
        "failed to bind to port 80",
    )}
}

func probeRawSocket() []Evidence {
    // Try to create a raw socket (requires CAP_NET_RAW or root)
    fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_ICMP)
    if err == nil {
        syscall.Close(fd)
        return []Evidence{NewEvidence(
            CategoryCapability,
            "can_raw_socket",
            true,
            0.95,
            "probe",
            "successfully created ICMP raw socket",
        )}
    }

    return []Evidence{NewEvidence(
        CategoryCapability,
        "can_raw_socket",
        false,
        0.90,
        "probe",
        "failed to create raw socket: " + err.Error(),
    )}
}

func probeNmap() []Evidence {
    path, err := exec.LookPath("nmap")
    if err != nil {
        return []Evidence{NewEvidence(
            CategoryCapability,
            "has_nmap",
            false,
            0.95,
            "probe",
            "nmap not in PATH",
        )}
    }

    // Check version to confirm it works
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()

    cmd := exec.CommandContext(ctx, path, "--version")
    output, err := cmd.Output()
    if err != nil {
        return []Evidence{NewEvidence(
            CategoryCapability,
            "has_nmap",
            false,
            0.85,
            "probe",
            "nmap exists but --version failed",
        )}
    }

    return []Evidence{NewEvidence(
        CategoryCapability,
        "has_nmap",
        true,
        0.99,
        "probe",
        "nmap --version succeeded",
    ).WithRaw(map[string]any{
        "version_output": strings.Split(string(output), "\n")[0],
    })}
}

// Helper to add raw data
func (e Evidence) WithRaw(raw map[string]any) Evidence {
    e.Raw = raw
    return e
}
```

### Step 5: Mode Synthesis

**File: `internal/core/bootstrap/synthesis.go`**

```go
package bootstrap

import (
    "specularium/internal/config"
)

// SynthesizeMode recommends an operational mode based on evidence
func SynthesizeMode(es *EvidenceSet) (config.Mode, float64, []string) {
    var reasons []string

    // Start assuming we can do everything
    canDiscovery := true
    canMonitor := true
    discoveryConfidence := 0.85
    monitorConfidence := 0.80

    // Check memory constraints
    memMB, memConf, hasMemory := es.BestValue(CategoryResources, "memory_mb")
    if hasMemory {
        mem := memMB.(int)
        if mem < 128 {
            canDiscovery = false
            canMonitor = false
            reasons = append(reasons, fmt.Sprintf("insufficient memory: %dMB < 128MB minimum", mem))
        } else if mem < 512 {
            canDiscovery = false
            reasons = append(reasons, fmt.Sprintf("limited memory: %dMB < 512MB for discovery", mem))
        } else {
            reasons = append(reasons, fmt.Sprintf("adequate memory: %dMB", mem))
        }
    } else {
        // Can't determine memory - be conservative
        canDiscovery = false
        discoveryConfidence -= 0.1
        reasons = append(reasons, "could not determine available memory")
    }

    // Check for nmap (required for discovery tier)
    hasNmap, nmapConf, _ := es.BestValue(CategoryCapability, "has_nmap")
    if hasNmap != nil && hasNmap.(bool) {
        reasons = append(reasons, "nmap available")
    } else {
        canDiscovery = false
        reasons = append(reasons, "nmap not available")
    }

    // Check raw socket capability (nice to have)
    canRaw, _, _ := es.BestValue(CategoryCapability, "can_raw_socket")
    if canRaw != nil && canRaw.(bool) {
        reasons = append(reasons, "raw socket capability available")
    } else {
        reasons = append(reasons, "no raw socket capability (ICMP ping unavailable)")
    }

    // Check ICMP ping
    canPing, _, _ := es.BestValue(CategoryCapability, "can_icmp_ping")
    if canPing == nil || !canPing.(bool) {
        reasons = append(reasons, "ICMP ping unavailable (TCP ping will be used)")
    }

    // Determine recommendation
    if canDiscovery {
        // Factor in capability confidence
        if nmapConf > 0 {
            discoveryConfidence = (discoveryConfidence + nmapConf) / 2
        }
        return config.ModeDiscovery, discoveryConfidence, reasons
    }

    if canMonitor {
        return config.ModeMonitor, monitorConfidence, reasons
    }

    return config.ModePassive, 0.90, reasons
}
```

### Step 6: Bootstrap Orchestrator

**File: `internal/core/bootstrap/bootstrap.go`**

```go
package bootstrap

import (
    "context"
    "log"
    "time"

    "specularium/internal/config"
)

// Result contains all bootstrap findings
type Result struct {
    Timestamp      time.Time
    Evidence       *EvidenceSet
    Recommendation config.ModeRecommendation
}

// Run executes the full bootstrap sequence
func Run(ctx context.Context) (*Result, error) {
    log.Println("Starting bootstrap self-discovery...")
    start := time.Now()

    evidence := NewEvidenceSet()

    // Phase 1: Environment detection
    log.Println("  Phase 1: Detecting environment...")
    evidence.AddAll(DetectEnvironment())

    // Phase 2: Resource assessment
    log.Println("  Phase 2: Assessing resources...")
    evidence.AddAll(DetectResources())

    // Phase 3: Permission probing
    log.Println("  Phase 3: Probing permissions...")
    evidence.AddAll(DetectPermissions())

    // Phase 4: Network discovery (basic)
    log.Println("  Phase 4: Discovering network...")
    evidence.AddAll(DetectNetwork())

    // Phase 5: Synthesize recommendation
    log.Println("  Phase 5: Synthesizing mode recommendation...")
    mode, confidence, reasons := SynthesizeMode(evidence)

    result := &Result{
        Timestamp: time.Now(),
        Evidence:  evidence,
        Recommendation: config.ModeRecommendation{
            Mode:       mode,
            Confidence: confidence,
            Reasons:    reasons,
        },
    }

    log.Printf("Bootstrap complete in %s", time.Since(start))
    log.Printf("  Gathered %d pieces of evidence", len(evidence.All()))
    log.Printf("  Recommended mode: %s (confidence: %.0f%%)", mode, confidence*100)
    for _, r := range reasons {
        log.Printf("    - %s", r)
    }

    return result, nil
}

// ToConfigBootstrap converts result to config-compatible format
func (r *Result) ToConfigBootstrap() *config.BootstrapResult {
    es := r.Evidence

    // Extract environment info
    envType, _, _ := es.BestValue(CategoryEnvironment, "environment_type")
    if envType == nil {
        envType = "unknown"
    }
    runtime, _, _ := es.BestValue(CategoryEnvironment, "container_runtime")
    if runtime == nil {
        runtime, _, _ = es.BestValue(CategoryEnvironment, "orchestrator")
    }
    if runtime == nil {
        runtime = "none"
    }
    envConf := es.AggregateConfidence(CategoryEnvironment, "environment_type")

    // Extract resource info
    cpuCores, _, _ := es.BestValue(CategoryResources, "cpu_cores")
    memMB, _, _ := es.BestValue(CategoryResources, "memory_mb")
    arch, _, _ := es.BestValue(CategoryResources, "architecture")

    // Extract permission info
    canPing, _, _ := es.BestValue(CategoryCapability, "can_icmp_ping")
    canRaw, _, _ := es.BestValue(CategoryCapability, "can_raw_socket")
    canProc, _, _ := es.BestValue(CategoryCapability, "can_read_procfs")
    username, _, _ := es.BestValue(CategoryPermissions, "username")
    uid, _, _ := es.BestValue(CategoryPermissions, "effective_uid")

    // Extract network info
    hostname, _, _ := es.BestValue(CategoryNetwork, "hostname")
    gateway, _, _ := es.BestValue(CategoryNetwork, "gateway")

    return &config.BootstrapResult{
        Timestamp: r.Timestamp,
        Environment: config.EnvironmentInfo{
            Type:       envType.(string),
            Runtime:    runtime.(string),
            Confidence: envConf,
        },
        Resources: config.ResourceInfo{
            CPUCores:     intOrDefault(cpuCores, 1),
            MemoryMB:     intOrDefault(memMB, 0),
            Architecture: stringOrDefault(arch, runtime.GOARCH),
        },
        Permissions: config.PermissionInfo{
            CanICMPPing:   boolOrDefault(canPing, false),
            CanRawSocket:  boolOrDefault(canRaw, false),
            CanReadProcFS: boolOrDefault(canProc, false),
            EffectiveUser: stringOrDefault(username, "unknown"),
            EffectiveUID:  intOrDefault(uid, -1),
        },
        Network: config.NetworkInfo{
            Hostname: stringOrDefault(hostname, "unknown"),
            Gateway:  stringOrDefault(gateway, ""),
        },
        Recommendation: r.Recommendation,
    }
}

// Helper functions for type conversion
func intOrDefault(v any, def int) int {
    if v == nil {
        return def
    }
    if i, ok := v.(int); ok {
        return i
    }
    return def
}

func stringOrDefault(v any, def string) string {
    if v == nil {
        return def
    }
    if s, ok := v.(string); ok {
        return s
    }
    return def
}

func boolOrDefault(v any, def bool) bool {
    if v == nil {
        return def
    }
    if b, ok := v.(bool); ok {
        return b
    }
    return def
}
```

### Step 7: Update Main Entry Point

**File: `cmd/server/main.go`** (integrate bootstrap)

```go
import (
    "specularium/internal/config"
    "specularium/internal/core/bootstrap"
)

func main() {
    // ... flag parsing ...

    // Load config
    cfg, configPath, err := config.Load()
    // ...

    // Check if bootstrap needed
    if cfg.NeedsBootstrap() || *forceBootstrap {
        log.Println("Running self-discovery bootstrap...")

        result, err := bootstrap.Run(context.Background())
        if err != nil {
            log.Fatalf("Bootstrap failed: %v", err)
        }

        // Update config with bootstrap results
        cfg.Bootstrap = result.ToConfigBootstrap()

        // Save config
        if configPath == "" {
            configPath = config.DefaultConfigPath()
        }
        if err := cfg.Save(configPath); err != nil {
            log.Printf("Warning: Failed to save config: %v", err)
        } else {
            log.Printf("Saved bootstrap results to: %s", configPath)
        }
    }

    // Check for mode override and warn if exceeds recommendation
    effectiveMode := cfg.EffectiveMode()
    if cfg.Mode != nil && cfg.Bootstrap != nil {
        if modeLevel(*cfg.Mode) > modeLevel(cfg.Bootstrap.Recommendation.Mode) {
            log.Printf("WARNING: Override mode %s exceeds recommended %s",
                *cfg.Mode, cfg.Bootstrap.Recommendation.Mode)
            log.Printf("         This may cause failures on resource-constrained hardware")
        }
    }

    // ... continue with server setup ...
}

func modeLevel(m config.Mode) int {
    switch m {
    case config.ModePassive:
        return 0
    case config.ModeMonitor:
        return 1
    case config.ModeDiscovery:
        return 2
    default:
        return 1
    }
}
```

---

## Integration

### Self as Graph Node

After bootstrap, create a "self" node in the database:

```go
// In main.go after bootstrap
selfNode := domain.Node{
    ID:     "self",
    Type:   domain.NodeTypeSelf,
    Label:  cfg.Bootstrap.Network.Hostname,
    Source: "bootstrap",
    Properties: map[string]any{
        "is_self": true,
    },
    Discovered: map[string]any{
        "environment":  cfg.Bootstrap.Environment,
        "resources":    cfg.Bootstrap.Resources,
        "permissions":  cfg.Bootstrap.Permissions,
        "network":      cfg.Bootstrap.Network,
    },
}
repo.CreateNode(ctx, &selfNode)
```

### Add NodeTypeSelf

In `internal/domain/node.go`:

```go
const (
    // ... existing types ...
    NodeTypeSelf      NodeType = "self"       // This Specularium instance
)
```

---

## Verification & Testing

### Unit Tests

```go
func TestEnvironmentDetection(t *testing.T) {
    evidence := DetectEnvironment()
    // Should always produce some evidence
    assert.NotEmpty(t, evidence)
}

func TestEvidenceAggregation(t *testing.T) {
    es := NewEvidenceSet()
    es.Add(NewEvidence(CategoryEnvironment, "type", "docker", 0.90, "a", ""))
    es.Add(NewEvidence(CategoryEnvironment, "type", "docker", 0.85, "b", ""))

    // Aggregate should be > 0.90 due to corroboration
    conf := es.AggregateConfidence(CategoryEnvironment, "type")
    assert.Greater(t, conf, 0.90)
}

func TestModeSynthesis(t *testing.T) {
    es := NewEvidenceSet()
    es.Add(NewEvidence(CategoryResources, "memory_mb", 4096, 0.95, "", ""))
    es.Add(NewEvidence(CategoryCapability, "has_nmap", true, 0.99, "", ""))

    mode, conf, _ := SynthesizeMode(es)
    assert.Equal(t, config.ModeDiscovery, mode)
    assert.Greater(t, conf, 0.80)
}
```

### Integration Test

```bash
# Test fresh bootstrap
rm -f ~/.config/specularium/config.yaml
./specularium --bootstrap

# Check config was created with bootstrap section
cat ~/.config/specularium/config.yaml | grep -A 20 "bootstrap:"

# Verify self node exists
curl -s http://localhost:3000/api/nodes/self | jq .

# Test idempotency - second run should skip bootstrap
./specularium &
# Should log "Loaded prior self-knowledge..."
```

---

## Completion Checklist

- [ ] Create `internal/core/bootstrap/` package
- [ ] Implement evidence system (`evidence.go`)
- [ ] Implement environment detection (`environment.go`)
- [ ] Implement resource detection (`resources.go`)
- [ ] Implement permission probing (`permissions.go`)
- [ ] Implement network detection (`network.go`)
- [ ] Implement mode synthesis (`synthesis.go`)
- [ ] Implement orchestrator (`bootstrap.go`)
- [ ] Add `NodeTypeSelf` to domain
- [ ] Integrate with main.go
- [ ] Write unit tests
- [ ] Test bootstrap persistence
- [ ] Test mode override warnings
- [ ] Verify "self" node creation

---

## Knowledge Capture (for CLAUDE.md)

After completion, update CLAUDE.md with:

```markdown
## Bootstrap System

Specularium uses evidence-based self-discovery at startup:

1. **Environment**: Detects container/VM/bare-metal with confidence scoring
2. **Resources**: Assesses CPU, memory, architecture
3. **Permissions**: Probes capabilities (ICMP, raw sockets, nmap)
4. **Network**: Discovers interfaces, gateway, DNS

Results persist in config file (survives DB wipes). The bootstrap recommends
an operational mode based on evidence:

- **passive**: HTTP server only (low resources, no capabilities)
- **monitor**: + lightweight verification (moderate resources)
- **discovery**: + full scanning (512MB+ RAM, nmap available)

Force re-bootstrap with `--bootstrap` flag. Override mode with config `mode:` field.

The system creates a "self" node in the graph representing its own knowledge.
```

---

## Plan File Removal

After successful merge:

```bash
rm PLAN-03-bootstrap-refactor.md
git add -A && git commit -m "Complete Phase 3: Bootstrap refactor"
```
