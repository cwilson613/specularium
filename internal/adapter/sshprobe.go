package adapter

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"specularium/internal/domain"
)

// SSHProbeAdapter performs SSH-based fact gathering on discovered hosts
// It uses stored SSH credentials to connect and run lightweight commands
type SSHProbeAdapter struct {
	secrets   SecretResolver
	publisher EventPublisher
	interval  time.Duration
	timeout   time.Duration
	commands  []FactCommand
	mu        sync.Mutex
	running   bool
}

// SSHProbeConfig holds configuration for the SSH probe adapter
type SSHProbeConfig struct {
	// Interval between probe cycles
	Interval time.Duration
	// Timeout for SSH connections
	ConnectionTimeout time.Duration
	// Timeout for command execution
	CommandTimeout time.Duration
	// MaxConcurrent limits parallel SSH sessions
	MaxConcurrent int
	// Commands to run for fact gathering
	Commands []FactCommand
}

// DefaultSSHProbeConfig returns sensible defaults
func DefaultSSHProbeConfig() SSHProbeConfig {
	return SSHProbeConfig{
		Interval:          10 * time.Minute,
		ConnectionTimeout: 10 * time.Second,
		CommandTimeout:    30 * time.Second,
		MaxConcurrent:     5,
		Commands:          DefaultFactCommands,
	}
}

// NewSSHProbeAdapter creates a new SSH probe adapter
func NewSSHProbeAdapter(secrets SecretResolver, config SSHProbeConfig) *SSHProbeAdapter {
	if config.Interval == 0 {
		config.Interval = 10 * time.Minute
	}
	if config.ConnectionTimeout == 0 {
		config.ConnectionTimeout = 10 * time.Second
	}
	if config.CommandTimeout == 0 {
		config.CommandTimeout = 30 * time.Second
	}
	if config.MaxConcurrent == 0 {
		config.MaxConcurrent = 5
	}
	if len(config.Commands) == 0 {
		config.Commands = DefaultFactCommands
	}

	return &SSHProbeAdapter{
		secrets:  secrets,
		interval: config.Interval,
		timeout:  config.ConnectionTimeout,
		commands: config.Commands,
	}
}

// SetEventPublisher sets the event publisher for progress updates
func (s *SSHProbeAdapter) SetEventPublisher(pub EventPublisher) {
	s.publisher = pub
}

// Name returns the adapter identifier
func (s *SSHProbeAdapter) Name() string {
	return "sshprobe"
}

// Type returns the adapter type
func (s *SSHProbeAdapter) Type() AdapterType {
	return AdapterTypePolling
}

// Priority returns the adapter priority
func (s *SSHProbeAdapter) Priority() int {
	return 60 // Lower priority than verifier, runs after basic discovery
}

// Start initializes the adapter
func (s *SSHProbeAdapter) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = true
	log.Printf("SSH probe adapter started (interval=%s, timeout=%s, commands=%d)",
		s.interval, s.timeout, len(s.commands))
	return nil
}

// Stop shuts down the adapter
func (s *SSHProbeAdapter) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	log.Printf("SSH probe adapter stopped")
	return nil
}

// Sync probes nodes with SSH and gathers facts
func (s *SSHProbeAdapter) Sync(ctx context.Context) (*domain.GraphFragment, error) {
	// Get SSH credentials
	sshSecrets, err := s.getSSHSecrets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get SSH secrets: %w", err)
	}

	if len(sshSecrets) == 0 {
		log.Printf("SSH probe: No SSH secrets configured, skipping")
		return nil, nil
	}

	log.Printf("SSH probe: Found %d SSH secret(s) to use", len(sshSecrets))

	// For now, return nil - this will be called periodically by the registry
	// A more complete implementation would query a node fetcher for nodes with port 22 open
	// and attempt to connect to them
	return nil, nil
}

// ProbeNode probes a single node via SSH and returns gathered evidence
// This is the main entry point for SSH probing a specific node
func (s *SSHProbeAdapter) ProbeNode(ctx context.Context, node domain.Node) (*domain.GraphFragment, error) {
	fragment := domain.NewGraphFragment()

	// Check if node has port 22 open
	openPorts, ok := node.GetDiscovered("open_ports")
	if !ok {
		log.Printf("SSH probe: Node %s has no discovered open_ports, skipping", node.ID)
		return nil, nil
	}

	hasSSH := false
	if ports, ok := openPorts.([]int); ok {
		for _, port := range ports {
			if port == 22 {
				hasSSH = true
				break
			}
		}
	}

	if !hasSSH {
		log.Printf("SSH probe: Node %s does not have port 22 open, skipping", node.ID)
		return nil, nil
	}

	// Get IP address
	ip := node.GetPropertyString("ip")
	if ip == "" {
		log.Printf("SSH probe: Node %s has no IP address, skipping", node.ID)
		return nil, nil
	}

	// Get SSH credentials
	sshSecrets, err := s.getSSHSecrets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get SSH secrets: %w", err)
	}

	if len(sshSecrets) == 0 {
		log.Printf("SSH probe: No SSH secrets configured")
		return nil, nil
	}

	// Try each SSH credential until one works
	var lastErr error
	for _, secret := range sshSecrets {
		log.Printf("SSH probe: Attempting connection to %s (%s) with secret %s",
			node.ID, ip, secret.ID)

		evidence, capabilities, err := s.probeWithSecret(ctx, ip, secret)
		if err != nil {
			log.Printf("SSH probe: Failed to connect to %s with secret %s: %v",
				ip, secret.ID, err)
			lastErr = err
			continue
		}

		// Success! Update node with evidence and capabilities
		updatedNode := node
		updatedNode.Source = "sshprobe"
		now := time.Now()
		updatedNode.LastVerified = &now
		updatedNode.LastSeen = &now
		updatedNode.Status = domain.NodeStatusVerified

		// Add evidence to discovered data
		if updatedNode.Discovered == nil {
			updatedNode.Discovered = make(map[string]any)
		}

		// Store facts
		for _, ev := range evidence {
			updatedNode.Discovered[ev.Property] = ev.Value
		}

		// Store capabilities as a structured field
		if len(capabilities) > 0 {
			updatedNode.Discovered["capabilities"] = capabilities
		}

		fragment.AddNode(updatedNode)

		log.Printf("SSH probe: Successfully gathered %d facts and %d capabilities from %s",
			len(evidence), len(capabilities), node.ID)

		// Emit progress event
		s.publishProgress("discovery-progress", map[string]interface{}{
			"node_id":      node.ID,
			"ip":           ip,
			"facts":        len(evidence),
			"capabilities": len(capabilities),
			"secret_id":    secret.ID,
			"message":      fmt.Sprintf("SSH probe: Gathered %d facts from %s", len(evidence), node.ID),
		})

		return fragment, nil
	}

	// None of the credentials worked
	if lastErr != nil {
		log.Printf("SSH probe: All credentials failed for %s: %v", node.ID, lastErr)
	}

	return nil, nil
}

// probeWithSecret attempts to connect and gather facts using a specific secret
func (s *SSHProbeAdapter) probeWithSecret(ctx context.Context, ip string, secret *domain.Secret) ([]domain.Evidence, []domain.Capability, error) {
	// Connect to host
	client, err := s.connect(ctx, ip, 22, secret)
	if err != nil {
		return nil, nil, fmt.Errorf("connection failed: %w", err)
	}
	defer client.Close()

	now := time.Now()
	evidence := []domain.Evidence{}
	capabilities := []domain.Capability{}

	// Run fact commands
	for _, factCmd := range s.commands {
		output, err := s.runCommand(client, factCmd.Command)
		if err != nil {
			log.Printf("SSH probe: Command '%s' failed on %s: %v", factCmd.Name, ip, err)
			continue
		}

		// Parse output
		facts, err := factCmd.Parser(output)
		if err != nil {
			log.Printf("SSH probe: Failed to parse '%s' output from %s: %v", factCmd.Name, ip, err)
			continue
		}

		// Convert facts to evidence
		for key, value := range facts {
			ev := domain.Evidence{
				ID:         fmt.Sprintf("%s-%s-%d", ip, key, now.Unix()),
				Source:     domain.EvidenceSourceSSHProbe,
				Property:   key,
				Value:      value,
				Confidence: domain.EvidenceConfidence[domain.EvidenceSourceSSHProbe],
				ObservedAt: now,
				SecretRef:  secret.ID,
				Raw: map[string]any{
					"command": factCmd.Command,
					"output":  output,
				},
			}
			evidence = append(evidence, ev)
		}
	}

	// Detect capabilities based on gathered facts
	capabilities = s.detectCapabilities(evidence, secret.ID, now)

	// Always add SSH capability since we successfully connected
	sshCap := domain.Capability{
		Type:       domain.CapabilitySSH,
		Confidence: 1.0,
		Status:     "confirmed",
		Properties: map[string]any{
			"port":      22,
			"accessible": true,
			"secret_ref": secret.ID,
		},
	}
	sshCap.AddEvidence(domain.Evidence{
		ID:         fmt.Sprintf("%s-ssh-access-%d", ip, now.Unix()),
		Source:     domain.EvidenceSourceSSHProbe,
		Property:   "ssh_accessible",
		Value:      true,
		Confidence: 1.0,
		ObservedAt: now,
		SecretRef:  secret.ID,
	})
	capabilities = append(capabilities, sshCap)

	return evidence, capabilities, nil
}

// detectCapabilities analyzes evidence to detect node capabilities
func (s *SSHProbeAdapter) detectCapabilities(evidence []domain.Evidence, secretRef string, now time.Time) []domain.Capability {
	capabilities := []domain.Capability{}

	// Check for Docker
	for _, ev := range evidence {
		if ev.Property == "has_docker" {
			if hasdocker, ok := ev.Value.(bool); ok && hasdocker {
				dockerCap := domain.Capability{
					Type:       domain.CapabilityDocker,
					Confidence: domain.EvidenceConfidence[domain.EvidenceSourceSSHProbe],
					Status:     "confirmed",
					Properties: make(map[string]any),
				}
				dockerCap.AddEvidence(ev)
				capabilities = append(capabilities, dockerCap)
			}
		}
	}

	// Check for Kubernetes
	for _, ev := range evidence {
		if ev.Property == "has_k8s" {
			if hask8s, ok := ev.Value.(bool); ok && hask8s {
				k8sCap := domain.Capability{
					Type:       domain.CapabilityKubernetes,
					Confidence: domain.EvidenceConfidence[domain.EvidenceSourceSSHProbe],
					Status:     "confirmed",
					Properties: make(map[string]any),
				}

				// Add distribution if available
				for _, ev2 := range evidence {
					if ev2.Property == "k8s_distribution" {
						if dist, ok := ev2.Value.(string); ok {
							k8sCap.Properties["distribution"] = dist
						}
					}
				}

				k8sCap.AddEvidence(ev)
				capabilities = append(capabilities, k8sCap)
			}
		}
	}

	return capabilities
}

// getSSHSecrets retrieves all configured SSH secrets
func (s *SSHProbeAdapter) getSSHSecrets(ctx context.Context) ([]*domain.Secret, error) {
	var secrets []*domain.Secret

	// Get SSH key secrets
	keySecrets, err := s.secrets.ListSecrets(ctx, string(domain.SecretTypeSSHKey), "")
	if err != nil {
		return nil, err
	}

	for _, summary := range keySecrets {
		secret, err := s.secrets.GetSecret(ctx, summary.ID)
		if err != nil {
			log.Printf("Failed to get SSH key secret %s: %v", summary.ID, err)
			continue
		}
		if secret != nil {
			secrets = append(secrets, secret)
		}
	}

	// Get SSH password secrets
	pwdSecrets, err := s.secrets.ListSecrets(ctx, string(domain.SecretTypeSSHPassword), "")
	if err != nil {
		return nil, err
	}

	for _, summary := range pwdSecrets {
		secret, err := s.secrets.GetSecret(ctx, summary.ID)
		if err != nil {
			log.Printf("Failed to get SSH password secret %s: %v", summary.ID, err)
			continue
		}
		if secret != nil {
			secrets = append(secrets, secret)
		}
	}

	return secrets, nil
}

// publishProgress emits a discovery progress event
func (s *SSHProbeAdapter) publishProgress(eventType string, payload interface{}) {
	if s.publisher != nil {
		s.publisher.PublishDiscoveryEvent(eventType, payload)
	}
}
