package bootstrap

import (
	"context"
	"net"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"syscall"
	"time"
)

// DetectPermissions probes what operations we're allowed to perform
func DetectPermissions() []Evidence {
	var evidence []Evidence

	// Current user info
	evidence = append(evidence, detectUserInfo()...)

	// Capability probes
	evidence = append(evidence, probeCapabilities()...)

	return evidence
}

func detectUserInfo() []Evidence {
	var evidence []Evidence

	// Effective UID
	euid := os.Geteuid()
	evidence = append(evidence, NewEvidence(
		CategoryPermissions,
		"effective_uid",
		euid,
		1.0,
		"syscall",
		"os.Geteuid()",
	))

	// Is root?
	isRoot := euid == 0
	evidence = append(evidence, NewEvidence(
		CategoryPermissions,
		"is_root",
		isRoot,
		1.0,
		"syscall",
		"os.Geteuid() == 0",
	))

	// Real UID
	ruid := os.Getuid()
	evidence = append(evidence, NewEvidence(
		CategoryPermissions,
		"real_uid",
		ruid,
		1.0,
		"syscall",
		"os.Getuid()",
	))

	// Username
	if u, err := user.Current(); err == nil {
		evidence = append(evidence, NewEvidence(
			CategoryPermissions,
			"username",
			u.Username,
			0.99,
			"user",
			"user.Current().Username",
		).WithRaw(map[string]any{
			"uid":      u.Uid,
			"gid":      u.Gid,
			"home_dir": u.HomeDir,
		}))
	}

	// Groups
	if gids, err := os.Getgroups(); err == nil {
		evidence = append(evidence, NewEvidence(
			CategoryPermissions,
			"groups",
			gids,
			0.95,
			"syscall",
			"os.Getgroups()",
		))
	}

	return evidence
}

func probeCapabilities() []Evidence {
	var evidence []Evidence

	// Probe: Can read /proc filesystem?
	evidence = append(evidence, probeReadProc()...)

	// Probe: Can execute ping?
	evidence = append(evidence, probePing()...)

	// Probe: Can bind to low ports?
	evidence = append(evidence, probeLowPorts()...)

	// Probe: Can create raw sockets?
	evidence = append(evidence, probeRawSocket()...)

	// Probe: Is nmap available?
	evidence = append(evidence, probeNmap()...)

	// Probe: Can read network tables?
	evidence = append(evidence, probeNetworkTables()...)

	return evidence
}

func probeReadProc() []Evidence {
	// Try reading /proc/1/cmdline (PID 1's command)
	_, err := os.ReadFile("/proc/1/cmdline")
	success := err == nil

	conf := 0.95
	method := "read /proc/1/cmdline succeeded"
	if !success {
		conf = 0.90
		method = "read /proc/1/cmdline failed: " + err.Error()
	}

	return []Evidence{NewEvidence(
		CategoryCapability,
		"can_read_procfs",
		success,
		conf,
		"probe",
		method,
	)}
}

func probePing() []Evidence {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Check if ping exists
	path, err := exec.LookPath("ping")
	if err != nil {
		return []Evidence{NewEvidence(
			CategoryCapability,
			"can_icmp_ping",
			false,
			0.90,
			"probe",
			"ping binary not found in PATH",
		)}
	}

	// Try to execute (to 127.0.0.1, should always work if permitted)
	cmd := exec.CommandContext(ctx, path, "-c", "1", "-W", "1", "127.0.0.1")
	err = cmd.Run()
	success := err == nil

	method := "ping -c 1 127.0.0.1 succeeded"
	if !success {
		method = "ping -c 1 127.0.0.1 failed"
	}

	return []Evidence{NewEvidence(
		CategoryCapability,
		"can_icmp_ping",
		success,
		0.95,
		"probe",
		method,
	).WithRaw(map[string]any{"ping_path": path})}
}

func probeLowPorts() []Evidence {
	// Try to bind to port 80 briefly
	ln, err := net.Listen("tcp", "127.0.0.1:80")
	if err == nil {
		ln.Close()
		return []Evidence{NewEvidence(
			CategoryCapability,
			"can_bind_low_ports",
			true,
			0.95,
			"probe",
			"successfully bound to port 80",
		)}
	}

	return []Evidence{NewEvidence(
		CategoryCapability,
		"can_bind_low_ports",
		false,
		0.90,
		"probe",
		"failed to bind to port 80: "+err.Error(),
	)}
}

func probeRawSocket() []Evidence {
	// Try to create a raw ICMP socket (requires CAP_NET_RAW or root)
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_ICMP)
	if err == nil {
		syscall.Close(fd)
		return []Evidence{NewEvidence(
			CategoryCapability,
			"can_raw_socket",
			true,
			0.95,
			"probe",
			"successfully created ICMP raw socket",
		)}
	}

	// Also try unprivileged ICMP (requires net.ipv4.ping_group_range)
	fd, err = syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_ICMP)
	if err == nil {
		syscall.Close(fd)
		return []Evidence{NewEvidence(
			CategoryCapability,
			"can_raw_socket",
			true,
			0.90,
			"probe",
			"can create unprivileged ICMP socket (SOCK_DGRAM)",
		)}
	}

	return []Evidence{NewEvidence(
		CategoryCapability,
		"can_raw_socket",
		false,
		0.90,
		"probe",
		"failed to create raw socket: "+err.Error(),
	)}
}

func probeNmap() []Evidence {
	path, err := exec.LookPath("nmap")
	if err != nil {
		return []Evidence{NewEvidence(
			CategoryCapability,
			"has_nmap",
			false,
			0.95,
			"probe",
			"nmap not in PATH",
		)}
	}

	// Check version to confirm it works
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "--version")
	output, err := cmd.Output()
	if err != nil {
		return []Evidence{NewEvidence(
			CategoryCapability,
			"has_nmap",
			false,
			0.85,
			"probe",
			"nmap exists but --version failed: "+err.Error(),
		).WithRaw(map[string]any{"nmap_path": path})}
	}

	// Extract version from first line
	version := strings.Split(string(output), "\n")[0]

	return []Evidence{NewEvidence(
		CategoryCapability,
		"has_nmap",
		true,
		0.99,
		"probe",
		"nmap --version succeeded",
	).WithRaw(map[string]any{
		"nmap_path":    path,
		"nmap_version": version,
	})}
}

func probeNetworkTables() []Evidence {
	var evidence []Evidence

	// Try to read /proc/net/tcp
	if _, err := os.ReadFile("/proc/net/tcp"); err == nil {
		evidence = append(evidence, NewEvidence(
			CategoryCapability,
			"can_read_net_tcp",
			true,
			0.95,
			"probe",
			"can read /proc/net/tcp",
		))
	} else {
		evidence = append(evidence, NewEvidence(
			CategoryCapability,
			"can_read_net_tcp",
			false,
			0.90,
			"probe",
			"cannot read /proc/net/tcp: "+err.Error(),
		))
	}

	// Try to read /proc/net/arp
	if _, err := os.ReadFile("/proc/net/arp"); err == nil {
		evidence = append(evidence, NewEvidence(
			CategoryCapability,
			"can_read_arp",
			true,
			0.95,
			"probe",
			"can read /proc/net/arp",
		))
	}

	return evidence
}
