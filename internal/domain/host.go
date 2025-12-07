package domain

// Classification indicates how a host should be displayed
type Classification string

const (
	ClassificationMachine Classification = "machine"
	ClassificationDevice  Classification = "device"
	ClassificationVirtual Classification = "virtual"
)

// Host represents a network host (server, workstation, or network device)
type Host struct {
	ID             string         `json:"id" yaml:"id"`
	IP             string         `json:"ip" yaml:"ip"`
	Role           string         `json:"role" yaml:"role"`
	Platform       string         `json:"platform" yaml:"platform"`
	Version        string         `json:"version,omitempty" yaml:"version,omitempty"`
	Description    string         `json:"description,omitempty" yaml:"description,omitempty"`
	Classification Classification `json:"classification" yaml:"classification,omitempty"`
	Hardware       *HardwareSpec  `json:"hardware,omitempty" yaml:"hardware,omitempty"`
	Network        *NetworkSpec   `json:"network,omitempty" yaml:"network,omitempty"`
	VMs            []VM           `json:"vms,omitempty" yaml:"vms,omitempty"`
	Position       *Position      `json:"position,omitempty" yaml:"-"`
	Groups         []string       `json:"groups,omitempty" yaml:"-"`
}

// HardwareSpec describes the hardware configuration of a host
type HardwareSpec struct {
	CPU          string `json:"cpu,omitempty" yaml:"cpu,omitempty"`
	RAMGB        int    `json:"ram_gb,omitempty" yaml:"ram_gb,omitempty"`
	GPU          string `json:"gpu,omitempty" yaml:"gpu,omitempty"`
	StorageGB    int    `json:"storage_gb,omitempty" yaml:"storage_gb,omitempty"`
	NetworkPorts int    `json:"network_ports,omitempty" yaml:"network_ports,omitempty"`
	FormFactor   string `json:"form_factor,omitempty" yaml:"form_factor,omitempty"`
}

// NetworkSpec describes the network configuration of a host
type NetworkSpec struct {
	Ports      []Port `json:"ports,omitempty" yaml:"ports,omitempty"`
	MACAddress string `json:"mac_address,omitempty" yaml:"mac_address,omitempty"`
}

// Port represents a network port on a host
type Port struct {
	Name        string `json:"name" yaml:"name"`
	SpeedGbps   int    `json:"speed_gbps,omitempty" yaml:"speed_gbps,omitempty"`
	ConnectedTo string `json:"connected_to,omitempty" yaml:"connected_to,omitempty"`
	Type        string `json:"type,omitempty" yaml:"type,omitempty"` // physical, virtual, wifi
}

// VM represents a virtual machine running on a host
type VM struct {
	Name  string `json:"name" yaml:"name"`
	IP    string `json:"ip" yaml:"ip"`
	Role  string `json:"role" yaml:"role"`
	VCPU  int    `json:"vcpu,omitempty" yaml:"vcpu,omitempty"`
	RAMGB int    `json:"ram_gb,omitempty" yaml:"ram_gb,omitempty"`
	OS    string `json:"os,omitempty" yaml:"os,omitempty"`
}

// Position represents x,y coordinates for visualization
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// InferClassification determines classification from role if not explicitly set
func (h *Host) InferClassification() Classification {
	if h.Classification != "" {
		return h.Classification
	}

	// Check if it's a network device based on role
	switch h.Role {
	case "router", "switch", "access_point", "firewall", "controller":
		return ClassificationDevice
	}

	// Check platform hints
	switch h.Platform {
	case "routeros", "openwrt", "edgeos", "unifi":
		return ClassificationDevice
	}

	return ClassificationMachine
}
