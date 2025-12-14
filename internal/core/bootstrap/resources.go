package bootstrap

import (
	"runtime"
	"strconv"
	"strings"
)

// DetectResources gathers evidence about available system resources
func DetectResources() []Evidence {
	var evidence []Evidence

	// CPU cores (Go runtime - highly reliable)
	evidence = append(evidence, NewEvidence(
		CategoryResources,
		"cpu_cores",
		runtime.NumCPU(),
		0.99,
		"runtime",
		"runtime.NumCPU()",
	))

	// Architecture (certain)
	evidence = append(evidence, NewEvidence(
		CategoryResources,
		"architecture",
		runtime.GOARCH,
		1.0,
		"runtime",
		"runtime.GOARCH",
	))

	// OS (certain)
	evidence = append(evidence, NewEvidence(
		CategoryResources,
		"os",
		runtime.GOOS,
		1.0,
		"runtime",
		"runtime.GOOS",
	))

	// Memory from /proc/meminfo
	evidence = append(evidence, detectMemory()...)

	// Cgroup memory limit (for containers)
	evidence = append(evidence, detectCgroupMemory()...)

	// Cgroup CPU limit
	evidence = append(evidence, detectCgroupCPU()...)

	return evidence
}

func detectMemory() []Evidence {
	var evidence []Evidence

	memInfo := readFileSafe("/proc/meminfo")
	if memInfo == "" {
		return evidence
	}

	for _, line := range strings.Split(memInfo, "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.ParseInt(fields[1], 10, 64)
				if err == nil {
					memMB := int(kb / 1024)
					evidence = append(evidence, NewEvidence(
						CategoryResources,
						"memory_mb",
						memMB,
						0.95,
						"procfs",
						"/proc/meminfo MemTotal",
					).WithRaw(map[string]any{"memory_kb": kb}))
				}
			}
		}
		if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.ParseInt(fields[1], 10, 64)
				if err == nil {
					memMB := int(kb / 1024)
					evidence = append(evidence, NewEvidence(
						CategoryResources,
						"memory_available_mb",
						memMB,
						0.90,
						"procfs",
						"/proc/meminfo MemAvailable",
					))
				}
			}
		}
	}

	return evidence
}

func detectCgroupMemory() []Evidence {
	var evidence []Evidence

	// Try cgroup v2 first
	if limit := readFileSafe("/sys/fs/cgroup/memory.max"); limit != "" {
		s := strings.TrimSpace(limit)
		if s != "max" {
			if bytes, err := strconv.ParseInt(s, 10, 64); err == nil {
				memMB := int(bytes / 1024 / 1024)
				evidence = append(evidence, NewEvidence(
					CategoryResources,
					"memory_limit_mb",
					memMB,
					0.92,
					"cgroup",
					"cgroup v2 memory.max",
				).WithRaw(map[string]any{"cgroup_version": 2, "bytes": bytes}))
				return evidence
			}
		}
	}

	// Try cgroup v1
	if limit := readFileSafe("/sys/fs/cgroup/memory/memory.limit_in_bytes"); limit != "" {
		if bytes, err := strconv.ParseInt(strings.TrimSpace(limit), 10, 64); err == nil {
			// Check for "unlimited" (very large number, typically > 2^62)
			if bytes < 1<<62 {
				memMB := int(bytes / 1024 / 1024)
				evidence = append(evidence, NewEvidence(
					CategoryResources,
					"memory_limit_mb",
					memMB,
					0.90,
					"cgroup",
					"cgroup v1 memory.limit_in_bytes",
				).WithRaw(map[string]any{"cgroup_version": 1, "bytes": bytes}))
			}
		}
	}

	return evidence
}

func detectCgroupCPU() []Evidence {
	var evidence []Evidence

	// Try cgroup v2 cpu.max
	if cpuMax := readFileSafe("/sys/fs/cgroup/cpu.max"); cpuMax != "" {
		fields := strings.Fields(strings.TrimSpace(cpuMax))
		if len(fields) >= 2 && fields[0] != "max" {
			quota, err1 := strconv.ParseInt(fields[0], 10, 64)
			period, err2 := strconv.ParseInt(fields[1], 10, 64)
			if err1 == nil && err2 == nil && period > 0 {
				cpuLimit := float64(quota) / float64(period)
				evidence = append(evidence, NewEvidence(
					CategoryResources,
					"cpu_limit",
					cpuLimit,
					0.92,
					"cgroup",
					"cgroup v2 cpu.max quota/period",
				).WithRaw(map[string]any{
					"cgroup_version": 2,
					"quota":          quota,
					"period":         period,
				}))
			}
		}
	}

	// Try cgroup v1
	quota := readFileSafe("/sys/fs/cgroup/cpu/cpu.cfs_quota_us")
	period := readFileSafe("/sys/fs/cgroup/cpu/cpu.cfs_period_us")
	if quota != "" && period != "" {
		q, err1 := strconv.ParseInt(strings.TrimSpace(quota), 10, 64)
		p, err2 := strconv.ParseInt(strings.TrimSpace(period), 10, 64)
		if err1 == nil && err2 == nil && q > 0 && p > 0 {
			cpuLimit := float64(q) / float64(p)
			evidence = append(evidence, NewEvidence(
				CategoryResources,
				"cpu_limit",
				cpuLimit,
				0.90,
				"cgroup",
				"cgroup v1 cpu.cfs_quota_us/period",
			).WithRaw(map[string]any{
				"cgroup_version": 1,
				"quota_us":       q,
				"period_us":      p,
			}))
		}
	}

	return evidence
}
