package bootstrap

import (
	"fmt"
	"net"
	"os"
	"strings"
)

// DetectNetwork gathers evidence about network configuration
func DetectNetwork() []Evidence {
	var evidence []Evidence

	// Hostname
	evidence = append(evidence, detectHostname()...)

	// Network interfaces
	evidence = append(evidence, detectInterfaces()...)

	// Default gateway
	evidence = append(evidence, detectGateway()...)

	// DNS configuration
	evidence = append(evidence, detectDNS()...)

	// Local subnet detection
	evidence = append(evidence, detectLocalSubnet()...)

	return evidence
}

func detectHostname() []Evidence {
	hostname, err := os.Hostname()
	if err != nil {
		return nil
	}

	return []Evidence{NewEvidence(
		CategoryNetwork,
		"hostname",
		hostname,
		0.99,
		"syscall",
		"os.Hostname()",
	)}
}

func detectInterfaces() []Evidence {
	var evidence []Evidence

	ifaces, err := net.Interfaces()
	if err != nil {
		return evidence
	}

	var activeIfaces []map[string]any

	for _, iface := range ifaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		// Skip virtual interfaces commonly used by containers
		if strings.HasPrefix(iface.Name, "veth") ||
			strings.HasPrefix(iface.Name, "docker") ||
			strings.HasPrefix(iface.Name, "br-") ||
			strings.HasPrefix(iface.Name, "cni") ||
			strings.HasPrefix(iface.Name, "flannel") {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				ip4 := ipnet.IP.To4()

				// Check for RFC1918 private addresses (most relevant for home/enterprise)
				isPrivate := (ip4[0] == 10) ||
					(ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) ||
					(ip4[0] == 192 && ip4[1] == 168)

				ones, bits := ipnet.Mask.Size()

				ifaceInfo := map[string]any{
					"name":       iface.Name,
					"ip":         ipnet.IP.String(),
					"subnet":     fmt.Sprintf("%s/%d", ipnet.IP.Mask(ipnet.Mask), ones),
					"mask_bits":  ones,
					"total_bits": bits,
					"is_private": isPrivate,
					"mac":        iface.HardwareAddr.String(),
				}
				activeIfaces = append(activeIfaces, ifaceInfo)

				// Add individual interface evidence
				evidence = append(evidence, NewEvidence(
					CategoryNetwork,
					"interface",
					iface.Name,
					0.95,
					"netlink",
					"net.Interfaces()",
				).WithRaw(ifaceInfo))
			}
		}
	}

	// Summary of all interfaces
	if len(activeIfaces) > 0 {
		evidence = append(evidence, NewEvidence(
			CategoryNetwork,
			"interface_count",
			len(activeIfaces),
			0.99,
			"netlink",
			"count of active network interfaces",
		).WithRaw(map[string]any{"interfaces": activeIfaces}))
	}

	return evidence
}

func detectGateway() []Evidence {
	// Read /proc/net/route on Linux
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return nil
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) < 2 {
		return nil
	}

	// Skip header
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		// Default route has destination 00000000
		if fields[1] == "00000000" {
			// Gateway is in hex, little-endian
			gw := fields[2]
			if len(gw) == 8 {
				// Parse hex gateway
				var b1, b2, b3, b4 uint8
				fmt.Sscanf(gw, "%02x%02x%02x%02x", &b4, &b3, &b2, &b1)
				gateway := fmt.Sprintf("%d.%d.%d.%d", b1, b2, b3, b4)

				return []Evidence{NewEvidence(
					CategoryNetwork,
					"gateway",
					gateway,
					0.95,
					"procfs",
					"/proc/net/route default route",
				).WithRaw(map[string]any{
					"interface": fields[0],
					"raw_hex":   gw,
				})}
			}
		}
	}

	return nil
}

func detectDNS() []Evidence {
	var evidence []Evidence

	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return evidence
	}

	var nameservers []string
	var searchDomains []string

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "nameserver ") {
			ns := strings.TrimSpace(strings.TrimPrefix(line, "nameserver "))
			nameservers = append(nameservers, ns)
		} else if strings.HasPrefix(line, "search ") {
			domains := strings.Fields(strings.TrimPrefix(line, "search "))
			searchDomains = append(searchDomains, domains...)
		}
	}

	if len(nameservers) > 0 {
		evidence = append(evidence, NewEvidence(
			CategoryNetwork,
			"dns_servers",
			nameservers,
			0.95,
			"filesystem",
			"/etc/resolv.conf nameserver entries",
		))

		// Check for cluster DNS (K8s indicator)
		for _, ns := range nameservers {
			if strings.HasPrefix(ns, "10.43.") || strings.HasPrefix(ns, "10.96.") {
				evidence = append(evidence, NewEvidence(
					CategoryNetwork,
					"cluster_dns",
					ns,
					0.90,
					"inference",
					"DNS server appears to be K8s cluster DNS (10.43.x or 10.96.x)",
				))
			}
		}
	}

	if len(searchDomains) > 0 {
		evidence = append(evidence, NewEvidence(
			CategoryNetwork,
			"search_domains",
			searchDomains,
			0.95,
			"filesystem",
			"/etc/resolv.conf search domains",
		))

		// Check for cluster search domains
		for _, domain := range searchDomains {
			if strings.HasSuffix(domain, ".svc.cluster.local") {
				evidence = append(evidence, NewEvidence(
					CategoryNetwork,
					"k8s_cluster_domain",
					domain,
					0.92,
					"inference",
					"Search domain indicates Kubernetes cluster",
				))
			}
		}
	}

	return evidence
}

func detectLocalSubnet() []Evidence {
	var evidence []Evidence

	// Try to determine the primary local subnet
	conn, err := net.Dial("udp", "8.8.8.8:53")
	if err != nil {
		return evidence
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	ip := localAddr.IP.To4()
	if ip == nil {
		return evidence
	}

	// Determine subnet based on IP class and common practices
	var subnet string
	if ip[0] == 192 && ip[1] == 168 {
		// Class C private - typically /24
		subnet = fmt.Sprintf("%d.%d.%d.0/24", ip[0], ip[1], ip[2])
	} else if ip[0] == 10 {
		// Class A private - could be /24 or /16
		// Check if it's a pod network (10.42.x.x, 10.244.x.x)
		if ip[1] == 42 || ip[1] == 244 {
			subnet = fmt.Sprintf("%d.%d.0.0/16", ip[0], ip[1])
		} else {
			subnet = fmt.Sprintf("%d.%d.%d.0/24", ip[0], ip[1], ip[2])
		}
	} else if ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
		// Class B private
		subnet = fmt.Sprintf("%d.%d.%d.0/24", ip[0], ip[1], ip[2])
	}

	if subnet != "" {
		evidence = append(evidence, NewEvidence(
			CategoryNetwork,
			"local_subnet",
			subnet,
			0.85,
			"inference",
			"Inferred from outbound connection local address",
		).WithRaw(map[string]any{"local_ip": ip.String()}))
	}

	// Also record the local IP
	evidence = append(evidence, NewEvidence(
		CategoryNetwork,
		"local_ip",
		ip.String(),
		0.95,
		"netlink",
		"UDP dial to 8.8.8.8:53 local address",
	))

	return evidence
}
