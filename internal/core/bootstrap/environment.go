package bootstrap

import (
	"os"
	"strings"
)

// EnvironmentType represents the broad category of deployment
type EnvironmentType string

const (
	EnvTypeBareMetal     EnvironmentType = "bare_metal"
	EnvTypeVM            EnvironmentType = "vm"
	EnvTypeContainerized EnvironmentType = "containerized"
)

// ContainerRuntime represents specific container runtime
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

// DetectEnvironment gathers evidence about the execution environment
func DetectEnvironment() []Evidence {
	var evidence []Evidence

	// Container runtime detection (most specific first)
	evidence = append(evidence, detectKubernetes()...)
	evidence = append(evidence, detectCRIO()...)
	evidence = append(evidence, detectContainerd()...)
	evidence = append(evidence, detectPodman()...)
	evidence = append(evidence, detectDocker()...)
	evidence = append(evidence, detectLXC()...)

	// VM detection
	evidence = append(evidence, detectVM()...)

	// Infer environment type from gathered evidence
	evidence = append(evidence, inferEnvironmentType(evidence)...)

	return evidence
}

func detectKubernetes() []Evidence {
	var evidence []Evidence

	// Method 1: KUBERNETES_SERVICE_HOST (very reliable)
	if host := os.Getenv("KUBERNETES_SERVICE_HOST"); host != "" {
		evidence = append(evidence, NewEvidence(
			CategoryEnvironment,
			"orchestrator",
			string(RuntimeKubernetes),
			0.98,
			"environment",
			"KUBERNETES_SERVICE_HOST set",
		).WithRaw(map[string]any{"service_host": host}))
	}

	// Method 2: Service account token
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
		evidence = append(evidence, NewEvidence(
			CategoryEnvironment,
			"orchestrator",
			string(RuntimeKubernetes),
			0.95,
			"filesystem",
			"Kubernetes service account token exists",
		))

		// Try to read namespace
		if ns, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
			evidence = append(evidence, NewEvidence(
				CategoryEnvironment,
				"k8s_namespace",
				strings.TrimSpace(string(ns)),
				0.99,
				"filesystem",
				"Read from service account namespace file",
			))
		}
	}

	// Method 3: cgroup contains kubepods
	if cgroup := readFileSafe("/proc/1/cgroup"); cgroup != "" {
		if strings.Contains(cgroup, "kubepods") {
			evidence = append(evidence, NewEvidence(
				CategoryEnvironment,
				"container_runtime",
				string(RuntimeKubernetes),
				0.92,
				"procfs",
				"/proc/1/cgroup contains 'kubepods'",
			))
		}
	}

	// Method 4: Downward API env vars
	if podName := os.Getenv("POD_NAME"); podName != "" {
		evidence = append(evidence, NewEvidence(
			CategoryEnvironment,
			"k8s_pod_name",
			podName,
			0.95,
			"environment",
			"POD_NAME env var",
		))
	}
	if nodeName := os.Getenv("NODE_NAME"); nodeName != "" {
		evidence = append(evidence, NewEvidence(
			CategoryEnvironment,
			"k8s_node_name",
			nodeName,
			0.95,
			"environment",
			"NODE_NAME env var",
		))
	}

	return evidence
}

func detectCRIO() []Evidence {
	var evidence []Evidence

	if cgroup := readFileSafe("/proc/1/cgroup"); cgroup != "" {
		if strings.Contains(cgroup, "crio-") || strings.Contains(cgroup, "/crio/") {
			evidence = append(evidence, NewEvidence(
				CategoryEnvironment,
				"container_runtime",
				string(RuntimeCRIO),
				0.90,
				"procfs",
				"/proc/1/cgroup contains CRI-O marker",
			))
		}
	}

	return evidence
}

func detectContainerd() []Evidence {
	var evidence []Evidence

	if cgroup := readFileSafe("/proc/1/cgroup"); cgroup != "" {
		if strings.Contains(cgroup, "containerd-") || strings.Contains(cgroup, "/containerd/") {
			evidence = append(evidence, NewEvidence(
				CategoryEnvironment,
				"container_runtime",
				string(RuntimeContainerd),
				0.90,
				"procfs",
				"/proc/1/cgroup contains containerd marker",
			))
		}
	}

	return evidence
}

func detectPodman() []Evidence {
	var evidence []Evidence

	// Podman creates /run/.containerenv
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		evidence = append(evidence, NewEvidence(
			CategoryEnvironment,
			"container_runtime",
			string(RuntimePodman),
			0.92,
			"filesystem",
			"/run/.containerenv exists",
		))
	}

	// cgroup markers
	if cgroup := readFileSafe("/proc/1/cgroup"); cgroup != "" {
		if strings.Contains(cgroup, "libpod-") || strings.Contains(cgroup, "/libpod/") {
			evidence = append(evidence, NewEvidence(
				CategoryEnvironment,
				"container_runtime",
				string(RuntimePodman),
				0.90,
				"procfs",
				"/proc/1/cgroup contains libpod marker",
			))
		}
	}

	// container env var set to podman
	if container := os.Getenv("container"); container == "podman" {
		evidence = append(evidence, NewEvidence(
			CategoryEnvironment,
			"container_runtime",
			string(RuntimePodman),
			0.88,
			"environment",
			"container=podman env var",
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
			string(RuntimeDocker),
			0.95,
			"filesystem",
			"/.dockerenv exists",
		))
	}

	// Method 2: cgroup contains docker
	if cgroup := readFileSafe("/proc/1/cgroup"); cgroup != "" {
		if strings.Contains(cgroup, "docker-") || strings.Contains(cgroup, "/docker/") {
			evidence = append(evidence, NewEvidence(
				CategoryEnvironment,
				"container_runtime",
				string(RuntimeDocker),
				0.90,
				"procfs",
				"/proc/1/cgroup contains 'docker'",
			))
		}
	}

	return evidence
}

func detectLXC() []Evidence {
	var evidence []Evidence

	if cgroup := readFileSafe("/proc/1/cgroup"); cgroup != "" {
		if strings.Contains(cgroup, "/lxc/") || strings.Contains(cgroup, "lxc.payload") {
			evidence = append(evidence, NewEvidence(
				CategoryEnvironment,
				"container_runtime",
				string(RuntimeLXC),
				0.88,
				"procfs",
				"/proc/1/cgroup contains LXC marker",
			))
		}
	}

	// container env var set to lxc
	if container := os.Getenv("container"); container == "lxc" {
		evidence = append(evidence, NewEvidence(
			CategoryEnvironment,
			"container_runtime",
			string(RuntimeLXC),
			0.85,
			"environment",
			"container=lxc env var",
		))
	}

	return evidence
}

func detectVM() []Evidence {
	var evidence []Evidence

	// Method 1: DMI product name
	if product := readFileSafe("/sys/class/dmi/id/product_name"); product != "" {
		product = strings.TrimSpace(product)
		vmIndicators := map[string]string{
			"VirtualBox":      "virtualbox",
			"VMware":          "vmware",
			"KVM":             "kvm",
			"QEMU":            "qemu",
			"Hyper-V":         "hyperv",
			"Virtual Machine": "unknown_hypervisor",
			"Bochs":           "bochs",
			"Parallels":       "parallels",
		}
		for indicator, vmType := range vmIndicators {
			if strings.Contains(product, indicator) {
				evidence = append(evidence, NewEvidence(
					CategoryEnvironment,
					"virtualization",
					vmType,
					0.90,
					"dmi",
					"/sys/class/dmi/id/product_name contains '"+indicator+"'",
				).WithRaw(map[string]any{"product_name": product}))
				break
			}
		}
	}

	// Method 2: Check for hypervisor flag in cpuinfo
	if cpuinfo := readFileSafe("/proc/cpuinfo"); cpuinfo != "" {
		if strings.Contains(cpuinfo, "hypervisor") {
			evidence = append(evidence, NewEvidence(
				CategoryEnvironment,
				"virtualization",
				"hypervisor_detected",
				0.75,
				"procfs",
				"/proc/cpuinfo contains 'hypervisor' flag",
			))
		}
	}

	return evidence
}

func inferEnvironmentType(evidence []Evidence) []Evidence {
	var result []Evidence

	hasContainer := false
	hasVM := false

	for _, e := range evidence {
		if e.Property == "container_runtime" || e.Property == "orchestrator" {
			hasContainer = true
		}
		if e.Property == "virtualization" {
			hasVM = true
		}
	}

	if hasContainer {
		result = append(result, NewEvidence(
			CategoryEnvironment,
			"environment_type",
			string(EnvTypeContainerized),
			0.90,
			"inference",
			"Container runtime evidence found",
		))
	} else if hasVM {
		result = append(result, NewEvidence(
			CategoryEnvironment,
			"environment_type",
			string(EnvTypeVM),
			0.85,
			"inference",
			"VM hypervisor evidence found",
		))
	} else {
		// No container or VM evidence - likely bare metal
		result = append(result, NewEvidence(
			CategoryEnvironment,
			"environment_type",
			string(EnvTypeBareMetal),
			0.60, // Lower confidence - absence of evidence
			"inference",
			"No container or VM indicators detected",
		))
	}

	return result
}

func readFileSafe(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
