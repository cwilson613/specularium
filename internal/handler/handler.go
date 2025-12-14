package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

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

// Bootstrapper performs initial self-discovery
type Bootstrapper interface {
	Bootstrap(ctx context.Context) error
	GetEnvironment() domain.EnvironmentInfo
	GetSuggestedScanTargets() []string
	GetScanTargets() domain.ScanTargets
}

// GraphHandler handles graph API requests
type GraphHandler struct {
	svc          *service.GraphService
	discovery    DiscoveryTrigger
	scanner      SubnetScanner
	bootstrapper Bootstrapper
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

// SetBootstrapper sets the bootstrapper for self-discovery
func (h *GraphHandler) SetBootstrapper(b Bootstrapper) {
	h.bootstrapper = b
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
	node, err := h.svc.GetNode(r.Context(), id)
	if err != nil {
		log.Printf("Failed to fetch updated node: %v", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}
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
	edge, err := h.svc.GetEdge(r.Context(), id)
	if err != nil {
		log.Printf("Failed to fetch updated edge: %v", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}
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

// Bootstrap triggers self-discovery from the current deployment environment
func (h *GraphHandler) Bootstrap(w http.ResponseWriter, r *http.Request) {
	if h.bootstrapper == nil {
		h.writeError(w, "Bootstrapper not configured", "No bootstrap adapter is registered", http.StatusServiceUnavailable)
		return
	}

	// Run bootstrap in background and return immediately
	go func() {
		if err := h.bootstrapper.Bootstrap(context.Background()); err != nil {
			log.Printf("Bootstrap failed: %v", err)
		}
	}()

	env := h.bootstrapper.GetEnvironment()
	targets := h.bootstrapper.GetSuggestedScanTargets()

	h.writeJSON(w, map[string]interface{}{
		"status":                "bootstrap_started",
		"environment":           env,
		"suggested_scan_targets": targets,
	}, http.StatusAccepted)
}

// GetEnvironment returns the detected deployment environment
func (h *GraphHandler) GetEnvironment(w http.ResponseWriter, r *http.Request) {
	if h.bootstrapper == nil {
		h.writeError(w, "Bootstrapper not configured", "No bootstrap adapter is registered", http.StatusServiceUnavailable)
		return
	}

	env := h.bootstrapper.GetEnvironment()
	scanTargets := h.bootstrapper.GetScanTargets()

	h.writeJSON(w, map[string]interface{}{
		"environment":            env,
		"suggested_scan_targets": scanTargets.Primary,   // Backwards compat
		"scan_targets":           scanTargets,           // New structured format
	}, http.StatusOK)
}

// ClearGraph removes all nodes, edges, and positions
// After clearing, it automatically re-runs bootstrap to rediscover infrastructure
func (h *GraphHandler) ClearGraph(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.ClearGraph(r.Context()); err != nil {
		log.Printf("Failed to clear graph: %v", err)
		h.writeError(w, "Failed to clear graph", err.Error(), http.StatusInternalServerError)
		return
	}

	// Auto-trigger bootstrap after clear to rediscover infrastructure
	if h.bootstrapper != nil {
		go func() {
			log.Printf("Auto-triggering bootstrap after graph clear...")
			if err := h.bootstrapper.Bootstrap(context.Background()); err != nil {
				log.Printf("Post-clear bootstrap failed: %v", err)
			}
		}()
	}

	// Also trigger discovery adapters (nmap, verifier, etc.)
	if h.discovery != nil {
		go func() {
			log.Printf("Auto-triggering discovery adapters after graph clear...")
			if err := h.discovery.TriggerSyncAll(context.Background()); err != nil {
				log.Printf("Post-clear discovery failed: %v", err)
			}
		}()
	}

	h.writeJSON(w, map[string]string{"status": "cleared", "bootstrap": "triggered"}, http.StatusOK)
}

// RegisterClient creates or updates a node for the browser client
// This allows passive discovery of clients connecting to the UI
func (h *GraphHandler) RegisterClient(w http.ResponseWriter, r *http.Request) {
	// Get client IP from request
	clientIP := getClientIP(r)
	if clientIP == "" {
		h.writeError(w, "Could not determine client IP", "", http.StatusBadRequest)
		return
	}

	// Parse optional user agent info from request body
	var req struct {
		UserAgent string `json:"user_agent,omitempty"`
		Hostname  string `json:"hostname,omitempty"`
	}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&req) // Ignore errors, fields are optional
	}

	// Generate node ID from IP
	nodeID := strings.ReplaceAll(clientIP, ".", "-")

	// Infer segmentum from IP
	segmentum := ""
	parts := strings.Split(clientIP, ".")
	if len(parts) == 4 {
		segmentum = fmt.Sprintf("%s.%s.%s.0/24", parts[0], parts[1], parts[2])
	}

	// Check if node already exists
	existing, _ := h.svc.GetNode(r.Context(), nodeID)
	now := time.Now()

	if existing != nil {
		// Update last seen via updates map
		updates := map[string]interface{}{
			"last_seen": now,
		}

		// Add discovered fields
		discovered := existing.Discovered
		if discovered == nil {
			discovered = make(map[string]any)
		}
		discovered["last_browser_visit"] = now.Format(time.RFC3339)
		if req.UserAgent != "" {
			discovered["user_agent"] = req.UserAgent
		}
		updates["discovered"] = discovered

		if err := h.svc.UpdateNode(r.Context(), nodeID, updates); err != nil {
			log.Printf("Failed to update client node %s: %v", nodeID, err)
		}

		h.writeJSON(w, map[string]any{
			"status":    "updated",
			"node_id":   nodeID,
			"client_ip": clientIP,
			"segmentum": segmentum,
		}, http.StatusOK)
		return
	}

	// Create new client node
	label := clientIP
	if req.Hostname != "" {
		label = req.Hostname
	}

	node := &domain.Node{
		ID:     nodeID,
		Type:   domain.NodeTypeServer, // Generic server type for client devices
		Label:  label,
		Source: "client",
		Status: domain.NodeStatusVerified, // We know it's alive - it's talking to us!
		Properties: map[string]any{
			"ip":        clientIP,
			"segmentum": segmentum,
			"role":      "client",
		},
		Discovered: map[string]any{
			"last_browser_visit": now.Format(time.RFC3339),
		},
	}

	if req.UserAgent != "" {
		node.Discovered["user_agent"] = req.UserAgent
	}

	node.LastVerified = &now
	node.LastSeen = &now

	if err := h.svc.CreateNode(r.Context(), node); err != nil {
		// Node might already exist from a scan - try update instead
		if existing, _ := h.svc.GetNode(r.Context(), nodeID); existing != nil {
			updates := map[string]interface{}{"last_seen": now}
			h.svc.UpdateNode(r.Context(), nodeID, updates)
			h.writeJSON(w, map[string]any{
				"status":    "updated",
				"node_id":   nodeID,
				"client_ip": clientIP,
				"segmentum": segmentum,
			}, http.StatusOK)
			return
		}
		log.Printf("Failed to create client node %s: %v", nodeID, err)
		h.writeError(w, "Failed to create client node", err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Registered new client: %s (segmentum: %s)", clientIP, segmentum)

	h.writeJSON(w, map[string]any{
		"status":    "created",
		"node_id":   nodeID,
		"client_ip": clientIP,
		"segmentum": segmentum,
	}, http.StatusCreated)
}

// getClientIP extracts the real client IP from the request
// Handles X-Forwarded-For and X-Real-IP headers from reverse proxies
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For first (may contain multiple IPs)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP (original client)
		if idx := strings.Index(xff, ","); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr (may include port)
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
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
	if err := json.NewEncoder(w).Encode(ErrorResponse{
		Error:   error,
		Details: details,
	}); err != nil {
		log.Printf("Failed to encode error response: %v", err)
	}
}

func extractPathParam(path, prefix string) string {
	if strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix)
	}
	return ""
}

// MergeRequest represents the request to merge nodes as interfaces
type MergeRequest struct {
	NodeIDs    []string `json:"node_ids"`
	ParentID   string   `json:"parent_id"`
	ParentType string   `json:"parent_type"`
}

// MergeResponse is returned after a successful merge
type MergeResponse struct {
	ParentID       string   `json:"parent_id"`
	InterfaceCount int      `json:"interface_count"`
	InterfaceIDs   []string `json:"interface_ids"`
}

// MergeNodes merges multiple nodes into a parent with interface children
func (h *GraphHandler) MergeNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, "Method not allowed", "", http.StatusMethodNotAllowed)
		return
	}

	var req MergeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, "Invalid request body", err.Error(), http.StatusBadRequest)
		return
	}

	if len(req.NodeIDs) < 2 {
		h.writeError(w, "At least 2 nodes required", "", http.StatusBadRequest)
		return
	}

	if req.ParentID == "" {
		h.writeError(w, "Parent ID is required", "", http.StatusBadRequest)
		return
	}

	if req.ParentType == "" {
		req.ParentType = "server" // Default type
	}

	// Call service to perform merge
	interfaceIDs, err := h.svc.MergeNodesAsInterfaces(r.Context(), req.NodeIDs, req.ParentID, domain.NodeType(req.ParentType))
	if err != nil {
		log.Printf("Failed to merge nodes: %v", err)
		h.writeError(w, "Failed to merge nodes", err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, MergeResponse{
		ParentID:       req.ParentID,
		InterfaceCount: len(interfaceIDs),
		InterfaceIDs:   interfaceIDs,
	}, http.StatusOK)
}
