package domain

import (
	"testing"
	"time"
)

func TestNodeTruthHasProperty(t *testing.T) {
	now := time.Now()
	truth := &NodeTruth{
		AssertedBy: "operator",
		AssertedAt: &now,
		Properties: map[string]any{
			"hostname": "server1",
			"ip":       "192.168.1.1",
		},
	}

	t.Run("returns true for existing property", func(t *testing.T) {
		if !truth.HasProperty("hostname") {
			t.Error("expected HasProperty to return true for 'hostname'")
		}
		if !truth.HasProperty("ip") {
			t.Error("expected HasProperty to return true for 'ip'")
		}
	})

	t.Run("returns false for non-existent property", func(t *testing.T) {
		if truth.HasProperty("mac_address") {
			t.Error("expected HasProperty to return false for 'mac_address'")
		}
	})

	t.Run("returns false for nil truth", func(t *testing.T) {
		var nilTruth *NodeTruth
		if nilTruth.HasProperty("hostname") {
			t.Error("expected HasProperty to return false for nil truth")
		}
	})

	t.Run("returns false for nil properties map", func(t *testing.T) {
		truth := &NodeTruth{}
		if truth.HasProperty("hostname") {
			t.Error("expected HasProperty to return false for nil properties")
		}
	})
}

func TestNodeTruthGetProperty(t *testing.T) {
	now := time.Now()
	truth := &NodeTruth{
		AssertedBy: "operator",
		AssertedAt: &now,
		Properties: map[string]any{
			"hostname": "server1",
			"ip":       "192.168.1.1",
		},
	}

	t.Run("gets existing property", func(t *testing.T) {
		val, ok := truth.GetProperty("hostname")
		if !ok {
			t.Error("expected property to exist")
		}
		if val != "server1" {
			t.Errorf("expected 'server1', got %v", val)
		}
	})

	t.Run("returns false for non-existent property", func(t *testing.T) {
		_, ok := truth.GetProperty("mac_address")
		if ok {
			t.Error("expected property not to exist")
		}
	})

	t.Run("returns false for nil truth", func(t *testing.T) {
		var nilTruth *NodeTruth
		_, ok := nilTruth.GetProperty("hostname")
		if ok {
			t.Error("expected GetProperty to return false for nil truth")
		}
	})
}

func TestDiscrepancyIsResolved(t *testing.T) {
	now := time.Now()

	t.Run("returns false for unresolved discrepancy", func(t *testing.T) {
		disc := &Discrepancy{
			ID:          "disc1",
			NodeID:      "node1",
			PropertyKey: "hostname",
			TruthValue:  "truth",
			ActualValue: "actual",
			DetectedAt:  now,
		}

		if disc.IsResolved() {
			t.Error("expected IsResolved to return false")
		}
	})

	t.Run("returns true for resolved discrepancy", func(t *testing.T) {
		resolvedAt := now.Add(time.Hour)
		disc := &Discrepancy{
			ID:          "disc1",
			NodeID:      "node1",
			PropertyKey: "hostname",
			TruthValue:  "truth",
			ActualValue: "actual",
			DetectedAt:  now,
			ResolvedAt:  &resolvedAt,
			Resolution:  string(ResolutionUpdatedTruth),
		}

		if !disc.IsResolved() {
			t.Error("expected IsResolved to return true")
		}
	})
}

func TestIsTruthable(t *testing.T) {
	t.Run("returns true for truthable properties", func(t *testing.T) {
		truthableProps := []string{
			"existence",
			"ip",
			"hostname",
			"mac_address",
			"type",
			"description",
			"location",
			"owner",
			"expected_ports",
		}

		for _, prop := range truthableProps {
			if !IsTruthable(prop) {
				t.Errorf("expected %s to be truthable", prop)
			}
		}
	})

	t.Run("returns false for non-truthable properties", func(t *testing.T) {
		nonTruthableProps := []string{
			"random_property",
			"uptime",
			"cpu_usage",
			"memory",
		}

		for _, prop := range nonTruthableProps {
			if IsTruthable(prop) {
				t.Errorf("expected %s not to be truthable", prop)
			}
		}
	})
}

func TestCompareValues(t *testing.T) {
	tests := []struct {
		name     string
		truth    any
		actual   any
		expected bool
	}{
		{
			name:     "both nil",
			truth:    nil,
			actual:   nil,
			expected: true,
		},
		{
			name:     "one nil",
			truth:    "value",
			actual:   nil,
			expected: false,
		},
		{
			name:     "identical strings",
			truth:    "server1",
			actual:   "server1",
			expected: true,
		},
		{
			name:     "different strings",
			truth:    "server1",
			actual:   "server2",
			expected: false,
		},
		{
			name:     "identical integers",
			truth:    42,
			actual:   42,
			expected: true,
		},
		{
			name:     "different integers",
			truth:    42,
			actual:   43,
			expected: false,
		},
		{
			name:     "string vs int same value",
			truth:    "42",
			actual:   42,
			expected: true,
		},
		{
			name:     "string vs float same value",
			truth:    "3.14",
			actual:   3.14,
			expected: true,
		},
		{
			name:     "boolean true",
			truth:    true,
			actual:   true,
			expected: true,
		},
		{
			name:     "boolean false",
			truth:    false,
			actual:   false,
			expected: true,
		},
		{
			name:     "boolean string comparison",
			truth:    "true",
			actual:   true,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareValues(tt.truth, tt.actual)
			if result != tt.expected {
				t.Errorf("CompareValues(%v, %v) = %v, expected %v", tt.truth, tt.actual, result, tt.expected)
			}
		})
	}
}

func TestExistenceAssertion(t *testing.T) {
	t.Run("existence assertion constants are defined", func(t *testing.T) {
		assertions := []ExistenceAssertion{
			ExistenceExpected,
			ExistenceRetired,
			ExistenceTemporary,
		}

		for _, assertion := range assertions {
			if assertion == "" {
				t.Error("expected assertion to have non-empty value")
			}
		}
	})
}

func TestDiscrepancyResolution(t *testing.T) {
	t.Run("resolution constants are defined", func(t *testing.T) {
		resolutions := []DiscrepancyResolution{
			ResolutionUpdatedTruth,
			ResolutionFixedReality,
			ResolutionDismissed,
		}

		for _, resolution := range resolutions {
			if resolution == "" {
				t.Error("expected resolution to have non-empty value")
			}
		}
	})
}

func TestTruthStatus(t *testing.T) {
	t.Run("truth status constants are defined", func(t *testing.T) {
		// None should be empty string
		if TruthStatusNone != "" {
			t.Error("expected TruthStatusNone to be empty string")
		}

		// Others should be non-empty
		if TruthStatusAsserted == "" {
			t.Error("expected TruthStatusAsserted to be non-empty")
		}
		if TruthStatusConflict == "" {
			t.Error("expected TruthStatusConflict to be non-empty")
		}
	})
}
