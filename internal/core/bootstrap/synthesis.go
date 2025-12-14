package bootstrap

import (
	"fmt"

	"specularium/internal/config"
)

// SynthesizeMode recommends an operational mode based on gathered evidence
func SynthesizeMode(es *EvidenceSet) (config.Mode, float64, []string) {
	var reasons []string

	// Start with highest capability assumption
	canDiscovery := true
	canMonitor := true
	discoveryConfidence := 0.85
	monitorConfidence := 0.80

	// === Memory Constraints ===
	memMB, _, hasMemory := es.BestValue(CategoryResources, "memory_mb")
	memLimit, limitConf, hasLimit := es.BestValue(CategoryResources, "memory_limit_mb")

	// Use cgroup limit if available (more relevant for containers)
	effectiveMem := 0
	if hasLimit && limitConf > 0.5 {
		effectiveMem = memLimit.(int)
		reasons = append(reasons, fmt.Sprintf("Container memory limit: %dMB", effectiveMem))
	} else if hasMemory {
		effectiveMem = memMB.(int)
		reasons = append(reasons, fmt.Sprintf("System memory: %dMB", effectiveMem))
	}

	if effectiveMem > 0 {
		if effectiveMem < 128 {
			canDiscovery = false
			canMonitor = false
			reasons = append(reasons, fmt.Sprintf("Insufficient memory: %dMB < 128MB minimum", effectiveMem))
		} else if effectiveMem < 256 {
			canDiscovery = false
			reasons = append(reasons, fmt.Sprintf("Low memory: %dMB < 256MB for discovery", effectiveMem))
		} else if effectiveMem < 512 {
			canDiscovery = false
			reasons = append(reasons, fmt.Sprintf("Limited memory: %dMB < 512MB for full discovery", effectiveMem))
		}
	} else {
		// Can't determine memory - be conservative
		discoveryConfidence -= 0.1
		reasons = append(reasons, "Could not determine available memory")
	}

	// === Environment Type ===
	envType, _, hasEnv := es.BestValue(CategoryEnvironment, "environment_type")
	if hasEnv {
		switch envType.(string) {
		case string(EnvTypeContainerized):
			// Containers have limited network visibility by default
			reasons = append(reasons, "Running in container - network access may be limited")
			// Check if it's Kubernetes specifically
			if orch, _, has := es.BestValue(CategoryEnvironment, "orchestrator"); has {
				if orch.(string) == string(RuntimeKubernetes) {
					reasons = append(reasons, "Kubernetes pod - cluster network only by default")
				}
			}
		case string(EnvTypeVM):
			reasons = append(reasons, "Running in VM - full network access likely")
		case string(EnvTypeBareMetal):
			reasons = append(reasons, "Running on bare metal - full network access available")
		}
	}

	// === Nmap Availability ===
	hasNmap, nmapConf, _ := es.BestValue(CategoryCapability, "has_nmap")
	if hasNmap != nil && hasNmap.(bool) {
		reasons = append(reasons, "nmap available for network scanning")
	} else {
		canDiscovery = false
		reasons = append(reasons, "nmap not available - discovery mode requires nmap")
	}

	// === Raw Socket Capability ===
	canRaw, _, _ := es.BestValue(CategoryCapability, "can_raw_socket")
	if canRaw != nil && canRaw.(bool) {
		reasons = append(reasons, "Raw socket capability available")
	} else {
		reasons = append(reasons, "No raw socket capability (some scans limited)")
	}

	// === ICMP Ping ===
	canPing, _, _ := es.BestValue(CategoryCapability, "can_icmp_ping")
	if canPing != nil && canPing.(bool) {
		reasons = append(reasons, "ICMP ping available")
	} else {
		reasons = append(reasons, "ICMP ping unavailable (TCP ping fallback)")
	}

	// === Root Access ===
	isRoot, _, _ := es.BestValue(CategoryPermissions, "is_root")
	if isRoot != nil && isRoot.(bool) {
		reasons = append(reasons, "Running as root - elevated privileges available")
	}

	// === Network Visibility ===
	if _, _, hasGW := es.BestValue(CategoryNetwork, "gateway"); hasGW {
		reasons = append(reasons, "Default gateway detected")
	} else {
		reasons = append(reasons, "No default gateway - network scanning may be limited")
		discoveryConfidence -= 0.1
	}

	// === Determine Final Recommendation ===
	if canDiscovery {
		// Factor in capability confidence
		if nmapConf > 0 {
			discoveryConfidence = (discoveryConfidence + nmapConf) / 2
		}
		// Cap at 0.95
		if discoveryConfidence > 0.95 {
			discoveryConfidence = 0.95
		}
		return config.ModeDiscovery, discoveryConfidence, reasons
	}

	if canMonitor {
		return config.ModeMonitor, monitorConfidence, reasons
	}

	return config.ModePassive, 0.90, reasons
}

// SynthesisResult contains the full synthesis output
type SynthesisResult struct {
	Mode       config.Mode
	Confidence float64
	Reasons    []string
	Warnings   []string
}

// FullSynthesis performs complete mode synthesis with warnings
func FullSynthesis(es *EvidenceSet) SynthesisResult {
	mode, confidence, reasons := SynthesizeMode(es)

	result := SynthesisResult{
		Mode:       mode,
		Confidence: confidence,
		Reasons:    reasons,
	}

	// Add warnings for potential issues
	if effectiveMem := getEffectiveMemory(es); effectiveMem > 0 && effectiveMem < 512 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Low memory (%dMB) may cause performance issues", effectiveMem))
	}

	// Warn if running as root in container (security concern)
	isRoot, _, _ := es.BestValue(CategoryPermissions, "is_root")
	envType, _, _ := es.BestValue(CategoryEnvironment, "environment_type")
	if isRoot != nil && isRoot.(bool) && envType != nil && envType.(string) == string(EnvTypeContainerized) {
		result.Warnings = append(result.Warnings,
			"Running as root in container - consider using non-root user")
	}

	return result
}

func getEffectiveMemory(es *EvidenceSet) int {
	// Prefer cgroup limit for containers
	if limit, _, has := es.BestValue(CategoryResources, "memory_limit_mb"); has {
		if l, ok := limit.(int); ok {
			return l
		}
	}
	// Fall back to system memory
	if mem, _, has := es.BestValue(CategoryResources, "memory_mb"); has {
		if m, ok := mem.(int); ok {
			return m
		}
	}
	return 0
}
