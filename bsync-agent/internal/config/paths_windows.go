//go:build windows
// +build windows

package config

import (
	"os"
	"path/filepath"
)

// getConfigPaths returns platform-specific configuration paths for Windows
func getConfigPaths(configType string) []string {
	// Use standard Windows application data directories
	appData := os.Getenv("PROGRAMDATA")
	if appData == "" {
		appData = "C:\\ProgramData"
	}
	
	switch configType {
	case "server":
		return []string{
			filepath.Join(appData, "SyncTool", "config"),
			".\\configs\\",
			".",
		}
	case "agent":
		return []string{
			filepath.Join(appData, "SyncAgent", "config"),
			".\\configs\\",
			".",
		}
	default:
		return []string{".\\configs\\", "."}
	}
}

// getTLSDefaults returns platform-specific TLS certificate paths
func getTLSDefaults() (string, string, string) {
	appData := os.Getenv("PROGRAMDATA")
	if appData == "" {
		appData = "C:\\ProgramData"
	}
	
	certDir := filepath.Join(appData, "SyncTool", "certs")
	return filepath.Join(certDir, "server.crt"),
		filepath.Join(certDir, "server.key"),
		filepath.Join(certDir, "ca.crt")
}

// getLogFilePath returns platform-specific log file path
func getLogFilePath() string {
	appData := os.Getenv("PROGRAMDATA")
	if appData == "" {
		appData = "C:\\ProgramData"
	}
	return filepath.Join(appData, "SyncTool", "logs", "server.log")
}

// getSystemDirectories returns platform-specific system directories that need to be created
func getSystemDirectories() []string {
	appData := os.Getenv("PROGRAMDATA")
	if appData == "" {
		appData = "C:\\ProgramData"
	}
	
	return []string{
		filepath.Join(appData, "SyncTool", "logs"),
		filepath.Join(appData, "SyncTool", "data"),
		filepath.Join(appData, "SyncTool", "certs"),
	}
}