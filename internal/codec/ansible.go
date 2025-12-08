package codec

import (
	"fmt"
	"io"
	"strings"

	"specularium/internal/domain"

	"gopkg.in/yaml.v3"
)

// AnsibleCodec handles Ansible inventory import/export
type AnsibleCodec struct{}

// NewAnsibleCodec creates a new Ansible codec
func NewAnsibleCodec() *AnsibleCodec {
	return &AnsibleCodec{}
}

// Format returns the codec format identifier
func (c *AnsibleCodec) Format() string {
	return "ansible-inventory"
}

// ansibleInventory represents the Ansible inventory structure
type ansibleInventory struct {
	All ansibleGroup `yaml:"all"`
}

type ansibleGroup struct {
	Children map[string]ansibleGroupDef `yaml:"children,omitempty"`
	Hosts    map[string]ansibleHost     `yaml:"hosts,omitempty"`
	Vars     map[string]interface{}     `yaml:"vars,omitempty"`
}

type ansibleGroupDef struct {
	Hosts map[string]ansibleHost `yaml:"hosts,omitempty"`
	Vars  map[string]interface{} `yaml:"vars,omitempty"`
}

type ansibleHost struct {
	AnsibleHost string                 `yaml:"ansible_host,omitempty"`
	Vars        map[string]interface{} `yaml:",inline"`
}

// Parse imports graph data from Ansible inventory
func (c *AnsibleCodec) Parse(r io.Reader) (*domain.GraphFragment, error) {
	var inv ansibleInventory
	decoder := yaml.NewDecoder(r)
	if err := decoder.Decode(&inv); err != nil {
		return nil, fmt.Errorf("failed to parse Ansible inventory: %w", err)
	}

	fragment := domain.NewGraphFragment()
	nodeMap := make(map[string]*domain.Node)

	// Find router/gateway for connection inference
	var routerID string

	// Process all groups
	for groupName, group := range inv.All.Children {
		for hostID, host := range group.Hosts {
			node := c.hostToNode(hostID, groupName, host)
			nodeMap[hostID] = &node
			fragment.AddNode(node)

			// Track potential router
			role := node.GetPropertyString("role")
			if strings.Contains(strings.ToLower(role), "router") ||
				strings.Contains(strings.ToLower(role), "gateway") {
				routerID = hostID
			}
		}
	}

	// Process hosts in the 'all' group directly
	for hostID, host := range inv.All.Hosts {
		if _, exists := nodeMap[hostID]; !exists {
			node := c.hostToNode(hostID, "all", host)
			nodeMap[hostID] = &node
			fragment.AddNode(node)

			role := node.GetPropertyString("role")
			if strings.Contains(strings.ToLower(role), "router") ||
				strings.Contains(strings.ToLower(role), "gateway") {
				routerID = hostID
			}
		}
	}

	// Infer connections - connect all hosts to router if found
	if routerID != "" {
		for hostID := range nodeMap {
			if hostID != routerID {
				edge := domain.NewEdge(hostID, routerID, domain.EdgeTypeEthernet)
				edge.SetProperty("speed", "1GbE")
				fragment.AddEdge(*edge)
			}
		}
	}

	return fragment, nil
}

// hostToNode converts an Ansible host to a domain.Node
func (c *AnsibleCodec) hostToNode(hostID, groupName string, host ansibleHost) domain.Node {
	node := domain.Node{
		ID:         hostID,
		Label:      hostID,
		Properties: make(map[string]any),
		Source:     "ansible",
	}

	// Add ansible_host as ip property
	if host.AnsibleHost != "" {
		node.SetProperty("ip", host.AnsibleHost)
	}

	// Add group as a property
	node.SetProperty("group", groupName)

	// Add all other host vars as properties
	for key, value := range host.Vars {
		if key == "ansible_host" {
			continue
		}
		node.SetProperty(key, value)
	}

	// Infer node type - check device_type first, then group name, then role
	node.Type = c.inferNodeType(groupName, host.Vars)

	return node
}

// inferNodeType infers the node type from host vars and group name
func (c *AnsibleCodec) inferNodeType(groupName string, vars map[string]interface{}) domain.NodeType {
	// First check device_type (explicit)
	if deviceType, ok := vars["device_type"].(string); ok {
		switch strings.ToLower(deviceType) {
		case "router", "gateway":
			return domain.NodeTypeRouter
		case "switch":
			return domain.NodeTypeSwitch
		case "access_point", "ap", "wifi":
			return domain.NodeTypeSwitch // APs are like switches
		case "controller":
			return domain.NodeTypeServer
		}
	}

	// Check role property
	if role, ok := vars["role"].(string); ok {
		roleLower := strings.ToLower(role)
		switch {
		case strings.Contains(roleLower, "router") || strings.Contains(roleLower, "gateway"):
			return domain.NodeTypeRouter
		case strings.Contains(roleLower, "switch"):
			return domain.NodeTypeSwitch
		case strings.Contains(roleLower, "ingress") || strings.Contains(roleLower, "loadbalancer"):
			return domain.NodeTypeVIP
		case strings.Contains(roleLower, "storage"):
			return domain.NodeTypeServer
		}
	}

	// Check group name
	groupLower := strings.ToLower(groupName)
	switch {
	case strings.Contains(groupLower, "network"):
		return domain.NodeTypeRouter // Network group likely contains network devices
	case strings.Contains(groupLower, "loadbalancer") || strings.Contains(groupLower, "k8s_loadbalancer"):
		return domain.NodeTypeVIP
	case strings.Contains(groupLower, "server") || strings.Contains(groupLower, "infrastructure"):
		return domain.NodeTypeServer
	case strings.Contains(groupLower, "switch"):
		return domain.NodeTypeSwitch
	case strings.Contains(groupLower, "router"):
		return domain.NodeTypeRouter
	case strings.Contains(groupLower, "vm") || strings.Contains(groupLower, "virtual"):
		return domain.NodeTypeVM
	case strings.Contains(groupLower, "container") || strings.Contains(groupLower, "docker"):
		return domain.NodeTypeContainer
	case strings.Contains(groupLower, "vip"):
		return domain.NodeTypeVIP
	}

	// Default to server
	return domain.NodeTypeServer
}

// Export exports graph data to Ansible inventory format
func (c *AnsibleCodec) Export(fragment *domain.GraphFragment, w io.Writer) error {
	inv := ansibleInventory{
		All: ansibleGroup{
			Children: make(map[string]ansibleGroupDef),
		},
	}

	// Group nodes by their type or group property
	groups := make(map[string]map[string]ansibleHost)

	for _, node := range fragment.Nodes {
		groupName := node.GetPropertyString("group")
		if groupName == "" {
			// Use node type as group name
			groupName = string(node.Type) + "s"
		}

		if groups[groupName] == nil {
			groups[groupName] = make(map[string]ansibleHost)
		}

		host := ansibleHost{
			Vars: make(map[string]interface{}),
		}

		// Extract ansible_host from properties
		if ip := node.GetPropertyString("ip"); ip != "" {
			host.AnsibleHost = ip
		}

		// Add other properties as vars
		for key, value := range node.Properties {
			if key != "ip" && key != "group" {
				host.Vars[key] = value
			}
		}

		groups[groupName][node.ID] = host
	}

	// Convert groups to Ansible format
	for groupName, hosts := range groups {
		inv.All.Children[groupName] = ansibleGroupDef{
			Hosts: hosts,
		}
	}

	encoder := yaml.NewEncoder(w)
	encoder.SetIndent(2)
	defer encoder.Close()

	if err := encoder.Encode(&inv); err != nil {
		return fmt.Errorf("failed to encode Ansible inventory: %w", err)
	}

	return nil
}
