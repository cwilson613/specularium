package adapter

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"specularium/internal/domain"
)

// BootstrapAdapter performs initial self-discovery on startup
// It detects the deployment environment and expands knowledge outward
type BootstrapAdapter struct {
	publisher EventPublisher
	env       EnvironmentInfo
}

// EnvironmentInfo holds detected runtime environment details
type EnvironmentInfo struct {
	// Deployment context
	InKubernetes  bool
	InDocker      bool
	Hostname      string
	PodName       string
	PodNamespace  string
	PodIP         string
	NodeName      string
	ServiceAccount string

	// Network context
	DefaultGateway string
	DNSServers     []string
	SearchDomains  []string
	LocalSubnet    string

	// K8s cluster info (if applicable)
	ClusterDNS       string
	KubernetesAPIIP  string

	// Operator-configured scan targets (from SCAN_SUBNETS env var)
	ConfiguredSubnets []string
}

// NewBootstrapAdapter creates a new bootstrap adapter
func NewBootstrapAdapter() *BootstrapAdapter {
	return &BootstrapAdapter{}
}

// SetEventPublisher sets the event publisher for progress updates
func (b *BootstrapAdapter) SetEventPublisher(pub EventPublisher) {
	b.publisher = pub
}

// Name returns the adapter identifier
func (b *BootstrapAdapter) Name() string {
	return "bootstrap"
}

// Type returns the adapter type
func (b *BootstrapAdapter) Type() AdapterType {
	return AdapterTypeOneShot
}

// Priority returns the adapter priority (highest - runs first)
func (b *BootstrapAdapter) Priority() int {
	return 0
}

// Start performs initial environment detection
func (b *BootstrapAdapter) Start(ctx context.Context) error {
	b.env = b.detectEnvironment()
	log.Printf("Bootstrap: Environment detected - K8s=%v, Docker=%v, Hostname=%s",
		b.env.InKubernetes, b.env.InDocker, b.env.Hostname)
	if b.env.InKubernetes {
		log.Printf("Bootstrap: K8s context - Pod=%s/%s, Node=%s, ClusterDNS=%s",
			b.env.PodNamespace, b.env.PodName, b.env.NodeName, b.env.ClusterDNS)
	}
	log.Printf("Bootstrap: Network context - Gateway=%s, DNS=%v, Subnet=%s",
		b.env.DefaultGateway, b.env.DNSServers, b.env.LocalSubnet)
	return nil
}

// Stop shuts down the adapter
func (b *BootstrapAdapter) Stop() error {
	return nil
}

// Sync is not used for bootstrap adapter
func (b *BootstrapAdapter) Sync(ctx context.Context) (*domain.GraphFragment, error) {
	return nil, nil
}

// Bootstrap performs the initial self-discovery and returns discovered nodes
func (b *BootstrapAdapter) Bootstrap(ctx context.Context) (*domain.GraphFragment, error) {
	fragment := domain.NewGraphFragment()
	now := time.Now()

	b.publishProgress("discovery-started", map[string]interface{}{
		"message": "Bootstrap: Detecting deployment environment",
		"phase":   "bootstrap",
	})

	// 1. Create self node (Specularium itself)
	selfNode := b.createSelfNode(now)
	fragment.AddNode(selfNode)

	b.publishProgress("discovery-progress", map[string]interface{}{
		"node_id": selfNode.ID,
		"message": fmt.Sprintf("Created self node: %s", selfNode.ID),
		"phase":   "bootstrap",
	})

	// 2. If in K8s, discover cluster infrastructure
	if b.env.InKubernetes {
		k8sNodes := b.discoverK8sInfrastructure(now)
		for _, node := range k8sNodes {
			fragment.AddNode(node)
			b.publishProgress("discovery-progress", map[string]interface{}{
				"node_id": node.ID,
				"message": fmt.Sprintf("Discovered K8s: %s (%s)", node.Label, node.Type),
				"phase":   "bootstrap",
			})
		}
	}

	// 3. Discover network infrastructure (gateway, DNS)
	netNodes := b.discoverNetworkInfrastructure(now)
	for _, node := range netNodes {
		fragment.AddNode(node)
		b.publishProgress("discovery-progress", map[string]interface{}{
			"node_id": node.ID,
			"message": fmt.Sprintf("Discovered network: %s (%s)", node.Label, node.Type),
			"phase":   "bootstrap",
		})
	}

	// 4. Create edges connecting self to discovered infrastructure
	edges := b.createInfrastructureEdges(selfNode.ID, fragment.Nodes)
	for _, edge := range edges {
		fragment.AddEdge(edge)
	}

	b.publishProgress("discovery-complete", map[string]interface{}{
		"total":   len(fragment.Nodes),
		"message": fmt.Sprintf("Bootstrap complete: %d nodes discovered", len(fragment.Nodes)),
		"phase":   "bootstrap",
	})

	return fragment, nil
}

// GetEnvironment returns the detected environment info
func (b *BootstrapAdapter) GetEnvironment() EnvironmentInfo {
	return b.env
}

// ScanTargets contains categorized scan targets
type ScanTargets struct {
	// Primary targets - operator-configured or detected from environment
	Primary []string `json:"primary"`
	// Discovery targets - RFC1918 ranges for network discovery mode
	Discovery []string `json:"discovery"`
}

// GetSuggestedScanTargets returns network ranges to scan based on environment
// Returns only primary (configured/detected) subnets - use GetDiscoveryTargets for discovery mode
func (b *BootstrapAdapter) GetSuggestedScanTargets() []string {
	return b.GetScanTargets().Primary
}

// GetScanTargets returns categorized scan targets:
// - Primary: Operator-configured or auto-detected subnets (safe to scan)
// - Discovery: RFC1918 ranges for network discovery mode (broader scan)
func (b *BootstrapAdapter) GetScanTargets() ScanTargets {
	seen := make(map[string]bool)
	primary := []string{}

	addPrimary := func(subnet string) {
		if subnet != "" && !seen[subnet] && !isClusterNetwork(subnet) {
			seen[subnet] = true
			primary = append(primary, subnet)
		}
	}

	// 1. Operator-configured subnets take priority (from SCAN_SUBNETS env var)
	for _, subnet := range b.env.ConfiguredSubnets {
		addPrimary(subnet)
	}

	// 2. Local subnet from interface detection
	addPrimary(b.env.LocalSubnet)

	// 3. Gateway's /24 network (may differ from local subnet in NAT scenarios)
	if b.env.DefaultGateway != "" {
		gwSubnet := ipToSubnet24(b.env.DefaultGateway)
		addPrimary(gwSubnet)
	}

	// 4. DNS servers' networks (Technitium, Pi-hole, etc. often on their own subnet)
	for _, dns := range b.env.DNSServers {
		if dns == b.env.ClusterDNS || dns == "127.0.0.1" || dns == "::1" {
			continue
		}
		dnsSubnet := ipToSubnet24(dns)
		addPrimary(dnsSubnet)
	}

	// Discovery targets: RFC1918 private ranges commonly used in home/enterprise networks
	// These are shown when "Discovery Mode" is enabled in the UI
	discovery := b.getDiscoveryTargets(seen)

	return ScanTargets{
		Primary:   primary,
		Discovery: discovery,
	}
}

// getDiscoveryTargets returns RFC1918 subnets for network discovery
// Excludes subnets already in the seen map
func (b *BootstrapAdapter) getDiscoveryTargets(seen map[string]bool) []string {
	discovery := []string{}

	// Standard RFC1918 ranges commonly used in home networks
	// 192.168.x.0/24 - most common home router ranges
	commonSubnets := []string{
		"192.168.0.0/24",
		"192.168.1.0/24",
		"192.168.2.0/24",
		"192.168.10.0/24",
		"192.168.100.0/24",
		// 10.0.x.0/24 - common in enterprise/advanced home setups
		"10.0.0.0/24",
		"10.0.1.0/24",
		"10.1.0.0/24",
		"10.1.1.0/24",
		// 172.16.x.0/24 - less common but used in some setups
		"172.16.0.0/24",
		"172.16.1.0/24",
	}

	for _, subnet := range commonSubnets {
		if !seen[subnet] && !isClusterNetwork(subnet) {
			discovery = append(discovery, subnet)
		}
	}

	// If we have a base subnet, add adjacent ranges
	baseSubnet := ""
	if len(b.env.ConfiguredSubnets) > 0 {
		baseSubnet = b.env.ConfiguredSubnets[0]
	} else if b.env.LocalSubnet != "" {
		baseSubnet = b.env.LocalSubnet
	}

	if baseSubnet != "" {
		adjacents := getAdjacentSubnets(baseSubnet)
		for _, adj := range adjacents {
			if !seen[adj] && !isClusterNetwork(adj) {
				// Check if not already in discovery list
				found := false
				for _, d := range discovery {
					if d == adj {
						found = true
						break
					}
				}
				if !found {
					discovery = append(discovery, adj)
				}
			}
		}
	}

	return discovery
}

// ipToSubnet24 converts an IP address to its /24 subnet
func ipToSubnet24(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	ip4 := parsed.To4()
	if ip4 == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.0/24", ip4[0], ip4[1], ip4[2])
}

// isClusterNetwork returns true if the subnet is a Kubernetes pod/service network
func isClusterNetwork(subnet string) bool {
	// K8s typically uses 10.42.x.x (K3s pods), 10.43.x.x (K3s services),
	// 10.96.x.x (default service CIDR), 10.244.x.x (Flannel)
	return strings.HasPrefix(subnet, "10.42.") ||
		strings.HasPrefix(subnet, "10.43.") ||
		strings.HasPrefix(subnet, "10.96.") ||
		strings.HasPrefix(subnet, "10.244.")
}

// getAdjacentSubnets returns adjacent /24 subnets that are likely to exist
// in a typical home network with VLANs
func getAdjacentSubnets(subnet string) []string {
	// Parse the subnet to get base octets
	_, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil
	}
	ip4 := ipNet.IP.To4()
	if ip4 == nil {
		return nil
	}

	var adjacents []string

	// Only generate adjacents for private ranges
	// 192.168.x.x - suggest ±1 and ±2 in third octet
	if ip4[0] == 192 && ip4[1] == 168 {
		thirdOctet := int(ip4[2])
		for _, delta := range []int{-2, -1, 1, 2} {
			newThird := thirdOctet + delta
			if newThird >= 0 && newThird <= 255 {
				adjacents = append(adjacents, fmt.Sprintf("192.168.%d.0/24", newThird))
			}
		}
	}

	// 10.x.x.x - only suggest adjacent if in reasonable range (not cluster networks)
	// Common patterns: 10.0.0.x, 10.0.1.x, 10.1.0.x, etc.
	if ip4[0] == 10 && ip4[1] < 10 {
		thirdOctet := int(ip4[2])
		for _, delta := range []int{-1, 1} {
			newThird := thirdOctet + delta
			if newThird >= 0 && newThird <= 10 {
				adjacents = append(adjacents, fmt.Sprintf("10.%d.%d.0/24", ip4[1], newThird))
			}
		}
	}

	return adjacents
}

// detectEnvironment probes the runtime environment
func (b *BootstrapAdapter) detectEnvironment() EnvironmentInfo {
	env := EnvironmentInfo{}

	// Basic hostname
	env.Hostname, _ = os.Hostname()

	// Operator-configured scan subnets (comma-separated)
	// Example: SCAN_SUBNETS=192.168.0.0/24,192.168.1.0/24
	if scanSubnets := os.Getenv("SCAN_SUBNETS"); scanSubnets != "" {
		for _, subnet := range strings.Split(scanSubnets, ",") {
			subnet = strings.TrimSpace(subnet)
			if subnet != "" {
				// Validate it's a valid CIDR
				if _, _, err := net.ParseCIDR(subnet); err == nil {
					env.ConfiguredSubnets = append(env.ConfiguredSubnets, subnet)
				} else {
					log.Printf("Bootstrap: Invalid CIDR in SCAN_SUBNETS: %s", subnet)
				}
			}
		}
		if len(env.ConfiguredSubnets) > 0 {
			log.Printf("Bootstrap: Operator-configured scan subnets: %v", env.ConfiguredSubnets)
		}
	}

	// Check for Kubernetes (presence of service account)
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
		env.InKubernetes = true
		env.ServiceAccount = "default"

		// Read namespace
		if ns, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
			env.PodNamespace = strings.TrimSpace(string(ns))
		}

		// Pod name is usually the hostname in K8s
		env.PodName = env.Hostname

		// Node name from downward API (if available)
		env.NodeName = os.Getenv("NODE_NAME")
		if env.NodeName == "" {
			env.NodeName = os.Getenv("KUBERNETES_NODE_NAME")
		}

		// Pod IP from downward API or network detection
		env.PodIP = os.Getenv("POD_IP")
		if env.PodIP == "" {
			env.PodIP = b.detectLocalIP()
		}

		// K8s API server is at kubernetes.default.svc
		env.KubernetesAPIIP = b.resolveHost("kubernetes.default.svc")
	}

	// Check for Docker (presence of .dockerenv or cgroup)
	if _, err := os.Stat("/.dockerenv"); err == nil {
		env.InDocker = true
	}

	// Parse resolv.conf for DNS info
	env.DNSServers, env.SearchDomains = b.parseResolvConf()

	// Identify cluster DNS (usually 10.43.0.10 or 10.96.0.10 for K8s)
	for _, dns := range env.DNSServers {
		if strings.HasPrefix(dns, "10.43.") || strings.HasPrefix(dns, "10.96.") {
			env.ClusterDNS = dns
			break
		}
	}

	// Detect default gateway
	env.DefaultGateway = b.detectDefaultGateway()

	// Detect local subnet from pod IP or local interfaces
	env.LocalSubnet = b.detectLocalSubnet(env.PodIP)

	return env
}

// detectLocalIP finds the primary local IP address
func (b *BootstrapAdapter) detectLocalIP() string {
	// Try to connect to a known address to determine local IP
	conn, err := net.Dial("udp", "8.8.8.8:53")
	if err != nil {
		return ""
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

// resolveHost attempts DNS resolution for a hostname
func (b *BootstrapAdapter) resolveHost(hostname string) string {
	addrs, err := net.LookupHost(hostname)
	if err != nil || len(addrs) == 0 {
		return ""
	}
	return addrs[0]
}

// parseResolvConf reads DNS configuration from resolv.conf
func (b *BootstrapAdapter) parseResolvConf() ([]string, []string) {
	var dnsServers, searchDomains []string

	data, err := ioutil.ReadFile("/etc/resolv.conf")
	if err != nil {
		return dnsServers, searchDomains
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver ") {
			dns := strings.TrimPrefix(line, "nameserver ")
			dnsServers = append(dnsServers, strings.TrimSpace(dns))
		} else if strings.HasPrefix(line, "search ") {
			domains := strings.Fields(strings.TrimPrefix(line, "search "))
			searchDomains = append(searchDomains, domains...)
		}
	}

	return dnsServers, searchDomains
}

// detectDefaultGateway attempts to find the default gateway
func (b *BootstrapAdapter) detectDefaultGateway() string {
	// Read /proc/net/route on Linux
	data, err := ioutil.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(data), "\n")[1:] { // Skip header
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
				return fmt.Sprintf("%d.%d.%d.%d", b1, b2, b3, b4)
			}
		}
	}

	return ""
}

// detectLocalSubnet determines the local network subnet
func (b *BootstrapAdapter) detectLocalSubnet(podIP string) string {
	// If we have a pod IP, derive subnet from it
	if podIP != "" {
		ip := net.ParseIP(podIP)
		if ip != nil && ip.To4() != nil {
			// Check for common pod networks (10.x.x.x) vs node networks (192.168.x.x)
			ip4 := ip.To4()
			if ip4[0] == 192 && ip4[1] == 168 {
				// Likely a host network pod or the actual LAN
				return fmt.Sprintf("%d.%d.%d.0/24", ip4[0], ip4[1], ip4[2])
			}
		}
	}

	// Try to find a non-pod network interface
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range ifaces {
		// Skip loopback and virtual interfaces
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if strings.HasPrefix(iface.Name, "veth") || strings.HasPrefix(iface.Name, "docker") ||
			strings.HasPrefix(iface.Name, "cni") || strings.HasPrefix(iface.Name, "flannel") {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				ip4 := ipnet.IP.To4()
				// Look for RFC1918 addresses that aren't pod networks
				if ip4[0] == 192 && ip4[1] == 168 {
					ones, _ := ipnet.Mask.Size()
					return fmt.Sprintf("%s/%d", ipnet.IP.Mask(ipnet.Mask).String(), ones)
				}
				if ip4[0] == 10 && ip4[1] != 42 && ip4[1] != 43 { // Skip typical K8s pod/service nets
					ones, _ := ipnet.Mask.Size()
					return fmt.Sprintf("%s/%d", ipnet.IP.Mask(ipnet.Mask).String(), ones)
				}
			}
		}
	}

	return ""
}

// createSelfNode creates a node representing Specularium itself
func (b *BootstrapAdapter) createSelfNode(now time.Time) domain.Node {
	nodeID := "specularium"
	label := "specularium"

	properties := map[string]any{
		"role":     "observer",
		"hostname": b.env.Hostname,
	}

	discovered := map[string]any{
		"in_kubernetes": b.env.InKubernetes,
		"in_docker":     b.env.InDocker,
	}

	if b.env.InKubernetes {
		properties["pod_name"] = b.env.PodName
		properties["namespace"] = b.env.PodNamespace
		if b.env.NodeName != "" {
			properties["k8s_node"] = b.env.NodeName
		}
		if b.env.PodIP != "" {
			properties["ip"] = b.env.PodIP
			// Infer pod network segmentum from pod IP (10.42.x.x -> 10.42.0.0/16)
			parts := strings.Split(b.env.PodIP, ".")
			if len(parts) == 4 {
				properties["segmentum"] = fmt.Sprintf("%s.%s.0.0/16", parts[0], parts[1])
			}
		}
	}

	node := domain.Node{
		ID:         nodeID,
		Type:       domain.NodeTypeServer, // Specularium is a server/service
		Label:      label,
		Source:     "bootstrap",
		Status:     domain.NodeStatusVerified,
		Properties: properties,
		Discovered: discovered,
	}
	node.LastVerified = &now
	node.LastSeen = &now

	return node
}

// discoverK8sInfrastructure discovers K8s cluster components
func (b *BootstrapAdapter) discoverK8sInfrastructure(now time.Time) []domain.Node {
	var nodes []domain.Node

	// Determine the K8s service network segmentum (10.43.0.0/16 for K3s services)
	k8sServiceSegmentum := "10.43.0.0/16"
	if b.env.ClusterDNS != "" {
		// Infer service CIDR from cluster DNS IP
		parts := strings.Split(b.env.ClusterDNS, ".")
		if len(parts) == 4 {
			k8sServiceSegmentum = fmt.Sprintf("%s.%s.0.0/16", parts[0], parts[1])
		}
	}

	// K8s API Server
	if b.env.KubernetesAPIIP != "" {
		apiNode := domain.Node{
			ID:     "k8s-api",
			Type:   domain.NodeTypeServer,
			Label:  "kubernetes-api",
			Source: "bootstrap",
			Status: domain.NodeStatusVerified,
			Properties: map[string]any{
				"ip":        b.env.KubernetesAPIIP,
				"role":      "k8s-control-plane",
				"segmentum": k8sServiceSegmentum,
			},
			Discovered: map[string]any{
				"service": "kubernetes.default.svc",
			},
		}
		apiNode.LastVerified = &now
		apiNode.LastSeen = &now
		nodes = append(nodes, apiNode)
	}

	// CoreDNS / Cluster DNS
	if b.env.ClusterDNS != "" {
		dnsNode := domain.Node{
			ID:     "k8s-dns",
			Type:   domain.NodeTypeServer,
			Label:  "coredns",
			Source: "bootstrap",
			Status: domain.NodeStatusVerified,
			Properties: map[string]any{
				"ip":        b.env.ClusterDNS,
				"role":      "k8s-dns",
				"segmentum": k8sServiceSegmentum,
			},
			Discovered: map[string]any{
				"search_domains": b.env.SearchDomains,
			},
		}
		dnsNode.LastVerified = &now
		dnsNode.LastSeen = &now
		nodes = append(nodes, dnsNode)
	}

	// If we know the K8s node, create a node for it
	if b.env.NodeName != "" {
		hostNode := domain.Node{
			ID:     fmt.Sprintf("k8s-node-%s", strings.ToLower(b.env.NodeName)),
			Type:   domain.NodeTypeServer,
			Label:  b.env.NodeName,
			Source: "bootstrap",
			Status: domain.NodeStatusUnverified, // Need to verify via API
			Properties: map[string]any{
				"role": "k8s-node",
			},
		}
		hostNode.LastSeen = &now
		nodes = append(nodes, hostNode)
	}

	return nodes
}

// discoverNetworkInfrastructure discovers network components (gateway, external DNS)
func (b *BootstrapAdapter) discoverNetworkInfrastructure(now time.Time) []domain.Node {
	var nodes []domain.Node

	// Determine the pod network segmentum (10.42.0.0/16 for K3s pods)
	podNetworkSegmentum := ""
	if b.env.DefaultGateway != "" {
		parts := strings.Split(b.env.DefaultGateway, ".")
		if len(parts) == 4 {
			podNetworkSegmentum = fmt.Sprintf("%s.%s.0.0/16", parts[0], parts[1])
		}
	}

	// Default gateway (likely a router - in K8s context this is the pod network gateway)
	if b.env.DefaultGateway != "" {
		gwNode := domain.Node{
			ID:     strings.ReplaceAll(b.env.DefaultGateway, ".", "-"),
			Type:   domain.NodeTypeRouter,
			Label:  "gateway",
			Source: "bootstrap",
			Status: domain.NodeStatusUnverified,
			Properties: map[string]any{
				"ip":        b.env.DefaultGateway,
				"role":      "gateway",
				"segmentum": podNetworkSegmentum,
			},
		}
		gwNode.LastSeen = &now
		nodes = append(nodes, gwNode)
	}

	// External DNS servers (non-cluster)
	for _, dns := range b.env.DNSServers {
		// Skip cluster DNS
		if dns == b.env.ClusterDNS {
			continue
		}
		// Skip localhost
		if dns == "127.0.0.1" || dns == "::1" {
			continue
		}

		// Infer segmentum from DNS IP
		dnsSegmentum := ""
		dnsParts := strings.Split(dns, ".")
		if len(dnsParts) == 4 {
			dnsSegmentum = fmt.Sprintf("%s.%s.%s.0/24", dnsParts[0], dnsParts[1], dnsParts[2])
		}

		dnsNode := domain.Node{
			ID:     strings.ReplaceAll(dns, ".", "-"),
			Type:   domain.NodeTypeServer,
			Label:  fmt.Sprintf("dns-%s", dns),
			Source: "bootstrap",
			Status: domain.NodeStatusUnverified,
			Properties: map[string]any{
				"ip":        dns,
				"role":      "dns",
				"segmentum": dnsSegmentum,
			},
		}
		dnsNode.LastSeen = &now
		nodes = append(nodes, dnsNode)
	}

	return nodes
}

// createInfrastructureEdges creates edges from self to discovered nodes
func (b *BootstrapAdapter) createInfrastructureEdges(selfID string, nodes []domain.Node) []domain.Edge {
	var edges []domain.Edge

	for _, node := range nodes {
		if node.ID == selfID {
			continue
		}

		// Determine edge type based on node role
		edgeType := domain.EdgeTypeEthernet
		props := map[string]any{}

		if role, ok := node.Properties["role"].(string); ok {
			switch role {
			case "k8s-control-plane", "k8s-dns":
				edgeType = domain.EdgeTypeEthernet
				props["connection"] = "cluster-network"
			case "gateway":
				edgeType = domain.EdgeTypeEthernet
				props["connection"] = "default-route"
			case "dns":
				edgeType = domain.EdgeTypeEthernet
				props["connection"] = "dns-resolver"
			}
		}

		edge := domain.Edge{
			ID:         fmt.Sprintf("%s-to-%s", selfID, node.ID),
			FromID:     selfID,
			ToID:       node.ID,
			Type:       edgeType,
			Properties: props,
		}
		edges = append(edges, edge)
	}

	return edges
}

// publishProgress emits a discovery progress event
func (b *BootstrapAdapter) publishProgress(eventType string, payload interface{}) {
	if b.publisher != nil {
		b.publisher.PublishDiscoveryEvent(eventType, payload)
	}
}
