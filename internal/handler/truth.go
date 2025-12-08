package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"specularium/internal/domain"
	"specularium/internal/service"
)

// TruthHandler handles truth and discrepancy API requests
type TruthHandler struct {
	svc *service.TruthService
}

// NewTruthHandler creates a new truth handler
func NewTruthHandler(svc *service.TruthService) *TruthHandler {
	return &TruthHandler{svc: svc}
}

// SetTruthRequest represents the request body for setting truth
type SetTruthRequest struct {
	Properties map[string]any `json:"properties"`
	Operator   string         `json:"operator,omitempty"`
}

// ResolveDiscrepancyRequest represents the request body for resolving a discrepancy
type ResolveDiscrepancyRequest struct {
	Resolution string `json:"resolution"` // "updated_truth", "fixed_reality", "dismissed"
}

// GetNodeTruth returns the truth assertion for a node
func (h *TruthHandler) GetNodeTruth(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	if nodeID == "" {
		h.writeError(w, "Node ID is required", "", http.StatusBadRequest)
		return
	}

	truth, err := h.svc.GetTruth(r.Context(), nodeID)
	if err != nil {
		log.Printf("Failed to get truth for node %s: %v", nodeID, err)
		h.writeError(w, "Failed to get truth", err.Error(), http.StatusInternalServerError)
		return
	}

	if truth == nil {
		h.writeJSON(w, map[string]any{"truth": nil}, http.StatusOK)
		return
	}

	h.writeJSON(w, map[string]any{"truth": truth}, http.StatusOK)
}

// SetNodeTruth sets or updates the truth assertion for a node
func (h *TruthHandler) SetNodeTruth(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	if nodeID == "" {
		h.writeError(w, "Node ID is required", "", http.StatusBadRequest)
		return
	}

	var req SetTruthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, "Invalid request body", err.Error(), http.StatusBadRequest)
		return
	}

	if len(req.Properties) == 0 {
		h.writeError(w, "At least one property is required", "", http.StatusBadRequest)
		return
	}

	operator := req.Operator
	if operator == "" {
		operator = "operator" // Default operator name
	}

	if err := h.svc.SetTruth(r.Context(), nodeID, req.Properties, operator); err != nil {
		log.Printf("Failed to set truth for node %s: %v", nodeID, err)
		h.writeError(w, "Failed to set truth", err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, map[string]string{"status": "ok", "node_id": nodeID}, http.StatusOK)
}

// ClearNodeTruth removes the truth assertion from a node
func (h *TruthHandler) ClearNodeTruth(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	if nodeID == "" {
		h.writeError(w, "Node ID is required", "", http.StatusBadRequest)
		return
	}

	if err := h.svc.ClearTruth(r.Context(), nodeID); err != nil {
		log.Printf("Failed to clear truth for node %s: %v", nodeID, err)
		h.writeError(w, "Failed to clear truth", err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, map[string]string{"status": "ok", "node_id": nodeID}, http.StatusOK)
}

// ListDiscrepancies returns all unresolved discrepancies
func (h *TruthHandler) ListDiscrepancies(w http.ResponseWriter, r *http.Request) {
	discrepancies, err := h.svc.GetUnresolvedDiscrepancies(r.Context())
	if err != nil {
		log.Printf("Failed to list discrepancies: %v", err)
		h.writeError(w, "Failed to list discrepancies", err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, discrepancies, http.StatusOK)
}

// GetDiscrepancy returns a single discrepancy by ID
func (h *TruthHandler) GetDiscrepancy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.writeError(w, "Discrepancy ID is required", "", http.StatusBadRequest)
		return
	}

	discrepancy, err := h.svc.GetDiscrepancy(r.Context(), id)
	if err != nil {
		log.Printf("Failed to get discrepancy %s: %v", id, err)
		h.writeError(w, "Failed to get discrepancy", err.Error(), http.StatusInternalServerError)
		return
	}

	if discrepancy == nil {
		h.writeError(w, "Discrepancy not found", "", http.StatusNotFound)
		return
	}

	h.writeJSON(w, discrepancy, http.StatusOK)
}

// ResolveDiscrepancy marks a discrepancy as resolved
func (h *TruthHandler) ResolveDiscrepancy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.writeError(w, "Discrepancy ID is required", "", http.StatusBadRequest)
		return
	}

	var req ResolveDiscrepancyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, "Invalid request body", err.Error(), http.StatusBadRequest)
		return
	}

	// Validate resolution type
	resolution := domain.DiscrepancyResolution(req.Resolution)
	switch resolution {
	case domain.ResolutionUpdatedTruth, domain.ResolutionFixedReality, domain.ResolutionDismissed:
		// Valid
	default:
		h.writeError(w, "Invalid resolution type", "Must be: updated_truth, fixed_reality, or dismissed", http.StatusBadRequest)
		return
	}

	if err := h.svc.ResolveDiscrepancy(r.Context(), id, resolution); err != nil {
		log.Printf("Failed to resolve discrepancy %s: %v", id, err)
		h.writeError(w, "Failed to resolve discrepancy", err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, map[string]string{"status": "ok", "discrepancy_id": id, "resolution": req.Resolution}, http.StatusOK)
}

// GetNodeDiscrepancies returns all discrepancies for a specific node
func (h *TruthHandler) GetNodeDiscrepancies(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	if nodeID == "" {
		h.writeError(w, "Node ID is required", "", http.StatusBadRequest)
		return
	}

	discrepancies, err := h.svc.GetDiscrepanciesByNode(r.Context(), nodeID)
	if err != nil {
		log.Printf("Failed to get discrepancies for node %s: %v", nodeID, err)
		h.writeError(w, "Failed to get discrepancies", err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, discrepancies, http.StatusOK)
}

// writeJSON writes a JSON response
func (h *TruthHandler) writeJSON(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}

// writeError writes an error response
func (h *TruthHandler) writeError(w http.ResponseWriter, message, details string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(ErrorResponse{Error: message, Details: details}); err != nil {
		log.Printf("Failed to encode error response: %v", err)
	}
}
