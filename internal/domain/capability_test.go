package domain

import (
	"testing"
	"time"
)

func TestCapability_AddEvidence(t *testing.T) {
	t.Run("adds evidence and recalculates confidence", func(t *testing.T) {
		cap := &Capability{
			Type:       CapabilityKubernetes,
			Properties: make(map[string]any),
		}

		evidence := Evidence{
			ID:         "ev1",
			Source:     EvidenceSourceK8sAPI,
			Property:   "is_k8s_node",
			Value:      true,
			Confidence: EvidenceConfidence[EvidenceSourceK8sAPI],
			ObservedAt: time.Now(),
		}

		cap.AddEvidence(evidence)

		if len(cap.Evidence) != 1 {
			t.Errorf("expected 1 evidence, got %d", len(cap.Evidence))
		}
		if cap.Confidence != EvidenceConfidence[EvidenceSourceK8sAPI] {
			t.Errorf("expected confidence %f, got %f", EvidenceConfidence[EvidenceSourceK8sAPI], cap.Confidence)
		}
	})

	t.Run("multiple evidence pieces increase confidence", func(t *testing.T) {
		cap := &Capability{
			Type:       CapabilityKubernetes,
			Properties: make(map[string]any),
		}

		// Add primary evidence
		ev1 := Evidence{
			ID:         "ev1",
			Source:     EvidenceSourceK8sAPI,
			Property:   "is_k8s_node",
			Value:      true,
			Confidence: EvidenceConfidence[EvidenceSourceK8sAPI],
			ObservedAt: time.Now(),
		}
		cap.AddEvidence(ev1)

		initialConfidence := cap.Confidence

		// Add corroborating evidence
		ev2 := Evidence{
			ID:         "ev2",
			Source:     EvidenceSourceProcessList,
			Property:   "kubelet_running",
			Value:      true,
			Confidence: EvidenceConfidence[EvidenceSourceProcessList],
			ObservedAt: time.Now(),
		}
		cap.AddEvidence(ev2)

		if cap.Confidence <= initialConfidence {
			t.Errorf("expected confidence to increase, got %f (was %f)", cap.Confidence, initialConfidence)
		}
		if cap.Confidence > 1.0 {
			t.Errorf("confidence should not exceed 1.0, got %f", cap.Confidence)
		}
	})
}

func TestCapability_ConfidenceCalculation(t *testing.T) {
	t.Run("uses max confidence from evidence", func(t *testing.T) {
		cap := &Capability{
			Type:       CapabilityDocker,
			Properties: make(map[string]any),
		}

		// Add lower confidence evidence first
		cap.AddEvidence(Evidence{
			Source:     EvidenceSourcePortScan,
			Confidence: EvidenceConfidence[EvidenceSourcePortScan],
			ObservedAt: time.Now(),
		})

		lowConfidence := cap.Confidence

		// Add higher confidence evidence
		cap.AddEvidence(Evidence{
			Source:     EvidenceSourceDockerAPI,
			Confidence: EvidenceConfidence[EvidenceSourceDockerAPI],
			ObservedAt: time.Now(),
		})

		if cap.Confidence <= lowConfidence {
			t.Errorf("expected confidence to jump to higher value, got %f (was %f)", cap.Confidence, lowConfidence)
		}
		// Should be at least the max confidence
		maxConf := EvidenceConfidence[EvidenceSourceDockerAPI]
		if cap.Confidence < maxConf {
			t.Errorf("expected confidence >= %f, got %f", maxConf, cap.Confidence)
		}
	})

	t.Run("bonus from corroborating evidence", func(t *testing.T) {
		cap := &Capability{
			Type:       CapabilitySSH,
			Properties: make(map[string]any),
		}

		// Add primary evidence
		cap.AddEvidence(Evidence{
			Source:     EvidenceSourceSSHProbe,
			Confidence: EvidenceConfidence[EvidenceSourceSSHProbe],
			ObservedAt: time.Now(),
		})

		singleEvidence := cap.Confidence

		// Add multiple corroborating pieces
		cap.AddEvidence(Evidence{
			Source:     EvidenceSourceBanner,
			Confidence: EvidenceConfidence[EvidenceSourceBanner],
			ObservedAt: time.Now(),
		})
		cap.AddEvidence(Evidence{
			Source:     EvidenceSourcePortScan,
			Confidence: EvidenceConfidence[EvidenceSourcePortScan],
			ObservedAt: time.Now(),
		})

		if cap.Confidence <= singleEvidence {
			t.Errorf("expected corroborating evidence to add bonus, got %f (was %f)", cap.Confidence, singleEvidence)
		}
	})

	t.Run("confidence never exceeds 1.0", func(t *testing.T) {
		cap := &Capability{
			Type:       CapabilityKubernetes,
			Properties: make(map[string]any),
		}

		// Add operator truth (1.0 confidence)
		cap.AddEvidence(Evidence{
			Source:     EvidenceSourceOperator,
			Confidence: EvidenceConfidence[EvidenceSourceOperator],
			ObservedAt: time.Now(),
		})

		// Add many corroborating pieces
		for i := 0; i < 10; i++ {
			cap.AddEvidence(Evidence{
				Source:     EvidenceSourceK8sAPI,
				Confidence: EvidenceConfidence[EvidenceSourceK8sAPI],
				ObservedAt: time.Now(),
			})
		}

		if cap.Confidence > 1.0 {
			t.Errorf("confidence exceeded 1.0: %f", cap.Confidence)
		}
	})
}

func TestCapability_ConfidenceStatus(t *testing.T) {
	tests := []struct {
		name               string
		confidence         float64
		expectedStatus     string
	}{
		{"confirmed at 1.0", 1.0, "confirmed"},
		{"confirmed at 0.9", 0.9, "confirmed"},
		{"confirmed at 0.7", 0.7, "confirmed"},
		{"probable at 0.69", 0.69, "probable"},
		{"probable at 0.5", 0.5, "probable"},
		{"probable at 0.4", 0.4, "probable"},
		{"speculative at 0.39", 0.39, "speculative"},
		{"speculative at 0.2", 0.2, "speculative"},
		{"speculative at 0.0", 0.0, "speculative"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cap := &Capability{
				Type:       CapabilityKubernetes,
				Confidence: tt.confidence,
				Properties: make(map[string]any),
			}

			status := cap.ConfidenceStatus()
			if status != tt.expectedStatus {
				t.Errorf("expected status %s, got %s", tt.expectedStatus, status)
			}
		})
	}
}

func TestNode_AddEvidence(t *testing.T) {
	t.Run("creates capability if it doesn't exist", func(t *testing.T) {
		node := NewNode("test", NodeTypeServer, "Test Server")

		evidence := Evidence{
			ID:         "ev1",
			Source:     EvidenceSourceK8sAPI,
			Property:   "is_k8s_node",
			Value:      true,
			Confidence: EvidenceConfidence[EvidenceSourceK8sAPI],
			ObservedAt: time.Now(),
		}

		node.AddEvidence(CapabilityKubernetes, evidence)

		cap := node.GetCapability(CapabilityKubernetes)
		if cap == nil {
			t.Fatal("expected capability to be created")
		}
		if cap.Type != CapabilityKubernetes {
			t.Errorf("expected type %s, got %s", CapabilityKubernetes, cap.Type)
		}
		if len(cap.Evidence) != 1 {
			t.Errorf("expected 1 evidence, got %d", len(cap.Evidence))
		}
	})

	t.Run("adds to existing capability", func(t *testing.T) {
		node := NewNode("test", NodeTypeServer, "Test Server")

		// Add first evidence
		ev1 := Evidence{
			ID:         "ev1",
			Source:     EvidenceSourceDockerAPI,
			Property:   "has_docker",
			Value:      true,
			Confidence: EvidenceConfidence[EvidenceSourceDockerAPI],
			ObservedAt: time.Now(),
		}
		node.AddEvidence(CapabilityDocker, ev1)

		// Add second evidence
		ev2 := Evidence{
			ID:         "ev2",
			Source:     EvidenceSourceProcessList,
			Property:   "dockerd_running",
			Value:      true,
			Confidence: EvidenceConfidence[EvidenceSourceProcessList],
			ObservedAt: time.Now(),
		}
		node.AddEvidence(CapabilityDocker, ev2)

		cap := node.GetCapability(CapabilityDocker)
		if cap == nil {
			t.Fatal("expected capability to exist")
		}
		if len(cap.Evidence) != 2 {
			t.Errorf("expected 2 evidence pieces, got %d", len(cap.Evidence))
		}
	})

	t.Run("initializes capabilities map if nil", func(t *testing.T) {
		node := &Node{
			ID:    "test",
			Type:  NodeTypeServer,
			Label: "Test",
		}

		if node.Capabilities != nil {
			t.Error("expected Capabilities to be nil initially")
		}

		evidence := Evidence{
			Source:     EvidenceSourceSSHProbe,
			Confidence: EvidenceConfidence[EvidenceSourceSSHProbe],
			ObservedAt: time.Now(),
		}
		node.AddEvidence(CapabilitySSH, evidence)

		if node.Capabilities == nil {
			t.Error("expected Capabilities to be initialized")
		}
	})
}

func TestNode_GetCapability(t *testing.T) {
	t.Run("returns nil for non-existent capability", func(t *testing.T) {
		node := NewNode("test", NodeTypeServer, "Test")

		cap := node.GetCapability(CapabilityKubernetes)
		if cap != nil {
			t.Errorf("expected nil, got %v", cap)
		}
	})

	t.Run("returns capability when it exists", func(t *testing.T) {
		node := NewNode("test", NodeTypeServer, "Test")
		node.AddEvidence(CapabilityDocker, Evidence{
			Source:     EvidenceSourceDockerAPI,
			Confidence: EvidenceConfidence[EvidenceSourceDockerAPI],
			ObservedAt: time.Now(),
		})

		cap := node.GetCapability(CapabilityDocker)
		if cap == nil {
			t.Error("expected capability to be returned")
		}
		if cap.Type != CapabilityDocker {
			t.Errorf("expected type %s, got %s", CapabilityDocker, cap.Type)
		}
	})

	t.Run("returns nil when capabilities map is nil", func(t *testing.T) {
		node := &Node{
			ID:           "test",
			Type:         NodeTypeServer,
			Label:        "Test",
			Capabilities: nil,
		}

		cap := node.GetCapability(CapabilitySSH)
		if cap != nil {
			t.Errorf("expected nil, got %v", cap)
		}
	})
}

func TestNode_GetConfidence(t *testing.T) {
	t.Run("returns 0 for non-existent capability", func(t *testing.T) {
		node := NewNode("test", NodeTypeServer, "Test")

		conf := node.GetConfidence(CapabilityKubernetes)
		if conf != 0 {
			t.Errorf("expected 0, got %f", conf)
		}
	})

	t.Run("returns confidence when capability exists", func(t *testing.T) {
		node := NewNode("test", NodeTypeServer, "Test")
		expectedConf := EvidenceConfidence[EvidenceSourceK8sAPI]

		node.AddEvidence(CapabilityKubernetes, Evidence{
			Source:     EvidenceSourceK8sAPI,
			Confidence: expectedConf,
			ObservedAt: time.Now(),
		})

		conf := node.GetConfidence(CapabilityKubernetes)
		if conf != expectedConf {
			t.Errorf("expected %f, got %f", expectedConf, conf)
		}
	})
}

func TestNode_HasCapability(t *testing.T) {
	t.Run("returns false for non-existent capability", func(t *testing.T) {
		node := NewNode("test", NodeTypeServer, "Test")

		if node.HasCapability(CapabilityKubernetes, 0.5) {
			t.Error("expected false for non-existent capability")
		}
	})

	t.Run("returns true when confidence meets threshold", func(t *testing.T) {
		node := NewNode("test", NodeTypeServer, "Test")
		node.AddEvidence(CapabilityDocker, Evidence{
			Source:     EvidenceSourceDockerAPI,
			Confidence: 0.95,
			ObservedAt: time.Now(),
		})

		if !node.HasCapability(CapabilityDocker, 0.7) {
			t.Error("expected true when confidence exceeds threshold")
		}
		if !node.HasCapability(CapabilityDocker, 0.95) {
			t.Error("expected true when confidence equals threshold")
		}
	})

	t.Run("returns false when confidence below threshold", func(t *testing.T) {
		node := NewNode("test", NodeTypeServer, "Test")
		node.AddEvidence(CapabilitySSH, Evidence{
			Source:     EvidenceSourcePortScan,
			Confidence: 0.5,
			ObservedAt: time.Now(),
		})

		if node.HasCapability(CapabilitySSH, 0.7) {
			t.Error("expected false when confidence below threshold")
		}
	})

	t.Run("threshold of 0.0 returns true for any evidence", func(t *testing.T) {
		node := NewNode("test", NodeTypeServer, "Test")
		node.AddEvidence(CapabilityHTTP, Evidence{
			Source:     EvidenceSourcePortScan,
			Confidence: 0.1,
			ObservedAt: time.Now(),
		})

		if !node.HasCapability(CapabilityHTTP, 0.0) {
			t.Error("expected true with 0.0 threshold")
		}
	})
}

func TestEvidenceConfidence(t *testing.T) {
	t.Run("operator has highest confidence", func(t *testing.T) {
		if EvidenceConfidence[EvidenceSourceOperator] != 1.0 {
			t.Errorf("expected operator confidence to be 1.0, got %f", EvidenceConfidence[EvidenceSourceOperator])
		}
	})

	t.Run("all confidence values are between 0 and 1", func(t *testing.T) {
		for source, confidence := range EvidenceConfidence {
			if confidence < 0 || confidence > 1 {
				t.Errorf("source %s has invalid confidence %f (must be 0-1)", source, confidence)
			}
		}
	})

	t.Run("API sources have high confidence", func(t *testing.T) {
		sources := []EvidenceSource{
			EvidenceSourceK8sAPI,
			EvidenceSourceDockerAPI,
		}
		for _, source := range sources {
			if EvidenceConfidence[source] < 0.9 {
				t.Errorf("expected API source %s to have confidence >= 0.9, got %f", source, EvidenceConfidence[source])
			}
		}
	})

	t.Run("port scan has lower confidence than authenticated probes", func(t *testing.T) {
		portScanConf := EvidenceConfidence[EvidenceSourcePortScan]
		sshProbeConf := EvidenceConfidence[EvidenceSourceSSHProbe]
		if portScanConf >= sshProbeConf {
			t.Errorf("expected port scan (%f) < ssh probe (%f)", portScanConf, sshProbeConf)
		}
	})
}

func TestCapability_RecalculateConfidence(t *testing.T) {
	t.Run("zero evidence gives zero confidence", func(t *testing.T) {
		cap := &Capability{
			Type:       CapabilityKubernetes,
			Properties: make(map[string]any),
			Evidence:   []Evidence{},
		}

		cap.recalculateConfidence()

		if cap.Confidence != 0 {
			t.Errorf("expected 0 confidence with no evidence, got %f", cap.Confidence)
		}
		if cap.Status != "speculative" {
			t.Errorf("expected speculative status, got %s", cap.Status)
		}
	})

	t.Run("updates status based on confidence", func(t *testing.T) {
		cap := &Capability{
			Type:       CapabilityDocker,
			Properties: make(map[string]any),
		}

		// Add low confidence evidence
		cap.AddEvidence(Evidence{
			Source:     EvidenceSourceCorrelation,
			Confidence: 0.3,
			ObservedAt: time.Now(),
		})
		if cap.Status != "speculative" {
			t.Errorf("expected speculative status, got %s", cap.Status)
		}

		// Add medium confidence evidence
		cap.AddEvidence(Evidence{
			Source:     EvidenceSourceBanner,
			Confidence: 0.6,
			ObservedAt: time.Now(),
		})
		if cap.Status != "probable" {
			t.Errorf("expected probable status, got %s", cap.Status)
		}

		// Add high confidence evidence
		cap.AddEvidence(Evidence{
			Source:     EvidenceSourceDockerAPI,
			Confidence: 0.95,
			ObservedAt: time.Now(),
		})
		if cap.Status != "confirmed" {
			t.Errorf("expected confirmed status, got %s", cap.Status)
		}
	})
}
