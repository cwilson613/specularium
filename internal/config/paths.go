package config

import (
	"os"
	"path/filepath"
)

const (
	// EnvConfigPath is the environment variable for explicit config path
	EnvConfigPath = "SPECULARIUM_CONFIG"
	// ConfigFileName is the default config file name
	ConfigFileName = "specularium.yaml"
	// ConfigDirName is the config directory name under XDG
	ConfigDirName = "specularium"
)

// FindConfigPath searches for config file in priority order:
// 1. $SPECULARIUM_CONFIG (explicit path)
// 2. ./specularium.yaml (working directory)
// 3. $XDG_CONFIG_HOME/specularium/config.yaml
// 4. ~/.config/specularium/config.yaml
// 5. /etc/specularium/config.yaml
//
// Returns empty string if no config file found
func FindConfigPath() string {
	// 1. Explicit environment variable
	if path := os.Getenv(EnvConfigPath); path != "" {
		if fileExists(path) {
			return path
		}
	}

	// 2. Working directory
	if fileExists(ConfigFileName) {
		if abs, err := filepath.Abs(ConfigFileName); err == nil {
			return abs
		}
		return ConfigFileName
	}

	// 3. XDG config home
	if xdgHome := os.Getenv("XDG_CONFIG_HOME"); xdgHome != "" {
		path := filepath.Join(xdgHome, ConfigDirName, "config.yaml")
		if fileExists(path) {
			return path
		}
	}

	// 4. Default XDG location (~/.config)
	if home := os.Getenv("HOME"); home != "" {
		path := filepath.Join(home, ".config", ConfigDirName, "config.yaml")
		if fileExists(path) {
			return path
		}
	}

	// 5. System-wide
	systemPath := filepath.Join("/etc", ConfigDirName, "config.yaml")
	if fileExists(systemPath) {
		return systemPath
	}

	// No config found
	return ""
}

// DefaultConfigPath returns the preferred location for a new config file
// Prefers XDG config home, falls back to working directory
func DefaultConfigPath() string {
	// Prefer XDG config home
	if xdgHome := os.Getenv("XDG_CONFIG_HOME"); xdgHome != "" {
		return filepath.Join(xdgHome, ConfigDirName, "config.yaml")
	}

	// Default XDG location
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".config", ConfigDirName, "config.yaml")
	}

	// Fallback to working directory
	return ConfigFileName
}

// EnsureConfigDir creates the config directory if it doesn't exist
func EnsureConfigDir(configPath string) error {
	dir := filepath.Dir(configPath)
	return os.MkdirAll(dir, 0755)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
