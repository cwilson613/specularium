package adapter

import (
	"context"
	"testing"
	"time"

	nmap "github.com/Ullaakut/nmap/v3"
	"specularium/internal/domain"
)

// TestNmapAdapter_Creation tests adapter creation with various options
func TestNmapAdapter_Creation(t *testing.T) {
	tests := []struct {
		name        string
		targets     []string
		opts        []NmapOption
		wantTargets int
	}{
		{
			name:        "default configuration",
			targets:     []string{"192.168.1.0/24"},
			opts:        nil,
			wantTargets: 1,
		},
		{
			name:    "with custom interval",
			targets: []string{"10.0.0.0/24"},
			opts: []NmapOption{
				WithInterval(10 * time.Minute),
			},
			wantTargets: 1,
		},
		{
			name:    "with custom port range",
			targets: []string{"192.168.1.1"},
			opts: []NmapOption{
				WithPortRange("80,443,8080"),
			},
			wantTargets: 1,
		},
		{
			name:    "with service detection disabled",
			targets: []string{"192.168.1.1"},
			opts: []NmapOption{
				WithServiceDetection(false),
			},
			wantTargets: 1,
		},
		{
			name:    "fast scan mode",
			targets: []string{"192.168.1.1"},
			opts: []NmapOption{
				WithFastScan(),
			},
			wantTargets: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewNmapAdapter(tt.targets, tt.opts...)
			if adapter == nil {
				t.Fatal("expected adapter, got nil")
			}
			if len(adapter.targets) != tt.wantTargets {
				t.Errorf("expected %d targets, got %d", tt.wantTargets, len(adapter.targets))
			}
		})
	}
}

// TestNmapAdapter_Options tests option functions
func TestNmapAdapter_Options(t *testing.T) {
	t.Run("WithInterval", func(t *testing.T) {
		adapter := NewNmapAdapter([]string{"192.168.1.1"}, WithInterval(15*time.Minute))
		if adapter.interval != 15*time.Minute {
			t.Errorf("expected interval 15m, got %v", adapter.interval)
		}
	})

	t.Run("WithTimeout", func(t *testing.T) {
		adapter := NewNmapAdapter([]string{"192.168.1.1"}, WithTimeout(20*time.Minute))
		if adapter.timeout != 20*time.Minute {
			t.Errorf("expected timeout 20m, got %v", adapter.timeout)
		}
	})

	t.Run("WithPortRange", func(t *testing.T) {
		adapter := NewNmapAdapter([]string{"192.168.1.1"}, WithPortRange("1-1000"))
		if adapter.portRange != "1-1000" {
			t.Errorf("expected port range 1-1000, got %s", adapter.portRange)
		}
	})

	t.Run("WithServiceDetection", func(t *testing.T) {
		adapter := NewNmapAdapter([]string{"192.168.1.1"}, WithServiceDetection(false))
		if adapter.serviceDetection != false {
			t.Error("expected service detection disabled")
		}
	})

	t.Run("WithOSDetection", func(t *testing.T) {
		adapter := NewNmapAdapter([]string{"192.168.1.1"}, WithOSDetection(true))
		if adapter.osDetection != true {
			t.Error("expected OS detection enabled")
		}
	})

	t.Run("WithSkipHostDiscovery", func(t *testing.T) {
		adapter := NewNmapAdapter([]string{"192.168.1.1"}, WithSkipHostDiscovery(true))
		if adapter.skipHostDiscovery != true {
			t.Error("expected skip host discovery enabled")
		}
	})

	t.Run("WithCommonPorts", func(t *testing.T) {
		adapter := NewNmapAdapter([]string{"192.168.1.1"}, WithCommonPorts())
		if adapter.portRange == "" {
			t.Error("expected port range to be set")
		}
	})

	t.Run("WithTopPorts", func(t *testing.T) {
		adapter := NewNmapAdapter([]string{"192.168.1.1"}, WithTopPorts(10))
		if adapter.portRange == "" {
			t.Error("expected port range to be set for top 10 ports")
		}
	})

	t.Run("WithFastScan", func(t *testing.T) {
		adapter := NewNmapAdapter([]string{"192.168.1.1"}, WithFastScan())
		if adapter.serviceDetection != false {
			t.Error("expected service detection disabled in fast scan")
		}
		if adapter.timeout != 5*time.Minute {
			t.Errorf("expected timeout 5m in fast scan, got %v", adapter.timeout)
		}
	})

	t.Run("WithAggressiveScan", func(t *testing.T) {
		adapter := NewNmapAdapter([]string{"192.168.1.1"}, WithAggressiveScan())
		if adapter.serviceDetection != true {
			t.Error("expected service detection enabled in aggressive scan")
		}
		if adapter.osDetection != true {
			t.Error("expected OS detection enabled in aggressive scan")
		}
		if adapter.portRange != "1-65535" {
			t.Errorf("expected full port range in aggressive scan, got %s", adapter.portRange)
		}
	})
}

// TestNmapAdapter_Interface tests adapter interface implementation
func TestNmapAdapter_Interface(t *testing.T) {
	adapter := NewNmapAdapter([]string{"192.168.1.1"})

	if adapter.Name() != "nmap" {
		t.Errorf("expected name 'nmap', got %s", adapter.Name())
	}

	if adapter.Type() != AdapterTypePolling {
		t.Errorf("expected type polling, got %s", adapter.Type())
	}

	if adapter.Priority() != 80 {
		t.Errorf("expected priority 80, got %d", adapter.Priority())
	}
}

// TestNmapAdapter_ParseResults tests parsing of mock nmap results
func TestNmapAdapter_ParseResults(t *testing.T) {
	adapter := NewNmapAdapter([]string{"192.168.1.1"})

	// Create mock nmap result
	mockResult := &nmap.Run{
		Hosts: []nmap.Host{
			{
				Addresses: []nmap.Address{
					{Addr: "192.168.1.100", AddrType: "ipv4"},
					{Addr: "AA:BB:CC:DD:EE:FF", AddrType: "mac", Vendor: "Test Vendor"},
				},
				Hostnames: []nmap.Hostname{
					{Name: "testhost.local"},
				},
				Status: nmap.Status{State: "up"},
				Ports: []nmap.Port{
					{
						ID:       22,
						Protocol: "tcp",
						State:    nmap.State{State: "open"},
						Service: nmap.Service{
							Name:    "ssh",
							Product: "OpenSSH",
							Version: "8.9p1",
						},
					},
					{
						ID:       80,
						Protocol: "tcp",
						State:    nmap.State{State: "open"},
						Service: nmap.Service{
							Name:    "http",
							Product: "nginx",
							Version: "1.18.0",
						},
					},
					{
						ID:       443,
						Protocol: "tcp",
						State:    nmap.State{State: "closed"},
						Service:  nmap.Service{},
					},
				},
			},
		},
	}

	fragment := domain.NewGraphFragment()
	err := adapter.processResults(mockResult, fragment)
	if err != nil {
		t.Fatalf("processResults failed: %v", err)
	}

	if len(fragment.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(fragment.Nodes))
	}

	node := fragment.Nodes[0]

	// Check node ID
	expectedID := "192-168-1-100"
	if node.ID != expectedID {
		t.Errorf("expected node ID %s, got %s", expectedID, node.ID)
	}

	// Check label (should use hostname)
	if node.Label != "testhost" {
		t.Errorf("expected label 'testhost', got %s", node.Label)
	}

	// Check IP property
	ip := node.GetPropertyString("ip")
	if ip != "192.168.1.100" {
		t.Errorf("expected IP 192.168.1.100, got %s", ip)
	}

	// Check discovered MAC address
	mac, ok := node.GetDiscovered("mac_address")
	if !ok {
		t.Error("expected mac_address in discovered")
	}
	if mac != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("expected MAC AA:BB:CC:DD:EE:FF, got %v", mac)
	}

	// Check discovered MAC vendor
	vendor, ok := node.GetDiscovered("mac_vendor")
	if !ok {
		t.Error("expected mac_vendor in discovered")
	}
	if vendor != "Test Vendor" {
		t.Errorf("expected vendor 'Test Vendor', got %v", vendor)
	}

	// Check open ports (should only include open ports, not closed)
	openPorts, ok := node.GetDiscovered("open_ports")
	if !ok {
		t.Error("expected open_ports in discovered")
	}
	ports, ok := openPorts.([]int)
	if !ok {
		t.Errorf("expected []int for open_ports, got %T", openPorts)
	}
	if len(ports) != 2 {
		t.Errorf("expected 2 open ports, got %d", len(ports))
	}

	// Check services
	services, ok := node.GetDiscovered("services")
	if !ok {
		t.Error("expected services in discovered")
	}
	portDetails, ok := services.([]PortInfo)
	if !ok {
		t.Errorf("expected []PortInfo for services, got %T", services)
	}
	if len(portDetails) != 2 {
		t.Errorf("expected 2 services, got %d", len(portDetails))
	}

	// Verify SSH service
	sshFound := false
	for _, svc := range portDetails {
		if svc.Port == 22 && svc.Service == "ssh" {
			sshFound = true
			if svc.Banner != "OpenSSH 8.9p1" {
				t.Errorf("expected SSH banner 'OpenSSH 8.9p1', got %s", svc.Banner)
			}
		}
	}
	if !sshFound {
		t.Error("SSH service not found in port details")
	}
}

// TestNmapAdapter_EvidenceGeneration tests evidence creation
func TestNmapAdapter_EvidenceGeneration(t *testing.T) {
	adapter := NewNmapAdapter([]string{"192.168.1.1"})
	now := time.Now()

	ports := []nmap.Port{
		{
			ID:       22,
			Protocol: "tcp",
			State:    nmap.State{State: "open"},
			Service: nmap.Service{
				Name:    "ssh",
				Product: "OpenSSH",
				Version: "8.9p1",
			},
		},
		{
			ID:       80,
			Protocol: "tcp",
			State:    nmap.State{State: "closed"},
		},
	}

	evidence := adapter.createEvidenceFromPorts(ports, now)

	// Should have 3 pieces of evidence for port 22:
	// 1. Port open
	// 2. Service name
	// 3. Product/version
	// Port 80 is closed, so no evidence
	if len(evidence) != 3 {
		t.Errorf("expected 3 evidence entries, got %d", len(evidence))
	}

	// Check port scan evidence
	portEvidence := evidence[0]
	if portEvidence.Source != domain.EvidenceSourcePortScan {
		t.Errorf("expected source port_scan, got %s", portEvidence.Source)
	}
	if portEvidence.Property != "service:22" {
		t.Errorf("expected property service:22, got %s", portEvidence.Property)
	}
	if portEvidence.Confidence != 0.5 {
		t.Errorf("expected confidence 0.5, got %f", portEvidence.Confidence)
	}

	// Check service name evidence
	serviceEvidence := evidence[1]
	if serviceEvidence.Source != domain.EvidenceSourceBanner {
		t.Errorf("expected source banner, got %s", serviceEvidence.Source)
	}
	if serviceEvidence.Property != "service:22:name" {
		t.Errorf("expected property service:22:name, got %s", serviceEvidence.Property)
	}
	if serviceEvidence.Value != "ssh" {
		t.Errorf("expected value ssh, got %v", serviceEvidence.Value)
	}
	if serviceEvidence.Confidence != 0.7 {
		t.Errorf("expected confidence 0.7, got %f", serviceEvidence.Confidence)
	}

	// Check version evidence
	versionEvidence := evidence[2]
	if versionEvidence.Source != domain.EvidenceSourceBanner {
		t.Errorf("expected source banner, got %s", versionEvidence.Source)
	}
	if versionEvidence.Property != "service:22:product" {
		t.Errorf("expected property service:22:product, got %s", versionEvidence.Property)
	}
	if versionEvidence.Confidence != 0.8 {
		t.Errorf("expected confidence 0.8, got %f", versionEvidence.Confidence)
	}
}

// TestNmapAdapter_NodeTypeInference tests node type detection
func TestNmapAdapter_NodeTypeInference(t *testing.T) {
	tests := []struct {
		name     string
		ports    []nmap.Port
		wantType domain.NodeType
	}{
		{
			name: "router (DNS + HTTP)",
			ports: []nmap.Port{
				{ID: 53, State: nmap.State{State: "open"}},
				{ID: 80, State: nmap.State{State: "open"}},
			},
			wantType: domain.NodeTypeRouter,
		},
		{
			name: "kubernetes node",
			ports: []nmap.Port{
				{ID: 6443, State: nmap.State{State: "open"}},
				{ID: 22, State: nmap.State{State: "open"}},
			},
			wantType: domain.NodeTypeServer,
		},
		{
			name: "windows server (RDP)",
			ports: []nmap.Port{
				{ID: 3389, State: nmap.State{State: "open"}},
				{ID: 445, State: nmap.State{State: "open"}},
			},
			wantType: domain.NodeTypeServer,
		},
		{
			name: "linux server (SSH + HTTP)",
			ports: []nmap.Port{
				{ID: 22, State: nmap.State{State: "open"}},
				{ID: 80, State: nmap.State{State: "open"}},
			},
			wantType: domain.NodeTypeServer,
		},
		{
			name: "ssh only",
			ports: []nmap.Port{
				{ID: 22, State: nmap.State{State: "open"}},
			},
			wantType: domain.NodeTypeServer,
		},
		{
			name: "web server only",
			ports: []nmap.Port{
				{ID: 443, State: nmap.State{State: "open"}},
			},
			wantType: domain.NodeTypeServer,
		},
		{
			name:     "unknown",
			ports:    []nmap.Port{},
			wantType: domain.NodeTypeUnknown,
		},
	}

	adapter := NewNmapAdapter([]string{"192.168.1.1"})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeType := adapter.inferNodeType(tt.ports)
			if nodeType != tt.wantType {
				t.Errorf("expected type %s, got %s", tt.wantType, nodeType)
			}
		})
	}
}

// TestParsePorts tests port range validation
func TestParsePorts(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{"single port", "80", false},
		{"multiple ports", "80,443,8080", false},
		{"port range", "1-1000", false},
		{"mixed format", "22,80-443,8080", false},
		{"with spaces", "22, 80, 443", false},
		{"invalid range", "80-", true},
		{"invalid port", "99999", true},
		{"invalid format", "abc", true},
		{"negative port", "-1", true},
		{"reversed range", "443-80", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parsePorts(tt.input)
			if (err != nil) != tt.wantError {
				t.Errorf("parsePorts(%s) error = %v, wantError %v", tt.input, err, tt.wantError)
			}
		})
	}
}

// TestExpandTargets tests CIDR expansion
func TestExpandTargets(t *testing.T) {
	tests := []struct {
		name      string
		input     []string
		wantCount int
		wantError bool
	}{
		{
			name:      "single IP",
			input:     []string{"192.168.1.1"},
			wantCount: 1,
			wantError: false,
		},
		{
			name:      "CIDR notation",
			input:     []string{"192.168.1.0/30"},
			wantCount: 1,
			wantError: false,
		},
		{
			name:      "multiple targets",
			input:     []string{"192.168.1.1", "10.0.0.0/28"},
			wantCount: 2,
			wantError: false,
		},
		{
			name:      "invalid CIDR",
			input:     []string{"192.168.1.0/99"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := expandTargets(tt.input)
			if (err != nil) != tt.wantError {
				t.Errorf("expandTargets() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if !tt.wantError && len(result) != tt.wantCount {
				t.Errorf("expandTargets() got %d targets, want %d", len(result), tt.wantCount)
			}
		})
	}
}

// TestSanitizeIP tests IP sanitization
func TestSanitizeIP(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"IPv4", "192.168.1.1", "192-168-1-1"},
		{"IPv4 with zeros", "10.0.0.1", "10-0-0-1"},
		{"malformed IP passthrough", "test-host", "test-host"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeIP(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeIP(%s) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

// TestNmapAdapter_StartStop tests lifecycle methods
func TestNmapAdapter_StartStop(t *testing.T) {
	adapter := NewNmapAdapter([]string{"192.168.1.1"})

	// Note: Start will fail if nmap is not installed, which is expected in CI
	ctx := context.Background()
	err := adapter.Start(ctx)
	if err != nil {
		t.Logf("Start failed (expected if nmap not installed): %v", err)
		// Don't fail the test if nmap is not available
		return
	}

	if !adapter.running {
		t.Error("expected adapter to be running after Start")
	}

	err = adapter.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	if adapter.running {
		t.Error("expected adapter to not be running after Stop")
	}
}

// TestNmapAdapter_Sync tests the sync operation (without actual nmap)
func TestNmapAdapter_Sync(t *testing.T) {
	adapter := NewNmapAdapter([]string{})

	ctx := context.Background()

	// Start without nmap check (will fail if we try to scan)
	adapter.running = true

	// Sync with no targets should return nil fragment
	fragment, err := adapter.Sync(ctx)
	if err != nil {
		t.Errorf("Sync with no targets failed: %v", err)
	}
	if fragment != nil {
		t.Error("expected nil fragment for no targets")
	}
}

// TestNmapAdapter_OSDetection tests OS detection parsing
func TestNmapAdapter_OSDetection(t *testing.T) {
	adapter := NewNmapAdapter([]string{"192.168.1.1"})

	osData := nmap.OS{
		Matches: []nmap.OSMatch{
			{
				Name:     "Linux 5.4",
				Accuracy: 95,
				Classes: []nmap.OSClass{
					{
						Type:   "general purpose",
						Vendor: "Linux",
						Family: "Linux",
					},
				},
			},
		},
	}

	osInfo := adapter.extractOSInfo(osData)

	if osInfo == nil {
		t.Fatal("expected OS info, got nil")
	}

	if osInfo["name"] != "Linux 5.4" {
		t.Errorf("expected OS name 'Linux 5.4', got %v", osInfo["name"])
	}

	if osInfo["accuracy"] != 95 {
		t.Errorf("expected accuracy 95, got %v", osInfo["accuracy"])
	}

	if osInfo["family"] != "Linux" {
		t.Errorf("expected family 'Linux', got %v", osInfo["family"])
	}
}
