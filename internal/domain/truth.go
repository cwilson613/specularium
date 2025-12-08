package domain

import (
	"fmt"
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

// TruthableProperties defines which properties can be locked as operator truth
var TruthableProperties = []string{
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
// Handles type coercion for common cases
func CompareValues(truth, actual any) bool {
	if truth == nil && actual == nil {
		return true
	}
	if truth == nil || actual == nil {
		return false
	}

	// Try direct comparison
	if truth == actual {
		return true
	}

	// Handle string comparisons
	truthStr := fmt.Sprintf("%v", truth)
	actualStr := fmt.Sprintf("%v", actual)
	return truthStr == actualStr
}
