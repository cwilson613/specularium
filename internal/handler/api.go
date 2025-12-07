package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"netdiagram/internal/domain"
	"netdiagram/internal/service"
)

// APIHandler handles API requests
type APIHandler struct {
	infraSvc *service.InfrastructureService
}

// NewAPIHandler creates a new API handler
func NewAPIHandler(infraSvc *service.InfrastructureService) *APIHandler {
	return &APIHandler{infraSvc: infraSvc}
}

// GetGraph returns the network graph for visualization
func (h *APIHandler) GetGraph(w http.ResponseWriter, r *http.Request) {
	graph, err := h.infraSvc.GetGraph(r.Context())
	if err != nil {
		log.Printf("Failed to get graph: %v", err)
		http.Error(w, "Failed to get graph", http.StatusInternalServerError)
		return
	}

	writeJSON(w, graph)
}

// GetInfrastructure returns the full infrastructure data
func (h *APIHandler) GetInfrastructure(w http.ResponseWriter, r *http.Request) {
	infra, err := h.infraSvc.GetInfrastructure(r.Context())
	if err != nil {
		log.Printf("Failed to get infrastructure: %v", err)
		http.Error(w, "Failed to get infrastructure", http.StatusInternalServerError)
		return
	}

	writeJSON(w, infra)
}

// GetHost returns a single host by ID
func (h *APIHandler) GetHost(w http.ResponseWriter, r *http.Request) {
	id := extractPathParam(r.URL.Path, "/api/hosts/")
	if id == "" {
		http.Error(w, "Host ID required", http.StatusBadRequest)
		return
	}

	host, err := h.infraSvc.GetHost(r.Context(), id)
	if err != nil {
		log.Printf("Failed to get host: %v", err)
		http.Error(w, "Failed to get host", http.StatusInternalServerError)
		return
	}

	if host == nil {
		http.Error(w, "Host not found", http.StatusNotFound)
		return
	}

	writeJSON(w, host)
}

// CreateHost creates a new host
func (h *APIHandler) CreateHost(w http.ResponseWriter, r *http.Request) {
	var host domain.Host
	if err := json.NewDecoder(r.Body).Decode(&host); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.infraSvc.CreateHost(r.Context(), &host); err != nil {
		log.Printf("Failed to create host: %v", err)
		if strings.Contains(err.Error(), "already exists") {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, host)
}

// UpdateHost updates an existing host
func (h *APIHandler) UpdateHost(w http.ResponseWriter, r *http.Request) {
	id := extractPathParam(r.URL.Path, "/api/hosts/")
	if id == "" {
		http.Error(w, "Host ID required", http.StatusBadRequest)
		return
	}

	var host domain.Host
	if err := json.NewDecoder(r.Body).Decode(&host); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	host.ID = id // Ensure ID matches path

	if err := h.infraSvc.UpdateHost(r.Context(), &host); err != nil {
		log.Printf("Failed to update host: %v", err)
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, host)
}

// DeleteHost deletes a host
func (h *APIHandler) DeleteHost(w http.ResponseWriter, r *http.Request) {
	id := extractPathParam(r.URL.Path, "/api/hosts/")
	if id == "" {
		http.Error(w, "Host ID required", http.StatusBadRequest)
		return
	}

	if err := h.infraSvc.DeleteHost(r.Context(), id); err != nil {
		log.Printf("Failed to delete host: %v", err)
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// CreateConnection creates a new connection
func (h *APIHandler) CreateConnection(w http.ResponseWriter, r *http.Request) {
	var conn domain.Connection
	if err := json.NewDecoder(r.Body).Decode(&conn); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.infraSvc.CreateConnection(r.Context(), &conn); err != nil {
		log.Printf("Failed to create connection: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, conn)
}

// DeleteConnection deletes a connection
func (h *APIHandler) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	id := extractPathParam(r.URL.Path, "/api/connections/")
	if id == "" {
		http.Error(w, "Connection ID required", http.StatusBadRequest)
		return
	}

	if err := h.infraSvc.DeleteConnection(r.Context(), id); err != nil {
		log.Printf("Failed to delete connection: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SavePositions saves node positions
func (h *APIHandler) SavePositions(w http.ResponseWriter, r *http.Request) {
	var positions map[string]domain.Position
	if err := json.NewDecoder(r.Body).Decode(&positions); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.infraSvc.SavePositions(r.Context(), positions); err != nil {
		log.Printf("Failed to save positions: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ExportYAML exports infrastructure to YAML
func (h *APIHandler) ExportYAML(w http.ResponseWriter, r *http.Request) {
	data, err := h.infraSvc.ExportToYAML(r.Context())
	if err != nil {
		log.Printf("Failed to export YAML: %v", err)
		http.Error(w, "Failed to export YAML", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Content-Disposition", "attachment; filename=infrastructure.yml")
	w.Write(data)
}

// ImportYAML imports infrastructure from YAML
func (h *APIHandler) ImportYAML(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "Path parameter required", http.StatusBadRequest)
		return
	}

	if err := h.infraSvc.ImportFromYAML(r.Context(), path); err != nil {
		log.Printf("Failed to import YAML: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Reload forces a reload of infrastructure data
func (h *APIHandler) Reload(w http.ResponseWriter, r *http.Request) {
	if err := h.infraSvc.Reload(r.Context()); err != nil {
		log.Printf("Failed to reload: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Helper functions

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON: %v", err)
	}
}

func extractPathParam(path, prefix string) string {
	if strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix)
	}
	return ""
}
