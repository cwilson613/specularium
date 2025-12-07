package loader

import (
	"fmt"
	"os"
	"strings"

	"netdiagram/internal/domain"

	"gopkg.in/yaml.v3"
)

// InfrastructureYAML represents the YAML file structure
type InfrastructureYAML struct {
	Version  string                    `yaml:"version"`
	Metadata *MetadataYAML             `yaml:"metadata,omitempty"`
	Groups   map[string]*HostGroupYAML `yaml:"groups,omitempty"`
	Hosts    map[string]*HostYAML      `yaml:"hosts"`
}

// MetadataYAML represents the metadata section
type MetadataYAML struct {
	Network *struct {
		CIDR    string `yaml:"cidr"`
		Gateway string `yaml:"gateway"`
		Domains *struct {
			Internal string `yaml:"internal"`
			External string `yaml:"external"`
		} `yaml:"domains"`
	} `yaml:"network,omitempty"`
	Description string `yaml:"description,omitempty"`
	LastUpdated string `yaml:"last_updated,omitempty"`
}

// HostGroupYAML represents a host group
type HostGroupYAML struct {
	Members     []string               `yaml:"members"`
	Description string                 `yaml:"description,omitempty"`
	Vars        map[string]interface{} `yaml:"vars,omitempty"`
}

// HostYAML represents a host in YAML format
type HostYAML struct {
	IP             string        `yaml:"ip"`
	Role           string        `yaml:"role"`
	Platform       string        `yaml:"platform"`
	Version        string        `yaml:"version,omitempty"`
	Description    string        `yaml:"description,omitempty"`
	Classification string        `yaml:"classification,omitempty"`
	Hardware       *HardwareYAML `yaml:"hardware,omitempty"`
	Network        *NetworkYAML  `yaml:"network,omitempty"`
	VMs            []VMYAML      `yaml:"vms,omitempty"`
}

// HardwareYAML represents hardware specs
type HardwareYAML struct {
	CPU          string `yaml:"cpu,omitempty"`
	RAMGB        int    `yaml:"ram_gb,omitempty"`
	GPU          string `yaml:"gpu,omitempty"`
	StorageGB    int    `yaml:"storage_gb,omitempty"`
	NetworkPorts int    `yaml:"network_ports,omitempty"`
	FormFactor   string `yaml:"form_factor,omitempty"`
}

// NetworkYAML represents network configuration
type NetworkYAML struct {
	Ports      []PortYAML `yaml:"ports,omitempty"`
	MACAddress string     `yaml:"mac_address,omitempty"`
}

// PortYAML represents a network port
type PortYAML struct {
	Name        string `yaml:"name"`
	SpeedGbps   int    `yaml:"speed_gbps,omitempty"`
	ConnectedTo string `yaml:"connected_to,omitempty"`
	Type        string `yaml:"type,omitempty"`
}

// VMYAML represents a virtual machine
type VMYAML struct {
	Name  string `yaml:"name"`
	IP    string `yaml:"ip"`
	Role  string `yaml:"role"`
	VCPU  int    `yaml:"vcpu,omitempty"`
	RAMGB int    `yaml:"ram_gb,omitempty"`
	OS    string `yaml:"os,omitempty"`
}

// LoadYAML loads infrastructure from a YAML file
func LoadYAML(path string) (*domain.Infrastructure, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return ParseYAML(data)
}

// ParseYAML parses infrastructure from YAML bytes
func ParseYAML(data []byte) (*domain.Infrastructure, error) {
	var yamlData InfrastructureYAML
	if err := yaml.Unmarshal(data, &yamlData); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return convertYAMLToInfrastructure(&yamlData)
}

func convertYAMLToInfrastructure(y *InfrastructureYAML) (*domain.Infrastructure, error) {
	infra := domain.NewInfrastructure()
	infra.Version = y.Version

	// Convert metadata
	if y.Metadata != nil {
		infra.Metadata = &domain.Metadata{
			Description: y.Metadata.Description,
		}
		if y.Metadata.Network != nil {
			infra.Metadata.Network = &domain.NetworkMetadata{
				CIDR:    y.Metadata.Network.CIDR,
				Gateway: y.Metadata.Network.Gateway,
			}
			if y.Metadata.Network.Domains != nil {
				infra.Metadata.Network.Domains = &domain.DomainMetadata{
					Internal: y.Metadata.Network.Domains.Internal,
					External: y.Metadata.Network.Domains.External,
				}
			}
		}
	}

	// Convert groups
	for name, g := range y.Groups {
		infra.Groups[name] = &domain.HostGroup{
			Members:     g.Members,
			Description: g.Description,
			Vars:        g.Vars,
		}
	}

	// Convert hosts
	for id, h := range y.Hosts {
		host := &domain.Host{
			ID:          id,
			IP:          h.IP,
			Role:        h.Role,
			Platform:    h.Platform,
			Version:     h.Version,
			Description: h.Description,
		}

		// Set classification
		if h.Classification != "" {
			host.Classification = domain.Classification(h.Classification)
		}

		// Convert hardware
		if h.Hardware != nil {
			host.Hardware = &domain.HardwareSpec{
				CPU:          h.Hardware.CPU,
				RAMGB:        h.Hardware.RAMGB,
				GPU:          h.Hardware.GPU,
				StorageGB:    h.Hardware.StorageGB,
				NetworkPorts: h.Hardware.NetworkPorts,
				FormFactor:   h.Hardware.FormFactor,
			}
		}

		// Convert network
		if h.Network != nil {
			host.Network = &domain.NetworkSpec{
				MACAddress: h.Network.MACAddress,
			}
			for _, p := range h.Network.Ports {
				port := domain.Port{
					Name:        p.Name,
					SpeedGbps:   p.SpeedGbps,
					ConnectedTo: p.ConnectedTo,
					Type:        p.Type,
				}
				if port.SpeedGbps == 0 {
					port.SpeedGbps = 1 // Default to 1GbE
				}
				host.Network.Ports = append(host.Network.Ports, port)
			}
		}

		// Convert VMs
		for _, v := range h.VMs {
			vm := domain.VM{
				Name:  v.Name,
				IP:    v.IP,
				Role:  v.Role,
				VCPU:  v.VCPU,
				RAMGB: v.RAMGB,
				OS:    v.OS,
			}
			host.VMs = append(host.VMs, vm)
		}

		infra.Hosts[id] = host
	}

	// Build group memberships on hosts
	infra.BuildGroupMembership()

	// Extract connections from port definitions
	infra.ExtractConnections()

	// Infer connections for hosts without explicit port connections
	inferConnections(infra)

	return infra, nil
}

// inferConnections creates connections for hosts that don't have explicit port connections
// based on their group membership (similar to the TypeScript version)
func inferConnections(infra *domain.Infrastructure) {
	// Find router/gateway device
	var routerID string
	for id, host := range infra.Hosts {
		if strings.Contains(strings.ToLower(host.Role), "router") ||
			strings.Contains(strings.ToLower(host.Role), "gateway") {
			routerID = id
			break
		}
	}

	if routerID == "" {
		return // No router found, skip inference
	}

	// Track which hosts already have connections
	connected := make(map[string]bool)
	for _, conn := range infra.Connections {
		connected[conn.SourceID] = true
		connected[conn.TargetID] = true
	}

	// Connect unconnected hosts to router based on groups
	for id, host := range infra.Hosts {
		if connected[id] || id == routerID {
			continue
		}

		speed := 1 // Default 1GbE
		if host.Network != nil && len(host.Network.Ports) > 0 {
			speed = host.Network.Ports[0].SpeedGbps
		}

		conn := &domain.Connection{
			SourceID:  id,
			TargetID:  routerID,
			SpeedGbps: speed,
		}
		infra.AddConnection(conn)
	}
}

// ExportYAML exports infrastructure to YAML format
func ExportYAML(infra *domain.Infrastructure) ([]byte, error) {
	yamlData := &InfrastructureYAML{
		Version: infra.Version,
		Groups:  make(map[string]*HostGroupYAML),
		Hosts:   make(map[string]*HostYAML),
	}

	// Export metadata
	if infra.Metadata != nil {
		yamlData.Metadata = &MetadataYAML{
			Description: infra.Metadata.Description,
		}
		if infra.Metadata.Network != nil {
			yamlData.Metadata.Network = &struct {
				CIDR    string `yaml:"cidr"`
				Gateway string `yaml:"gateway"`
				Domains *struct {
					Internal string `yaml:"internal"`
					External string `yaml:"external"`
				} `yaml:"domains"`
			}{
				CIDR:    infra.Metadata.Network.CIDR,
				Gateway: infra.Metadata.Network.Gateway,
			}
			if infra.Metadata.Network.Domains != nil {
				yamlData.Metadata.Network.Domains = &struct {
					Internal string `yaml:"internal"`
					External string `yaml:"external"`
				}{
					Internal: infra.Metadata.Network.Domains.Internal,
					External: infra.Metadata.Network.Domains.External,
				}
			}
		}
	}

	// Export groups
	for name, g := range infra.Groups {
		yamlData.Groups[name] = &HostGroupYAML{
			Members:     g.Members,
			Description: g.Description,
			Vars:        g.Vars,
		}
	}

	// Export hosts
	for id, host := range infra.Hosts {
		h := &HostYAML{
			IP:          host.IP,
			Role:        host.Role,
			Platform:    host.Platform,
			Version:     host.Version,
			Description: host.Description,
		}

		if host.Classification != "" {
			h.Classification = string(host.Classification)
		}

		if host.Hardware != nil {
			h.Hardware = &HardwareYAML{
				CPU:          host.Hardware.CPU,
				RAMGB:        host.Hardware.RAMGB,
				GPU:          host.Hardware.GPU,
				StorageGB:    host.Hardware.StorageGB,
				NetworkPorts: host.Hardware.NetworkPorts,
				FormFactor:   host.Hardware.FormFactor,
			}
		}

		if host.Network != nil {
			h.Network = &NetworkYAML{
				MACAddress: host.Network.MACAddress,
			}
			for _, p := range host.Network.Ports {
				h.Network.Ports = append(h.Network.Ports, PortYAML{
					Name:        p.Name,
					SpeedGbps:   p.SpeedGbps,
					ConnectedTo: p.ConnectedTo,
					Type:        p.Type,
				})
			}
		}

		for _, vm := range host.VMs {
			h.VMs = append(h.VMs, VMYAML{
				Name:  vm.Name,
				IP:    vm.IP,
				Role:  vm.Role,
				VCPU:  vm.VCPU,
				RAMGB: vm.RAMGB,
				OS:    vm.OS,
			})
		}

		yamlData.Hosts[id] = h
	}

	return yaml.Marshal(yamlData)
}
