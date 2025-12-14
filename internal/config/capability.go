package config

// CapabilityType distinguishes built-in from optional capabilities
type CapabilityType string

const (
	CapabilityTypeCore   CapabilityType = "core"   // Compiled in, always available
	CapabilityTypePlugin CapabilityType = "plugin" // Optional, may need external deps
)

// CapabilityConfig defines settings for a single capability
type CapabilityConfig struct {
	Enabled    bool    `yaml:"enabled"`
	MinMode    Mode    `yaml:"min_mode,omitempty"`    // Minimum mode required
	BinaryPath *string `yaml:"binary_path,omitempty"` // Path to external binary (plugins)
}

// CoreCapabilities defines the built-in capabilities
type CoreCapabilities struct {
	HTTPServer        CapabilityConfig `yaml:"http_server"`
	SSEEvents         CapabilityConfig `yaml:"sse_events"`
	ImportExport      CapabilityConfig `yaml:"import_export"`
	BasicVerification CapabilityConfig `yaml:"basic_verification"`
}

// PluginCapabilities defines optional capabilities
type PluginCapabilities struct {
	Scanner  CapabilityConfig `yaml:"scanner"`
	Nmap     CapabilityConfig `yaml:"nmap"`
	SSHProbe CapabilityConfig `yaml:"ssh_probe"`
	SNMP     CapabilityConfig `yaml:"snmp"`
}

// CapabilitiesConfig holds all capability settings
type CapabilitiesConfig struct {
	Core    CoreCapabilities   `yaml:"core"`
	Plugins PluginCapabilities `yaml:"plugins"`
}

// DefaultCapabilities returns the default capability configuration
func DefaultCapabilities() CapabilitiesConfig {
	return CapabilitiesConfig{
		Core: CoreCapabilities{
			HTTPServer:        CapabilityConfig{Enabled: true},
			SSEEvents:         CapabilityConfig{Enabled: true},
			ImportExport:      CapabilityConfig{Enabled: true},
			BasicVerification: CapabilityConfig{Enabled: true},
		},
		Plugins: PluginCapabilities{
			Scanner: CapabilityConfig{
				Enabled: true,
				MinMode: ModeMonitor,
			},
			Nmap: CapabilityConfig{
				Enabled: false, // Requires nmap binary
				MinMode: ModeDiscovery,
			},
			SSHProbe: CapabilityConfig{
				Enabled: false, // Requires secrets
				MinMode: ModeDiscovery,
			},
			SNMP: CapabilityConfig{
				Enabled: false, // Future capability
				MinMode: ModeDiscovery,
			},
		},
	}
}

// CapabilityInfo provides runtime info about a capability
type CapabilityInfo struct {
	Name        string         `json:"name"`
	Type        CapabilityType `json:"type"`
	Enabled     bool           `json:"enabled"`
	Available   bool           `json:"available"`   // Has required deps
	MinMode     Mode           `json:"min_mode"`
	Description string         `json:"description"`
}

// ListCapabilities returns info about all capabilities
func (c *CapabilitiesConfig) ListCapabilities() []CapabilityInfo {
	return []CapabilityInfo{
		// Core capabilities
		{
			Name:        "http_server",
			Type:        CapabilityTypeCore,
			Enabled:     c.Core.HTTPServer.Enabled,
			Available:   true,
			MinMode:     ModePassive,
			Description: "Web UI and API server",
		},
		{
			Name:        "sse_events",
			Type:        CapabilityTypeCore,
			Enabled:     c.Core.SSEEvents.Enabled,
			Available:   true,
			MinMode:     ModePassive,
			Description: "Server-Sent Events for real-time updates",
		},
		{
			Name:        "import_export",
			Type:        CapabilityTypeCore,
			Enabled:     c.Core.ImportExport.Enabled,
			Available:   true,
			MinMode:     ModePassive,
			Description: "YAML/JSON/Ansible import and export",
		},
		{
			Name:        "basic_verification",
			Type:        CapabilityTypeCore,
			Enabled:     c.Core.BasicVerification.Enabled,
			Available:   true,
			MinMode:     ModeMonitor,
			Description: "TCP ping and DNS lookup verification",
		},
		// Plugin capabilities
		{
			Name:        "scanner",
			Type:        CapabilityTypePlugin,
			Enabled:     c.Plugins.Scanner.Enabled,
			Available:   true, // Pure Go, always available
			MinMode:     c.Plugins.Scanner.MinMode,
			Description: "Subnet discovery via TCP probes",
		},
		{
			Name:        "nmap",
			Type:        CapabilityTypePlugin,
			Enabled:     c.Plugins.Nmap.Enabled,
			Available:   false, // Checked at runtime
			MinMode:     c.Plugins.Nmap.MinMode,
			Description: "Service fingerprinting via nmap",
		},
		{
			Name:        "ssh_probe",
			Type:        CapabilityTypePlugin,
			Enabled:     c.Plugins.SSHProbe.Enabled,
			Available:   true, // Pure Go SSH client
			MinMode:     c.Plugins.SSHProbe.MinMode,
			Description: "SSH-based fact gathering",
		},
		{
			Name:        "snmp",
			Type:        CapabilityTypePlugin,
			Enabled:     c.Plugins.SNMP.Enabled,
			Available:   false, // Future
			MinMode:     c.Plugins.SNMP.MinMode,
			Description: "SNMP discovery (future)",
		},
	}
}

// IsEnabled checks if a capability is enabled and available for the given mode
func (c *CapabilitiesConfig) IsEnabled(name string, currentMode Mode) bool {
	for _, cap := range c.ListCapabilities() {
		if cap.Name == name {
			return cap.Enabled && cap.Available && currentMode.Allows(cap.MinMode)
		}
	}
	return false
}
