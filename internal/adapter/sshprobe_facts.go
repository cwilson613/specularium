package adapter

import (
	"fmt"
	"strings"
)

// FactCommand defines a command to run over SSH for fact gathering
type FactCommand struct {
	Name    string                                    // e.g., "os_info"
	Command string                                    // e.g., "cat /etc/os-release"
	Parser  func(output string) (map[string]any, error) // Parse command output into facts
}

// DefaultFactCommands are the standard fact-gathering commands
var DefaultFactCommands = []FactCommand{
	{
		Name:    "hostname",
		Command: "hostname -f 2>/dev/null || hostname",
		Parser:  parseHostname,
	},
	{
		Name:    "os_release",
		Command: "cat /etc/os-release 2>/dev/null",
		Parser:  parseOSRelease,
	},
	{
		Name:    "uname",
		Command: "uname -a",
		Parser:  parseUname,
	},
	{
		Name:    "docker_check",
		Command: "docker ps -q 2>/dev/null | head -1",
		Parser:  parseDockerCheck,
	},
	{
		Name:    "k8s_check",
		Command: "kubectl version --client=true --output=yaml 2>/dev/null || ls /etc/rancher/k3s 2>/dev/null",
		Parser:  parseK8sCheck,
	},
}

// parseHostname extracts hostname from hostname command
func parseHostname(output string) (map[string]any, error) {
	hostname := strings.TrimSpace(output)
	if hostname == "" {
		return nil, fmt.Errorf("empty hostname")
	}

	facts := map[string]any{
		"hostname": hostname,
	}

	// Extract short hostname if FQDN
	if idx := strings.Index(hostname, "."); idx > 0 {
		facts["hostname_short"] = hostname[:idx]
		facts["domain"] = hostname[idx+1:]
	}

	return facts, nil
}

// parseOSRelease parses /etc/os-release file
// Format: KEY=value or KEY="value"
func parseOSRelease(output string) (map[string]any, error) {
	if output == "" {
		return nil, fmt.Errorf("empty os-release output")
	}

	facts := make(map[string]any)
	osInfo := make(map[string]string)

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes
		value = strings.Trim(value, "\"'")

		osInfo[key] = value
	}

	if len(osInfo) == 0 {
		return nil, fmt.Errorf("no OS information found")
	}

	// Extract key fields
	if name, ok := osInfo["NAME"]; ok {
		facts["os_name"] = name
	}
	if version, ok := osInfo["VERSION"]; ok {
		facts["os_version"] = version
	}
	if id, ok := osInfo["ID"]; ok {
		facts["os_id"] = id
	}
	if versionID, ok := osInfo["VERSION_ID"]; ok {
		facts["os_version_id"] = versionID
	}
	if prettyName, ok := osInfo["PRETTY_NAME"]; ok {
		facts["os_pretty_name"] = prettyName
	}

	// Store full os-release data
	facts["os_release"] = osInfo

	return facts, nil
}

// parseUname parses uname -a output
// Format: Linux hostname 5.15.0-76-generic #83-Ubuntu SMP Thu Jun 15 19:16:32 UTC 2023 x86_64 x86_64 x86_64 GNU/Linux
func parseUname(output string) (map[string]any, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, fmt.Errorf("empty uname output")
	}

	parts := strings.Fields(output)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid uname output format")
	}

	facts := map[string]any{
		"kernel_name":    parts[0], // Linux
		"kernel_release": parts[2], // 5.15.0-76-generic
	}

	// Try to extract architecture (usually last meaningful field)
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "x86_64" || parts[i] == "aarch64" || parts[i] == "armv7l" {
			facts["architecture"] = parts[i]
			break
		}
	}

	// Full uname string
	facts["uname"] = output

	return facts, nil
}

// parseDockerCheck checks if Docker is running
// Output: container ID if running, empty if not
func parseDockerCheck(output string) (map[string]any, error) {
	output = strings.TrimSpace(output)

	facts := map[string]any{
		"has_docker": output != "",
	}

	if output != "" {
		facts["docker_running_containers"] = true
	}

	return facts, nil
}

// parseK8sCheck checks if Kubernetes is present
// This command tries kubectl first, then checks for k3s directory
func parseK8sCheck(output string) (map[string]any, error) {
	output = strings.TrimSpace(output)

	facts := map[string]any{
		"has_k8s": false,
	}

	if output == "" {
		return facts, nil
	}

	// Check if output contains k8s version info (from kubectl)
	if strings.Contains(output, "clientVersion") || strings.Contains(output, "gitVersion") {
		facts["has_k8s"] = true
		facts["k8s_distribution"] = "k8s"

		// Try to extract version
		for _, line := range strings.Split(output, "\n") {
			if strings.Contains(line, "gitVersion") {
				// Extract version string
				if idx := strings.Index(line, "v1."); idx >= 0 {
					versionEnd := idx + 2
					for versionEnd < len(line) && (line[versionEnd] >= '0' && line[versionEnd] <= '9' || line[versionEnd] == '.') {
						versionEnd++
					}
					if versionEnd > idx {
						facts["k8s_version"] = line[idx:versionEnd]
					}
				}
			}
		}
	} else if strings.Contains(output, "k3s") {
		// k3s directory exists
		facts["has_k8s"] = true
		facts["k8s_distribution"] = "k3s"
	}

	return facts, nil
}

// Additional helper parsers can be added here for future commands

// parseDockerVersion parses docker version output (optional future command)
func parseDockerVersion(output string) (map[string]any, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, fmt.Errorf("empty docker version output")
	}

	facts := map[string]any{}

	// Simple version extraction from "Docker version X.Y.Z"
	if strings.Contains(output, "version") {
		parts := strings.Fields(output)
		for i, part := range parts {
			if part == "version" && i+1 < len(parts) {
				version := strings.TrimSuffix(parts[i+1], ",")
				facts["docker_version"] = version
				break
			}
		}
	}

	return facts, nil
}

// parseSystemctl parses systemctl output (optional future command)
func parseSystemctl(output string) (map[string]any, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, fmt.Errorf("empty systemctl output")
	}

	facts := map[string]any{
		"systemd_available": true,
	}

	// Check for specific services
	services := []string{}
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "active") {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				services = append(services, parts[0])
			}
		}
	}

	if len(services) > 0 {
		facts["active_services"] = services
	}

	return facts, nil
}
