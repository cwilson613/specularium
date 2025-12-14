package adapter

import (
	"context"
	"fmt"
	"log"

	"specularium/internal/domain"
)

// SecretResolver provides access to secrets for adapters
type SecretResolver interface {
	GetSecret(ctx context.Context, id string) (*domain.Secret, error)
	GetSecretValue(ctx context.Context, id, key string) (string, error)
	ListSecrets(ctx context.Context, secretType string, source string) ([]domain.SecretSummary, error)
}

// CapabilityManager provides capability-based access to discovery features
// It wraps secrets and provides a clean interface for adapters to use
type CapabilityManager struct {
	secrets SecretResolver
}

// NewCapabilityManager creates a new capability manager
func NewCapabilityManager(secrets SecretResolver) *CapabilityManager {
	return &CapabilityManager{
		secrets: secrets,
	}
}

// DNSCapability provides DNS resolution capabilities
type DNSCapability struct {
	Server string
}

// SSHCapability provides SSH connection capabilities
type SSHCapability struct {
	Username   string
	KeyPath    string
	Passphrase string
}

// SNMPv2Capability provides SNMPv2c capabilities
type SNMPv2Capability struct {
	Community string
}

// SNMPv3Capability provides SNMPv3 capabilities
type SNMPv3Capability struct {
	Username     string
	AuthProtocol string
	AuthPassword string
	PrivProtocol string
	PrivPassword string
}

// APICapability provides API access capabilities
type APICapability struct {
	Token   string
	BaseURL string
}

// GetDNSCapability returns DNS capability from configured secrets
func (c *CapabilityManager) GetDNSCapability(ctx context.Context) (*DNSCapability, error) {
	// Look for DNS secrets
	secrets, err := c.secrets.ListSecrets(ctx, string(domain.SecretTypeDNS), "")
	if err != nil {
		return nil, fmt.Errorf("failed to list DNS secrets: %w", err)
	}

	// Try each DNS secret until we find one with a server
	for _, summary := range secrets {
		secret, err := c.secrets.GetSecret(ctx, summary.ID)
		if err != nil {
			log.Printf("Failed to get DNS secret %s: %v", summary.ID, err)
			continue
		}
		if secret == nil {
			continue
		}

		// Look for server value
		server := secret.Data["server"]
		if server == "" {
			server = secret.Data["dns_server"]
		}
		if server == "" {
			server = secret.Data["value"]
		}

		if server != "" {
			log.Printf("DNS capability loaded from secret %s: server=%s", summary.ID, server)
			return &DNSCapability{Server: server}, nil
		}
	}

	return nil, nil // No DNS capability configured
}

// GetSSHCapability returns SSH capability from configured secrets
func (c *CapabilityManager) GetSSHCapability(ctx context.Context) (*SSHCapability, error) {
	// Look for SSH key secrets
	secrets, err := c.secrets.ListSecrets(ctx, string(domain.SecretTypeSSHKey), "")
	if err != nil {
		return nil, fmt.Errorf("failed to list SSH secrets: %w", err)
	}

	// Try each SSH secret
	for _, summary := range secrets {
		secret, err := c.secrets.GetSecret(ctx, summary.ID)
		if err != nil {
			log.Printf("Failed to get SSH secret %s: %v", summary.ID, err)
			continue
		}
		if secret == nil {
			continue
		}

		cap := &SSHCapability{
			Username:   secret.Data["username"],
			KeyPath:    secret.Data["key_path"],
			Passphrase: secret.Data["passphrase"],
		}

		// Need at least a key path or username
		if cap.KeyPath != "" || cap.Username != "" {
			log.Printf("SSH capability loaded from secret %s: user=%s, key=%s",
				summary.ID, cap.Username, cap.KeyPath)
			return cap, nil
		}
	}

	return nil, nil // No SSH capability configured
}

// GetSNMPv2Capability returns SNMPv2c capability from configured secrets
func (c *CapabilityManager) GetSNMPv2Capability(ctx context.Context) (*SNMPv2Capability, error) {
	// Look for SNMP community secrets
	secrets, err := c.secrets.ListSecrets(ctx, string(domain.SecretTypeSNMPCommunity), "")
	if err != nil {
		return nil, fmt.Errorf("failed to list SNMP secrets: %w", err)
	}

	// Try each SNMP secret
	for _, summary := range secrets {
		secret, err := c.secrets.GetSecret(ctx, summary.ID)
		if err != nil {
			log.Printf("Failed to get SNMP secret %s: %v", summary.ID, err)
			continue
		}
		if secret == nil {
			continue
		}

		community := secret.Data["community"]
		if community == "" {
			community = secret.Data["value"]
		}

		if community != "" {
			log.Printf("SNMPv2 capability loaded from secret %s", summary.ID)
			return &SNMPv2Capability{Community: community}, nil
		}
	}

	return nil, nil
}

// GetSNMPv3Capability returns SNMPv3 capability from configured secrets
func (c *CapabilityManager) GetSNMPv3Capability(ctx context.Context) (*SNMPv3Capability, error) {
	// Look for SNMPv3 secrets
	secrets, err := c.secrets.ListSecrets(ctx, string(domain.SecretTypeSNMPv3), "")
	if err != nil {
		return nil, fmt.Errorf("failed to list SNMPv3 secrets: %w", err)
	}

	// Try each SNMPv3 secret
	for _, summary := range secrets {
		secret, err := c.secrets.GetSecret(ctx, summary.ID)
		if err != nil {
			log.Printf("Failed to get SNMPv3 secret %s: %v", summary.ID, err)
			continue
		}
		if secret == nil {
			continue
		}

		cap := &SNMPv3Capability{
			Username:     secret.Data["username"],
			AuthProtocol: secret.Data["auth_protocol"],
			AuthPassword: secret.Data["auth_password"],
			PrivProtocol: secret.Data["priv_protocol"],
			PrivPassword: secret.Data["priv_password"],
		}

		if cap.Username != "" {
			log.Printf("SNMPv3 capability loaded from secret %s: user=%s", summary.ID, cap.Username)
			return cap, nil
		}
	}

	return nil, nil
}

// GetAPICapability returns API token capability for a specific service
func (c *CapabilityManager) GetAPICapability(ctx context.Context, serviceName string) (*APICapability, error) {
	// Look for API token secrets
	secrets, err := c.secrets.ListSecrets(ctx, string(domain.SecretTypeAPIToken), "")
	if err != nil {
		return nil, fmt.Errorf("failed to list API secrets: %w", err)
	}

	// Try each API secret, preferring ones that match the service name
	var fallback *APICapability
	for _, summary := range secrets {
		secret, err := c.secrets.GetSecret(ctx, summary.ID)
		if err != nil {
			log.Printf("Failed to get API secret %s: %v", summary.ID, err)
			continue
		}
		if secret == nil {
			continue
		}

		token := secret.Data["token"]
		if token == "" {
			token = secret.Data["value"]
		}
		baseURL := secret.Data["base_url"]

		if token == "" {
			continue
		}

		cap := &APICapability{
			Token:   token,
			BaseURL: baseURL,
		}

		// Check if this secret is for the requested service
		if secret.Metadata != nil {
			if svc, ok := secret.Metadata["service"]; ok && svc == serviceName {
				log.Printf("API capability loaded from secret %s for service %s", summary.ID, serviceName)
				return cap, nil
			}
		}

		// Keep as fallback if no service-specific match
		if fallback == nil {
			fallback = cap
		}
	}

	if fallback != nil {
		log.Printf("API capability loaded from fallback secret")
		return fallback, nil
	}

	return nil, nil
}

// GetAllCapabilities returns a summary of available capabilities
func (c *CapabilityManager) GetAllCapabilities(ctx context.Context) map[string]bool {
	caps := make(map[string]bool)

	if dns, _ := c.GetDNSCapability(ctx); dns != nil {
		caps["dns"] = true
	}
	if ssh, _ := c.GetSSHCapability(ctx); ssh != nil {
		caps["ssh"] = true
	}
	if snmpv2, _ := c.GetSNMPv2Capability(ctx); snmpv2 != nil {
		caps["snmpv2"] = true
	}
	if snmpv3, _ := c.GetSNMPv3Capability(ctx); snmpv3 != nil {
		caps["snmpv3"] = true
	}

	return caps
}
