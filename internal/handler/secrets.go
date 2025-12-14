package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"specularium/internal/domain"
)

// SecretsService defines the interface for secrets operations
type SecretsService interface {
	GetSecret(ctx context.Context, id string) (*domain.Secret, error)
	ListSecrets(ctx context.Context, secretType string, source string) ([]domain.SecretSummary, error)
	CreateSecret(ctx context.Context, secret *domain.Secret) error
	UpdateSecret(ctx context.Context, secret *domain.Secret) error
	DeleteSecret(ctx context.Context, id string) error
	GetSecretTypes() []domain.SecretTypeInfo
	LoadMountedSecrets() error
}

// CapabilityChecker checks what discovery capabilities are available
type CapabilityChecker interface {
	GetAllCapabilities(ctx context.Context) map[string]bool
}

// SecretsHandler handles secrets API requests
type SecretsHandler struct {
	svc          SecretsService
	capabilities CapabilityChecker
}

// NewSecretsHandler creates a new secrets handler
func NewSecretsHandler(svc SecretsService) *SecretsHandler {
	return &SecretsHandler{svc: svc}
}

// SetCapabilityChecker sets the capability checker
func (h *SecretsHandler) SetCapabilityChecker(c CapabilityChecker) {
	h.capabilities = c
}

// GetCapabilities returns available discovery capabilities
// GET /api/capabilities
func (h *SecretsHandler) GetCapabilities(w http.ResponseWriter, r *http.Request) {
	if h.capabilities == nil {
		h.writeJSON(w, map[string]bool{}, http.StatusOK)
		return
	}

	caps := h.capabilities.GetAllCapabilities(r.Context())
	h.writeJSON(w, caps, http.StatusOK)
}

// ListSecrets returns all secrets (summaries only)
// GET /api/secrets?type=ssh_key&source=operator
func (h *SecretsHandler) ListSecrets(w http.ResponseWriter, r *http.Request) {
	secretType := r.URL.Query().Get("type")
	source := r.URL.Query().Get("source")

	secrets, err := h.svc.ListSecrets(r.Context(), secretType, source)
	if err != nil {
		log.Printf("Failed to list secrets: %v", err)
		h.writeError(w, "Failed to list secrets", err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, secrets, http.StatusOK)
}

// GetSecret returns a single secret summary (no sensitive data)
// GET /api/secrets/{id}
func (h *SecretsHandler) GetSecret(w http.ResponseWriter, r *http.Request) {
	id := extractSecretID(r.URL.Path)
	if id == "" {
		h.writeError(w, "Invalid secret ID", "Secret ID is required", http.StatusBadRequest)
		return
	}

	secret, err := h.svc.GetSecret(r.Context(), id)
	if err != nil {
		log.Printf("Failed to get secret: %v", err)
		h.writeError(w, "Failed to get secret", err.Error(), http.StatusInternalServerError)
		return
	}
	if secret == nil {
		h.writeError(w, "Secret not found", "No secret with ID: "+id, http.StatusNotFound)
		return
	}

	// Return summary (no sensitive data) unless explicitly requested
	if r.URL.Query().Get("include_data") == "true" {
		// Only allow viewing data for operator secrets
		if secret.Source == domain.SecretSourceOperator {
			h.writeJSON(w, secret, http.StatusOK)
			return
		}
	}

	h.writeJSON(w, secret.ToSummary(), http.StatusOK)
}

// CreateSecretRequest is the request body for creating a secret
type CreateSecretRequest struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        domain.SecretType `json:"type"`
	Description string            `json:"description,omitempty"`
	Data        map[string]string `json:"data"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// CreateSecret creates a new operator secret
// POST /api/secrets
func (h *SecretsHandler) CreateSecret(w http.ResponseWriter, r *http.Request) {
	var req CreateSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, "Invalid request body", err.Error(), http.StatusBadRequest)
		return
	}

	secret := &domain.Secret{
		ID:          req.ID,
		Name:        req.Name,
		Type:        req.Type,
		Description: req.Description,
		Data:        req.Data,
		Metadata:    req.Metadata,
	}

	if err := h.svc.CreateSecret(r.Context(), secret); err != nil {
		if strings.Contains(err.Error(), "conflicts") {
			h.writeError(w, "Conflict", err.Error(), http.StatusConflict)
			return
		}
		log.Printf("Failed to create secret: %v", err)
		h.writeError(w, "Failed to create secret", err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, secret.ToSummary(), http.StatusCreated)
}

// UpdateSecretRequest is the request body for updating a secret
type UpdateSecretRequest struct {
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Data        map[string]string `json:"data,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// UpdateSecret updates an existing operator secret
// PUT /api/secrets/{id}
func (h *SecretsHandler) UpdateSecret(w http.ResponseWriter, r *http.Request) {
	id := extractSecretID(r.URL.Path)
	if id == "" {
		h.writeError(w, "Invalid secret ID", "Secret ID is required", http.StatusBadRequest)
		return
	}

	// Get existing secret
	existing, err := h.svc.GetSecret(r.Context(), id)
	if err != nil {
		log.Printf("Failed to get secret for update: %v", err)
		h.writeError(w, "Failed to get secret", err.Error(), http.StatusInternalServerError)
		return
	}
	if existing == nil {
		h.writeError(w, "Secret not found", "No secret with ID: "+id, http.StatusNotFound)
		return
	}
	if existing.Immutable {
		h.writeError(w, "Immutable secret", "Cannot modify mounted secrets", http.StatusForbidden)
		return
	}

	var req UpdateSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, "Invalid request body", err.Error(), http.StatusBadRequest)
		return
	}

	// Update fields
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Description != "" {
		existing.Description = req.Description
	}
	if req.Data != nil {
		existing.Data = req.Data
	}
	if req.Metadata != nil {
		existing.Metadata = req.Metadata
	}

	if err := h.svc.UpdateSecret(r.Context(), existing); err != nil {
		log.Printf("Failed to update secret: %v", err)
		h.writeError(w, "Failed to update secret", err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, existing.ToSummary(), http.StatusOK)
}

// DeleteSecret deletes an operator secret
// DELETE /api/secrets/{id}
func (h *SecretsHandler) DeleteSecret(w http.ResponseWriter, r *http.Request) {
	id := extractSecretID(r.URL.Path)
	if id == "" {
		h.writeError(w, "Invalid secret ID", "Secret ID is required", http.StatusBadRequest)
		return
	}

	// Check if secret exists and is deletable
	existing, err := h.svc.GetSecret(r.Context(), id)
	if err != nil {
		log.Printf("Failed to get secret for delete: %v", err)
		h.writeError(w, "Failed to get secret", err.Error(), http.StatusInternalServerError)
		return
	}
	if existing == nil {
		h.writeError(w, "Secret not found", "No secret with ID: "+id, http.StatusNotFound)
		return
	}
	if existing.Immutable {
		h.writeError(w, "Immutable secret", "Cannot delete mounted secrets", http.StatusForbidden)
		return
	}

	if err := h.svc.DeleteSecret(r.Context(), id); err != nil {
		log.Printf("Failed to delete secret: %v", err)
		h.writeError(w, "Failed to delete secret", err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, map[string]string{"status": "deleted", "id": id}, http.StatusOK)
}

// GetSecretTypes returns metadata about available secret types
// GET /api/secrets/types
func (h *SecretsHandler) GetSecretTypes(w http.ResponseWriter, r *http.Request) {
	types := h.svc.GetSecretTypes()
	h.writeJSON(w, types, http.StatusOK)
}

// RefreshMountedSecrets triggers a reload of mounted secrets
// POST /api/secrets/refresh
func (h *SecretsHandler) RefreshMountedSecrets(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.LoadMountedSecrets(); err != nil {
		log.Printf("Failed to refresh mounted secrets: %v", err)
		h.writeError(w, "Failed to refresh", err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, map[string]string{"status": "refreshed"}, http.StatusOK)
}

// extractSecretID extracts the secret ID from a URL path
func extractSecretID(path string) string {
	// Handle /api/secrets/{id} pattern
	// The ID may contain dots (e.g., "mounted.ssh.key")
	prefix := "/api/secrets/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	id := strings.TrimPrefix(path, prefix)
	// Remove any trailing segments (for sub-resources)
	if idx := strings.Index(id, "/"); idx > 0 {
		id = id[:idx]
	}
	return id
}

// writeJSON writes a JSON response
func (h *SecretsHandler) writeJSON(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes an error response
func (h *SecretsHandler) writeError(w http.ResponseWriter, message, details string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":   message,
		"details": details,
	})
}
