package domain

// EnvironmentInfo holds detected runtime environment details.
// Used by bootstrap to record self-discovery findings and by API
// to report environment to clients.
type EnvironmentInfo struct {
	// Deployment context
	InKubernetes   bool   `json:"in_kubernetes"`
	InDocker       bool   `json:"in_docker"`
	Hostname       string `json:"hostname"`
	PodName        string `json:"pod_name,omitempty"`
	PodNamespace   string `json:"pod_namespace,omitempty"`
	PodIP          string `json:"pod_ip,omitempty"`
	NodeName       string `json:"node_name,omitempty"`
	ServiceAccount string `json:"service_account,omitempty"`

	// Network context
	DefaultGateway string   `json:"default_gateway,omitempty"`
	DNSServers     []string `json:"dns_servers,omitempty"`
	SearchDomains  []string `json:"search_domains,omitempty"`
	LocalSubnet    string   `json:"local_subnet,omitempty"`

	// K8s cluster info (if applicable)
	ClusterDNS      string `json:"cluster_dns,omitempty"`
	KubernetesAPIIP string `json:"kubernetes_api_ip,omitempty"`

	// Operator-configured scan targets (from SCAN_SUBNETS env var)
	ConfiguredSubnets []string `json:"configured_subnets,omitempty"`
}

// ScanTargets contains categorized network scan targets
type ScanTargets struct {
	// Primary targets - operator-configured or detected from environment
	Primary []string `json:"primary"`
	// Discovery targets - RFC1918 ranges for network discovery mode
	Discovery []string `json:"discovery"`
}
