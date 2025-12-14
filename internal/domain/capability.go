package domain

import (
	"time"
)

// CapabilityType identifies a class of capability a node might have
type CapabilityType string

const (
	CapabilityKubernetes CapabilityType = "kubernetes"
	CapabilityDocker     CapabilityType = "docker"
	CapabilitySSH        CapabilityType = "ssh"
	CapabilityHTTP       CapabilityType = "http"
	CapabilityDNS        CapabilityType = "dns"
	CapabilityDHCP       CapabilityType = "dhcp"
	CapabilitySMB        CapabilityType = "smb"
	CapabilityNFS        CapabilityType = "nfs"
)

// EvidenceSource identifies how evidence was gathered
type EvidenceSource string

const (
	EvidenceSourcePortScan     EvidenceSource = "port_scan"      // Open port detected
	EvidenceSourceBanner       EvidenceSource = "banner"         // Service banner grabbed
	EvidenceSourceSSHProbe     EvidenceSource = "ssh_probe"      // SSH session gathered facts
	EvidenceSourceK8sAPI       EvidenceSource = "k8s_api"        // Kubernetes API query
	EvidenceSourceDockerAPI    EvidenceSource = "docker_api"     // Docker API query
	EvidenceSourceProcessList  EvidenceSource = "process_list"   // Process enumeration via SSH
	EvidenceSourceFileSystem   EvidenceSource = "filesystem"     // File existence/content check
	EvidenceSourceDNS          EvidenceSource = "dns"            // DNS query result
	EvidenceSourceCorrelation  EvidenceSource = "correlation"    // Inferred from other evidence
	EvidenceSourceOperator     EvidenceSource = "operator"       // Operator assertion
)

// Evidence represents a single piece of discovered information
type Evidence struct {
	ID         string         `json:"id"`
	Source     EvidenceSource `json:"source"`
	Property   string         `json:"property"`    // What was discovered (e.g., "is_k8s_node", "k8s_role")
	Value      any            `json:"value"`       // The discovered value
	Confidence float64        `json:"confidence"`  // 0.0 - 1.0
	ObservedAt time.Time      `json:"observed_at"`
	ExpiresAt  *time.Time     `json:"expires_at,omitempty"` // When this evidence should be re-validated
	SecretRef  string         `json:"secret_ref,omitempty"` // Reference to secret that enabled discovery
	Raw        map[string]any `json:"raw,omitempty"`        // Raw data from probe (for debugging)
}

// EvidenceConfidence maps sources to base confidence levels
var EvidenceConfidence = map[EvidenceSource]float64{
	EvidenceSourceOperator:     1.0,  // Operator truth is authoritative
	EvidenceSourceK8sAPI:       0.95, // Authenticated API query
	EvidenceSourceDockerAPI:    0.95, // Authenticated API query
	EvidenceSourceSSHProbe:     0.90, // Direct system access
	EvidenceSourceProcessList:  0.85, // Process enumeration
	EvidenceSourceFileSystem:   0.85, // File checks
	EvidenceSourceBanner:       0.70, // Service self-identification
	EvidenceSourceDNS:          0.60, // DNS can be stale
	EvidenceSourcePortScan:     0.50, // Port open doesn't confirm service
	EvidenceSourceCorrelation:  0.40, // Inference from other data
}

// Capability represents a detected capability with supporting evidence
type Capability struct {
	Type       CapabilityType `json:"type"`
	Confidence float64        `json:"confidence"` // Aggregate confidence from evidence
	Status     string         `json:"status"`     // "speculative", "probable", "confirmed"
	Properties map[string]any `json:"properties,omitempty"` // Capability-specific properties
	Evidence   []Evidence     `json:"evidence,omitempty"`
	LastProbed *time.Time     `json:"last_probed,omitempty"`
}

// ConfidenceStatus returns a human-readable status based on confidence level
func (c *Capability) ConfidenceStatus() string {
	switch {
	case c.Confidence >= 0.7:
		return "confirmed"
	case c.Confidence >= 0.4:
		return "probable"
	default:
		return "speculative"
	}
}

// AddEvidence adds evidence and recalculates aggregate confidence
func (c *Capability) AddEvidence(e Evidence) {
	c.Evidence = append(c.Evidence, e)
	c.recalculateConfidence()
}

// recalculateConfidence computes aggregate confidence from all evidence
// Uses a "max + bonus" approach: highest evidence confidence + small bonus for corroborating evidence
func (c *Capability) recalculateConfidence() {
	if len(c.Evidence) == 0 {
		c.Confidence = 0
		c.Status = "speculative"
		return
	}

	// Find max confidence
	maxConf := 0.0
	for _, e := range c.Evidence {
		if e.Confidence > maxConf {
			maxConf = e.Confidence
		}
	}

	// Add small bonus for corroborating evidence (diminishing returns)
	bonus := 0.0
	for _, e := range c.Evidence {
		if e.Confidence < maxConf {
			// Each corroborating piece adds up to 5% of remaining gap to 1.0
			bonus += (1.0 - maxConf) * 0.05 * (e.Confidence / maxConf)
		}
	}

	c.Confidence = min(1.0, maxConf+bonus)
	c.Status = c.ConfidenceStatus()
}

// KubernetesCapability holds K8s-specific capability details
type KubernetesCapability struct {
	Role        string `json:"role,omitempty"`         // "control-plane", "worker", "unknown"
	ClusterName string `json:"cluster_name,omitempty"` // Cluster identifier if known
	NodeName    string `json:"node_name,omitempty"`    // K8s node name
	Version     string `json:"version,omitempty"`      // K8s version
	Distribution string `json:"distribution,omitempty"` // "k3s", "k8s", "eks", etc.
}

// DockerCapability holds Docker-specific capability details
type DockerCapability struct {
	Version       string   `json:"version,omitempty"`
	ServerVersion string   `json:"server_version,omitempty"`
	Containers    int      `json:"containers,omitempty"`      // Number of containers
	ContainerIDs  []string `json:"container_ids,omitempty"`   // Known container IDs on this host
}

// SSHCapability holds SSH access details
type SSHCapability struct {
	Port        int    `json:"port,omitempty"`
	Version     string `json:"version,omitempty"`     // SSH server version
	AuthMethods []string `json:"auth_methods,omitempty"` // Available auth methods
	Accessible  bool   `json:"accessible"`             // Can we authenticate?
	SecretRef   string `json:"secret_ref,omitempty"`   // Secret used for access
}

// Relationship types for architectural connections
const (
	EdgeTypeHostedBy  EdgeType = "hosted_by"  // Service/VIP is hosted by a physical host
	EdgeTypeRunsOn    EdgeType = "runs_on"    // Container/pod runs on a node
	EdgeTypeBackedBy  EdgeType = "backed_by"  // VIP/LB is backed by hosts
	EdgeTypeMemberOf  EdgeType = "member_of"  // Node is member of a cluster/group
	EdgeTypeManages   EdgeType = "manages"    // Control plane manages workers
)

// Observation represents a point-in-time observation for the COP timeline
type Observation struct {
	ID         string         `json:"id"`
	EntityID   string         `json:"entity_id"`   // Node/Edge this observation is about
	Type       string         `json:"type"`        // "state_change", "new_entity", "evidence", "anomaly"
	Summary    string         `json:"summary"`     // Human-readable summary
	Details    map[string]any `json:"details,omitempty"`
	Source     string         `json:"source"`      // Adapter/probe that made the observation
	Severity   string         `json:"severity"`    // "info", "warning", "critical"
	ObservedAt time.Time      `json:"observed_at"`
}

// ObservationType constants
const (
	ObservationStateChange  = "state_change"  // Entity changed state (up/down, etc.)
	ObservationNewEntity    = "new_entity"    // New entity discovered
	ObservationEvidence     = "evidence"      // New evidence gathered
	ObservationAnomaly      = "anomaly"       // Something unexpected
	ObservationCorrelation  = "correlation"   // Entities were correlated
)

// min returns the smaller of two float64 values
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
