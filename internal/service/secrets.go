package service

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"specularium/internal/domain"
)

// SecretsRepository defines the interface for secret storage
type SecretsRepository interface {
	CreateSecret(ctx context.Context, secret *domain.Secret) error
	GetSecret(ctx context.Context, id string) (*domain.Secret, error)
	UpdateSecret(ctx context.Context, secret *domain.Secret) error
	DeleteSecret(ctx context.Context, id string) error
	ListSecrets(ctx context.Context, secretType string, source string) ([]domain.Secret, error)
	UpdateSecretUsage(ctx context.Context, id string) error
	UpdateSecretStatus(ctx context.Context, id string, status domain.SecretStatus, message string) error
}

// SecretsService provides unified access to secrets from all sources
type SecretsService struct {
	repo          SecretsRepository
	eventBus      *EventBus
	mountedPaths  []string // Paths to scan for mounted secrets
	mountedSecrets map[string]*domain.Secret // Cache of mounted secrets
	mu            sync.RWMutex
}

// NewSecretsService creates a new secrets service
func NewSecretsService(repo SecretsRepository, eventBus *EventBus) *SecretsService {
	return &SecretsService{
		repo:           repo,
		eventBus:       eventBus,
		mountedPaths:   []string{"/secrets", "/run/secrets"},
		mountedSecrets: make(map[string]*domain.Secret),
	}
}

// SetMountedPaths configures the paths to scan for mounted secrets
func (s *SecretsService) SetMountedPaths(paths []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mountedPaths = paths
}

// LoadMountedSecrets scans configured paths for mounted secrets
// Called at startup and can be called to refresh
func (s *SecretsService) LoadMountedSecrets() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.mountedSecrets = make(map[string]*domain.Secret)
	now := time.Now()

	for _, basePath := range s.mountedPaths {
		if _, err := os.Stat(basePath); os.IsNotExist(err) {
			continue
		}

		// Walk the secrets directory
		err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip errors
			}
			if info.IsDir() {
				return nil
			}

			// Read secret file
			data, err := ioutil.ReadFile(path)
			if err != nil {
				log.Printf("Failed to read mounted secret %s: %v", path, err)
				return nil
			}

			// Generate secret ID from path
			relPath, _ := filepath.Rel(basePath, path)
			secretID := "mounted." + strings.ReplaceAll(relPath, "/", ".")
			secretID = strings.TrimSuffix(secretID, filepath.Ext(secretID))

			// Infer secret type from filename/path
			secretType := s.inferSecretType(path, info.Name())

			// Create mounted secret
			secret := &domain.Secret{
				ID:          secretID,
				Name:        info.Name(),
				Type:        secretType,
				Source:      domain.SecretSourceMounted,
				Description: fmt.Sprintf("Mounted from %s", path),
				Data: map[string]string{
					"value": string(data),
				},
				Metadata: map[string]string{
					"path":     path,
					"filename": info.Name(),
				},
				Immutable: true,
				CreatedAt: info.ModTime(),
				UpdatedAt: info.ModTime(),
				Status:    domain.SecretStatusUnknown,
			}

			s.mountedSecrets[secretID] = secret
			log.Printf("Loaded mounted secret: %s (type: %s)", secretID, secretType)
			return nil
		})

		if err != nil {
			log.Printf("Error walking secrets path %s: %v", basePath, err)
		}
	}

	// Also check environment variables for secrets
	s.loadEnvSecrets(now)

	log.Printf("Loaded %d mounted secrets", len(s.mountedSecrets))
	return nil
}

// loadEnvSecrets loads secrets from environment variables
func (s *SecretsService) loadEnvSecrets(now time.Time) {
	envSecrets := map[string]domain.SecretType{
		"DNS_SERVER":         domain.SecretTypeDNS,
		"SSH_KEY_PATH":       domain.SecretTypeSSHKey,
		"SSH_KEY_PASSPHRASE": domain.SecretTypeSSHKey,
		"SSH_USERNAME":       domain.SecretTypeSSHKey,
		"SNMP_COMMUNITY":     domain.SecretTypeSNMPCommunity,
		"TECHNITIUM_TOKEN":   domain.SecretTypeAPIToken,
	}

	for envName, secretType := range envSecrets {
		if value := os.Getenv(envName); value != "" {
			secretID := "env." + strings.ToLower(envName)

			// Check if we already have this as part of a group
			// (e.g., SSH_KEY_PATH, SSH_KEY_PASSPHRASE, SSH_USERNAME should be one secret)
			if strings.HasPrefix(envName, "SSH_") {
				secretID = "env.ssh"
			}

			if existing, ok := s.mountedSecrets[secretID]; ok {
				// Add to existing secret's data
				key := strings.ToLower(strings.TrimPrefix(envName, "SSH_"))
				existing.Data[key] = value
			} else {
				secret := &domain.Secret{
					ID:          secretID,
					Name:        envName,
					Type:        secretType,
					Source:      domain.SecretSourceMounted,
					Description: fmt.Sprintf("From environment variable %s", envName),
					Data: map[string]string{
						strings.ToLower(envName): value,
					},
					Metadata: map[string]string{
						"source": "environment",
					},
					Immutable: true,
					CreatedAt: now,
					UpdatedAt: now,
					Status:    domain.SecretStatusUnknown,
				}
				s.mountedSecrets[secretID] = secret
			}
		}
	}
}

// inferSecretType guesses the secret type from filename/path
func (s *SecretsService) inferSecretType(path, filename string) domain.SecretType {
	lower := strings.ToLower(filename)
	lowerPath := strings.ToLower(path)

	// SSH keys
	if strings.Contains(lower, "ssh") || strings.Contains(lower, "id_rsa") ||
		strings.Contains(lower, "id_ed25519") || strings.Contains(lower, "id_ecdsa") {
		return domain.SecretTypeSSHKey
	}

	// SNMP
	if strings.Contains(lower, "snmp") {
		if strings.Contains(lower, "v3") {
			return domain.SecretTypeSNMPv3
		}
		return domain.SecretTypeSNMPCommunity
	}

	// API tokens
	if strings.Contains(lower, "token") || strings.Contains(lower, "api") {
		return domain.SecretTypeAPIToken
	}

	// DNS
	if strings.Contains(lower, "dns") || strings.Contains(lowerPath, "dns") {
		return domain.SecretTypeDNS
	}

	return domain.SecretTypeGeneric
}

// GetSecret retrieves a secret by ID from any source
func (s *SecretsService) GetSecret(ctx context.Context, id string) (*domain.Secret, error) {
	// Check mounted secrets first
	s.mu.RLock()
	if secret, ok := s.mountedSecrets[id]; ok {
		s.mu.RUnlock()
		return secret, nil
	}
	s.mu.RUnlock()

	// Check DB for operator secrets
	return s.repo.GetSecret(ctx, id)
}

// GetSecretValue retrieves a specific value from a secret
func (s *SecretsService) GetSecretValue(ctx context.Context, id, key string) (string, error) {
	secret, err := s.GetSecret(ctx, id)
	if err != nil {
		return "", err
	}
	if secret == nil {
		return "", fmt.Errorf("secret %s not found", id)
	}

	value, ok := secret.Data[key]
	if !ok {
		// If key not found, try "value" as default key
		if value, ok = secret.Data["value"]; ok {
			return value, nil
		}
		return "", fmt.Errorf("key %s not found in secret %s", key, id)
	}

	// Update usage tracking for operator secrets
	if secret.Source == domain.SecretSourceOperator {
		s.repo.UpdateSecretUsage(ctx, id)
	}

	return value, nil
}

// ListSecrets returns all secrets from all sources (summaries only, no sensitive data)
func (s *SecretsService) ListSecrets(ctx context.Context, secretType string, source string) ([]domain.SecretSummary, error) {
	var summaries []domain.SecretSummary

	// Add mounted secrets
	if source == "" || source == string(domain.SecretSourceMounted) {
		s.mu.RLock()
		for _, secret := range s.mountedSecrets {
			if secretType == "" || string(secret.Type) == secretType {
				summaries = append(summaries, secret.ToSummary())
			}
		}
		s.mu.RUnlock()
	}

	// Add operator secrets from DB
	if source == "" || source == string(domain.SecretSourceOperator) {
		dbSecrets, err := s.repo.ListSecrets(ctx, secretType, string(domain.SecretSourceOperator))
		if err != nil {
			return nil, err
		}
		for _, secret := range dbSecrets {
			summaries = append(summaries, secret.ToSummary())
		}
	}

	return summaries, nil
}

// CreateSecret creates a new operator secret
func (s *SecretsService) CreateSecret(ctx context.Context, secret *domain.Secret) error {
	// Validate secret
	if secret.ID == "" {
		return fmt.Errorf("secret ID is required")
	}
	if secret.Name == "" {
		return fmt.Errorf("secret name is required")
	}
	if secret.Type == "" {
		return fmt.Errorf("secret type is required")
	}

	// Check for conflicts with mounted secrets
	s.mu.RLock()
	if _, exists := s.mountedSecrets[secret.ID]; exists {
		s.mu.RUnlock()
		return fmt.Errorf("secret ID %s conflicts with a mounted secret", secret.ID)
	}
	s.mu.RUnlock()

	// Force operator source and mutable
	secret.Source = domain.SecretSourceOperator
	secret.Immutable = false
	secret.Status = domain.SecretStatusUnknown

	if err := s.repo.CreateSecret(ctx, secret); err != nil {
		return err
	}

	// Emit event
	s.eventBus.Publish(Event{
		Type:    EventType("secret-created"),
		Payload: secret.ToSummary(),
	})

	return nil
}

// UpdateSecret updates an existing operator secret
func (s *SecretsService) UpdateSecret(ctx context.Context, secret *domain.Secret) error {
	// Check if it's a mounted secret
	s.mu.RLock()
	if _, exists := s.mountedSecrets[secret.ID]; exists {
		s.mu.RUnlock()
		return fmt.Errorf("cannot modify mounted secret %s", secret.ID)
	}
	s.mu.RUnlock()

	if err := s.repo.UpdateSecret(ctx, secret); err != nil {
		return err
	}

	// Emit event
	s.eventBus.Publish(Event{
		Type:    EventType("secret-updated"),
		Payload: secret.ToSummary(),
	})

	return nil
}

// DeleteSecret deletes an operator secret
func (s *SecretsService) DeleteSecret(ctx context.Context, id string) error {
	// Check if it's a mounted secret
	s.mu.RLock()
	if _, exists := s.mountedSecrets[id]; exists {
		s.mu.RUnlock()
		return fmt.Errorf("cannot delete mounted secret %s", id)
	}
	s.mu.RUnlock()

	if err := s.repo.DeleteSecret(ctx, id); err != nil {
		return err
	}

	// Emit event
	s.eventBus.Publish(Event{
		Type:    EventType("secret-deleted"),
		Payload: map[string]string{"id": id},
	})

	return nil
}

// UpdateSecretStatus updates the operational status of a secret
func (s *SecretsService) UpdateSecretStatus(ctx context.Context, id string, status domain.SecretStatus, message string) error {
	// For mounted secrets, just update the cache
	s.mu.Lock()
	if secret, exists := s.mountedSecrets[id]; exists {
		secret.Status = status
		secret.StatusMessage = message
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	// For operator secrets, update the DB
	return s.repo.UpdateSecretStatus(ctx, id, status, message)
}

// GetSecretTypes returns metadata about all secret types
func (s *SecretsService) GetSecretTypes() []domain.SecretTypeInfo {
	return domain.GetSecretTypeInfos()
}

// ResolveSecretRef resolves a secret reference to a value
func (s *SecretsService) ResolveSecretRef(ctx context.Context, ref domain.SecretRef) (string, error) {
	key := ref.Key
	if key == "" {
		key = "value"
	}
	return s.GetSecretValue(ctx, ref.ID, key)
}
