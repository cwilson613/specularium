package domain

import "time"

// Infrastructure represents the complete network infrastructure
type Infrastructure struct {
	Version     string                 `json:"version" yaml:"version"`
	LastUpdated time.Time              `json:"last_updated" yaml:"-"`
	Metadata    *Metadata              `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Hosts       map[string]*Host       `json:"hosts" yaml:"hosts"`
	Connections map[string]*Connection `json:"connections" yaml:"-"`
	Groups      map[string]*HostGroup  `json:"groups,omitempty" yaml:"groups,omitempty"`
}

// Metadata contains network-wide configuration
type Metadata struct {
	Network     *NetworkMetadata `json:"network,omitempty" yaml:"network,omitempty"`
	Description string           `json:"description,omitempty" yaml:"description,omitempty"`
}

// NetworkMetadata contains network configuration details
type NetworkMetadata struct {
	CIDR    string          `json:"cidr,omitempty" yaml:"cidr,omitempty"`
	Gateway string          `json:"gateway,omitempty" yaml:"gateway,omitempty"`
	Domains *DomainMetadata `json:"domains,omitempty" yaml:"domains,omitempty"`
}

// DomainMetadata contains domain configuration
type DomainMetadata struct {
	Internal string `json:"internal,omitempty" yaml:"internal,omitempty"`
	External string `json:"external,omitempty" yaml:"external,omitempty"`
}

// HostGroup represents a logical grouping of hosts
type HostGroup struct {
	Members     []string               `json:"members" yaml:"members"`
	Description string                 `json:"description,omitempty" yaml:"description,omitempty"`
	Vars        map[string]interface{} `json:"vars,omitempty" yaml:"vars,omitempty"`
}

// NewInfrastructure creates an empty infrastructure with initialized maps
func NewInfrastructure() *Infrastructure {
	return &Infrastructure{
		Version:     "1.0",
		LastUpdated: time.Now(),
		Hosts:       make(map[string]*Host),
		Connections: make(map[string]*Connection),
		Groups:      make(map[string]*HostGroup),
	}
}

// GetHost returns a host by ID, or nil if not found
func (i *Infrastructure) GetHost(id string) *Host {
	return i.Hosts[id]
}

// AddHost adds or updates a host
func (i *Infrastructure) AddHost(host *Host) {
	if host.ID == "" {
		return
	}
	i.Hosts[host.ID] = host
	i.LastUpdated = time.Now()
}

// RemoveHost removes a host and all its connections
func (i *Infrastructure) RemoveHost(id string) {
	delete(i.Hosts, id)

	// Remove connections involving this host
	for connID, conn := range i.Connections {
		if conn.Involves(id) {
			delete(i.Connections, connID)
		}
	}

	i.LastUpdated = time.Now()
}

// AddConnection adds a connection, generating its ID if not set
func (i *Infrastructure) AddConnection(conn *Connection) {
	conn.Normalize()
	if conn.ID == "" {
		conn.ID = conn.GenerateID()
	}
	i.Connections[conn.ID] = conn
	i.LastUpdated = time.Now()
}

// GetConnectionsFor returns all connections involving the given host
func (i *Infrastructure) GetConnectionsFor(hostID string) []*Connection {
	var result []*Connection
	for _, conn := range i.Connections {
		if conn.Involves(hostID) {
			result = append(result, conn)
		}
	}
	return result
}

// BuildGroupMembership populates the Groups field on each host
func (i *Infrastructure) BuildGroupMembership() {
	// Clear existing group memberships
	for _, host := range i.Hosts {
		host.Groups = nil
	}

	// Build from groups
	for groupName, group := range i.Groups {
		for _, memberID := range group.Members {
			if host := i.Hosts[memberID]; host != nil {
				host.Groups = append(host.Groups, groupName)
			}
		}
	}
}

// ExtractConnections derives connections from host port definitions
func (i *Infrastructure) ExtractConnections() {
	seen := make(map[string]bool)

	for hostID, host := range i.Hosts {
		if host.Network == nil {
			continue
		}

		for _, port := range host.Network.Ports {
			if port.ConnectedTo == "" {
				continue
			}

			// Parse "target:port" format
			targetID, targetPort := parseConnectedTo(port.ConnectedTo)
			if targetID == "" || i.Hosts[targetID] == nil {
				continue
			}

			conn := &Connection{
				SourceID:   hostID,
				SourcePort: port.Name,
				TargetID:   targetID,
				TargetPort: targetPort,
				SpeedGbps:  port.SpeedGbps,
			}
			conn.Normalize()
			conn.ID = conn.GenerateID()

			// Deduplicate
			if !seen[conn.ID] {
				seen[conn.ID] = true
				i.Connections[conn.ID] = conn
			}
		}
	}
}

// parseConnectedTo splits "target:port" into target and port
func parseConnectedTo(s string) (target, port string) {
	for i, c := range s {
		if c == ':' {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}
