package config

import (
	"testing"

	"specularium/internal/domain"
)

func TestDetectEnvironmentType(t *testing.T) {
	tests := []struct {
		name        string
		env         domain.EnvironmentInfo
		wantType    EnvironmentType
		wantRuntime ContainerRuntime
	}{
		{
			name:        "bare metal with no container indicators",
			env:         domain.EnvironmentInfo{Hostname: "myhost"},
			wantType:    EnvTypeBareMetal,
			wantRuntime: RuntimeNone,
		},
		{
			name:        "kubernetes flag set",
			env:         domain.EnvironmentInfo{InKubernetes: true, Hostname: "pod-123"},
			wantType:    EnvTypeContainerized,
			wantRuntime: RuntimeKubernetes,
		},
		{
			name:        "docker flag set",
			env:         domain.EnvironmentInfo{InDocker: true, Hostname: "container-456"},
			wantType:    EnvTypeContainerized,
			wantRuntime: RuntimeDocker,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectEnvironmentType(tt.env)
			if result.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", result.Type, tt.wantType)
			}
			if result.Runtime != tt.wantRuntime {
				t.Errorf("Runtime = %v, want %v", result.Runtime, tt.wantRuntime)
			}
			if result.Confidence <= 0 || result.Confidence > 1 {
				t.Errorf("Confidence = %v, want value in (0, 1]", result.Confidence)
			}
		})
	}
}

func TestRuntimeSignatures(t *testing.T) {
	// Verify signatures are well-formed
	if len(RuntimeSignatures) == 0 {
		t.Error("RuntimeSignatures should not be empty")
	}

	for i, sig := range RuntimeSignatures {
		if sig.Runtime == "" {
			t.Errorf("Signature %d has empty runtime", i)
		}
		if sig.Confidence <= 0 || sig.Confidence > 1 {
			t.Errorf("Signature %d (%s) has invalid confidence: %v", i, sig.Runtime, sig.Confidence)
		}
		// Each signature should have at least one detection method
		hasDetection := len(sig.FileExists) > 0 ||
			len(sig.EnvVars) > 0 ||
			len(sig.CGroupMarkers) > 0 ||
			len(sig.MountMarkers) > 0
		if !hasDetection {
			t.Errorf("Signature %d (%s) has no detection methods", i, sig.Runtime)
		}
	}
}

func TestBuildModeRecommendation(t *testing.T) {
	tests := []struct {
		name        string
		detection   DetectionResult
		resources   *ResourceInfo
		permissions *PermissionInfo
		wantMode    Mode
	}{
		{
			name: "bare metal with full permissions",
			detection: DetectionResult{
				Type:       EnvTypeBareMetal,
				Runtime:    RuntimeNone,
				Confidence: 0.8,
			},
			permissions: &PermissionInfo{
				CanRawSocket:  true,
				CanICMPPing:   true,
				CanReadProcFS: true,
				EffectiveUID:  0,
			},
			wantMode: ModeDiscovery,
		},
		{
			name: "kubernetes container",
			detection: DetectionResult{
				Type:       EnvTypeContainerized,
				Runtime:    RuntimeKubernetes,
				Confidence: 0.95,
			},
			permissions: &PermissionInfo{
				CanRawSocket:  false,
				CanICMPPing:   false,
				CanReadProcFS: true,
				EffectiveUID:  1000,
			},
			wantMode: ModeMonitor,
		},
		{
			name: "low memory system",
			detection: DetectionResult{
				Type:       EnvTypeBareMetal,
				Runtime:    RuntimeNone,
				Confidence: 0.7,
			},
			resources: &ResourceInfo{
				CPUCores:     1,
				MemoryMB:     128,
				Architecture: "arm",
			},
			permissions: &PermissionInfo{
				CanRawSocket: true,
				CanICMPPing:  true,
			},
			wantMode: ModePassive,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildModeRecommendation(tt.detection, tt.resources, tt.permissions)
			if result.Mode != tt.wantMode {
				t.Errorf("Mode = %v, want %v", result.Mode, tt.wantMode)
			}
			if len(result.Reasons) == 0 {
				t.Error("Expected at least one reason for recommendation")
			}
		})
	}
}
