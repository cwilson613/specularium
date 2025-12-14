package adapter

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	nmap "github.com/Ullaakut/nmap/v3"
	"specularium/internal/domain"
)

// NmapAdapter performs network scanning using nmap to discover services
type NmapAdapter struct {
	targets           []string
	interval          time.Duration
	timeout           time.Duration
	portRange         string
	serviceDetection  bool
	osDetection       bool
	skipHostDiscovery bool
	publisher         EventPublisher
	mu                sync.Mutex
	running           bool
	lastScanTime      time.Time
}

// NewNmapAdapter creates a new nmap-based scanning adapter
// targets: list of CIDR ranges or individual IPs to scan
// opts: optional configuration options
func NewNmapAdapter(targets []string, opts ...NmapOption) *NmapAdapter {
	adapter := &NmapAdapter{
		targets:          targets,
		interval:         5 * time.Minute,
		timeout:          10 * time.Minute,
		portRange:        "22,25,53,80,443,445,3389,5432,5900,6443,8080,8443,9090,9100",
		serviceDetection: true,
		osDetection:      false, // Requires root
	}

	// Apply options
	for _, opt := range opts {
		opt(adapter)
	}

	return adapter
}

// SetEventPublisher sets the event publisher for progress updates
func (n *NmapAdapter) SetEventPublisher(pub EventPublisher) {
	n.publisher = pub
}

// publishProgress emits a discovery progress event
func (n *NmapAdapter) publishProgress(eventType string, payload interface{}) {
	if n.publisher != nil {
		n.publisher.PublishDiscoveryEvent(eventType, payload)
	}
}

// Name returns the adapter identifier
func (n *NmapAdapter) Name() string {
	return "nmap"
}

// Type returns the adapter type
func (n *NmapAdapter) Type() AdapterType {
	return AdapterTypePolling
}

// Priority returns the adapter priority
func (n *NmapAdapter) Priority() int {
	return 80 // Higher than basic scanner, lower than bootstrap
}

// Start initializes the adapter
func (n *NmapAdapter) Start(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Check if nmap is available
	if !n.isNmapAvailable(ctx) {
		return fmt.Errorf("nmap binary not found in PATH")
	}

	n.running = true
	log.Printf("Nmap adapter started (targets=%v, port_range=%s, service_detection=%v, os_detection=%v)",
		n.targets, n.portRange, n.serviceDetection, n.osDetection)
	return nil
}

// Stop shuts down the adapter
func (n *NmapAdapter) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.running = false
	log.Printf("Nmap adapter stopped")
	return nil
}

// Sync runs an nmap scan and returns discovered evidence
func (n *NmapAdapter) Sync(ctx context.Context) (*domain.GraphFragment, error) {
	n.mu.Lock()
	if !n.running {
		n.mu.Unlock()
		return nil, fmt.Errorf("adapter not running")
	}
	n.lastScanTime = time.Now()
	n.mu.Unlock()

	if len(n.targets) == 0 {
		log.Printf("Nmap: no targets configured")
		return nil, nil
	}

	log.Printf("Nmap: starting scan of %d targets: %v", len(n.targets), n.targets)
	n.publishProgress("discovery-started", map[string]interface{}{
		"total":   len(n.targets),
		"message": fmt.Sprintf("Starting nmap scan of %d targets", len(n.targets)),
		"phase":   "nmap_scan",
	})

	fragment := domain.NewGraphFragment()

	for _, target := range n.targets {
		if err := n.scanTarget(ctx, target, fragment); err != nil {
			log.Printf("Nmap: error scanning %s: %v", target, err)
			continue
		}
	}

	n.publishProgress("discovery-complete", map[string]interface{}{
		"total":      len(n.targets),
		"discovered": len(fragment.Nodes),
		"message":    fmt.Sprintf("Nmap scan complete: %d hosts discovered", len(fragment.Nodes)),
	})

	log.Printf("Nmap: scan complete, discovered %d nodes", len(fragment.Nodes))
	return fragment, nil
}

// isNmapAvailable checks if nmap binary exists
func (n *NmapAdapter) isNmapAvailable(ctx context.Context) bool {
	scanner, err := nmap.NewScanner(
		ctx,
		nmap.WithTargets("localhost"),
		nmap.WithListScan(),
	)
	if err != nil {
		return false
	}

	// Try to run a simple list scan
	_, _, err = scanner.Run()
	return err == nil
}

// scanTarget performs nmap scan on a single target
func (n *NmapAdapter) scanTarget(ctx context.Context, target string, fragment *domain.GraphFragment) error {
	// Build nmap options
	opts := []nmap.Option{
		nmap.WithTargets(target),
		nmap.WithPorts(n.portRange),
	}

	// Add service detection if enabled
	if n.serviceDetection {
		opts = append(opts, nmap.WithServiceInfo())
	}

	// Add OS detection if enabled (requires root)
	if n.osDetection {
		opts = append(opts, nmap.WithOSDetection())
	}

	// Skip host discovery for specific targets
	if n.skipHostDiscovery {
		opts = append(opts, nmap.WithSkipHostDiscovery())
	}

	// Create scanner
	scanner, err := nmap.NewScanner(ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	// Run scan
	log.Printf("Nmap: scanning target %s", target)
	result, warnings, err := scanner.Run()
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	if warnings != nil && len(*warnings) > 0 {
		log.Printf("Nmap: warnings for %s: %v", target, *warnings)
	}

	// Process results
	return n.processResults(result, fragment)
}

// processResults converts nmap scan results to graph fragment with evidence
func (n *NmapAdapter) processResults(result *nmap.Run, fragment *domain.GraphFragment) error {
	if result == nil {
		return fmt.Errorf("nil scan result")
	}

	now := time.Now()

	for _, host := range result.Hosts {
		if len(host.Addresses) == 0 {
			continue
		}

		// Get primary IP address
		var ip string
		for _, addr := range host.Addresses {
			if addr.AddrType == "ipv4" {
				ip = addr.Addr
				break
			}
		}

		if ip == "" {
			// Fallback to first address
			ip = host.Addresses[0].Addr
		}

		// Skip if host is down
		if host.Status.State != "up" {
			continue
		}

		log.Printf("Nmap: processing host %s (%d ports)", ip, len(host.Ports))

		// Create or update node
		nodeID := sanitizeIP(ip)
		node := n.createNodeFromHost(host, ip, nodeID, now)

		// Add evidence for each discovered service
		evidence := n.createEvidenceFromPorts(host.Ports, now)
		if len(evidence) > 0 {
			node.SetDiscovered("nmap_evidence", evidence)
		}

		// Add OS detection results if available
		if len(host.OS.Matches) > 0 {
			osInfo := n.extractOSInfo(host.OS)
			node.SetDiscovered("os_detection", osInfo)

			// Add OS evidence
			osEvidence := domain.Evidence{
				Source:     domain.EvidenceSourceBanner,
				Property:   "os_family",
				Value:      osInfo["name"],
				Confidence: osInfo["accuracy"].(float64) / 100.0,
				ObservedAt: now,
			}
			evidence = append(evidence, osEvidence)
		}

		// Add service summary
		openPorts := n.getOpenPorts(host.Ports)
		if len(openPorts) > 0 {
			node.SetDiscovered("open_ports", openPorts)
			node.SetDiscovered("port_count", len(openPorts))
		}

		// Add port details
		portDetails := n.createPortDetails(host.Ports)
		if len(portDetails) > 0 {
			node.SetDiscovered("services", portDetails)
		}

		// Emit progress for this host
		n.publishProgress("discovery-progress", map[string]interface{}{
			"ip":       ip,
			"ports":    openPorts,
			"services": portDetails,
			"message":  fmt.Sprintf("Discovered %s: %d services", ip, len(openPorts)),
			"phase":    "nmap_scan",
		})

		fragment.AddNode(node)
	}

	return nil
}

// createNodeFromHost creates a node from nmap host results
func (n *NmapAdapter) createNodeFromHost(host nmap.Host, ip, nodeID string, now time.Time) domain.Node {
	node := domain.Node{
		ID:         nodeID,
		Type:       n.inferNodeType(host.Ports),
		Label:      ip,
		Source:     "nmap",
		Status:     domain.NodeStatusVerified,
		Properties: map[string]any{
			"ip": ip,
		},
		Discovered:   make(map[string]any),
		LastVerified: &now,
		LastSeen:     &now,
	}

	// Add hostname from nmap results
	if len(host.Hostnames) > 0 {
		hostname := host.Hostnames[0].Name
		node.Label = hostname
		node.SetDiscovered("reverse_dns", hostname)

		// Extract short name
		if idx := strings.Index(hostname, "."); idx > 0 {
			shortName := hostname[:idx]
			if len(shortName) > 2 {
				node.Label = shortName
			}
		}
	}

	// Add MAC address if available
	for _, addr := range host.Addresses {
		if addr.AddrType == "mac" {
			node.SetDiscovered("mac_address", strings.ToUpper(addr.Addr))
			if addr.Vendor != "" {
				node.SetDiscovered("mac_vendor", addr.Vendor)
			}
		}
	}

	return node
}

// createEvidenceFromPorts generates evidence entries from port scan results
func (n *NmapAdapter) createEvidenceFromPorts(ports []nmap.Port, now time.Time) []domain.Evidence {
	var evidence []domain.Evidence

	for _, port := range ports {
		if port.State.State != "open" {
			continue
		}

		// Evidence from port being open
		portEvidence := domain.Evidence{
			Source:     domain.EvidenceSourcePortScan,
			Property:   fmt.Sprintf("service:%d", port.ID),
			Value:      port.State.State,
			Confidence: 0.5, // Port open = moderate confidence
			ObservedAt: now,
			Raw: map[string]any{
				"port":     port.ID,
				"protocol": port.Protocol,
				"state":    port.State.State,
			},
		}
		evidence = append(evidence, portEvidence)

		// Evidence from service detection
		if port.Service.Name != "" {
			serviceEvidence := domain.Evidence{
				Source:     domain.EvidenceSourceBanner,
				Property:   fmt.Sprintf("service:%d:name", port.ID),
				Value:      port.Service.Name,
				Confidence: 0.7, // Service detection = higher confidence
				ObservedAt: now,
				Raw: map[string]any{
					"port":    port.ID,
					"service": port.Service.Name,
					"product": port.Service.Product,
					"version": port.Service.Version,
				},
			}
			evidence = append(evidence, serviceEvidence)
		}

		// Evidence from version detection
		if port.Service.Product != "" {
			versionEvidence := domain.Evidence{
				Source:     domain.EvidenceSourceBanner,
				Property:   fmt.Sprintf("service:%d:product", port.ID),
				Value:      fmt.Sprintf("%s %s", port.Service.Product, port.Service.Version),
				Confidence: 0.8, // Version info = high confidence
				ObservedAt: now,
				Raw: map[string]any{
					"port":       port.ID,
					"product":    port.Service.Product,
					"version":    port.Service.Version,
					"extra_info": port.Service.ExtraInfo,
				},
			}
			evidence = append(evidence, versionEvidence)
		}
	}

	return evidence
}

// createPortDetails creates PortInfo structures from nmap ports
func (n *NmapAdapter) createPortDetails(ports []nmap.Port) []PortInfo {
	var details []PortInfo

	for _, port := range ports {
		if port.State.State != "open" {
			continue
		}

		serviceName := port.Service.Name
		if serviceName == "" {
			serviceName = wellKnownPorts[int(port.ID)]
			if serviceName == "" {
				serviceName = fmt.Sprintf("unknown-%d", port.ID)
			}
		}

		info := PortInfo{
			Port:    int(port.ID),
			Service: serviceName,
		}

		// Build banner from service info
		if port.Service.Product != "" {
			banner := port.Service.Product
			if port.Service.Version != "" {
				banner += " " + port.Service.Version
			}
			if port.Service.ExtraInfo != "" {
				banner += " (" + port.Service.ExtraInfo + ")"
			}
			info.Banner = banner
		}

		details = append(details, info)
	}

	return details
}

// getOpenPorts extracts list of open port numbers
func (n *NmapAdapter) getOpenPorts(ports []nmap.Port) []int {
	var openPorts []int
	for _, port := range ports {
		if port.State.State == "open" {
			openPorts = append(openPorts, int(port.ID))
		}
	}
	return openPorts
}

// extractOSInfo converts nmap OS detection to map
func (n *NmapAdapter) extractOSInfo(os nmap.OS) map[string]any {
	if len(os.Matches) == 0 {
		return nil
	}

	// Use first (best) match
	match := os.Matches[0]

	info := map[string]any{
		"name":     match.Name,
		"accuracy": match.Accuracy,
	}

	// Extract OS family if available
	for _, class := range match.Classes {
		if class.Type != "" {
			info["type"] = class.Type
		}
		if class.Vendor != "" {
			info["vendor"] = class.Vendor
		}
		if class.Family != "" {
			info["family"] = class.Family
		}
	}

	return info
}

// inferNodeType guesses node type from open ports
func (n *NmapAdapter) inferNodeType(ports []nmap.Port) domain.NodeType {
	portSet := make(map[uint16]bool)
	for _, p := range ports {
		if p.State.State == "open" {
			portSet[p.ID] = true
		}
	}

	// Router indicators
	if portSet[53] && (portSet[80] || portSet[443]) {
		return domain.NodeTypeRouter
	}

	// Kubernetes node
	if portSet[6443] || portSet[10250] {
		return domain.NodeTypeServer
	}

	// Windows machine
	if portSet[3389] || portSet[445] {
		return domain.NodeTypeServer
	}

	// Linux server (SSH + web)
	if portSet[22] && (portSet[80] || portSet[443]) {
		return domain.NodeTypeServer
	}

	// Just SSH
	if portSet[22] {
		return domain.NodeTypeServer
	}

	// Web only
	if portSet[80] || portSet[443] || portSet[8080] {
		return domain.NodeTypeServer
	}

	return domain.NodeTypeUnknown
}

// sanitizeIP converts an IP address to a valid node ID
func sanitizeIP(ip string) string {
	// Parse IP to validate
	parsed := net.ParseIP(ip)
	if parsed != nil {
		ip = parsed.String()
	}
	return strings.ReplaceAll(ip, ".", "-")
}

// expandTargets expands CIDR notation targets (helper for configuration)
func expandTargets(targets []string) ([]string, error) {
	var expanded []string
	for _, target := range targets {
		// Check if it's CIDR notation
		if strings.Contains(target, "/") {
			_, ipNet, err := net.ParseCIDR(target)
			if err != nil {
				return nil, fmt.Errorf("invalid CIDR %s: %w", target, err)
			}
			// For nmap, we keep CIDR notation - it handles expansion
			expanded = append(expanded, ipNet.String())
		} else {
			// Single IP or hostname
			expanded = append(expanded, target)
		}
	}
	return expanded, nil
}

// parsePorts converts port range string to nmap format
func parsePorts(portRange string) (string, error) {
	// Validate port range format
	// Supported: "80,443,8080" or "1-1000" or "22,80-443,8080"
	parts := strings.Split(portRange, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "-") {
			// Range format
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return "", fmt.Errorf("invalid port range: %s", part)
			}
			start, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			if err != nil || start < 1 || start > 65535 {
				return "", fmt.Errorf("invalid port number: %s", rangeParts[0])
			}
			end, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err != nil || end < 1 || end > 65535 || end < start {
				return "", fmt.Errorf("invalid port number: %s", rangeParts[1])
			}
		} else {
			// Single port
			port, err := strconv.Atoi(part)
			if err != nil || port < 1 || port > 65535 {
				return "", fmt.Errorf("invalid port number: %s", part)
			}
		}
	}
	return portRange, nil
}
