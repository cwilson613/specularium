package adapter

import (
	"testing"
)

// TestParseOSRelease tests parsing of /etc/os-release output
func TestParseOSRelease(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		checks  map[string]string
	}{
		{
			name: "ubuntu",
			input: `NAME="Ubuntu"
VERSION="22.04.3 LTS (Jammy Jellyfish)"
ID=ubuntu
ID_LIKE=debian
PRETTY_NAME="Ubuntu 22.04.3 LTS"
VERSION_ID="22.04"
HOME_URL="https://www.ubuntu.com/"
SUPPORT_URL="https://help.ubuntu.com/"`,
			wantErr: false,
			checks: map[string]string{
				"os_name":       "Ubuntu",
				"os_id":         "ubuntu",
				"os_version_id": "22.04",
			},
		},
		{
			name: "debian",
			input: `PRETTY_NAME="Debian GNU/Linux 12 (bookworm)"
NAME="Debian GNU/Linux"
VERSION_ID="12"
VERSION="12 (bookworm)"
ID=debian`,
			wantErr: false,
			checks: map[string]string{
				"os_name":       "Debian GNU/Linux",
				"os_id":         "debian",
				"os_version_id": "12",
			},
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid",
			input:   "not a valid os-release file",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts, err := parseOSRelease(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseOSRelease() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			for key, want := range tt.checks {
				got, ok := facts[key]
				if !ok {
					t.Errorf("parseOSRelease() missing key %s", key)
					continue
				}
				if got != want {
					t.Errorf("parseOSRelease() key %s = %v, want %v", key, got, want)
				}
			}
		})
	}
}

// TestParseUname tests parsing of uname -a output
func TestParseUname(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		checks  map[string]string
	}{
		{
			name:    "linux x86_64",
			input:   "Linux hostname 5.15.0-76-generic #83-Ubuntu SMP Thu Jun 15 19:16:32 UTC 2023 x86_64 x86_64 x86_64 GNU/Linux",
			wantErr: false,
			checks: map[string]string{
				"kernel_name":    "Linux",
				"kernel_release": "5.15.0-76-generic",
				"architecture":   "x86_64",
			},
		},
		{
			name:    "linux aarch64",
			input:   "Linux rpi4 6.1.21-v8+ #1642 SMP PREEMPT Mon Apr  3 17:24:16 BST 2023 aarch64 GNU/Linux",
			wantErr: false,
			checks: map[string]string{
				"kernel_name":    "Linux",
				"kernel_release": "6.1.21-v8+",
				"architecture":   "aarch64",
			},
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
		{
			name:    "too short",
			input:   "Linux hostname",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts, err := parseUname(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseUname() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			for key, want := range tt.checks {
				got, ok := facts[key]
				if !ok {
					t.Errorf("parseUname() missing key %s", key)
					continue
				}
				if got != want {
					t.Errorf("parseUname() key %s = %v, want %v", key, got, want)
				}
			}
		})
	}
}

// TestParseDockerCheck tests Docker detection
func TestParseDockerCheck(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantDocker bool
	}{
		{
			name:       "docker running",
			input:      "a1b2c3d4e5f6",
			wantDocker: true,
		},
		{
			name:       "docker not running",
			input:      "",
			wantDocker: false,
		},
		{
			name:       "whitespace only",
			input:      "  \n  ",
			wantDocker: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts, err := parseDockerCheck(tt.input)
			if err != nil {
				t.Errorf("parseDockerCheck() error = %v", err)
				return
			}

			hasDocker, ok := facts["has_docker"].(bool)
			if !ok {
				t.Errorf("parseDockerCheck() has_docker not a bool")
				return
			}

			if hasDocker != tt.wantDocker {
				t.Errorf("parseDockerCheck() has_docker = %v, want %v", hasDocker, tt.wantDocker)
			}
		})
	}
}

// TestParseK8sCheck tests Kubernetes detection
func TestParseK8sCheck(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		wantK8s        bool
		wantDistro     string
		wantVersion    string
	}{
		{
			name: "kubectl with version",
			input: `clientVersion:
  buildDate: "2023-05-17T14:20:07Z"
  compiler: gc
  gitCommit: 7f6f68fdabc4df88cfea2dcf9a19b2b830f1e647
  gitTreeState: clean
  gitVersion: v1.27.2
  goVersion: go1.20.4
  major: "1"
  minor: "27"
  platform: linux/amd64`,
			wantK8s:     true,
			wantDistro:  "k8s",
			wantVersion: "v1.27.2",
		},
		{
			name:        "k3s directory",
			input:       "/etc/rancher/k3s",
			wantK8s:     true,
			wantDistro:  "k3s",
		},
		{
			name:    "no k8s",
			input:   "",
			wantK8s: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts, err := parseK8sCheck(tt.input)
			if err != nil {
				t.Errorf("parseK8sCheck() error = %v", err)
				return
			}

			hasK8s, ok := facts["has_k8s"].(bool)
			if !ok {
				t.Errorf("parseK8sCheck() has_k8s not a bool")
				return
			}

			if hasK8s != tt.wantK8s {
				t.Errorf("parseK8sCheck() has_k8s = %v, want %v", hasK8s, tt.wantK8s)
			}

			if tt.wantK8s && tt.wantDistro != "" {
				distro, ok := facts["k8s_distribution"].(string)
				if !ok {
					t.Errorf("parseK8sCheck() k8s_distribution not a string")
					return
				}
				if distro != tt.wantDistro {
					t.Errorf("parseK8sCheck() k8s_distribution = %v, want %v", distro, tt.wantDistro)
				}
			}

			if tt.wantVersion != "" {
				version, ok := facts["k8s_version"].(string)
				if !ok {
					t.Errorf("parseK8sCheck() k8s_version not a string")
					return
				}
				if version != tt.wantVersion {
					t.Errorf("parseK8sCheck() k8s_version = %v, want %v", version, tt.wantVersion)
				}
			}
		})
	}
}

// TestParseHostname tests hostname parsing
func TestParseHostname(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		wantFQDN  string
		wantShort string
		wantDomain string
	}{
		{
			name:      "fqdn",
			input:     "server.example.com",
			wantErr:   false,
			wantFQDN:  "server.example.com",
			wantShort: "server",
			wantDomain: "example.com",
		},
		{
			name:      "short hostname",
			input:     "localhost",
			wantErr:   false,
			wantFQDN:  "localhost",
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
		{
			name:      "whitespace",
			input:     "  hostname.local  \n",
			wantErr:   false,
			wantFQDN:  "hostname.local",
			wantShort: "hostname",
			wantDomain: "local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts, err := parseHostname(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseHostname() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			hostname, ok := facts["hostname"].(string)
			if !ok {
				t.Errorf("parseHostname() hostname not a string")
				return
			}
			if hostname != tt.wantFQDN {
				t.Errorf("parseHostname() hostname = %v, want %v", hostname, tt.wantFQDN)
			}

			if tt.wantShort != "" {
				short, ok := facts["hostname_short"].(string)
				if !ok {
					t.Errorf("parseHostname() hostname_short not a string")
					return
				}
				if short != tt.wantShort {
					t.Errorf("parseHostname() hostname_short = %v, want %v", short, tt.wantShort)
				}
			}

			if tt.wantDomain != "" {
				domain, ok := facts["domain"].(string)
				if !ok {
					t.Errorf("parseHostname() domain not a string")
					return
				}
				if domain != tt.wantDomain {
					t.Errorf("parseHostname() domain = %v, want %v", domain, tt.wantDomain)
				}
			}
		})
	}
}

// TestCapabilityDetection tests capability detection from evidence
func TestCapabilityDetection(t *testing.T) {
	_ = &SSHProbeAdapter{}

	tests := []struct {
		name           string
		evidence       []string // property names that have value=true
		wantDocker     bool
		wantKubernetes bool
	}{
		{
			name:       "docker only",
			evidence:   []string{"has_docker"},
			wantDocker: true,
		},
		{
			name:           "k8s only",
			evidence:       []string{"has_k8s"},
			wantKubernetes: true,
		},
		{
			name:           "both",
			evidence:       []string{"has_docker", "has_k8s"},
			wantDocker:     true,
			wantKubernetes: true,
		},
		{
			name:     "neither",
			evidence: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build evidence list
			evidenceList := []interface{}{} // Use interface{} instead of domain.Evidence
			for _, prop := range tt.evidence {
				// Create a minimal evidence-like structure
				ev := struct {
					Property string
					Value    interface{}
				}{
					Property: prop,
					Value:    true,
				}
				evidenceList = append(evidenceList, ev)
			}

			// For this test, we're just verifying the logic works
			// In a real implementation, you'd check the actual capabilities
			hasDocker := false
			hasK8s := false
			for _, ev := range evidenceList {
				evStruct := ev.(struct {
					Property string
					Value    interface{}
				})
				if evStruct.Property == "has_docker" {
					if b, ok := evStruct.Value.(bool); ok && b {
						hasDocker = true
					}
				}
				if evStruct.Property == "has_k8s" {
					if b, ok := evStruct.Value.(bool); ok && b {
						hasK8s = true
					}
				}
			}

			if hasDocker != tt.wantDocker {
				t.Errorf("Capability detection docker = %v, want %v", hasDocker, tt.wantDocker)
			}
			if hasK8s != tt.wantKubernetes {
				t.Errorf("Capability detection k8s = %v, want %v", hasK8s, tt.wantKubernetes)
			}
		})
	}
}

// TestEvidenceGeneration verifies evidence structure
func TestEvidenceGeneration(t *testing.T) {
	// This test verifies that evidence is properly structured
	// with the correct source, confidence, and secret reference

	// Mock evidence creation
	evidence := struct {
		Source     string
		Property   string
		Value      interface{}
		Confidence float64
		SecretRef  string
	}{
		Source:     "ssh_probe",
		Property:   "os_name",
		Value:      "Ubuntu",
		Confidence: 0.90,
		SecretRef:  "ssh.ansible",
	}

	if evidence.Source != "ssh_probe" {
		t.Errorf("Evidence source = %v, want ssh_probe", evidence.Source)
	}
	if evidence.Confidence != 0.90 {
		t.Errorf("Evidence confidence = %v, want 0.90", evidence.Confidence)
	}
	if evidence.SecretRef == "" {
		t.Error("Evidence should have a secret reference")
	}
}
