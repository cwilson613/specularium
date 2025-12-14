package bootstrap

import (
	"context"
	"log"
	"runtime"
	"time"

	"specularium/internal/config"
)

// Result contains all bootstrap findings
type Result struct {
	Timestamp      time.Time
	Duration       time.Duration
	Evidence       *EvidenceSet
	Recommendation config.ModeRecommendation
	Warnings       []string
}

// Run executes the full bootstrap sequence
func Run(ctx context.Context) (*Result, error) {
	log.Println("Bootstrap: Starting evidence-based self-discovery...")
	start := time.Now()

	evidence := NewEvidenceSet()

	// Phase 1: Environment detection
	log.Println("Bootstrap: Phase 1 - Detecting environment...")
	envEvidence := DetectEnvironment()
	evidence.AddAll(envEvidence)
	logPhaseStats("environment", envEvidence)

	// Phase 2: Resource assessment
	log.Println("Bootstrap: Phase 2 - Assessing resources...")
	resEvidence := DetectResources()
	evidence.AddAll(resEvidence)
	logPhaseStats("resources", resEvidence)

	// Phase 3: Permission probing
	log.Println("Bootstrap: Phase 3 - Probing permissions...")
	permEvidence := DetectPermissions()
	evidence.AddAll(permEvidence)
	logPhaseStats("permissions", permEvidence)

	// Phase 4: Network discovery
	log.Println("Bootstrap: Phase 4 - Discovering network...")
	netEvidence := DetectNetwork()
	evidence.AddAll(netEvidence)
	logPhaseStats("network", netEvidence)

	// Phase 5: Synthesize recommendation
	log.Println("Bootstrap: Phase 5 - Synthesizing mode recommendation...")
	synthesis := FullSynthesis(evidence)

	duration := time.Since(start)

	result := &Result{
		Timestamp: time.Now(),
		Duration:  duration,
		Evidence:  evidence,
		Recommendation: config.ModeRecommendation{
			Mode:       synthesis.Mode,
			Confidence: synthesis.Confidence,
			Reasons:    synthesis.Reasons,
		},
		Warnings: synthesis.Warnings,
	}

	// Log results
	log.Printf("Bootstrap: Complete in %s", duration)
	log.Printf("Bootstrap: Gathered %d pieces of evidence", evidence.Count())
	log.Printf("Bootstrap: Recommended mode: %s (confidence: %.0f%%)", synthesis.Mode, synthesis.Confidence*100)
	for _, r := range synthesis.Reasons {
		log.Printf("Bootstrap:   - %s", r)
	}
	for _, w := range synthesis.Warnings {
		log.Printf("Bootstrap: WARNING: %s", w)
	}

	return result, nil
}

func logPhaseStats(phase string, evidence []Evidence) {
	if len(evidence) == 0 {
		log.Printf("Bootstrap:   %s: no evidence gathered", phase)
		return
	}
	log.Printf("Bootstrap:   %s: gathered %d pieces of evidence", phase, len(evidence))
}

// ToConfigBootstrap converts Result to config-compatible format
func (r *Result) ToConfigBootstrap() *config.BootstrapResult {
	es := r.Evidence

	// Extract environment info
	envType, envConf, _ := es.BestValue(CategoryEnvironment, "environment_type")
	if envType == nil {
		envType = "unknown"
		envConf = 0.5
	}
	containerRuntime, _, _ := es.BestValue(CategoryEnvironment, "container_runtime")
	orchestrator, _, _ := es.BestValue(CategoryEnvironment, "orchestrator")

	// Determine runtime string
	runtimeStr := "none"
	if orchestrator != nil {
		runtimeStr = orchestrator.(string)
	} else if containerRuntime != nil {
		runtimeStr = containerRuntime.(string)
	}

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
	dnsServers, _, _ := es.BestValue(CategoryNetwork, "dns_servers")

	// Build interfaces list
	var interfaces []config.InterfaceInfo
	for _, e := range es.ByProperty(CategoryNetwork, "interface") {
		if raw := e.Raw; raw != nil {
			iface := config.InterfaceInfo{
				Name: stringOrDefault(raw["name"], ""),
				IP:   stringOrDefault(raw["ip"], ""),
			}
			if subnet, ok := raw["subnet"].(string); ok {
				iface.Subnet = subnet
			}
			if iface.Name != "" {
				interfaces = append(interfaces, iface)
			}
		}
	}

	// Build DNS servers list
	var dnsServerList []string
	if dnsServers != nil {
		if servers, ok := dnsServers.([]string); ok {
			dnsServerList = servers
		}
	}

	return &config.BootstrapResult{
		Timestamp: r.Timestamp,
		Environment: config.EnvironmentInfo{
			Type:       envType.(string),
			Runtime:    runtimeStr,
			Confidence: envConf,
		},
		Resources: config.ResourceInfo{
			CPUCores:     intOrDefault(cpuCores, runtime.NumCPU()),
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
			Hostname:   stringOrDefault(hostname, "unknown"),
			Gateway:    stringOrDefault(gateway, ""),
			DNSServers: dnsServerList,
			Interfaces: interfaces,
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
