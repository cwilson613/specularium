package handler

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"specularium/internal/domain"
	"specularium/internal/service"
)

// DiscoveryTrigger allows triggering discovery from the handler
type DiscoveryTrigger interface {
	TriggerSyncAll(ctx context.Context) error
}

// SubnetScanner allows scanning network subnets for hosts
type SubnetScanner interface {
	ScanSubnet(ctx context.Context, cidr string) error
}

// GraphHandler handles graph API requests
type GraphHandler struct {
	svc       *service.GraphService
	discovery DiscoveryTrigger
	scanner   SubnetScanner
}

// NewGraphHandler creates a new graph handler
func NewGraphHandler(svc *service.GraphService) *GraphHandler {
	return &GraphHandler{svc: svc}
}

// SetDiscoveryTrigger sets the discovery trigger (adapter registry)
func (h *GraphHandler) SetDiscoveryTrigger(d DiscoveryTrigger) {
	h.discovery = d
}

// SetSubnetScanner sets the subnet scanner
func (h *GraphHandler) SetSubnetScanner(s SubnetScanner) {
	h.scanner = s
}

// Error response structure
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// GetGraph returns the complete graph
func (h *GraphHandler) GetGraph(w http.ResponseWriter, r *http.Request) {
	graph, err := h.svc.GetGraph(r.Context())
	if err != nil {
		log.Printf("Failed to get graph: %v", err)
		h.writeError(w, "Failed to get graph", err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, graph, http.StatusOK)
}

// ListNodes returns all nodes
func (h *GraphHandler) ListNodes(w http.ResponseWriter, r *http.Request) {
	nodeType := r.URL.Query().Get("type")
	source := r.URL.Query().Get("source")

	nodes, err := h.svc.ListNodes(r.Context(), nodeType, source)
	if err != nil {
		log.Printf("Failed to list nodes: %v", err)
		h.writeError(w, "Failed to list nodes", err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, nodes, http.StatusOK)
}

// GetNode returns a single node
func (h *GraphHandler) GetNode(w http.ResponseWriter, r *http.Request) {
	id := extractPathParam(r.URL.Path, "/api/nodes/")
	if id == "" {
		h.writeError(w, "Invalid node ID", "Node ID is required", http.StatusBadRequest)
		return
	}

	node, err := h.svc.GetNode(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.writeError(w, "Not found", err.Error(), http.StatusNotFound)
			return
		}
		log.Printf("Failed to get node: %v", err)
		h.writeError(w, "Failed to get node", err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, node, http.StatusOK)
}

// CreateNode creates a new node
func (h *GraphHandler) CreateNode(w http.ResponseWriter, r *http.Request) {
	var node domain.Node
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		h.writeError(w, "Invalid request body", err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.svc.CreateNode(r.Context(), &node); err != nil {
		log.Printf("Failed to create node: %v", err)
		h.writeError(w, "Failed to create node", err.Error(), http.StatusBadRequest)
		return
	}

	h.writeJSON(w, node, http.StatusCreated)
}

// UpdateNode updates an existing node
func (h *GraphHandler) UpdateNode(w http.ResponseWriter, r *http.Request) {
	id := extractPathParam(r.URL.Path, "/api/nodes/")
	if id == "" {
		h.writeError(w, "Invalid node ID", "Node ID is required", http.StatusBadRequest)
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		h.writeError(w, "Invalid request body", err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.svc.UpdateNode(r.Context(), id, updates); err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.writeError(w, "Not found", err.Error(), http.StatusNotFound)
			return
		}
		log.Printf("Failed to update node: %v", err)
		h.writeError(w, "Failed to update node", err.Error(), http.StatusBadRequest)
		return
	}

	// Return updated node
	node, _ := h.svc.GetNode(r.Context(), id)
	h.writeJSON(w, node, http.StatusOK)
}

// DeleteNode deletes a node
func (h *GraphHandler) DeleteNode(w http.ResponseWriter, r *http.Request) {
	id := extractPathParam(r.URL.Path, "/api/nodes/")
	if id == "" {
		h.writeError(w, "Invalid node ID", "Node ID is required", http.StatusBadRequest)
		return
	}

	if err := h.svc.DeleteNode(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.writeError(w, "Not found", err.Error(), http.StatusNotFound)
			return
		}
		log.Printf("Failed to delete node: %v", err)
		h.writeError(w, "Failed to delete node", err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListEdges returns all edges
func (h *GraphHandler) ListEdges(w http.ResponseWriter, r *http.Request) {
	edgeType := r.URL.Query().Get("type")
	fromID := r.URL.Query().Get("from_id")
	toID := r.URL.Query().Get("to_id")

	edges, err := h.svc.ListEdges(r.Context(), edgeType, fromID, toID)
	if err != nil {
		log.Printf("Failed to list edges: %v", err)
		h.writeError(w, "Failed to list edges", err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, edges, http.StatusOK)
}

// GetEdge returns a single edge
func (h *GraphHandler) GetEdge(w http.ResponseWriter, r *http.Request) {
	id := extractPathParam(r.URL.Path, "/api/edges/")
	if id == "" {
		h.writeError(w, "Invalid edge ID", "Edge ID is required", http.StatusBadRequest)
		return
	}

	edge, err := h.svc.GetEdge(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.writeError(w, "Not found", err.Error(), http.StatusNotFound)
			return
		}
		log.Printf("Failed to get edge: %v", err)
		h.writeError(w, "Failed to get edge", err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, edge, http.StatusOK)
}

// CreateEdge creates a new edge
func (h *GraphHandler) CreateEdge(w http.ResponseWriter, r *http.Request) {
	var edge domain.Edge
	if err := json.NewDecoder(r.Body).Decode(&edge); err != nil {
		h.writeError(w, "Invalid request body", err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.svc.CreateEdge(r.Context(), &edge); err != nil {
		log.Printf("Failed to create edge: %v", err)
		h.writeError(w, "Failed to create edge", err.Error(), http.StatusBadRequest)
		return
	}

	h.writeJSON(w, edge, http.StatusCreated)
}

// UpdateEdge updates an existing edge
func (h *GraphHandler) UpdateEdge(w http.ResponseWriter, r *http.Request) {
	id := extractPathParam(r.URL.Path, "/api/edges/")
	if id == "" {
		h.writeError(w, "Invalid edge ID", "Edge ID is required", http.StatusBadRequest)
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		h.writeError(w, "Invalid request body", err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.svc.UpdateEdge(r.Context(), id, updates); err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.writeError(w, "Not found", err.Error(), http.StatusNotFound)
			return
		}
		log.Printf("Failed to update edge: %v", err)
		h.writeError(w, "Failed to update edge", err.Error(), http.StatusBadRequest)
		return
	}

	// Return updated edge
	edge, _ := h.svc.GetEdge(r.Context(), id)
	h.writeJSON(w, edge, http.StatusOK)
}

// DeleteEdge deletes an edge
func (h *GraphHandler) DeleteEdge(w http.ResponseWriter, r *http.Request) {
	id := extractPathParam(r.URL.Path, "/api/edges/")
	if id == "" {
		h.writeError(w, "Invalid edge ID", "Edge ID is required", http.StatusBadRequest)
		return
	}

	if err := h.svc.DeleteEdge(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.writeError(w, "Not found", err.Error(), http.StatusNotFound)
			return
		}
		log.Printf("Failed to delete edge: %v", err)
		h.writeError(w, "Failed to delete edge", err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetPositions returns all node positions
func (h *GraphHandler) GetPositions(w http.ResponseWriter, r *http.Request) {
	positions, err := h.svc.GetAllPositions(r.Context())
	if err != nil {
		log.Printf("Failed to get positions: %v", err)
		h.writeError(w, "Failed to get positions", err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, positions, http.StatusOK)
}

// SavePositions saves multiple node positions
func (h *GraphHandler) SavePositions(w http.ResponseWriter, r *http.Request) {
	var positions []domain.NodePosition
	if err := json.NewDecoder(r.Body).Decode(&positions); err != nil {
		h.writeError(w, "Invalid request body", err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.svc.SavePositions(r.Context(), positions); err != nil {
		log.Printf("Failed to save positions: %v", err)
		h.writeError(w, "Failed to save positions", err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, map[string]int{"saved": len(positions)}, http.StatusOK)
}

// UpdatePosition updates a single node position
func (h *GraphHandler) UpdatePosition(w http.ResponseWriter, r *http.Request) {
	nodeID := extractPathParam(r.URL.Path, "/api/positions/")
	if nodeID == "" {
		h.writeError(w, "Invalid node ID", "Node ID is required", http.StatusBadRequest)
		return
	}

	var pos domain.NodePosition
	if err := json.NewDecoder(r.Body).Decode(&pos); err != nil {
		h.writeError(w, "Invalid request body", err.Error(), http.StatusBadRequest)
		return
	}

	pos.NodeID = nodeID // Ensure ID matches path

	if err := h.svc.SavePosition(r.Context(), pos); err != nil {
		log.Printf("Failed to update position: %v", err)
		h.writeError(w, "Failed to update position", err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, pos, http.StatusOK)
}

// ImportYAML imports graph data from YAML
func (h *GraphHandler) ImportYAML(w http.ResponseWriter, r *http.Request) {
	strategy := r.URL.Query().Get("strategy")
	if strategy == "" {
		strategy = "merge"
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, "Failed to read request body", err.Error(), http.StatusBadRequest)
		return
	}

	result, err := h.svc.ImportYAML(r.Context(), data, strategy)
	if err != nil {
		log.Printf("Failed to import YAML: %v", err)
		h.writeError(w, "Failed to import YAML", err.Error(), http.StatusBadRequest)
		return
	}

	h.writeJSON(w, result, http.StatusOK)
}

// ImportAnsibleInventory imports graph data from Ansible inventory
func (h *GraphHandler) ImportAnsibleInventory(w http.ResponseWriter, r *http.Request) {
	strategy := r.URL.Query().Get("strategy")
	if strategy == "" {
		strategy = "merge"
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, "Failed to read request body", err.Error(), http.StatusBadRequest)
		return
	}

	result, err := h.svc.ImportAnsibleInventory(r.Context(), data, strategy)
	if err != nil {
		log.Printf("Failed to import Ansible inventory: %v", err)
		h.writeError(w, "Failed to import Ansible inventory", err.Error(), http.StatusBadRequest)
		return
	}

	h.writeJSON(w, result, http.StatusOK)
}

// ScanRequest represents a subnet scan request
type ScanRequest struct {
	CIDR string `json:"cidr"`
}

// ImportScan handles network scan requests
func (h *GraphHandler) ImportScan(w http.ResponseWriter, r *http.Request) {
	if h.scanner == nil {
		h.writeError(w, "Scanner not configured", "No subnet scanner is registered", http.StatusServiceUnavailable)
		return
	}

	var req ScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, "Invalid request body", err.Error(), http.StatusBadRequest)
		return
	}

	if req.CIDR == "" {
		h.writeError(w, "CIDR required", "Please provide a CIDR range to scan (e.g., 192.168.0.0/24)", http.StatusBadRequest)
		return
	}

	// Run scan in background and return immediately
	go func() {
		if err := h.scanner.ScanSubnet(context.Background(), req.CIDR); err != nil {
			log.Printf("Subnet scan failed: %v", err)
		}
	}()

	h.writeJSON(w, map[string]string{
		"status": "scan_started",
		"cidr":   req.CIDR,
	}, http.StatusAccepted)
}

// ClearGraph removes all nodes, edges, and positions
func (h *GraphHandler) ClearGraph(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.ClearGraph(r.Context()); err != nil {
		log.Printf("Failed to clear graph: %v", err)
		h.writeError(w, "Failed to clear graph", err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, map[string]string{"status": "cleared"}, http.StatusOK)
}

// TriggerDiscovery triggers the discovery/verification process for all nodes
func (h *GraphHandler) TriggerDiscovery(w http.ResponseWriter, r *http.Request) {
	if h.discovery == nil {
		h.writeError(w, "Discovery not configured", "No discovery adapters are registered", http.StatusServiceUnavailable)
		return
	}

	// Run discovery in background and return immediately
	go func() {
		if err := h.discovery.TriggerSyncAll(context.Background()); err != nil {
			log.Printf("Discovery sync failed: %v", err)
		}
	}()

	h.writeJSON(w, map[string]string{"status": "discovery_triggered"}, http.StatusAccepted)
}

// ExportJSON exports the graph as JSON
func (h *GraphHandler) ExportJSON(w http.ResponseWriter, r *http.Request) {
	data, err := h.svc.ExportJSON(r.Context())
	if err != nil {
		log.Printf("Failed to export JSON: %v", err)
		h.writeError(w, "Failed to export JSON", err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=graph.json")
	w.Write(data)
}

// ExportYAML exports the graph as YAML
func (h *GraphHandler) ExportYAML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Content-Disposition", "attachment; filename=graph.yml")

	if err := h.svc.ExportYAML(r.Context(), w); err != nil {
		log.Printf("Failed to export YAML: %v", err)
		// Can't write error response as we already set headers
		return
	}
}

// ExportAnsibleInventory exports the graph as Ansible inventory
func (h *GraphHandler) ExportAnsibleInventory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Content-Disposition", "attachment; filename=inventory.yml")

	if err := h.svc.ExportAnsibleInventory(r.Context(), w); err != nil {
		log.Printf("Failed to export Ansible inventory: %v", err)
		// Can't write error response as we already set headers
		return
	}
}

// Helper methods

func (h *GraphHandler) writeJSON(w http.ResponseWriter, data interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON: %v", err)
	}
}

func (h *GraphHandler) writeError(w http.ResponseWriter, error, details string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error:   error,
		Details: details,
	})
}

func extractPathParam(path, prefix string) string {
	if strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix)
	}
	return ""
}
