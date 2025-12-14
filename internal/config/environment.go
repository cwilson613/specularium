package config

import (
	"os"
	"runtime"
	"strings"

	"specularium/internal/domain"
)

// EnvironmentType represents the broad category of deployment environment
type EnvironmentType string

const (
	EnvTypeBareMetal     EnvironmentType = "bare_metal"
	EnvTypeVM            EnvironmentType = "vm"
	EnvTypeContainerized EnvironmentType = "containerized"
)

// ContainerRuntime represents specific container runtime implementations
type ContainerRuntime string

const (
	RuntimeNone       ContainerRuntime = "none"
	RuntimeDocker     ContainerRuntime = "docker"
	RuntimeKubernetes ContainerRuntime = "kubernetes"
	RuntimePodman     ContainerRuntime = "podman"
	RuntimeContainerd ContainerRuntime = "containerd"
	RuntimeCRIO       ContainerRuntime = "cri-o"
	RuntimeLXC        ContainerRuntime = "lxc"
)

// RuntimeSignature defines detection criteria for a container runtime
type RuntimeSignature struct {
	Runtime     ContainerRuntime
	Confidence  float64 // Base confidence when matched
	Description string
	// Detection functions - any match triggers this signature
	FileExists   []string            // Files that indicate this runtime
	EnvVars      []string            // Environment variables to check
	CGroupMarkers []string           // Patterns in /proc/1/cgroup
	MountMarkers []string            // Patterns in /proc/mounts
}

// RuntimeSignatures is the heuristic map of known container runtimes
// Ordered by specificity - more specific signatures first
var RuntimeSignatures = []RuntimeSignature{
	{
		Runtime:     RuntimeKubernetes,
		Confidence:  0.95,
		Description: "Kubernetes pod with service account",
		FileExists: []string{
			"/var/run/secrets/kubernetes.io/serviceaccount/token",
			"/var/run/secrets/kubernetes.io/serviceaccount/namespace",
		},
		EnvVars: []string{
			"KUBERNETES_SERVICE_HOST",
			"KUBERNETES_PORT",
		},
	},
	{
		Runtime:     RuntimeCRIO,
		Confidence:  0.90,
		Description: "CRI-O container runtime",
		CGroupMarkers: []string{
			"crio-",
			"/crio/",
		},
	},
	{
		Runtime:     RuntimeContainerd,
		Confidence:  0.90,
		Description: "containerd container runtime",
		CGroupMarkers: []string{
			"containerd-",
			"/containerd/",
		},
		MountMarkers: []string{
			"containerd",
		},
	},
	{
		Runtime:     RuntimePodman,
		Confidence:  0.90,
		Description: "Podman container",
		FileExists: []string{
			"/run/.containerenv",
		},
		CGroupMarkers: []string{
			"libpod-",
			"/libpod/",
		},
		EnvVars: []string{
			"container", // Podman sets container=podman
		},
	},
	{
		Runtime:     RuntimeDocker,
		Confidence:  0.85,
		Description: "Docker container",
		FileExists: []string{
			"/.dockerenv",
		},
		CGroupMarkers: []string{
			"docker-",
			"/docker/",
		},
	},
	{
		Runtime:     RuntimeLXC,
		Confidence:  0.80,
		Description: "LXC/LXD container",
		CGroupMarkers: []string{
			"/lxc/",
			"lxc.payload",
		},
		EnvVars: []string{
			"container", // LXC sets container=lxc
		},
	},
}

// DetectionResult holds the result of environment detection
type DetectionResult struct {
	Type        EnvironmentType
	Runtime     ContainerRuntime
	Confidence  float64
	Reasons     []string
	Signatures  []string // Which signatures matched
}

// DetectEnvironmentType analyzes the runtime environment using heuristic signatures
func DetectEnvironmentType(env domain.EnvironmentInfo) DetectionResult {
	result := DetectionResult{
		Type:       EnvTypeBareMetal,
		Runtime:    RuntimeNone,
		Confidence: 0.7, // Base confidence for bare metal
		Reasons:    []string{},
		Signatures: []string{},
	}

	// Check each signature in order (most specific first)
	for _, sig := range RuntimeSignatures {
		matches, matchReasons := checkSignature(sig)
		if matches {
			result.Type = EnvTypeContainerized
			result.Runtime = sig.Runtime
			result.Confidence = sig.Confidence
			result.Signatures = append(result.Signatures, string(sig.Runtime))
			result.Reasons = append(result.Reasons, matchReasons...)
			break // Use first (most specific) match
		}
	}

	// Supplement with additional detection from env struct
	if result.Runtime == RuntimeNone {
		if env.InKubernetes {
			result.Type = EnvTypeContainerized
			result.Runtime = RuntimeKubernetes
			result.Confidence = 0.90
			result.Reasons = append(result.Reasons, "InKubernetes flag set")
		} else if env.InDocker {
			result.Type = EnvTypeContainerized
			result.Runtime = RuntimeDocker
			result.Confidence = 0.85
			result.Reasons = append(result.Reasons, "InDocker flag set")
		}
	}

	// Check for VM indicators (if not containerized)
	if result.Type == EnvTypeBareMetal {
		if isVM := detectVM(); isVM {
			result.Type = EnvTypeVM
			result.Confidence = 0.75
			result.Reasons = append(result.Reasons, "VM hypervisor detected")
		}
	}

	// Add architecture info
	result.Reasons = append(result.Reasons,
		"Architecture: "+runtime.GOARCH)

	return result
}

// checkSignature tests if a runtime signature matches
func checkSignature(sig RuntimeSignature) (bool, []string) {
	var reasons []string

	// Check file existence
	for _, path := range sig.FileExists {
		if _, err := os.Stat(path); err == nil {
			reasons = append(reasons, "Found "+path)
		}
	}

	// Check environment variables
	for _, envVar := range sig.EnvVars {
		if val := os.Getenv(envVar); val != "" {
			reasons = append(reasons, "Env "+envVar+" set")
		}
	}

	// Check cgroup markers
	if cgroupContent := readFileSafe("/proc/1/cgroup"); cgroupContent != "" {
		for _, marker := range sig.CGroupMarkers {
			if strings.Contains(cgroupContent, marker) {
				reasons = append(reasons, "CGroup marker: "+marker)
			}
		}
	}

	// Check mount markers
	if mountContent := readFileSafe("/proc/mounts"); mountContent != "" {
		for _, marker := range sig.MountMarkers {
			if strings.Contains(mountContent, marker) {
				reasons = append(reasons, "Mount marker: "+marker)
			}
		}
	}

	// Match if we found any evidence
	return len(reasons) > 0, reasons
}

// detectVM checks for common VM hypervisor indicators
func detectVM() bool {
	// Check DMI product name
	if dmi := readFileSafe("/sys/class/dmi/id/product_name"); dmi != "" {
		vmIndicators := []string{
			"VirtualBox", "VMware", "QEMU", "KVM",
			"Hyper-V", "Xen", "Parallels", "Bochs",
		}
		for _, indicator := range vmIndicators {
			if strings.Contains(strings.ToLower(dmi), strings.ToLower(indicator)) {
				return true
			}
		}
	}

	// Check hypervisor flag in cpuinfo
	if cpuinfo := readFileSafe("/proc/cpuinfo"); cpuinfo != "" {
		if strings.Contains(cpuinfo, "hypervisor") {
			return true
		}
	}

	return false
}

// readFileSafe reads a file, returning empty string on error
func readFileSafe(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
