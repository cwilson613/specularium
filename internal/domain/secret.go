package domain

import "time"

// SecretSource indicates where a secret originated
type SecretSource string

const (
	// SecretSourceMounted indicates a secret mounted from K8s/Docker/file
	SecretSourceMounted SecretSource = "mounted"
	// SecretSourceOperator indicates a secret created via UI/API
	SecretSourceOperator SecretSource = "operator"
)

// SecretType categorizes secrets by their intended use
type SecretType string

const (
	SecretTypeSSHKey       SecretType = "ssh_key"
	SecretTypeSSHPassword  SecretType = "ssh_password"
	SecretTypeSNMPCommunity SecretType = "snmp_community"
	SecretTypeSNMPv3       SecretType = "snmpv3"
	SecretTypeAPIToken     SecretType = "api_token"
	SecretTypeDNS          SecretType = "dns"
	SecretTypeGeneric      SecretType = "generic"
)

// Secret represents a credential or sensitive configuration
// Unified interface regardless of source (K8s mount or operator-created)
type Secret struct {
	// ID is the unique identifier (e.g., "ssh.ansible", "snmp.switches")
	ID string `json:"id"`

	// Name is a human-readable display name
	Name string `json:"name"`

	// Type categorizes the secret for UI and validation
	Type SecretType `json:"type"`

	// Source indicates where the secret came from
	Source SecretSource `json:"source"`

	// Description explains what this secret is for
	Description string `json:"description,omitempty"`

	// Data holds the secret values (key-value pairs)
	// For mounted secrets, this is populated from files
	// For operator secrets, this is stored encrypted in SQLite
	Data map[string]string `json:"data,omitempty"`

	// Metadata holds non-sensitive configuration
	Metadata map[string]string `json:"metadata,omitempty"`

	// Immutable indicates the secret cannot be modified (mounted secrets)
	Immutable bool `json:"immutable"`

	// CreatedAt is when the secret was first seen/created
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the secret was last modified
	UpdatedAt time.Time `json:"updated_at"`

	// LastUsedAt is when the secret was last used for discovery
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`

	// UsageCount tracks how many times the secret has been used
	UsageCount int `json:"usage_count"`

	// Status indicates if the secret is valid/working
	Status SecretStatus `json:"status"`

	// StatusMessage provides details about the status
	StatusMessage string `json:"status_message,omitempty"`
}

// SecretStatus indicates the operational state of a secret
type SecretStatus string

const (
	SecretStatusUnknown  SecretStatus = "unknown"  // Not yet tested
	SecretStatusValid    SecretStatus = "valid"    // Successfully used
	SecretStatusInvalid  SecretStatus = "invalid"  // Failed validation/use
	SecretStatusExpired  SecretStatus = "expired"  // Token/cert expired
)

// SecretRef is a reference to a secret by ID, used in capability configs
type SecretRef struct {
	ID  string `json:"id" yaml:"id"`
	Key string `json:"key,omitempty" yaml:"key,omitempty"` // Specific key within secret data
}

// SecretSummary is a safe view of a secret (no sensitive data)
type SecretSummary struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Type          SecretType        `json:"type"`
	Source        SecretSource      `json:"source"`
	Description   string            `json:"description,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Immutable     bool              `json:"immutable"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	LastUsedAt    *time.Time        `json:"last_used_at,omitempty"`
	UsageCount    int               `json:"usage_count"`
	Status        SecretStatus      `json:"status"`
	StatusMessage string            `json:"status_message,omitempty"`
	// DataKeys lists the keys in Data without exposing values
	DataKeys []string `json:"data_keys"`
}

// ToSummary creates a safe summary view of the secret
func (s *Secret) ToSummary() SecretSummary {
	keys := make([]string, 0, len(s.Data))
	for k := range s.Data {
		keys = append(keys, k)
	}

	return SecretSummary{
		ID:            s.ID,
		Name:          s.Name,
		Type:          s.Type,
		Source:        s.Source,
		Description:   s.Description,
		Metadata:      s.Metadata,
		Immutable:     s.Immutable,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
		LastUsedAt:    s.LastUsedAt,
		UsageCount:    s.UsageCount,
		Status:        s.Status,
		StatusMessage: s.StatusMessage,
		DataKeys:      keys,
	}
}

// SecretTypeInfo provides metadata about a secret type for UI
type SecretTypeInfo struct {
	Type        SecretType `json:"type"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Fields      []SecretFieldInfo `json:"fields"`
}

// SecretFieldInfo describes a field within a secret type
type SecretFieldInfo struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Sensitive   bool   `json:"sensitive"` // Should be masked in UI
	Type        string `json:"type"`      // "text", "password", "textarea", "select"
	Options     []string `json:"options,omitempty"` // For select type
	Default     string `json:"default,omitempty"`
}

// GetSecretTypeInfos returns metadata for all secret types
func GetSecretTypeInfos() []SecretTypeInfo {
	return []SecretTypeInfo{
		{
			Type:        SecretTypeSSHKey,
			Name:        "SSH Private Key",
			Description: "SSH private key for host discovery",
			Fields: []SecretFieldInfo{
				{Key: "private_key", Name: "Private Key", Description: "PEM-encoded private key", Required: true, Sensitive: true, Type: "textarea"},
				{Key: "passphrase", Name: "Passphrase", Description: "Key passphrase (if encrypted)", Required: false, Sensitive: true, Type: "password"},
				{Key: "username", Name: "Username", Description: "SSH username", Required: true, Sensitive: false, Type: "text", Default: "root"},
			},
		},
		{
			Type:        SecretTypeSSHPassword,
			Name:        "SSH Password",
			Description: "SSH username/password for host discovery",
			Fields: []SecretFieldInfo{
				{Key: "username", Name: "Username", Description: "SSH username", Required: true, Sensitive: false, Type: "text"},
				{Key: "password", Name: "Password", Description: "SSH password", Required: true, Sensitive: true, Type: "password"},
			},
		},
		{
			Type:        SecretTypeSNMPCommunity,
			Name:        "SNMP Community String",
			Description: "SNMPv2c community string for network device discovery",
			Fields: []SecretFieldInfo{
				{Key: "community", Name: "Community", Description: "SNMP community string", Required: true, Sensitive: true, Type: "password", Default: "public"},
			},
		},
		{
			Type:        SecretTypeSNMPv3,
			Name:        "SNMPv3 Credentials",
			Description: "SNMPv3 authentication for network device discovery",
			Fields: []SecretFieldInfo{
				{Key: "username", Name: "Username", Description: "SNMPv3 security name", Required: true, Sensitive: false, Type: "text"},
				{Key: "security_level", Name: "Security Level", Description: "Authentication/privacy level", Required: true, Sensitive: false, Type: "select", Options: []string{"noAuthNoPriv", "authNoPriv", "authPriv"}, Default: "authPriv"},
				{Key: "auth_protocol", Name: "Auth Protocol", Description: "Authentication protocol", Required: false, Sensitive: false, Type: "select", Options: []string{"MD5", "SHA", "SHA256", "SHA512"}},
				{Key: "auth_password", Name: "Auth Password", Description: "Authentication password", Required: false, Sensitive: true, Type: "password"},
				{Key: "priv_protocol", Name: "Privacy Protocol", Description: "Encryption protocol", Required: false, Sensitive: false, Type: "select", Options: []string{"DES", "AES", "AES192", "AES256"}},
				{Key: "priv_password", Name: "Privacy Password", Description: "Encryption password", Required: false, Sensitive: true, Type: "password"},
			},
		},
		{
			Type:        SecretTypeAPIToken,
			Name:        "API Token",
			Description: "API token for service integration",
			Fields: []SecretFieldInfo{
				{Key: "token", Name: "Token", Description: "API token or key", Required: true, Sensitive: true, Type: "password"},
				{Key: "url", Name: "API URL", Description: "Base URL for the API", Required: false, Sensitive: false, Type: "text"},
			},
		},
		{
			Type:        SecretTypeDNS,
			Name:        "DNS Server",
			Description: "DNS server for hostname resolution",
			Fields: []SecretFieldInfo{
				{Key: "server", Name: "Server", Description: "DNS server IP or hostname", Required: true, Sensitive: false, Type: "text"},
				{Key: "port", Name: "Port", Description: "DNS port", Required: false, Sensitive: false, Type: "text", Default: "53"},
				{Key: "tsig_key", Name: "TSIG Key", Description: "TSIG key for zone transfers", Required: false, Sensitive: true, Type: "password"},
			},
		},
		{
			Type:        SecretTypeGeneric,
			Name:        "Generic Secret",
			Description: "Generic key-value secret",
			Fields: []SecretFieldInfo{
				{Key: "value", Name: "Value", Description: "Secret value", Required: true, Sensitive: true, Type: "textarea"},
			},
		},
	}
}
