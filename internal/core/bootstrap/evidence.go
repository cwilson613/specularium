// Package bootstrap provides evidence-based self-discovery for Specularium.
// It gathers knowledge about the execution environment, resources, and capabilities
// with confidence scoring, enabling intelligent mode recommendations.
package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"
)

// Category classifies types of evidence
type Category string

const (
	CategoryEnvironment Category = "environment"
	CategoryResources   Category = "resources"
	CategoryPermissions Category = "permissions"
	CategoryNetwork     Category = "network"
	CategoryCapability  Category = "capability"
)

// Evidence represents a single piece of discovered knowledge
type Evidence struct {
	ID         string         `json:"id"`
	Category   Category       `json:"category"`
	Property   string         `json:"property"`
	Value      any            `json:"value"`
	Confidence float64        `json:"confidence"` // 0.0-1.0
	Source     string         `json:"source"`     // e.g., "filesystem", "procfs", "syscall"
	Method     string         `json:"method"`     // e.g., "/.dockerenv exists"
	Timestamp  time.Time      `json:"timestamp"`
	DependsOn  []string       `json:"depends_on,omitempty"` // IDs of prerequisite evidence
	Raw        map[string]any `json:"raw,omitempty"`        // Additional raw data
}

// NewEvidence creates evidence with auto-generated ID
func NewEvidence(cat Category, prop string, value any, conf float64, source, method string) Evidence {
	e := Evidence{
		Category:   cat,
		Property:   prop,
		Value:      value,
		Confidence: conf,
		Source:     source,
		Method:     method,
		Timestamp:  time.Now(),
	}
	e.ID = e.generateID()
	return e
}

// WithRaw adds raw data to evidence and returns it (for chaining)
func (e Evidence) WithRaw(raw map[string]any) Evidence {
	e.Raw = raw
	return e
}

// WithDependsOn adds dependencies and returns the evidence (for chaining)
func (e Evidence) WithDependsOn(ids ...string) Evidence {
	e.DependsOn = append(e.DependsOn, ids...)
	return e
}

func (e *Evidence) generateID() string {
	data := fmt.Sprintf("%s:%s:%v:%s:%d", e.Category, e.Property, e.Value, e.Source, e.Timestamp.UnixNano())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}

// EvidenceSet aggregates multiple pieces of evidence
type EvidenceSet struct {
	items []Evidence
}

// NewEvidenceSet creates an empty evidence set
func NewEvidenceSet() *EvidenceSet {
	return &EvidenceSet{}
}

// Add appends a single piece of evidence
func (es *EvidenceSet) Add(e Evidence) {
	es.items = append(es.items, e)
}

// AddAll appends multiple pieces of evidence
func (es *EvidenceSet) AddAll(items []Evidence) {
	es.items = append(es.items, items...)
}

// All returns all evidence
func (es *EvidenceSet) All() []Evidence {
	return es.items
}

// Count returns the number of evidence items
func (es *EvidenceSet) Count() int {
	return len(es.items)
}

// ByCategory returns evidence filtered by category
func (es *EvidenceSet) ByCategory(cat Category) []Evidence {
	var result []Evidence
	for _, e := range es.items {
		if e.Category == cat {
			result = append(result, e)
		}
	}
	return result
}

// ByProperty returns all evidence for a specific property
func (es *EvidenceSet) ByProperty(cat Category, prop string) []Evidence {
	var result []Evidence
	for _, e := range es.items {
		if e.Category == cat && e.Property == prop {
			result = append(result, e)
		}
	}
	return result
}

// BestValue returns the highest-confidence value for a property
func (es *EvidenceSet) BestValue(cat Category, prop string) (any, float64, bool) {
	var best Evidence
	var found bool

	for _, e := range es.items {
		if e.Category == cat && e.Property == prop {
			if !found || e.Confidence > best.Confidence {
				best = e
				found = true
			}
		}
	}

	if !found {
		return nil, 0, false
	}
	return best.Value, best.Confidence, true
}

// HasProperty returns true if any evidence exists for the property
func (es *EvidenceSet) HasProperty(cat Category, prop string) bool {
	for _, e := range es.items {
		if e.Category == cat && e.Property == prop {
			return true
		}
	}
	return false
}

// AggregateConfidence combines evidence for the same property
// Uses: max(confidences) + diminishing bonus for corroboration
func (es *EvidenceSet) AggregateConfidence(cat Category, prop string) float64 {
	var confidences []float64
	for _, e := range es.items {
		if e.Category == cat && e.Property == prop {
			confidences = append(confidences, e.Confidence)
		}
	}

	if len(confidences) == 0 {
		return 0
	}

	// Sort descending
	sort.Sort(sort.Reverse(sort.Float64Slice(confidences)))

	// Start with max, add diminishing bonus for corroboration
	result := confidences[0]
	for i := 1; i < len(confidences); i++ {
		// Each additional source adds 10% of its confidence, decaying
		bonus := confidences[i] * 0.1 / float64(i)
		result += bonus
	}

	// Cap at 0.99 (never claim absolute certainty)
	if result > 0.99 {
		result = 0.99
	}

	return result
}

// Summary returns a map of best values per category/property
func (es *EvidenceSet) Summary() map[string]map[string]any {
	result := make(map[string]map[string]any)

	for _, e := range es.items {
		catKey := string(e.Category)
		if result[catKey] == nil {
			result[catKey] = make(map[string]any)
		}
		// Only update if this is higher confidence
		if existing, ok := result[catKey][e.Property]; ok {
			if ev, found := es.findByValue(e.Category, e.Property, existing); found {
				if e.Confidence <= ev.Confidence {
					continue
				}
			}
		}
		result[catKey][e.Property] = e.Value
	}

	return result
}

func (es *EvidenceSet) findByValue(cat Category, prop string, value any) (Evidence, bool) {
	for _, e := range es.items {
		if e.Category == cat && e.Property == prop && e.Value == value {
			return e, true
		}
	}
	return Evidence{}, false
}
