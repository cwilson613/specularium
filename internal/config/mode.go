package config

import "time"

// Mode defines the capability ceiling based on hardware/resources
type Mode string

const (
	ModePassive   Mode = "passive"   // HTTP server only, no active probing
	ModeMonitor   Mode = "monitor"   // + lightweight verification (TCP ping, DNS)
	ModeDiscovery Mode = "discovery" // + full scanning (nmap, SSH probe, subnet scan)
)

// ParseMode converts a string to Mode, defaulting to ModeMonitor
func ParseMode(s string) Mode {
	switch s {
	case "passive":
		return ModePassive
	case "monitor":
		return ModeMonitor
	case "discovery":
		return ModeDiscovery
	default:
		return ModeMonitor
	}
}

// Level returns numeric level for comparison (higher = more capabilities)
func (m Mode) Level() int {
	switch m {
	case ModePassive:
		return 0
	case ModeMonitor:
		return 1
	case ModeDiscovery:
		return 2
	default:
		return 1
	}
}

// Allows returns true if this mode allows the given mode's capabilities
func (m Mode) Allows(required Mode) bool {
	return m.Level() >= required.Level()
}

// Posture defines behavioral aggressiveness
type Posture string

const (
	PostureStealth    Posture = "stealth"    // Minimal footprint, slow, avoid detection
	PostureCautious   Posture = "cautious"   // Conservative, respect rate limits
	PostureBalanced   Posture = "balanced"   // Default homelab behavior
	PostureAggressive Posture = "aggressive" // Fast, thorough, persistent
)

// ParsePosture converts a string to Posture, defaulting to PostureBalanced
func ParsePosture(s string) Posture {
	switch s {
	case "stealth":
		return PostureStealth
	case "cautious":
		return PostureCautious
	case "balanced":
		return PostureBalanced
	case "aggressive":
		return PostureAggressive
	default:
		return PostureBalanced
	}
}

// BehaviorProfile defines timing and concurrency settings
type BehaviorProfile struct {
	VerifyInterval      time.Duration `yaml:"verify_interval"`
	ScanInterval        time.Duration `yaml:"scan_interval"`
	ProbeTimeout        time.Duration `yaml:"probe_timeout"`
	MaxConcurrentProbes int           `yaml:"max_concurrent_probes"`
	MaxConcurrentScans  int           `yaml:"max_concurrent_scans"`
	MaxRetries          int           `yaml:"max_retries"`
	RateLimitPerHost    int           `yaml:"rate_limit_per_host"` // probes per minute
	JitterPercent       int           `yaml:"jitter_percent"`      // timing variance
}

// PostureProfiles maps postures to their default behavior profiles
var PostureProfiles = map[Posture]BehaviorProfile{
	PostureStealth: {
		VerifyInterval:      4 * time.Hour,
		ScanInterval:        24 * time.Hour,
		ProbeTimeout:        5 * time.Second,
		MaxConcurrentProbes: 2,
		MaxConcurrentScans:  1,
		MaxRetries:          0,
		RateLimitPerHost:    1,
		JitterPercent:       30,
	},
	PostureCautious: {
		VerifyInterval:      30 * time.Minute,
		ScanInterval:        2 * time.Hour,
		ProbeTimeout:        3 * time.Second,
		MaxConcurrentProbes: 5,
		MaxConcurrentScans:  2,
		MaxRetries:          1,
		RateLimitPerHost:    5,
		JitterPercent:       20,
	},
	PostureBalanced: {
		VerifyInterval:      5 * time.Minute,
		ScanInterval:        15 * time.Minute,
		ProbeTimeout:        2 * time.Second,
		MaxConcurrentProbes: 10,
		MaxConcurrentScans:  3,
		MaxRetries:          2,
		RateLimitPerHost:    10,
		JitterPercent:       10,
	},
	PostureAggressive: {
		VerifyInterval:      30 * time.Second,
		ScanInterval:        5 * time.Minute,
		ProbeTimeout:        1 * time.Second,
		MaxConcurrentProbes: 100,
		MaxConcurrentScans:  10,
		MaxRetries:          3,
		RateLimitPerHost:    60,
		JitterPercent:       0,
	},
}

// GetProfile returns the behavior profile for a posture
func (p Posture) GetProfile() BehaviorProfile {
	if profile, ok := PostureProfiles[p]; ok {
		return profile
	}
	return PostureProfiles[PostureBalanced]
}
