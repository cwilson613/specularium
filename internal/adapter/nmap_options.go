package adapter

import "time"

// NmapOption is a functional option for configuring NmapAdapter
type NmapOption func(*NmapAdapter)

// WithInterval sets the polling interval for periodic scans
func WithInterval(d time.Duration) NmapOption {
	return func(n *NmapAdapter) {
		n.interval = d
	}
}

// WithTimeout sets the timeout for the entire nmap scan
func WithTimeout(d time.Duration) NmapOption {
	return func(n *NmapAdapter) {
		n.timeout = d
	}
}

// WithPortRange sets the ports to scan
// Format: "80,443,8080" or "1-1000" or "22,80-443,8080"
func WithPortRange(ports string) NmapOption {
	return func(n *NmapAdapter) {
		// Validate and set port range
		if validated, err := parsePorts(ports); err == nil {
			n.portRange = validated
		}
	}
}

// WithServiceDetection enables or disables service version detection (-sV)
func WithServiceDetection(enabled bool) NmapOption {
	return func(n *NmapAdapter) {
		n.serviceDetection = enabled
	}
}

// WithOSDetection enables or disables OS detection (-O)
// Note: OS detection requires root privileges
func WithOSDetection(enabled bool) NmapOption {
	return func(n *NmapAdapter) {
		n.osDetection = enabled
	}
}

// WithSkipHostDiscovery sets whether to skip ping and treat all hosts as online (-Pn)
// Useful for networks that block ICMP
func WithSkipHostDiscovery(skip bool) NmapOption {
	return func(n *NmapAdapter) {
		n.skipHostDiscovery = skip
	}
}

// WithTargets sets or replaces the target list
// Can be used to dynamically update targets
func WithTargets(targets []string) NmapOption {
	return func(n *NmapAdapter) {
		n.targets = targets
	}
}

// WithCommonPorts configures scanning of common service ports
// This is a convenience option for common homelab services
func WithCommonPorts() NmapOption {
	return func(n *NmapAdapter) {
		n.portRange = "22,25,53,80,110,143,443,445,993,995,3306,3389,5432,5900,6443,8080,8443,9090,9100"
	}
}

// WithTopPorts configures scanning of top N ports
// Common values: 10, 100, 1000
func WithTopPorts(n int) NmapOption {
	// Note: The nmap library doesn't directly support --top-ports
	// We'll implement common port lists based on frequency
	return func(adapter *NmapAdapter) {
		switch {
		case n <= 10:
			adapter.portRange = "21,22,23,25,80,110,139,443,445,3389"
		case n <= 100:
			adapter.portRange = "21-23,25,53,80,110,111,135,139,143,443,445,993,995,1723,3306,3389,5900,8080"
		default:
			adapter.portRange = "1-1024"
		}
	}
}

// WithFastScan enables fast scan mode (fewer ports, quicker results)
func WithFastScan() NmapOption {
	return func(n *NmapAdapter) {
		n.portRange = "22,80,443"
		n.serviceDetection = false
		n.timeout = 5 * time.Minute
	}
}

// WithAggressiveScan enables aggressive scan mode (more ports, service detection, OS detection)
// Note: Requires root for OS detection
func WithAggressiveScan() NmapOption {
	return func(n *NmapAdapter) {
		n.portRange = "1-65535"
		n.serviceDetection = true
		n.osDetection = true
		n.timeout = 30 * time.Minute
	}
}
