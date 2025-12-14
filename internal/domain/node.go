package domain

import (
	"strings"
	"time"
)

// NodeType represents the type of network node
type NodeType string

const (
	NodeTypeServer      NodeType = "server"
	NodeTypeSwitch      NodeType = "switch"
	NodeTypeRouter      NodeType = "router"
	NodeTypeAccessPoint NodeType = "access_point"
	NodeTypeVM          NodeType = "vm"
	NodeTypeVIP         NodeType = "vip"
	NodeTypeContainer   NodeType = "container"
	NodeTypeInterface   NodeType = "interface" // Network interface, USB port, radio, etc. (child of parent node)
	NodeTypeSelf        NodeType = "self"      // This Specularium instance
	NodeTypeUnknown     NodeType = "unknown"
)

// NodeStatus represents the verification status of a node
type NodeStatus string

const (
	NodeStatusUnverified  NodeStatus = "unverified"  // Imported but not yet checked
	NodeStatusVerifying   NodeStatus = "verifying"   // Currently being probed
	NodeStatusVerified    NodeStatus = "verified"    // Successfully contacted
	NodeStatusUnreachable NodeStatus = "unreachable" // Failed to contact
	NodeStatusDegraded    NodeStatus = "degraded"    // Partially reachable (some probes failed)
)

// Node represents a network entity in the graph
type Node struct {
	ID         string         `json:"id"`
	Type       NodeType       `json:"type"`
	Label      string         `json:"label"`
	ParentID   string         `json:"parent_id,omitempty"` // Parent node ID for interface/satellite nodes
	Properties map[string]any `json:"properties,omitempty"`
	Source     string         `json:"source,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`

	// Verification fields
	Status       NodeStatus `json:"status"`
	LastVerified *time.Time `json:"last_verified,omitempty"`
	LastSeen     *time.Time `json:"last_seen,omitempty"`

	// Discovered properties (auto-populated by adapters)
	Discovered map[string]any `json:"discovered,omitempty"`

	// Operator Truth fields
	Truth          *NodeTruth  `json:"truth,omitempty"`
	TruthStatus    TruthStatus `json:"truth_status,omitempty"`
	HasDiscrepancy bool        `json:"has_discrepancy,omitempty"`

	// Capabilities detected for this node (K8s, Docker, SSH, etc.)
	Capabilities map[CapabilityType]*Capability `json:"capabilities,omitempty"`
}

// IsInterface returns true if this node is a child interface node
func (n *Node) IsInterface() bool {
	return n.ParentID != ""
}

// NewNode creates a new node with initialized properties
func NewNode(id string, nodeType NodeType, label string) *Node {
	now := time.Now()
	return &Node{
		ID:         id,
		Type:       nodeType,
		Label:      label,
		Properties: make(map[string]any),
		Discovered: make(map[string]any),
		Status:     NodeStatusUnverified,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// SetDiscovered sets a discovered property value
func (n *Node) SetDiscovered(key string, value any) {
	if n.Discovered == nil {
		n.Discovered = make(map[string]any)
	}
	n.Discovered[key] = value
}

// GetDiscovered gets a discovered property value
func (n *Node) GetDiscovered(key string) (any, bool) {
	if n.Discovered == nil {
		return nil, false
	}
	val, ok := n.Discovered[key]
	return val, ok
}

// SetProperty sets a property value
func (n *Node) SetProperty(key string, value any) {
	if n.Properties == nil {
		n.Properties = make(map[string]any)
	}
	n.Properties[key] = value
}

// GetProperty gets a property value
func (n *Node) GetProperty(key string) (any, bool) {
	if n.Properties == nil {
		return nil, false
	}
	val, ok := n.Properties[key]
	return val, ok
}

// GetPropertyString gets a property as a string
func (n *Node) GetPropertyString(key string) string {
	val, ok := n.GetProperty(key)
	if !ok {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	return ""
}

// ConfidenceSource identifies where a discovered value came from
type ConfidenceSource string

const (
	SourceOperatorTruth ConfidenceSource = "operator_truth" // Explicitly set by operator
	SourcePTR           ConfidenceSource = "ptr"            // Reverse DNS lookup
	SourceForwardDNS    ConfidenceSource = "forward_dns"    // A record match
	SourceSMTPBanner    ConfidenceSource = "smtp_banner"    // SMTP EHLO/HELO hostname
	SourceSSHBanner     ConfidenceSource = "ssh_banner"     // SSH server identification
	SourceHTTPHeader    ConfidenceSource = "http_header"    // HTTP Server header
	SourceMDNS          ConfidenceSource = "mdns"           // Multicast DNS
	SourceNetBIOS       ConfidenceSource = "netbios"        // NetBIOS name
	SourceSNMP          ConfidenceSource = "snmp"           // SNMP sysName
	SourceIPDerived     ConfidenceSource = "ip_derived"     // Derived from IP address
	SourceImport        ConfidenceSource = "import"         // Imported from inventory
	SourceUnknown       ConfidenceSource = "unknown"        // Unknown source
)

// ConfidenceScores maps sources to their default confidence values
var ConfidenceScores = map[ConfidenceSource]float64{
	SourceOperatorTruth: 1.0,  // Operator truth is absolute
	SourcePTR:           0.95, // Authoritative DNS
	SourceForwardDNS:    0.90, // A record verification
	SourceSMTPBanner:    0.85, // Server self-identification
	SourceSNMP:          0.85, // SNMP is usually reliable
	SourceMDNS:          0.80, // Local discovery
	SourceNetBIOS:       0.75, // Windows naming
	SourceSSHBanner:     0.70, // Often contains hints
	SourceHTTPHeader:    0.60, // Sometimes hostname in headers
	SourceImport:        0.50, // Imported data, unverified
	SourceIPDerived:     0.10, // Just the IP, placeholder
	SourceUnknown:       0.05, // Unknown origin
}

// DiscoveredValue represents a value discovered with confidence metadata
type DiscoveredValue struct {
	Value      string           `json:"value"`
	Confidence float64          `json:"confidence"` // 0.0 - 1.0
	Source     ConfidenceSource `json:"source"`
	ObservedAt time.Time        `json:"observed_at"`
}

// HostnameCandidate represents a potential hostname with confidence
type HostnameCandidate struct {
	Hostname   string           `json:"hostname"`
	Confidence float64          `json:"confidence"`
	Source     ConfidenceSource `json:"source"`
	ObservedAt time.Time        `json:"observed_at"`
}

// HostnameInference holds all hostname candidates and the best selection
type HostnameInference struct {
	Candidates []HostnameCandidate `json:"candidates,omitempty"`
	Best       *HostnameCandidate  `json:"best,omitempty"`
}

// AddCandidate adds a hostname candidate and updates the best selection
func (h *HostnameInference) AddCandidate(hostname string, source ConfidenceSource, observedAt time.Time) {
	if hostname == "" {
		return
	}

	// Clean hostname
	hostname = strings.TrimSpace(hostname)
	hostname = strings.ToLower(hostname)

	// Get confidence for this source
	confidence := ConfidenceScores[source]
	if confidence == 0 {
		confidence = ConfidenceScores[SourceUnknown]
	}

	candidate := HostnameCandidate{
		Hostname:   hostname,
		Confidence: confidence,
		Source:     source,
		ObservedAt: observedAt,
	}

	// Check if we already have this hostname from this source
	for i, existing := range h.Candidates {
		if existing.Hostname == hostname && existing.Source == source {
			// Update existing
			h.Candidates[i] = candidate
			h.updateBest()
			return
		}
	}

	// Add new candidate
	h.Candidates = append(h.Candidates, candidate)
	h.updateBest()
}

// updateBest selects the highest confidence candidate as best
func (h *HostnameInference) updateBest() {
	if len(h.Candidates) == 0 {
		h.Best = nil
		return
	}

	best := &h.Candidates[0]
	for i := 1; i < len(h.Candidates); i++ {
		if h.Candidates[i].Confidence > best.Confidence {
			best = &h.Candidates[i]
		}
	}
	h.Best = best
}

// GetBestHostname returns the highest confidence hostname, or empty string
func (h *HostnameInference) GetBestHostname() string {
	if h.Best == nil {
		return ""
	}
	return h.Best.Hostname
}

// GetBestConfidence returns the confidence of the best hostname
func (h *HostnameInference) GetBestConfidence() float64 {
	if h.Best == nil {
		return 0
	}
	return h.Best.Confidence
}

// ExtractShortName extracts the short hostname (without domain) from an FQDN
func ExtractShortName(fqdn string) string {
	if fqdn == "" {
		return ""
	}
	parts := strings.Split(fqdn, ".")
	if len(parts) > 0 && len(parts[0]) > 1 {
		return parts[0]
	}
	return fqdn
}

// AddEvidence adds evidence to a capability and creates the capability if needed
func (n *Node) AddEvidence(capType CapabilityType, evidence Evidence) {
	if n.Capabilities == nil {
		n.Capabilities = make(map[CapabilityType]*Capability)
	}

	cap, exists := n.Capabilities[capType]
	if !exists {
		cap = &Capability{
			Type:       capType,
			Properties: make(map[string]any),
		}
		n.Capabilities[capType] = cap
	}

	cap.AddEvidence(evidence)
}

// GetCapability returns the capability for the given type, or nil if not found
func (n *Node) GetCapability(capType CapabilityType) *Capability {
	if n.Capabilities == nil {
		return nil
	}
	return n.Capabilities[capType]
}

// GetConfidence returns the aggregate confidence for a capability, or 0 if not found
func (n *Node) GetConfidence(capType CapabilityType) float64 {
	cap := n.GetCapability(capType)
	if cap == nil {
		return 0
	}
	return cap.Confidence
}

// HasCapability checks if the node has the given capability above the minimum confidence threshold
func (n *Node) HasCapability(capType CapabilityType, minConfidence float64) bool {
	return n.GetConfidence(capType) >= minConfidence
}
