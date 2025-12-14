package domain

import (
	"reflect"
	"strconv"
	"time"
)

// TruthStatus indicates whether a node has operator truth assertions
type TruthStatus string

const (
	TruthStatusNone     TruthStatus = ""          // No truth assertion
	TruthStatusAsserted TruthStatus = "asserted"  // Operator has asserted truth
	TruthStatusConflict TruthStatus = "conflict"  // Truth conflicts with discovered values
)

// NodeTruth represents operator-asserted truth values for a node
type NodeTruth struct {
	// AssertedBy records who set the truth (operator name/ID)
	AssertedBy string `json:"asserted_by,omitempty"`
	// AssertedAt records when truth was set
	AssertedAt *time.Time `json:"asserted_at,omitempty"`
	// Properties holds the truth values (IP, hostname, type, etc.)
	Properties map[string]any `json:"properties,omitempty"`
}

// HasProperty checks if a truth assertion exists for a property
func (t *NodeTruth) HasProperty(key string) bool {
	if t == nil || t.Properties == nil {
		return false
	}
	_, ok := t.Properties[key]
	return ok
}

// GetProperty gets a truth value for a property
func (t *NodeTruth) GetProperty(key string) (any, bool) {
	if t == nil || t.Properties == nil {
		return nil, false
	}
	val, ok := t.Properties[key]
	return val, ok
}

// Discrepancy represents a conflict between operator truth and discovered values
type Discrepancy struct {
	ID          string     `json:"id"`
	NodeID      string     `json:"node_id"`
	PropertyKey string     `json:"property_key"`
	TruthValue  any        `json:"truth_value"`
	ActualValue any        `json:"actual_value"`
	Source      string     `json:"source"` // verifier, scanner, etc.
	DetectedAt  time.Time  `json:"detected_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	Resolution  string     `json:"resolution,omitempty"` // "updated_truth", "fixed_reality", "dismissed"
}

// IsResolved returns true if the discrepancy has been resolved
func (d *Discrepancy) IsResolved() bool {
	return d.ResolvedAt != nil
}

// DiscrepancyResolution defines how a discrepancy was resolved
type DiscrepancyResolution string

const (
	ResolutionUpdatedTruth DiscrepancyResolution = "updated_truth" // Operator updated truth to match reality
	ResolutionFixedReality DiscrepancyResolution = "fixed_reality" // Reality was fixed to match truth
	ResolutionDismissed    DiscrepancyResolution = "dismissed"     // Discrepancy was dismissed/ignored
)

// ExistenceAssertion defines the expected existence state of a node
type ExistenceAssertion string

const (
	ExistenceExpected  ExistenceAssertion = "expected"  // Node should exist and be reachable
	ExistenceRetired   ExistenceAssertion = "retired"   // Node should be offline/decommissioned
	ExistenceTemporary ExistenceAssertion = "temporary" // Node may come and go (no discrepancy either way)
)

// TruthableProperties defines which properties can be locked as operator truth
var TruthableProperties = []string{
	"existence", // Whether node should exist (expected, retired, temporary)
	"ip",
	"hostname",
	"mac_address",
	"type",
	"description",
	"location",
	"owner",
	"expected_ports",
}

// IsTruthable returns true if the property can be set as truth
func IsTruthable(key string) bool {
	for _, p := range TruthableProperties {
		if p == key {
			return true
		}
	}
	return false
}

// CompareValues compares truth and actual values for equality
// Handles type coercion for common cases including string-to-primitive conversion
func CompareValues(truth, actual any) bool {
	if truth == nil && actual == nil {
		return true
	}
	if truth == nil || actual == nil {
		return false
	}

	// Try direct comparison for simple types
	if truth == actual {
		return true
	}

	// Handle type-specific comparisons
	switch t := truth.(type) {
	case string:
		// Try direct string comparison
		if a, ok := actual.(string); ok {
			return t == a
		}
		// Compare string representation for cross-type comparison
		return t == formatValue(actual)
	case int:
		if a, ok := actual.(int); ok {
			return t == a
		}
		return formatValue(t) == formatValue(actual)
	case int64:
		if a, ok := actual.(int64); ok {
			return t == a
		}
		return formatValue(t) == formatValue(actual)
	case float64:
		if a, ok := actual.(float64); ok {
			return t == a
		}
		return formatValue(t) == formatValue(actual)
	case bool:
		if a, ok := actual.(bool); ok {
			return t == a
		}
		return formatValue(t) == formatValue(actual)
	case []any, []string, map[string]any:
		// For complex types, use deep equality
		return reflect.DeepEqual(truth, actual)
	}

	// Fallback to deep equality for unknown types
	return reflect.DeepEqual(truth, actual)
}

// formatValue converts a value to its string representation for comparison
func formatValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		// Use precise formatting without trailing zeros
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	default:
		return ""
	}
}
