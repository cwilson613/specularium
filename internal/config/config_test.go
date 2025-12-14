package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestModeLevel(t *testing.T) {
	tests := []struct {
		mode  Mode
		level int
	}{
		{ModePassive, 0},
		{ModeMonitor, 1},
		{ModeDiscovery, 2},
	}

	for _, tt := range tests {
		if got := tt.mode.Level(); got != tt.level {
			t.Errorf("Mode(%s).Level() = %d, want %d", tt.mode, got, tt.level)
		}
	}
}

func TestModeAllows(t *testing.T) {
	tests := []struct {
		current  Mode
		required Mode
		allowed  bool
	}{
		{ModeDiscovery, ModePassive, true},
		{ModeDiscovery, ModeMonitor, true},
		{ModeDiscovery, ModeDiscovery, true},
		{ModeMonitor, ModePassive, true},
		{ModeMonitor, ModeMonitor, true},
		{ModeMonitor, ModeDiscovery, false},
		{ModePassive, ModePassive, true},
		{ModePassive, ModeMonitor, false},
		{ModePassive, ModeDiscovery, false},
	}

	for _, tt := range tests {
		if got := tt.current.Allows(tt.required); got != tt.allowed {
			t.Errorf("Mode(%s).Allows(%s) = %v, want %v",
				tt.current, tt.required, got, tt.allowed)
		}
	}
}

func TestParseMode(t *testing.T) {
	tests := []struct {
		input string
		want  Mode
	}{
		{"passive", ModePassive},
		{"monitor", ModeMonitor},
		{"discovery", ModeDiscovery},
		{"invalid", ModeMonitor}, // Default
		{"", ModeMonitor},        // Default
	}

	for _, tt := range tests {
		if got := ParseMode(tt.input); got != tt.want {
			t.Errorf("ParseMode(%q) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestPostureGetProfile(t *testing.T) {
	// Verify each posture has a profile
	postures := []Posture{PostureStealth, PostureCautious, PostureBalanced, PostureAggressive}

	for _, p := range postures {
		profile := p.GetProfile()
		if profile.VerifyInterval == 0 {
			t.Errorf("Posture(%s).GetProfile().VerifyInterval should not be 0", p)
		}
		if profile.MaxConcurrentProbes == 0 {
			t.Errorf("Posture(%s).GetProfile().MaxConcurrentProbes should not be 0", p)
		}
	}

	// Verify ordering (stealth should be slowest, aggressive fastest)
	stealth := PostureStealth.GetProfile()
	aggressive := PostureAggressive.GetProfile()

	if stealth.VerifyInterval <= aggressive.VerifyInterval {
		t.Error("Stealth should have longer verify interval than aggressive")
	}
	if stealth.MaxConcurrentProbes >= aggressive.MaxConcurrentProbes {
		t.Error("Stealth should have fewer concurrent probes than aggressive")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Version != 1 {
		t.Errorf("Version = %d, want 1", cfg.Version)
	}
	if cfg.Posture != PostureBalanced {
		t.Errorf("Posture = %s, want %s", cfg.Posture, PostureBalanced)
	}
	if cfg.Database.Path == "" {
		t.Error("Database.Path should not be empty")
	}

	// Core capabilities should be enabled
	if !cfg.Capabilities.Core.HTTPServer.Enabled {
		t.Error("Core.HTTPServer should be enabled")
	}
	if !cfg.Capabilities.Core.SSEEvents.Enabled {
		t.Error("Core.SSEEvents should be enabled")
	}
}

func TestEffectiveMode(t *testing.T) {
	// Default (no mode, no bootstrap) -> ModeMonitor
	cfg := DefaultConfig()
	if got := cfg.EffectiveMode(); got != ModeMonitor {
		t.Errorf("EffectiveMode() = %s, want %s (default)", got, ModeMonitor)
	}

	// With bootstrap recommendation
	cfg.Bootstrap = &BootstrapResult{
		Recommendation: ModeRecommendation{Mode: ModePassive},
	}
	if got := cfg.EffectiveMode(); got != ModePassive {
		t.Errorf("EffectiveMode() = %s, want %s (bootstrap)", got, ModePassive)
	}

	// Override takes precedence
	mode := ModeDiscovery
	cfg.Mode = &mode
	if got := cfg.EffectiveMode(); got != ModeDiscovery {
		t.Errorf("EffectiveMode() = %s, want %s (override)", got, ModeDiscovery)
	}
}

func TestEffectiveBehavior(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Posture = PostureBalanced

	// Without overrides, should match posture profile
	behavior := cfg.EffectiveBehavior()
	expected := PostureBalanced.GetProfile()

	if behavior.VerifyInterval != expected.VerifyInterval {
		t.Errorf("VerifyInterval = %s, want %s", behavior.VerifyInterval, expected.VerifyInterval)
	}

	// With override
	override := 10 * time.Minute
	cfg.Behavior = &BehaviorOverride{
		VerifyInterval: (*Duration)(&override),
	}
	behavior = cfg.EffectiveBehavior()

	if behavior.VerifyInterval != override {
		t.Errorf("VerifyInterval = %s, want %s (override)", behavior.VerifyInterval, override)
	}
	// Other fields should still be from posture
	if behavior.MaxConcurrentProbes != expected.MaxConcurrentProbes {
		t.Errorf("MaxConcurrentProbes = %d, want %d (posture default)",
			behavior.MaxConcurrentProbes, expected.MaxConcurrentProbes)
	}
}

func TestModeExceedsRecommendation(t *testing.T) {
	cfg := DefaultConfig()

	// No bootstrap -> false
	if cfg.ModeExceedsRecommendation() {
		t.Error("Should be false with no bootstrap")
	}

	// With bootstrap, no override -> false
	cfg.Bootstrap = &BootstrapResult{
		Recommendation: ModeRecommendation{Mode: ModeMonitor},
	}
	if cfg.ModeExceedsRecommendation() {
		t.Error("Should be false with no override")
	}

	// Override equals recommendation -> false
	mode := ModeMonitor
	cfg.Mode = &mode
	if cfg.ModeExceedsRecommendation() {
		t.Error("Should be false when override equals recommendation")
	}

	// Override exceeds recommendation -> true
	mode = ModeDiscovery
	cfg.Mode = &mode
	if !cfg.ModeExceedsRecommendation() {
		t.Error("Should be true when override exceeds recommendation")
	}
}

func TestCapabilitiesIsEnabled(t *testing.T) {
	cfg := DefaultConfig()
	caps := &cfg.Capabilities

	// Core capability always enabled
	if !caps.IsEnabled("http_server", ModePassive) {
		t.Error("http_server should be enabled in passive mode")
	}

	// Plugin with min_mode check
	if !caps.IsEnabled("scanner", ModeMonitor) {
		t.Error("scanner should be enabled in monitor mode")
	}
	if caps.IsEnabled("scanner", ModePassive) {
		t.Error("scanner should not be enabled in passive mode")
	}

	// Disabled plugin
	if caps.IsEnabled("nmap", ModeDiscovery) {
		t.Error("nmap should not be enabled (disabled by default)")
	}
}

func TestSaveAndLoad(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create and save config
	cfg := DefaultConfig()
	cfg.Posture = PostureAggressive
	mode := ModeDiscovery
	cfg.Mode = &mode
	cfg.Targets.Primary = []string{"192.168.1.0/24"}

	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Load config
	loaded, path, err := LoadFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadFromPath() error: %v", err)
	}
	if path != configPath {
		t.Errorf("path = %s, want %s", path, configPath)
	}

	// Verify values
	if loaded.Posture != PostureAggressive {
		t.Errorf("Posture = %s, want %s", loaded.Posture, PostureAggressive)
	}
	if loaded.Mode == nil || *loaded.Mode != ModeDiscovery {
		t.Error("Mode should be discovery")
	}
	if len(loaded.Targets.Primary) != 1 || loaded.Targets.Primary[0] != "192.168.1.0/24" {
		t.Errorf("Targets.Primary = %v, want [192.168.1.0/24]", loaded.Targets.Primary)
	}
}

func TestFindConfigPath(t *testing.T) {
	// Create temp directory with config
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ConfigFileName)

	cfg := DefaultConfig()
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Set working directory to temp
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Should find config in working directory
	found := FindConfigPath()
	if found == "" {
		t.Error("FindConfigPath() should find config in working directory")
	}

	// Should prefer explicit env var
	os.Setenv(EnvConfigPath, "/nonexistent/path.yaml")
	defer os.Unsetenv(EnvConfigPath)

	// Explicit path doesn't exist, should fall back
	found = FindConfigPath()
	if found == "" {
		t.Error("FindConfigPath() should fall back when env path doesn't exist")
	}
}

func TestDuration(t *testing.T) {
	d := Duration(5 * time.Minute)

	if d.Duration() != 5*time.Minute {
		t.Errorf("Duration() = %s, want 5m", d.Duration())
	}

	// Test YAML marshaling
	marshaled, err := d.MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML() error: %v", err)
	}
	if marshaled != "5m0s" {
		t.Errorf("MarshalYAML() = %v, want 5m0s", marshaled)
	}
}
