package config

import (
	"time"
)

// Config is the root configuration structure
type Config struct {
	Version      int                `yaml:"version"`
	Bootstrap    *BootstrapResult   `yaml:"bootstrap,omitempty"`
	Mode         *Mode              `yaml:"mode"`    // nil = use bootstrap recommendation
	Posture      Posture            `yaml:"posture"`
	Behavior     *BehaviorOverride  `yaml:"behavior,omitempty"`
	Database     DatabaseConfig     `yaml:"database"`
	Capabilities CapabilitiesConfig `yaml:"capabilities"`
	Targets      TargetConfig       `yaml:"targets"`
	Secrets      SecretsConfig      `yaml:"secrets"`
}

// BootstrapResult stores self-discovery findings (written by bootstrap)
type BootstrapResult struct {
	Timestamp      time.Time          `yaml:"timestamp"`
	Environment    EnvironmentInfo    `yaml:"environment"`
	Resources      ResourceInfo       `yaml:"resources"`
	Permissions    PermissionInfo     `yaml:"permissions"`
	Network        NetworkInfo        `yaml:"network"`
	Recommendation ModeRecommendation `yaml:"recommendation"`
}

// EnvironmentInfo describes the execution environment
type EnvironmentInfo struct {
	Type       string  `yaml:"type"`       // bare_metal, vm, container
	Runtime    string  `yaml:"runtime"`    // none, docker, kubernetes, podman
	Confidence float64 `yaml:"confidence"` // 0.0-1.0
}

// ResourceInfo describes available resources
type ResourceInfo struct {
	CPUCores     int    `yaml:"cpu_cores"`
	MemoryMB     int    `yaml:"memory_mb"`
	Architecture string `yaml:"architecture"`
}

// PermissionInfo describes probed permissions
type PermissionInfo struct {
	CanICMPPing   bool   `yaml:"can_icmp_ping"`
	CanRawSocket  bool   `yaml:"can_raw_socket"`
	CanReadProcFS bool   `yaml:"can_read_procfs"`
	EffectiveUser string `yaml:"effective_user"`
	EffectiveUID  int    `yaml:"effective_uid"`
}

// NetworkInfo describes network configuration
type NetworkInfo struct {
	Hostname   string          `yaml:"hostname"`
	Interfaces []InterfaceInfo `yaml:"interfaces,omitempty"`
	Gateway    string          `yaml:"gateway,omitempty"`
	DNSServers []string        `yaml:"dns_servers,omitempty"`
}

// InterfaceInfo describes a network interface
type InterfaceInfo struct {
	Name   string `yaml:"name"`
	IP     string `yaml:"ip"`
	Subnet string `yaml:"subnet,omitempty"`
}

// ModeRecommendation is the bootstrap's suggested mode
type ModeRecommendation struct {
	Mode       Mode     `yaml:"mode"`
	Confidence float64  `yaml:"confidence"`
	Reasons    []string `yaml:"reasons,omitempty"`
}

// BehaviorOverride allows overriding posture defaults
type BehaviorOverride struct {
	VerifyInterval      *Duration `yaml:"verify_interval,omitempty"`
	ScanInterval        *Duration `yaml:"scan_interval,omitempty"`
	ProbeTimeout        *Duration `yaml:"probe_timeout,omitempty"`
	MaxConcurrentProbes *int      `yaml:"max_concurrent_probes,omitempty"`
	MaxConcurrentScans  *int      `yaml:"max_concurrent_scans,omitempty"`
}

// DatabaseConfig holds database settings
type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// TargetConfig holds discovery targets
type TargetConfig struct {
	Primary   []string `yaml:"primary,omitempty"`   // Main monitored networks
	Discovery []string `yaml:"discovery,omitempty"` // Additional discovery targets
}

// SecretsConfig holds references to secrets (paths, not values)
type SecretsConfig struct {
	SSHKeyPath *string `yaml:"ssh_key_path,omitempty"`
	DNSServer  *string `yaml:"dns_server,omitempty"`
}

// Duration wraps time.Duration for YAML unmarshaling
type Duration time.Duration

// UnmarshalYAML implements yaml.Unmarshaler
func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(parsed)
	return nil
}

// MarshalYAML implements yaml.Marshaler
func (d Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}

// Duration returns the underlying time.Duration
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}
