package bootstrap

import (
	"testing"

	"specularium/internal/config"
)

func TestNewEvidence(t *testing.T) {
	e := NewEvidence(CategoryEnvironment, "test_prop", "test_value", 0.95, "test_source", "test method")

	if e.Category != CategoryEnvironment {
		t.Errorf("Category = %v, want %v", e.Category, CategoryEnvironment)
	}
	if e.Property != "test_prop" {
		t.Errorf("Property = %v, want test_prop", e.Property)
	}
	if e.Value != "test_value" {
		t.Errorf("Value = %v, want test_value", e.Value)
	}
	if e.Confidence != 0.95 {
		t.Errorf("Confidence = %v, want 0.95", e.Confidence)
	}
	if e.ID == "" {
		t.Error("ID should not be empty")
	}
}

func TestEvidenceSet_BestValue(t *testing.T) {
	es := NewEvidenceSet()

	// Add multiple evidence for same property with different confidences
	es.Add(NewEvidence(CategoryEnvironment, "type", "docker", 0.80, "source1", "method1"))
	es.Add(NewEvidence(CategoryEnvironment, "type", "docker", 0.95, "source2", "method2"))
	es.Add(NewEvidence(CategoryEnvironment, "type", "docker", 0.85, "source3", "method3"))

	val, conf, found := es.BestValue(CategoryEnvironment, "type")
	if !found {
		t.Error("BestValue should find evidence")
	}
	if val != "docker" {
		t.Errorf("Value = %v, want docker", val)
	}
	if conf != 0.95 {
		t.Errorf("Confidence = %v, want 0.95", conf)
	}
}

func TestEvidenceSet_AggregateConfidence(t *testing.T) {
	es := NewEvidenceSet()

	// Single evidence
	es.Add(NewEvidence(CategoryEnvironment, "single", "value", 0.80, "source", "method"))
	conf := es.AggregateConfidence(CategoryEnvironment, "single")
	if conf != 0.80 {
		t.Errorf("Single evidence confidence = %v, want 0.80", conf)
	}

	// Multiple corroborating evidence - should boost confidence
	es.Add(NewEvidence(CategoryEnvironment, "multi", "value", 0.80, "source1", "method1"))
	es.Add(NewEvidence(CategoryEnvironment, "multi", "value", 0.70, "source2", "method2"))
	multiConf := es.AggregateConfidence(CategoryEnvironment, "multi")
	if multiConf <= 0.80 {
		t.Errorf("Corroborated confidence = %v, should be > 0.80", multiConf)
	}
	if multiConf > 0.99 {
		t.Errorf("Confidence = %v, should be capped at 0.99", multiConf)
	}
}

func TestEvidenceSet_ByCategory(t *testing.T) {
	es := NewEvidenceSet()

	es.Add(NewEvidence(CategoryEnvironment, "env1", "v1", 0.9, "s", "m"))
	es.Add(NewEvidence(CategoryEnvironment, "env2", "v2", 0.9, "s", "m"))
	es.Add(NewEvidence(CategoryResources, "res1", "v3", 0.9, "s", "m"))
	es.Add(NewEvidence(CategoryNetwork, "net1", "v4", 0.9, "s", "m"))

	envEvidence := es.ByCategory(CategoryEnvironment)
	if len(envEvidence) != 2 {
		t.Errorf("ByCategory(environment) returned %d items, want 2", len(envEvidence))
	}

	resEvidence := es.ByCategory(CategoryResources)
	if len(resEvidence) != 1 {
		t.Errorf("ByCategory(resources) returned %d items, want 1", len(resEvidence))
	}
}

func TestDetectEnvironment(t *testing.T) {
	evidence := DetectEnvironment()

	// Should always produce some evidence
	if len(evidence) == 0 {
		t.Error("DetectEnvironment should produce evidence")
	}

	// Should always infer environment type
	hasEnvType := false
	for _, e := range evidence {
		if e.Property == "environment_type" {
			hasEnvType = true
			break
		}
	}
	if !hasEnvType {
		t.Error("DetectEnvironment should produce environment_type evidence")
	}
}

func TestDetectResources(t *testing.T) {
	evidence := DetectResources()

	// Should always have CPU and architecture
	hasCPU := false
	hasArch := false
	for _, e := range evidence {
		if e.Property == "cpu_cores" {
			hasCPU = true
			if cores, ok := e.Value.(int); !ok || cores <= 0 {
				t.Errorf("cpu_cores should be positive int, got %v", e.Value)
			}
		}
		if e.Property == "architecture" {
			hasArch = true
		}
	}

	if !hasCPU {
		t.Error("DetectResources should produce cpu_cores evidence")
	}
	if !hasArch {
		t.Error("DetectResources should produce architecture evidence")
	}
}

func TestDetectPermissions(t *testing.T) {
	evidence := DetectPermissions()

	// Should always have UID info
	hasUID := false
	for _, e := range evidence {
		if e.Property == "effective_uid" {
			hasUID = true
		}
	}

	if !hasUID {
		t.Error("DetectPermissions should produce effective_uid evidence")
	}
}

func TestDetectNetwork(t *testing.T) {
	evidence := DetectNetwork()

	// Should have hostname at minimum
	hasHostname := false
	for _, e := range evidence {
		if e.Property == "hostname" {
			hasHostname = true
		}
	}

	if !hasHostname {
		t.Error("DetectNetwork should produce hostname evidence")
	}
}

func TestSynthesizeMode(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*EvidenceSet)
		wantMode config.Mode
	}{
		{
			name: "full capabilities",
			setup: func(es *EvidenceSet) {
				es.Add(NewEvidence(CategoryResources, "memory_mb", 4096, 0.95, "", ""))
				es.Add(NewEvidence(CategoryCapability, "has_nmap", true, 0.99, "", ""))
				es.Add(NewEvidence(CategoryCapability, "can_raw_socket", true, 0.95, "", ""))
				es.Add(NewEvidence(CategoryNetwork, "gateway", "192.168.1.1", 0.95, "", ""))
			},
			wantMode: config.ModeDiscovery,
		},
		{
			name: "no nmap",
			setup: func(es *EvidenceSet) {
				es.Add(NewEvidence(CategoryResources, "memory_mb", 4096, 0.95, "", ""))
				es.Add(NewEvidence(CategoryCapability, "has_nmap", false, 0.95, "", ""))
			},
			wantMode: config.ModeMonitor,
		},
		{
			name: "very low memory",
			setup: func(es *EvidenceSet) {
				es.Add(NewEvidence(CategoryResources, "memory_mb", 64, 0.95, "", ""))
			},
			wantMode: config.ModePassive,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := NewEvidenceSet()
			tt.setup(es)

			mode, _, reasons := SynthesizeMode(es)
			if mode != tt.wantMode {
				t.Errorf("Mode = %v, want %v", mode, tt.wantMode)
				t.Logf("Reasons: %v", reasons)
			}
		})
	}
}

func TestEvidenceWithRaw(t *testing.T) {
	e := NewEvidence(CategoryEnvironment, "test", "value", 0.9, "source", "method").
		WithRaw(map[string]any{"key": "value", "num": 42})

	if e.Raw == nil {
		t.Error("Raw should not be nil")
	}
	if e.Raw["key"] != "value" {
		t.Errorf("Raw[key] = %v, want value", e.Raw["key"])
	}
	if e.Raw["num"] != 42 {
		t.Errorf("Raw[num] = %v, want 42", e.Raw["num"])
	}
}
