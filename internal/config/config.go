// Package config provides configuration management for Specularium.
//
// The config system separates identity (config file) from knowledge (database):
// - Config file persists "who I am" and survives DB wipes
// - Database stores "what I know" and can be reset
//
// Config file locations (priority order):
//  1. $SPECULARIUM_CONFIG
//  2. ./specularium.yaml
//  3. ~/.config/specularium/config.yaml
//  4. /etc/specularium/config.yaml
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"specularium/internal/domain"
)

// Load finds and loads the config file, or returns defaults if none found
func Load() (*Config, string, error) {
	path := FindConfigPath()

	if path == "" {
		// No config found - return defaults
		return DefaultConfig(), "", nil
	}

	return LoadFromPath(path)
}

// LoadFromPath loads config from a specific path
func LoadFromPath(path string) (*Config, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, path, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, path, fmt.Errorf("parse config: %w", err)
	}

	cfg.applyDefaults()

	return &cfg, path, nil
}

// Save writes config to the specified path
func (c *Config) Save(path string) error {
	if err := EnsureConfigDir(path); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// DefaultConfig returns sensible defaults for a new installation
func DefaultConfig() *Config {
	return &Config{
		Version:      1,
		Posture:      PostureBalanced,
		Database:     DatabaseConfig{Path: "./specularium.db"},
		Capabilities: DefaultCapabilities(),
		Targets:      TargetConfig{},
		Secrets:      SecretsConfig{},
	}
}

// applyDefaults fills in missing values with defaults
func (c *Config) applyDefaults() {
	if c.Version == 0 {
		c.Version = 1
	}
	if c.Posture == "" {
		c.Posture = PostureBalanced
	}
	if c.Database.Path == "" {
		c.Database.Path = "./specularium.db"
	}

	// Ensure core capabilities are always enabled
	c.Capabilities.Core.HTTPServer.Enabled = true
	c.Capabilities.Core.SSEEvents.Enabled = true
}

// EffectiveMode returns the mode to use (override > recommendation > default)
func (c *Config) EffectiveMode() Mode {
	// Explicit override takes precedence
	if c.Mode != nil {
		return *c.Mode
	}

	// Use bootstrap recommendation if available
	if c.Bootstrap != nil {
		return c.Bootstrap.Recommendation.Mode
	}

	// Default to monitor
	return ModeMonitor
}

// EffectiveBehavior returns behavior profile with overrides applied
func (c *Config) EffectiveBehavior() BehaviorProfile {
	base := c.Posture.GetProfile()

	if c.Behavior == nil {
		return base
	}

	// Apply overrides
	if c.Behavior.VerifyInterval != nil {
		base.VerifyInterval = c.Behavior.VerifyInterval.Duration()
	}
	if c.Behavior.ScanInterval != nil {
		base.ScanInterval = c.Behavior.ScanInterval.Duration()
	}
	if c.Behavior.ProbeTimeout != nil {
		base.ProbeTimeout = c.Behavior.ProbeTimeout.Duration()
	}
	if c.Behavior.MaxConcurrentProbes != nil {
		base.MaxConcurrentProbes = *c.Behavior.MaxConcurrentProbes
	}
	if c.Behavior.MaxConcurrentScans != nil {
		base.MaxConcurrentScans = *c.Behavior.MaxConcurrentScans
	}

	return base
}

// NeedsBootstrap returns true if bootstrap should run
func (c *Config) NeedsBootstrap() bool {
	return c.Bootstrap == nil
}

// SetBootstrapResult updates the config with bootstrap findings
func (c *Config) SetBootstrapResult(result *BootstrapResult) {
	c.Bootstrap = result
}

// ModeExceedsRecommendation returns true if mode override exceeds recommendation
func (c *Config) ModeExceedsRecommendation() bool {
	if c.Mode == nil || c.Bootstrap == nil {
		return false
	}
	return c.Mode.Level() > c.Bootstrap.Recommendation.Mode.Level()
}

// GetEnabledCapabilities returns list of capabilities enabled for current mode
func (c *Config) GetEnabledCapabilities() []CapabilityInfo {
	mode := c.EffectiveMode()
	var enabled []CapabilityInfo

	for _, cap := range c.Capabilities.ListCapabilities() {
		if cap.Enabled && mode.Allows(cap.MinMode) {
			enabled = append(enabled, cap)
		}
	}

	return enabled
}

// Summary returns a human-readable config summary
func (c *Config) Summary() string {
	mode := c.EffectiveMode()
	behavior := c.EffectiveBehavior()
	caps := c.GetEnabledCapabilities()

	summary := fmt.Sprintf("Mode: %s, Posture: %s\n", mode, c.Posture)
	summary += fmt.Sprintf("Verify: %s, Scan: %s, Concurrency: %d\n",
		behavior.VerifyInterval, behavior.ScanInterval, behavior.MaxConcurrentProbes)
	summary += fmt.Sprintf("Enabled capabilities (%d):", len(caps))
	for _, cap := range caps {
		summary += fmt.Sprintf(" %s", cap.Name)
	}

	return summary
}

// NewBootstrapResult creates a BootstrapResult with the current timestamp
func NewBootstrapResult() *BootstrapResult {
	return &BootstrapResult{
		Timestamp: time.Now(),
	}
}

// BuildBootstrapResult constructs a BootstrapResult from detected environment
// Takes full domain.EnvironmentInfo and optional resource/permission info
func BuildBootstrapResult(env domain.EnvironmentInfo, resources *ResourceInfo, permissions *PermissionInfo) *BootstrapResult {
	result := NewBootstrapResult()

	// Use heuristic signature detection
	detection := DetectEnvironmentType(env)

	result.Environment = EnvironmentInfo{
		Type:       string(detection.Type),
		Runtime:    string(detection.Runtime),
		Confidence: detection.Confidence,
	}

	// Network info from environment
	result.Network = NetworkInfo{
		Hostname:   env.Hostname,
		Gateway:    env.DefaultGateway,
		DNSServers: env.DNSServers,
	}

	// Add interface info if we have local subnet
	if env.LocalSubnet != "" {
		result.Network.Interfaces = []InterfaceInfo{
			{Name: "primary", Subnet: env.LocalSubnet},
		}
	}

	// Resources (if provided)
	if resources != nil {
		result.Resources = *resources
	}

	// Permissions (if provided)
	if permissions != nil {
		result.Permissions = *permissions
	}

	// Build mode recommendation using all available signals
	result.Recommendation = buildModeRecommendation(detection, resources, permissions)

	return result
}

// buildModeRecommendation determines the recommended mode based on environment, resources, and permissions
func buildModeRecommendation(detection DetectionResult, resources *ResourceInfo, permissions *PermissionInfo) ModeRecommendation {
	mode := ModeMonitor
	confidence := detection.Confidence
	reasons := append([]string{}, detection.Reasons...)

	// Container environment analysis
	if detection.Type == EnvTypeContainerized {
		switch detection.Runtime {
		case RuntimeKubernetes:
			mode = ModeMonitor
			reasons = append(reasons, "Kubernetes pod - network visibility limited to cluster")
		case RuntimeDocker, RuntimePodman, RuntimeContainerd, RuntimeCRIO:
			mode = ModeMonitor
			reasons = append(reasons, "Container - network access depends on configuration")
		case RuntimeLXC:
			// LXC often has more network access than Docker
			mode = ModeMonitor
			reasons = append(reasons, "LXC container - may have host network access")
		}
	} else if detection.Type == EnvTypeVM {
		mode = ModeDiscovery
		confidence = 0.8
		reasons = append(reasons, "VM detected - full network access likely available")
	} else {
		// Bare metal - highest capability
		mode = ModeDiscovery
		confidence = 0.85
		reasons = append(reasons, "Bare metal host - full network access available")
	}

	// Permission-based adjustments
	if permissions != nil {
		if permissions.CanRawSocket && permissions.CanICMPPing {
			// Full network capabilities
			if mode == ModeMonitor && detection.Type != EnvTypeContainerized {
				mode = ModeDiscovery
				reasons = append(reasons, "Raw socket and ICMP available - upgrading to discovery")
			}
		} else if !permissions.CanRawSocket {
			// Limited capabilities, stay in monitor mode
			if mode == ModeDiscovery {
				mode = ModeMonitor
				confidence *= 0.9
				reasons = append(reasons, "No raw socket access - limiting to monitor mode")
			}
		}

		if permissions.EffectiveUID == 0 {
			reasons = append(reasons, "Running as root - elevated privileges available")
		}
	}

	// Resource-based adjustments
	if resources != nil {
		if resources.MemoryMB < 256 {
			// Very constrained - prefer passive mode
			mode = ModePassive
			confidence *= 0.85
			reasons = append(reasons, "Low memory (<256MB) - recommending passive mode")
		} else if resources.MemoryMB < 512 && mode == ModeDiscovery {
			mode = ModeMonitor
			reasons = append(reasons, "Limited memory (<512MB) - limiting to monitor mode")
		}
	}

	return ModeRecommendation{
		Mode:       mode,
		Confidence: confidence,
		Reasons:    reasons,
	}
}
