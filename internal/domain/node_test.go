package domain

import (
	"testing"
	"time"
)

func TestNewNode(t *testing.T) {
	t.Run("creates node with defaults", func(t *testing.T) {
		node := NewNode("test-id", NodeTypeServer, "Test Server")

		if node.ID != "test-id" {
			t.Errorf("expected ID 'test-id', got %s", node.ID)
		}
		if node.Type != NodeTypeServer {
			t.Errorf("expected type %s, got %s", NodeTypeServer, node.Type)
		}
		if node.Label != "Test Server" {
			t.Errorf("expected label 'Test Server', got %s", node.Label)
		}
		if node.Status != NodeStatusUnverified {
			t.Errorf("expected status %s, got %s", NodeStatusUnverified, node.Status)
		}
		if node.Properties == nil {
			t.Error("expected Properties to be initialized")
		}
		if node.Discovered == nil {
			t.Error("expected Discovered to be initialized")
		}
		if node.CreatedAt.IsZero() {
			t.Error("expected CreatedAt to be set")
		}
		if node.UpdatedAt.IsZero() {
			t.Error("expected UpdatedAt to be set")
		}
	})
}

func TestNodeIsInterface(t *testing.T) {
	t.Run("returns false for standalone node", func(t *testing.T) {
		node := NewNode("standalone", NodeTypeServer, "Standalone")
		if node.IsInterface() {
			t.Error("expected IsInterface to return false for node without parent")
		}
	})

	t.Run("returns true for child node", func(t *testing.T) {
		node := NewNode("child", NodeTypeInterface, "eth0")
		node.ParentID = "parent"
		if !node.IsInterface() {
			t.Error("expected IsInterface to return true for node with parent")
		}
	})
}

func TestNodeSetGetProperty(t *testing.T) {
	node := NewNode("test", NodeTypeServer, "Test")

	t.Run("set and get string property", func(t *testing.T) {
		node.SetProperty("ip", "192.168.1.1")
		val, ok := node.GetProperty("ip")
		if !ok {
			t.Error("expected property to exist")
		}
		if val != "192.168.1.1" {
			t.Errorf("expected '192.168.1.1', got %v", val)
		}
	})

	t.Run("get non-existent property", func(t *testing.T) {
		_, ok := node.GetProperty("nonexistent")
		if ok {
			t.Error("expected property not to exist")
		}
	})

	t.Run("set property on nil map initializes map", func(t *testing.T) {
		node := &Node{}
		node.SetProperty("key", "value")
		if node.Properties == nil {
			t.Error("expected Properties to be initialized")
		}
	})
}

func TestGetPropertyString(t *testing.T) {
	node := NewNode("test", NodeTypeServer, "Test")

	t.Run("returns string value", func(t *testing.T) {
		node.SetProperty("hostname", "server1")
		val := node.GetPropertyString("hostname")
		if val != "server1" {
			t.Errorf("expected 'server1', got %s", val)
		}
	})

	t.Run("returns empty string for non-string value", func(t *testing.T) {
		node.SetProperty("port", 8080)
		val := node.GetPropertyString("port")
		if val != "" {
			t.Errorf("expected empty string, got %s", val)
		}
	})

	t.Run("returns empty string for non-existent property", func(t *testing.T) {
		val := node.GetPropertyString("nonexistent")
		if val != "" {
			t.Errorf("expected empty string, got %s", val)
		}
	})
}

func TestNodeSetGetDiscovered(t *testing.T) {
	node := NewNode("test", NodeTypeServer, "Test")

	t.Run("set and get discovered value", func(t *testing.T) {
		node.SetDiscovered("hostname", "discovered-host")
		val, ok := node.GetDiscovered("hostname")
		if !ok {
			t.Error("expected discovered value to exist")
		}
		if val != "discovered-host" {
			t.Errorf("expected 'discovered-host', got %v", val)
		}
	})

	t.Run("set discovered on nil map initializes map", func(t *testing.T) {
		node := &Node{}
		node.SetDiscovered("key", "value")
		if node.Discovered == nil {
			t.Error("expected Discovered to be initialized")
		}
	})
}

func TestHostnameInference(t *testing.T) {
	t.Run("add single candidate", func(t *testing.T) {
		inference := &HostnameInference{}
		now := time.Now()
		inference.AddCandidate("server1.local", SourcePTR, now)

		if len(inference.Candidates) != 1 {
			t.Errorf("expected 1 candidate, got %d", len(inference.Candidates))
		}
		if inference.Best == nil {
			t.Fatal("expected Best to be set")
		}
		if inference.Best.Hostname != "server1.local" {
			t.Errorf("expected 'server1.local', got %s", inference.Best.Hostname)
		}
		if inference.Best.Confidence != ConfidenceScores[SourcePTR] {
			t.Errorf("expected confidence %f, got %f", ConfidenceScores[SourcePTR], inference.Best.Confidence)
		}
	})

	t.Run("add multiple candidates selects highest confidence", func(t *testing.T) {
		inference := &HostnameInference{}
		now := time.Now()

		inference.AddCandidate("ip-derived", SourceIPDerived, now)
		inference.AddCandidate("ptr-hostname", SourcePTR, now)
		inference.AddCandidate("import-hostname", SourceImport, now)

		if len(inference.Candidates) != 3 {
			t.Errorf("expected 3 candidates, got %d", len(inference.Candidates))
		}
		if inference.Best.Hostname != "ptr-hostname" {
			t.Errorf("expected 'ptr-hostname' to have highest confidence, got %s", inference.Best.Hostname)
		}
		if inference.Best.Source != SourcePTR {
			t.Errorf("expected source %s, got %s", SourcePTR, inference.Best.Source)
		}
	})

	t.Run("update existing candidate from same source", func(t *testing.T) {
		inference := &HostnameInference{}
		now := time.Now()

		inference.AddCandidate("hostname1", SourcePTR, now)
		if len(inference.Candidates) != 1 {
			t.Errorf("expected 1 candidate after first add, got %d", len(inference.Candidates))
		}

		// Add same hostname from same source
		inference.AddCandidate("hostname1", SourcePTR, now.Add(time.Minute))
		if len(inference.Candidates) != 1 {
			t.Errorf("expected 1 candidate after update, got %d", len(inference.Candidates))
		}
		if !inference.Candidates[0].ObservedAt.Equal(now.Add(time.Minute)) {
			t.Error("expected ObservedAt to be updated")
		}
	})

	t.Run("ignore empty hostname", func(t *testing.T) {
		inference := &HostnameInference{}
		now := time.Now()

		inference.AddCandidate("", SourcePTR, now)
		if len(inference.Candidates) != 0 {
			t.Errorf("expected 0 candidates, got %d", len(inference.Candidates))
		}
	})

	t.Run("normalizes hostname", func(t *testing.T) {
		inference := &HostnameInference{}
		now := time.Now()

		inference.AddCandidate("  Server1.LOCAL  ", SourcePTR, now)
		if len(inference.Candidates) != 1 {
			t.Fatalf("expected 1 candidate, got %d", len(inference.Candidates))
		}
		if inference.Candidates[0].Hostname != "server1.local" {
			t.Errorf("expected normalized hostname 'server1.local', got %s", inference.Candidates[0].Hostname)
		}
	})

	t.Run("GetBestHostname returns empty for no candidates", func(t *testing.T) {
		inference := &HostnameInference{}
		hostname := inference.GetBestHostname()
		if hostname != "" {
			t.Errorf("expected empty string, got %s", hostname)
		}
	})

	t.Run("GetBestConfidence returns zero for no candidates", func(t *testing.T) {
		inference := &HostnameInference{}
		confidence := inference.GetBestConfidence()
		if confidence != 0 {
			t.Errorf("expected 0, got %f", confidence)
		}
	})
}

func TestExtractShortName(t *testing.T) {
	tests := []struct {
		name     string
		fqdn     string
		expected string
	}{
		{"FQDN with domain", "server1.local", "server1"},
		{"FQDN with multiple levels", "server1.subdomain.example.com", "server1"},
		{"short hostname", "server1", "server1"},
		{"empty string", "", ""},
		{"single character", "s", "s"},
		{"single dot", ".", "."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractShortName(tt.fqdn)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestConfidenceScores(t *testing.T) {
	t.Run("operator truth has highest confidence", func(t *testing.T) {
		if ConfidenceScores[SourceOperatorTruth] != 1.0 {
			t.Errorf("expected operator truth confidence to be 1.0, got %f", ConfidenceScores[SourceOperatorTruth])
		}
	})

	t.Run("all scores are between 0 and 1", func(t *testing.T) {
		for source, score := range ConfidenceScores {
			if score < 0 || score > 1 {
				t.Errorf("source %s has invalid confidence %f (must be 0-1)", source, score)
			}
		}
	})

	t.Run("PTR has high confidence", func(t *testing.T) {
		if ConfidenceScores[SourcePTR] < 0.9 {
			t.Errorf("expected PTR confidence >= 0.9, got %f", ConfidenceScores[SourcePTR])
		}
	})

	t.Run("unknown has low confidence", func(t *testing.T) {
		if ConfidenceScores[SourceUnknown] > 0.1 {
			t.Errorf("expected unknown confidence <= 0.1, got %f", ConfidenceScores[SourceUnknown])
		}
	})
}
