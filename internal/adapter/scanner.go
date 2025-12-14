package adapter

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"specularium/internal/domain"
)

// ScannerConfig holds configuration for the subnet scanner
type ScannerConfig struct {
	// DiscoveryPorts are probed to find live hosts
	DiscoveryPorts []int
	// ScanPorts are scanned on discovered hosts for service detection
	ScanPorts []int
	// Timeout for individual connection attempts
	Timeout time.Duration
	// MaxConcurrent limits parallel probe operations
	MaxConcurrent int
	// BannerTimeout for reading service banners
	BannerTimeout time.Duration
	// DNSServer is an optional DNS server to use for PTR lookups
	// If empty, the system resolver is used
	DNSServer string
	// CapabilityManager provides access to secrets for enhanced discovery
	Capabilities *CapabilityManager
}

// DefaultScannerConfig returns sensible defaults for homelab scanning
func DefaultScannerConfig() ScannerConfig {
	return ScannerConfig{
		// Common ports to probe for host discovery
		DiscoveryPorts: []int{22, 80, 443, 445, 3389, 5900, 8080},
		// Extended ports for service detection on found hosts
		ScanPorts: []int{
			21, 22, 23, 25, 53, 80, 110, 143, 443, 445,
			993, 995, 3306, 3389, 5432, 5900, 6443,
			8080, 8443, 9090, 9100,
		},
		Timeout:       1 * time.Second,
		MaxConcurrent: 200,
		BannerTimeout: 1 * time.Second,
	}
}

// DiscoveredHost represents a host found during scanning
type DiscoveredHost struct {
	IP          string
	Hostname    string
	OpenPorts   []int
	PortDetails []PortInfo
	MACAddress  string // Populated from ARP cache if available
}

// ScannerAdapter discovers new hosts on a network subnet
type ScannerAdapter struct {
	config    ScannerConfig
	publisher EventPublisher
	mu        sync.Mutex
	scanning  bool
}

// NewScannerAdapter creates a new subnet scanner adapter
func NewScannerAdapter(config ScannerConfig) *ScannerAdapter {
	return &ScannerAdapter{
		config: config,
	}
}

// SetEventPublisher sets the event publisher for progress updates
func (s *ScannerAdapter) SetEventPublisher(pub EventPublisher) {
	s.publisher = pub
}

// publishProgress emits a discovery progress event
func (s *ScannerAdapter) publishProgress(eventType string, payload interface{}) {
	if s.publisher != nil {
		s.publisher.PublishDiscoveryEvent(eventType, payload)
	}
}

// Name returns the adapter identifier
func (s *ScannerAdapter) Name() string {
	return "scanner"
}

// Type returns the adapter type
func (s *ScannerAdapter) Type() AdapterType {
	return AdapterTypeOneShot
}

// Priority returns the adapter priority
func (s *ScannerAdapter) Priority() int {
	return 100 // High priority - scanner creates new nodes
}

// Start initializes the adapter
func (s *ScannerAdapter) Start(ctx context.Context) error {
	log.Printf("Scanner adapter started (discovery_ports=%v, max_concurrent=%d)",
		s.config.DiscoveryPorts, s.config.MaxConcurrent)
	return nil
}

// Stop shuts down the adapter
func (s *ScannerAdapter) Stop() error {
	log.Printf("Scanner adapter stopped")
	return nil
}

// Sync is not used for on-demand adapters
func (s *ScannerAdapter) Sync(ctx context.Context) (*domain.GraphFragment, error) {
	return nil, nil
}

// ScanSubnet scans a CIDR range and returns discovered hosts as a graph fragment
func (s *ScannerAdapter) ScanSubnet(ctx context.Context, cidr string) (*domain.GraphFragment, error) {
	s.mu.Lock()
	if s.scanning {
		s.mu.Unlock()
		return nil, fmt.Errorf("scan already in progress")
	}
	s.scanning = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.scanning = false
		s.mu.Unlock()
	}()

	// Parse CIDR
	ips, err := expandCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR: %w", err)
	}

	log.Printf("Starting subnet scan: %s (%d IPs), publisher=%v", cidr, len(ips), s.publisher != nil)

	s.publishProgress("discovery-started", map[string]interface{}{
		"total":   len(ips),
		"message": fmt.Sprintf("Scanning %s (%d IPs)", cidr, len(ips)),
		"phase":   "host_discovery",
	})

	// Phase 1: Host discovery - probe common ports to find live hosts
	log.Printf("Phase 1: Discovering hosts on %d IPs with ports %v", len(ips), s.config.DiscoveryPorts)
	liveHosts := s.discoverHosts(ctx, ips)
	log.Printf("Phase 1 complete: Found %d live hosts", len(liveHosts))

	if len(liveHosts) == 0 {
		log.Printf("No live hosts found in %s", cidr)
		s.publishProgress("discovery-complete", map[string]interface{}{
			"total":      len(ips),
			"discovered": 0,
			"message":    "No live hosts found",
		})
		return nil, nil
	}

	s.publishProgress("discovery-progress", map[string]interface{}{
		"message": fmt.Sprintf("Found %d live hosts, scanning services...", len(liveHosts)),
		"phase":   "service_scan",
	})

	// Phase 2: Service detection on live hosts
	log.Printf("Phase 2: Scanning services on %d hosts", len(liveHosts))
	hosts := s.scanHosts(ctx, liveHosts)
	log.Printf("Phase 2 complete: Scanned %d hosts", len(hosts))

	// Phase 3: Convert to graph fragment
	log.Printf("Phase 3: Converting %d hosts to graph fragment", len(hosts))
	fragment := s.hostsToFragment(hosts, cidr)
	log.Printf("Phase 3 complete: Created fragment with %d nodes", len(fragment.Nodes))

	s.publishProgress("discovery-complete", map[string]interface{}{
		"total":      len(ips),
		"discovered": len(hosts),
		"message":    fmt.Sprintf("Discovered %d hosts with services", len(hosts)),
	})

	log.Printf("Scan complete: returning fragment with %d nodes", len(fragment.Nodes))
	return fragment, nil
}

// discoverHosts finds live hosts by probing discovery ports
func (s *ScannerAdapter) discoverHosts(ctx context.Context, ips []string) []string {
	liveHosts := make(map[string]bool)
	var mu sync.Mutex

	// Create work channel
	type probeJob struct {
		ip   string
		port int
	}
	jobs := make(chan probeJob, len(ips)*len(s.config.DiscoveryPorts))

	// Start worker pool
	var wg sync.WaitGroup
	for i := 0; i < s.config.MaxConcurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
					if s.probePort(ctx, job.ip, job.port) {
						mu.Lock()
						if !liveHosts[job.ip] {
							liveHosts[job.ip] = true
							// Emit progress for each newly discovered host
							s.publishProgress("discovery-progress", map[string]interface{}{
								"ip":      job.ip,
								"port":    job.port,
								"message": fmt.Sprintf("Host alive: %s (port %d)", job.ip, job.port),
								"phase":   "host_discovery",
							})
						}
						mu.Unlock()
					}
				}
			}
		}()
	}

	// Queue all probe jobs
	for _, ip := range ips {
		for _, port := range s.config.DiscoveryPorts {
			jobs <- probeJob{ip: ip, port: port}
		}
	}
	close(jobs)

	wg.Wait()

	// Convert map to sorted slice
	result := make([]string, 0, len(liveHosts))
	for ip := range liveHosts {
		result = append(result, ip)
	}
	sort.Strings(result)

	return result
}

// scanHosts performs detailed scanning on discovered hosts
func (s *ScannerAdapter) scanHosts(ctx context.Context, ips []string) []DiscoveredHost {
	hosts := make([]DiscoveredHost, 0, len(ips))
	var mu sync.Mutex

	// Scan hosts in parallel (but limit concurrency per host)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // Max 10 hosts scanned concurrently

	for _, ip := range ips {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			select {
			case <-ctx.Done():
				return
			default:
				host := s.scanHost(ctx, ip)
				mu.Lock()
				hosts = append(hosts, host)
				mu.Unlock()

				// Emit detailed progress
				s.publishProgress("discovery-progress", map[string]interface{}{
					"ip":       host.IP,
					"hostname": host.Hostname,
					"ports":    host.OpenPorts,
					"services": host.PortDetails,
					"mac":      host.MACAddress,
					"message":  fmt.Sprintf("Scanned %s: %d services", host.IP, len(host.OpenPorts)),
					"phase":    "service_scan",
				})
			}
		}(ip)
	}

	wg.Wait()

	// Sort by IP for consistent ordering
	sort.Slice(hosts, func(i, j int) bool {
		return hosts[i].IP < hosts[j].IP
	})

	return hosts
}

// scanHost performs a detailed scan of a single host
func (s *ScannerAdapter) scanHost(ctx context.Context, ip string) DiscoveredHost {
	host := DiscoveredHost{
		IP: ip,
	}

	// Reverse DNS lookup
	host.Hostname = s.reverseDNS(ip)

	// Try to get MAC from ARP cache
	host.MACAddress = s.arpLookup(ip)

	// Scan all configured ports
	var openPorts []int
	var portDetails []PortInfo

	type portResult struct {
		port   int
		open   bool
		detail PortInfo
	}
	results := make(chan portResult, len(s.config.ScanPorts))

	var wg sync.WaitGroup
	for _, port := range s.config.ScanPorts {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			if s.probePort(ctx, ip, p) {
				serviceName := wellKnownPorts[p]
				if serviceName == "" {
					serviceName = fmt.Sprintf("unknown-%d", p)
				}
				detail := PortInfo{
					Port:    p,
					Service: serviceName,
				}
				// Try banner grab
				detail.Banner = s.grabBanner(ip, p)
				results <- portResult{port: p, open: true, detail: detail}
			}
		}(port)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for r := range results {
		if r.open {
			openPorts = append(openPorts, r.port)
			portDetails = append(portDetails, r.detail)
		}
	}

	// Sort ports
	sort.Ints(openPorts)
	sort.Slice(portDetails, func(i, j int) bool {
		return portDetails[i].Port < portDetails[j].Port
	})

	host.OpenPorts = openPorts
	host.PortDetails = portDetails

	return host
}

// probePort attempts to connect to a TCP port
func (s *ScannerAdapter) probePort(ctx context.Context, ip string, port int) bool {
	addr := fmt.Sprintf("%s:%d", ip, port)
	dialer := net.Dialer{Timeout: s.config.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// reverseDNS performs a reverse DNS lookup
// Priority: 1) Static DNSServer config, 2) DNS capability from secrets, 3) System resolver
func (s *ScannerAdapter) reverseDNS(ip string) string {
	dnsServer := s.config.DNSServer

	// If no static DNS configured, try to get from capabilities
	if dnsServer == "" && s.config.Capabilities != nil {
		if dnsCap, err := s.config.Capabilities.GetDNSCapability(context.Background()); err == nil && dnsCap != nil {
			dnsServer = dnsCap.Server
		}
	}

	if dnsServer != "" {
		// Use custom DNS server for PTR lookup
		return s.reverseDNSCustom(ip, dnsServer)
	}

	// Fall back to system resolver
	names, err := net.LookupAddr(ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	hostname := names[0]
	if len(hostname) > 0 && hostname[len(hostname)-1] == '.' {
		hostname = hostname[:len(hostname)-1]
	}
	return hostname
}

// reverseDNSCustom performs PTR lookup against a specific DNS server
func (s *ScannerAdapter) reverseDNSCustom(ip, dnsServer string) string {
	// Create a custom resolver
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: s.config.Timeout}
			// Always connect to the configured DNS server
			return d.DialContext(ctx, "udp", dnsServer+":53")
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.config.Timeout*2)
	defer cancel()

	// Use LookupAddr with our custom resolver
	names, err := resolver.LookupAddr(ctx, ip)
	if err != nil || len(names) == 0 {
		log.Printf("PTR lookup for %s via %s failed: %v", ip, dnsServer, err)
		return ""
	}

	hostname := names[0]
	if len(hostname) > 0 && hostname[len(hostname)-1] == '.' {
		hostname = hostname[:len(hostname)-1]
	}
	log.Printf("PTR lookup for %s via %s: %s", ip, dnsServer, hostname)
	return hostname
}

// arpLookup retrieves MAC address from ARP cache
func (s *ScannerAdapter) arpLookup(ip string) string {
	// Try to read from /proc/net/arp (Linux)
	// This is best-effort - won't work in all containers
	// The ARP cache might not be populated for hosts we haven't communicated with
	// MAC discovery works better via DHCP query
	return ""
}

// grabBanner attempts to read a service banner
func (s *ScannerAdapter) grabBanner(ip string, port int) string {
	addr := fmt.Sprintf("%s:%d", ip, port)
	conn, err := net.DialTimeout("tcp", addr, s.config.Timeout)
	if err != nil {
		return ""
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(s.config.BannerTimeout))

	// For HTTP, send a request
	if port == 80 || port == 8080 {
		fmt.Fprintf(conn, "HEAD / HTTP/1.0\r\nHost: %s\r\n\r\n", ip)
	}

	// Read response
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return ""
	}

	banner := string(buf[:n])
	// Clean up - get first line only
	if idx := strings.Index(banner, "\n"); idx > 0 {
		banner = banner[:idx]
	}
	banner = strings.TrimSpace(banner)

	// Limit length
	if len(banner) > 100 {
		banner = banner[:100] + "..."
	}

	return banner
}

// hostsToFragment converts discovered hosts to a graph fragment
// Groups hosts by PTR hostname - multiple IPs with the same hostname become
// a parent node with interface children
// The segmentum parameter is the CIDR that was scanned (e.g., "192.168.0.0/24")
func (s *ScannerAdapter) hostsToFragment(hosts []DiscoveredHost, segmentum string) *domain.GraphFragment {
	fragment := domain.NewGraphFragment()

	// Group hosts by their resolved hostname (PTR)
	// Hosts without PTR get their own group keyed by IP
	hostGroups := make(map[string][]DiscoveredHost)
	for _, host := range hosts {
		groupKey := host.Hostname
		if groupKey == "" {
			// No PTR - use IP as unique key (won't group with anything)
			groupKey = "_ip_" + host.IP
		}
		hostGroups[groupKey] = append(hostGroups[groupKey], host)
	}

	now := time.Now()

	for groupKey, groupHosts := range hostGroups {
		if len(groupHosts) == 1 && strings.HasPrefix(groupKey, "_ip_") {
			// Single host with no PTR - create standalone node
			host := groupHosts[0]
			node := s.createStandaloneNode(host, segmentum, now)
			fragment.AddNode(node)
		} else if len(groupHosts) == 1 {
			// Single host with PTR - still create standalone (no need for interfaces)
			host := groupHosts[0]
			node := s.createStandaloneNode(host, segmentum, now)
			fragment.AddNode(node)
		} else {
			// Multiple hosts with same PTR - create parent + interface children
			s.createHostWithInterfaces(fragment, groupKey, groupHosts, segmentum, now)
		}
	}

	return fragment
}

// createStandaloneNode creates a single node for a discovered host
// segmentum is the CIDR range this host was discovered in (for visual grouping)
func (s *ScannerAdapter) createStandaloneNode(host DiscoveredHost, segmentum string, now time.Time) domain.Node {
	// Generate node ID from IP (sanitized)
	nodeID := strings.ReplaceAll(host.IP, ".", "-")

	// Determine node type based on open ports
	nodeType := inferNodeType(host.OpenPorts)

	// Use hostname as label if available, otherwise IP
	label := host.Hostname
	if label == "" {
		label = host.IP
	} else {
		// Clean up label - remove domain suffix for readability
		if idx := strings.Index(label, "."); idx > 0 {
			shortLabel := label[:idx]
			if len(shortLabel) > 2 {
				label = shortLabel
			}
		}
	}

	node := domain.Node{
		ID:     nodeID,
		Type:   nodeType,
		Label:  label,
		Source: "scanner",
		Status: domain.NodeStatusVerified,
		Properties: map[string]any{
			"ip":        host.IP,
			"segmentum": segmentum, // CIDR for visual fabric grouping
		},
		Discovered: map[string]any{
			"open_ports":  host.OpenPorts,
			"services":    host.PortDetails,
			"reverse_dns": host.Hostname,
		},
	}

	if host.MACAddress != "" {
		node.Discovered["mac_address"] = host.MACAddress
	}

	node.LastVerified = &now
	node.LastSeen = &now

	return node
}

// createHostWithInterfaces creates a parent node with interface children
// when multiple IPs resolve to the same PTR hostname
// segmentum is the CIDR range for visual fabric grouping
func (s *ScannerAdapter) createHostWithInterfaces(fragment *domain.GraphFragment, hostname string, hosts []DiscoveredHost, segmentum string, now time.Time) {
	// Extract short hostname for parent ID and label
	shortName := hostname
	if idx := strings.Index(hostname, "."); idx > 0 {
		shortName = hostname[:idx]
	}

	// Determine parent node type from combined port analysis
	allPorts := []int{}
	for _, h := range hosts {
		allPorts = append(allPorts, h.OpenPorts...)
	}
	parentType := inferNodeType(allPorts)

	// Create parent node
	parentNode := domain.Node{
		ID:     shortName,
		Type:   parentType,
		Label:  shortName,
		Source: "scanner",
		Status: domain.NodeStatusVerified,
		Properties: map[string]any{
			"hostname":  hostname,
			"segmentum": segmentum, // CIDR for visual fabric grouping
		},
		Discovered: map[string]any{
			"interface_count": len(hosts),
			"reverse_dns":     hostname,
		},
	}
	parentNode.LastVerified = &now
	parentNode.LastSeen = &now
	fragment.AddNode(parentNode)

	// Create interface nodes for each IP
	// Sort hosts by IP for consistent interface naming
	sort.Slice(hosts, func(i, j int) bool {
		return hosts[i].IP < hosts[j].IP
	})

	for i, host := range hosts {
		interfaceName := fmt.Sprintf("eth%d", i)
		interfaceID := fmt.Sprintf("%s:%s", shortName, interfaceName)

		interfaceNode := domain.Node{
			ID:       interfaceID,
			Type:     domain.NodeTypeInterface,
			Label:    interfaceName,
			ParentID: shortName,
			Source:   "scanner",
			Status:   domain.NodeStatusVerified,
			Properties: map[string]any{
				"ip":             host.IP,
				"interface_name": interfaceName,
				"segmentum":      segmentum, // CIDR for visual fabric grouping
			},
			Discovered: map[string]any{
				"open_ports":  host.OpenPorts,
				"services":    host.PortDetails,
				"reverse_dns": host.Hostname,
			},
		}

		if host.MACAddress != "" {
			interfaceNode.Discovered["mac_address"] = host.MACAddress
		}

		interfaceNode.LastVerified = &now
		interfaceNode.LastSeen = &now
		fragment.AddNode(interfaceNode)
	}

	log.Printf("Created parent node %s with %d interfaces (IPs: %v)",
		shortName, len(hosts), func() []string {
			ips := make([]string, len(hosts))
			for i, h := range hosts {
				ips[i] = h.IP
			}
			return ips
		}())
}

// inferNodeType guesses the device type based on open ports
func inferNodeType(ports []int) domain.NodeType {
	portSet := make(map[int]bool)
	for _, p := range ports {
		portSet[p] = true
	}

	// Router indicators
	if portSet[53] && (portSet[80] || portSet[443]) {
		return domain.NodeTypeRouter
	}

	// Network switch/AP (SNMP, web interface)
	if portSet[161] || (portSet[80] && !portSet[22] && !portSet[443]) {
		return domain.NodeTypeSwitch
	}

	// Windows machine
	if portSet[3389] || portSet[445] {
		return domain.NodeTypeServer
	}

	// Linux server (SSH + web)
	if portSet[22] && (portSet[80] || portSet[443]) {
		return domain.NodeTypeServer
	}

	// VNC suggests desktop/VM
	if portSet[5900] {
		return domain.NodeTypeVM
	}

	// Just SSH - likely a server
	if portSet[22] {
		return domain.NodeTypeServer
	}

	// Web only
	if portSet[80] || portSet[443] || portSet[8080] {
		return domain.NodeTypeServer
	}

	return domain.NodeTypeUnknown
}

// expandCIDR converts a CIDR notation to a list of IPs
func expandCIDR(cidr string) ([]string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		// Try parsing as single IP
		ip := net.ParseIP(cidr)
		if ip != nil {
			return []string{ip.String()}, nil
		}
		return nil, err
	}

	var ips []string

	// Get the network and broadcast addresses
	ip := ipNet.IP.To4()
	if ip == nil {
		return nil, fmt.Errorf("only IPv4 supported")
	}

	mask := ipNet.Mask

	// Calculate range
	networkInt := binary.BigEndian.Uint32(ip)
	maskInt := binary.BigEndian.Uint32(mask)

	// First and last addresses
	firstIP := networkInt & maskInt
	lastIP := firstIP | ^maskInt

	// Skip network and broadcast addresses for /24 and larger
	ones, bits := mask.Size()
	if ones <= 24 && bits == 32 {
		firstIP++
		lastIP--
	}

	// Safety limit - don't scan more than 1024 IPs
	if lastIP-firstIP > 1024 {
		return nil, fmt.Errorf("CIDR range too large (max 1024 IPs)")
	}

	for i := firstIP; i <= lastIP; i++ {
		ipBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(ipBytes, i)
		ips = append(ips, net.IP(ipBytes).String())
	}

	return ips, nil
}
