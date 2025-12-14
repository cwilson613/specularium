package adapter

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"specularium/internal/domain"
)

// Common service ports with their typical service names
var wellKnownPorts = map[int]string{
	21:    "ftp",
	22:    "ssh",
	23:    "telnet",
	25:    "smtp",
	53:    "dns",
	80:    "http",
	110:   "pop3",
	143:   "imap",
	443:   "https",
	445:   "smb",
	993:   "imaps",
	995:   "pop3s",
	3306:  "mysql",
	3389:  "rdp",
	5432:  "postgres",
	5900:  "vnc",
	6443:  "k8s-api",
	8080:  "http-alt",
	8443:  "https-alt",
	9090:  "prometheus",
	9100:  "node-exporter",
}

// NodeFetcher retrieves nodes that need verification
type NodeFetcher interface {
	// GetNodesForVerification returns nodes that need to be verified
	GetNodesForVerification(ctx context.Context) ([]domain.Node, error)
}

// PortInfo contains details about an open port
type PortInfo struct {
	Port    int    `json:"port"`
	Service string `json:"service"`
	Banner  string `json:"banner,omitempty"`
}

// ProbeResult contains the results of probing a single node
type ProbeResult struct {
	NodeID       string
	Status       domain.NodeStatus
	PingSuccess  bool
	PingLatency  time.Duration
	ICMPSuccess  bool
	ICMPLatency  time.Duration
	OpenPorts    []int
	ClosedPorts  []int
	PortDetails  []PortInfo
	MACAddress   string
	Hostname     string // Reverse DNS
	Error        string
	VerifiedAt   time.Time
}

// VerifierConfig holds configuration for the verifier adapter
type VerifierConfig struct {
	// PingTimeout for ICMP/TCP ping attempts
	PingTimeout time.Duration
	// PortTimeout for individual port probes
	PortTimeout time.Duration
	// BannerTimeout for reading service banners
	BannerTimeout time.Duration
	// CommonPorts to probe on all nodes
	CommonPorts []int
	// MaxConcurrent limits parallel probe operations
	MaxConcurrent int
	// VerifyInterval determines how often to re-verify already-verified nodes
	VerifyInterval time.Duration
	// EnableICMP enables ICMP ping (requires ping binary)
	EnableICMP bool
	// EnableBannerGrab enables reading service banners
	EnableBannerGrab bool
	// EnableARPLookup enables MAC address discovery
	EnableARPLookup bool
	// DNSServer is an optional DNS server to use for PTR lookups
	DNSServer string
	// CapabilityManager provides access to secrets for enhanced discovery
	Capabilities *CapabilityManager
}

// DefaultVerifierConfig returns sensible defaults
func DefaultVerifierConfig() VerifierConfig {
	return VerifierConfig{
		PingTimeout:      3 * time.Second,
		PortTimeout:      2 * time.Second,
		BannerTimeout:    2 * time.Second,
		CommonPorts:      []int{22, 25, 80, 443, 53, 8080, 8443, 3389, 5900},
		MaxConcurrent:    10,
		VerifyInterval:   5 * time.Minute,
		EnableICMP:       true,
		EnableBannerGrab: true,
		EnableARPLookup:  true,
	}
}

// VerifierAdapter probes nodes to verify reachability and discover metadata
type VerifierAdapter struct {
	config    VerifierConfig
	fetcher   NodeFetcher
	publisher EventPublisher
	mu        sync.Mutex
	running   bool
}

// NewVerifierAdapter creates a new verifier adapter
func NewVerifierAdapter(fetcher NodeFetcher, config VerifierConfig) *VerifierAdapter {
	return &VerifierAdapter{
		config:  config,
		fetcher: fetcher,
	}
}

// SetEventPublisher sets the event publisher for progress updates
func (v *VerifierAdapter) SetEventPublisher(pub EventPublisher) {
	v.publisher = pub
}

// publishProgress emits a discovery progress event
func (v *VerifierAdapter) publishProgress(payload interface{}) {
	if v.publisher != nil {
		v.publisher.PublishDiscoveryEvent("discovery-progress", payload)
	}
}

// Name returns the adapter identifier
func (v *VerifierAdapter) Name() string {
	return "verifier"
}

// Type returns the adapter type
func (v *VerifierAdapter) Type() AdapterType {
	return AdapterTypePolling
}

// Priority returns the adapter priority
func (v *VerifierAdapter) Priority() int {
	return 50 // Medium priority - discovered data supplements but doesn't override
}

// Start initializes the adapter
func (v *VerifierAdapter) Start(ctx context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.running = true
	log.Printf("Verifier adapter started (timeout=%s, ports=%v, concurrency=%d)",
		v.config.PingTimeout, v.config.CommonPorts, v.config.MaxConcurrent)
	return nil
}

// Stop shuts down the adapter
func (v *VerifierAdapter) Stop() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.running = false
	log.Printf("Verifier adapter stopped")
	return nil
}

// Sync probes all nodes that need verification and returns updated status
func (v *VerifierAdapter) Sync(ctx context.Context) (*domain.GraphFragment, error) {
	nodes, err := v.fetcher.GetNodesForVerification(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch nodes: %w", err)
	}

	if len(nodes) == 0 {
		// Emit complete event with zero nodes message
		if v.publisher != nil {
			v.publisher.PublishDiscoveryEvent("discovery-complete", map[string]interface{}{
				"total":       0,
				"verified":    0,
				"unreachable": 0,
				"degraded":    0,
				"message":     "All nodes recently verified",
			})
		}
		return nil, nil
	}

	log.Printf("Verifying %d nodes", len(nodes))

	// Emit discovery started event
	if v.publisher != nil {
		v.publisher.PublishDiscoveryEvent("discovery-started", map[string]interface{}{
			"total":   len(nodes),
			"message": fmt.Sprintf("Starting discovery of %d nodes", len(nodes)),
		})
	}

	// Create work channel and results
	workCh := make(chan domain.Node, len(nodes))
	resultCh := make(chan ProbeResult, len(nodes))

	// Start worker pool
	var wg sync.WaitGroup
	for i := 0; i < v.config.MaxConcurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for node := range workCh {
				select {
				case <-ctx.Done():
					return
				default:
					result := v.probeNode(ctx, node)
					// Emit progress event for each node
					v.publishProgress(map[string]interface{}{
						"node_id":  result.NodeID,
						"status":   string(result.Status),
						"ip":       node.GetPropertyString("ip"),
						"icmp":     result.ICMPSuccess,
						"ping":     result.PingSuccess,
						"latency":  result.PingLatency.Milliseconds(),
						"ports":    result.OpenPorts,
						"services": result.PortDetails,
						"mac":      result.MACAddress,
						"hostname": result.Hostname,
						"error":    result.Error,
					})
					resultCh <- result
				}
			}
		}()
	}

	// Queue work
	for _, node := range nodes {
		workCh <- node
	}
	close(workCh)

	// Wait for workers and close results
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results into fragment
	fragment := domain.NewGraphFragment()
	verified := 0
	unreachable := 0
	degraded := 0

	for result := range resultCh {
		node := v.resultToNode(result)
		fragment.AddNode(node)

		switch result.Status {
		case domain.NodeStatusVerified:
			verified++
		case domain.NodeStatusUnreachable:
			unreachable++
		case domain.NodeStatusDegraded:
			degraded++
		}
	}

	// Emit discovery complete event
	if v.publisher != nil {
		v.publisher.PublishDiscoveryEvent("discovery-complete", map[string]interface{}{
			"total":       len(nodes),
			"verified":    verified,
			"unreachable": unreachable,
			"degraded":    degraded,
			"message":     fmt.Sprintf("Discovery complete: %d verified, %d degraded, %d unreachable", verified, degraded, unreachable),
		})
	}

	return fragment, nil
}

// probeNode performs all probes on a single node
func (v *VerifierAdapter) probeNode(ctx context.Context, node domain.Node) ProbeResult {
	result := ProbeResult{
		NodeID:     node.ID,
		VerifiedAt: time.Now(),
	}

	// Get IP address
	ip := node.GetPropertyString("ip")
	if ip == "" {
		result.Status = domain.NodeStatusUnreachable
		result.Error = "no IP address"
		return result
	}

	// ICMP ping (if enabled)
	if v.config.EnableICMP {
		result.ICMPSuccess, result.ICMPLatency = v.icmpPing(ctx, ip)
	}

	// TCP ping (more reliable than ICMP which often requires root)
	pingSuccess, latency := v.tcpPing(ctx, ip)
	result.PingSuccess = pingSuccess
	result.PingLatency = latency

	// Use ICMP result if TCP ping failed but ICMP succeeded
	if !result.PingSuccess && result.ICMPSuccess {
		result.PingSuccess = true
		result.PingLatency = result.ICMPLatency
	}

	// Port probes with service identification
	if result.PingSuccess {
		result.OpenPorts, result.ClosedPorts, result.PortDetails = v.probePortsWithDetails(ctx, ip)
	}

	// Reverse DNS lookup
	result.Hostname = v.reverseDNS(ip)

	// ARP lookup for MAC address (if enabled)
	if v.config.EnableARPLookup {
		result.MACAddress = v.arpLookup(ip)
	}

	// Determine status
	if result.PingSuccess {
		if len(result.OpenPorts) > 0 {
			result.Status = domain.NodeStatusVerified
		} else {
			// Reachable but no open ports - might be heavily firewalled
			result.Status = domain.NodeStatusDegraded
		}
	} else {
		result.Status = domain.NodeStatusUnreachable
	}

	log.Printf("Verified %s (%s): status=%s, icmp=%v, tcp=%v (%s), mac=%s, ports=%v",
		node.ID, ip, result.Status, result.ICMPSuccess, result.PingSuccess, result.PingLatency, result.MACAddress, result.OpenPorts)

	return result
}

// tcpPing attempts a TCP connection to common ports to check reachability
func (v *VerifierAdapter) tcpPing(ctx context.Context, ip string) (bool, time.Duration) {
	// Try common ports for TCP ping
	ports := []int{22, 80, 443, 53}

	for _, port := range ports {
		addr := fmt.Sprintf("%s:%d", ip, port)
		start := time.Now()

		dialer := net.Dialer{Timeout: v.config.PingTimeout}
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err == nil {
			conn.Close()
			return true, time.Since(start)
		}

		// Check if it's a connection refused (host is up, port closed)
		if opErr, ok := err.(*net.OpError); ok {
			if _, ok := opErr.Err.(*net.DNSError); !ok {
				// Not a DNS error - host responded (even if refused)
				return true, time.Since(start)
			}
		}
	}

	return false, 0
}

// probePorts checks which common ports are open
func (v *VerifierAdapter) probePorts(ctx context.Context, ip string) (open, closed []int) {
	for _, port := range v.config.CommonPorts {
		addr := fmt.Sprintf("%s:%d", ip, port)

		dialer := net.Dialer{Timeout: v.config.PortTimeout}
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err == nil {
			conn.Close()
			open = append(open, port)
		} else {
			closed = append(closed, port)
		}
	}
	return
}

// reverseDNS performs a reverse DNS lookup
// Priority: 1) Static DNSServer config, 2) DNS capability from secrets, 3) System resolver
func (v *VerifierAdapter) reverseDNS(ip string) string {
	dnsServer := v.config.DNSServer

	// If no static DNS configured, try to get from capabilities
	if dnsServer == "" && v.config.Capabilities != nil {
		if dnsCap, err := v.config.Capabilities.GetDNSCapability(context.Background()); err == nil && dnsCap != nil {
			dnsServer = dnsCap.Server
		}
	}

	if dnsServer != "" {
		// Use custom DNS server for PTR lookup
		return v.reverseDNSCustom(ip, dnsServer)
	}

	// Fall back to system resolver
	names, err := net.LookupAddr(ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	// Remove trailing dot from FQDN
	hostname := names[0]
	if len(hostname) > 0 && hostname[len(hostname)-1] == '.' {
		hostname = hostname[:len(hostname)-1]
	}
	return hostname
}

// reverseDNSCustom performs PTR lookup against a specific DNS server
func (v *VerifierAdapter) reverseDNSCustom(ip, dnsServer string) string {
	// Create a custom resolver
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: v.config.PingTimeout}
			return d.DialContext(ctx, "udp", dnsServer+":53")
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), v.config.PingTimeout*2)
	defer cancel()

	names, err := resolver.LookupAddr(ctx, ip)
	if err != nil || len(names) == 0 {
		return ""
	}

	hostname := names[0]
	if len(hostname) > 0 && hostname[len(hostname)-1] == '.' {
		hostname = hostname[:len(hostname)-1]
	}
	return hostname
}

// icmpPing performs an ICMP ping using the system ping command
func (v *VerifierAdapter) icmpPing(ctx context.Context, ip string) (bool, time.Duration) {
	// Use system ping command with 1 packet and timeout
	timeoutSec := int(v.config.PingTimeout.Seconds())
	if timeoutSec < 1 {
		timeoutSec = 1
	}

	ctx, cancel := context.WithTimeout(ctx, v.config.PingTimeout+time.Second)
	defer cancel()

	// Linux ping: -c count, -W timeout in seconds
	cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", strconv.Itoa(timeoutSec), ip)
	output, err := cmd.Output()
	if err != nil {
		return false, 0
	}

	// Parse latency from output: "time=X.XX ms"
	latencyRe := regexp.MustCompile(`time[=<](\d+\.?\d*)\s*ms`)
	matches := latencyRe.FindSubmatch(output)
	if len(matches) >= 2 {
		if latencyMs, err := strconv.ParseFloat(string(matches[1]), 64); err == nil {
			return true, time.Duration(latencyMs * float64(time.Millisecond))
		}
	}

	// Ping succeeded but couldn't parse latency
	return true, 0
}

// arpLookup retrieves the MAC address for an IP from the ARP cache
func (v *VerifierAdapter) arpLookup(ip string) string {
	// Read /proc/net/arp on Linux
	cmd := exec.Command("cat", "/proc/net/arp")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Parse ARP table
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[0] == ip {
			mac := fields[3]
			// Skip incomplete entries (00:00:00:00:00:00)
			if mac != "00:00:00:00:00:00" {
				return strings.ToUpper(mac)
			}
		}
	}

	// Try arping to populate ARP cache (non-blocking attempt)
	// This requires the host to respond and may need root
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		exec.CommandContext(ctx, "arping", "-c", "1", "-w", "1", ip).Run()
	}()

	return ""
}

// probePortsWithDetails checks ports and identifies services
func (v *VerifierAdapter) probePortsWithDetails(ctx context.Context, ip string) (open, closed []int, details []PortInfo) {
	for _, port := range v.config.CommonPorts {
		addr := fmt.Sprintf("%s:%d", ip, port)

		dialer := net.Dialer{Timeout: v.config.PortTimeout}
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			closed = append(closed, port)
			continue
		}

		open = append(open, port)

		// Get service name from well-known ports
		serviceName := wellKnownPorts[port]
		if serviceName == "" {
			serviceName = fmt.Sprintf("unknown-%d", port)
		}

		info := PortInfo{
			Port:    port,
			Service: serviceName,
		}

		// Try banner grabbing if enabled
		if v.config.EnableBannerGrab {
			info.Banner = v.grabBanner(conn, port)
		}

		conn.Close()
		details = append(details, info)
	}
	return
}

// grabBanner attempts to read a service banner from an open connection
func (v *VerifierAdapter) grabBanner(conn net.Conn, port int) string {
	conn.SetReadDeadline(time.Now().Add(v.config.BannerTimeout))

	// For HTTP ports, send a request to get headers
	if port == 80 || port == 8080 {
		fmt.Fprintf(conn, "HEAD / HTTP/1.0\r\nHost: %s\r\n\r\n", conn.RemoteAddr().String())
	} else if port == 443 || port == 8443 {
		// Skip TLS ports for plain banner grab
		return ""
	}

	// Read response
	reader := bufio.NewReader(conn)
	banner, err := reader.ReadString('\n')
	if err != nil {
		return ""
	}

	// Clean up banner
	banner = strings.TrimSpace(banner)
	// Limit length
	if len(banner) > 100 {
		banner = banner[:100] + "..."
	}

	return banner
}

// extractHostnameFromSMTPBanner parses SMTP banner for hostname
// Format: "220 hostname.domain.tld ESMTP ..."
func extractHostnameFromSMTPBanner(banner string) string {
	if banner == "" {
		return ""
	}
	// SMTP banners typically start with "220 hostname ..."
	if !strings.HasPrefix(banner, "220 ") {
		return ""
	}
	parts := strings.Fields(banner)
	if len(parts) < 2 {
		return ""
	}
	hostname := parts[1]
	// Validate it looks like a hostname (contains at least one dot or is alphanumeric)
	if strings.Contains(hostname, ".") || isValidHostname(hostname) {
		return strings.ToLower(hostname)
	}
	return ""
}

// extractHostnameFromSSHBanner parses SSH banner for hostname hints
// Format: "SSH-2.0-OpenSSH_8.9p1 Ubuntu-3ubuntu0.13" (usually no hostname)
// Some custom configs include hostname in comments
func extractHostnameFromSSHBanner(banner string) string {
	if banner == "" {
		return ""
	}
	// SSH banners rarely contain hostnames, but check for common patterns
	// Some servers include hostname after version string
	// Example: "SSH-2.0-OpenSSH_8.9 hostname.domain.tld"
	parts := strings.Fields(banner)
	for _, part := range parts {
		// Look for FQDN patterns (word.word.word)
		if strings.Count(part, ".") >= 2 && isValidHostname(part) {
			return strings.ToLower(part)
		}
	}
	return ""
}

// isValidHostname checks if a string looks like a valid hostname
func isValidHostname(s string) bool {
	if len(s) == 0 || len(s) > 255 {
		return false
	}
	// Must not be an IP address
	if net.ParseIP(s) != nil {
		return false
	}
	// Basic validation: alphanumeric, hyphens, dots
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

// resultToNode converts a probe result to a node with updated fields
func (v *VerifierAdapter) resultToNode(result ProbeResult) domain.Node {
	now := result.VerifiedAt
	node := domain.Node{
		ID:           result.NodeID,
		Status:       result.Status,
		LastVerified: &now,
		Discovered:   make(map[string]any),
		Source:       "verifier",
	}

	if result.PingSuccess {
		node.LastSeen = &now
		node.SetDiscovered("ping_latency_ms", result.PingLatency.Milliseconds())
	}

	if result.ICMPSuccess {
		node.SetDiscovered("icmp_latency_ms", result.ICMPLatency.Milliseconds())
	}

	if len(result.OpenPorts) > 0 {
		node.SetDiscovered("open_ports", result.OpenPorts)
	}

	if len(result.PortDetails) > 0 {
		node.SetDiscovered("services", result.PortDetails)
	}

	if result.Hostname != "" {
		node.SetDiscovered("reverse_dns", result.Hostname)
	}

	if result.MACAddress != "" {
		node.SetDiscovered("mac_address", result.MACAddress)
	}

	if result.Error != "" {
		node.SetDiscovered("last_error", result.Error)
	}

	// Build hostname inference from all available sources
	inference := v.buildHostnameInference(result, now)
	if len(inference.Candidates) > 0 {
		node.SetDiscovered("hostname_inference", inference)
	}

	return node
}

// buildHostnameInference gathers hostname candidates from all sources
func (v *VerifierAdapter) buildHostnameInference(result ProbeResult, now time.Time) domain.HostnameInference {
	inference := domain.HostnameInference{}

	// Source 1: Reverse DNS (PTR record) - highest confidence
	if result.Hostname != "" {
		inference.AddCandidate(result.Hostname, domain.SourcePTR, now)
	}

	// Source 2-N: Service banners
	for _, svc := range result.PortDetails {
		switch svc.Service {
		case "smtp":
			if hostname := extractHostnameFromSMTPBanner(svc.Banner); hostname != "" {
				inference.AddCandidate(hostname, domain.SourceSMTPBanner, now)
			}
		case "ssh":
			if hostname := extractHostnameFromSSHBanner(svc.Banner); hostname != "" {
				inference.AddCandidate(hostname, domain.SourceSSHBanner, now)
			}
		}
	}

	return inference
}
