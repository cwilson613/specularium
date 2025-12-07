package domain

import (
	"crypto/sha256"
	"fmt"
	"sort"
)

// ConnectionType indicates the purpose of a network connection
type ConnectionType string

const (
	ConnTypeUplink     ConnectionType = "uplink"
	ConnTypeTrunk      ConnectionType = "trunk"
	ConnTypeAccess     ConnectionType = "access"
	ConnTypeCluster    ConnectionType = "cluster"
	ConnTypeStorage    ConnectionType = "storage"
	ConnTypeManagement ConnectionType = "management"
)

// Connection represents a network link between two hosts
type Connection struct {
	ID         string         `json:"id"`
	SourceID   string         `json:"source_id"`
	SourcePort string         `json:"source_port,omitempty"`
	TargetID   string         `json:"target_id"`
	TargetPort string         `json:"target_port,omitempty"`
	SpeedGbps  int            `json:"speed_gbps"`
	Type       ConnectionType `json:"type,omitempty"`
}

// GenerateID creates a deterministic ID for the connection based on endpoints
// This ensures the same connection always gets the same ID regardless of direction
func (c *Connection) GenerateID() string {
	// Sort endpoints to ensure consistent ID regardless of direction
	endpoints := []string{c.SourceID, c.TargetID}
	sort.Strings(endpoints)

	key := fmt.Sprintf("%s:%s-%s:%s",
		endpoints[0], c.portOrDefault(endpoints[0] == c.SourceID),
		endpoints[1], c.portOrDefault(endpoints[1] == c.SourceID))

	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", hash[:8])
}

func (c *Connection) portOrDefault(isSource bool) string {
	if isSource {
		if c.SourcePort != "" {
			return c.SourcePort
		}
	} else {
		if c.TargetPort != "" {
			return c.TargetPort
		}
	}
	return "default"
}

// Normalize ensures the connection has consistent ordering (source < target alphabetically)
func (c *Connection) Normalize() {
	if c.SourceID > c.TargetID {
		c.SourceID, c.TargetID = c.TargetID, c.SourceID
		c.SourcePort, c.TargetPort = c.TargetPort, c.SourcePort
	}
}

// Involves checks if this connection involves the given host ID
func (c *Connection) Involves(hostID string) bool {
	return c.SourceID == hostID || c.TargetID == hostID
}

// OtherEnd returns the host ID on the other end of this connection
func (c *Connection) OtherEnd(hostID string) string {
	if c.SourceID == hostID {
		return c.TargetID
	}
	return c.SourceID
}
