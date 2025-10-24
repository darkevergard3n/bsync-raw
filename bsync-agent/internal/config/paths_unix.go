//go:build !windows
// +build !windows

package config

import (
	"path/filepath"
)

// getConfigPaths returns platform-specific configuration paths for Unix/Linux systems
func getConfigPaths(configType string) []string {
	switch configType {
	case "server":
		return []string{
			"/etc/synctool/",
			"./configs/",
			".",
		}
	case "agent":
		return []string{
			"/etc/sync-agent/",
			"./configs/",
			".",
		}
	default:
		return []string{"./configs/", "."}
	}
}

// getTLSDefaults returns platform-specific TLS certificate paths
func getTLSDefaults() (string, string, string) {
	return "/etc/synctool/certs/server.crt",
		"/etc/synctool/certs/server.key",
		"/etc/synctool/certs/ca.crt"
}

// getLogFilePath returns platform-specific log file path
func getLogFilePath() string {
	return "/var/log/synctool/server.log"
}

// getSystemDirectories returns platform-specific system directories that need to be created
func getSystemDirectories() []string {
	return []string{
		"/var/log/synctool",
		"/var/lib/synctool",
		"/etc/synctool/certs",
	}
}