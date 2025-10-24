//go:build windows
// +build windows

package agent

import (
	"os"
	"path/filepath"
	"runtime"
)

// getOSDistribution detects the Windows version for Windows systems
func getOSDistribution() string {
	// Start with basic Windows info
	osName := "Windows"
	
	// Try to get more specific Windows version info
	if version := getWindowsVersion(); version != "" {
		return version
	}
	
	// Fallback to basic detection
	return osName
}

// getTestTriggerPath returns the test trigger file path for Windows systems
// Uses Windows temporary directory instead of /tmp/
func getTestTriggerPath() string {
	tempDir := os.TempDir()
	return filepath.Join(tempDir, "test-pause-trigger.txt")
}

// getWindowsVersion attempts to detect specific Windows version
func getWindowsVersion() string {
	// For Go 1.12, we have limited options for Windows version detection
	// We'll use runtime info and environment variables
	
	// Check if we're on Windows Server
	if productName := os.Getenv("OS"); productName != "" {
		if productName == "Windows_NT" {
			// Try to detect server vs client
			if serverCore := os.Getenv("SERVER_CORE"); serverCore != "" {
				return "Windows Server (Core)"
			}
			
			// Check for common server indicators
			if _, err := os.Stat(`C:\Windows\System32\ServerManager.exe`); err == nil {
				return "Windows Server"
			}
			
			return "Windows"
		}
	}
	
	// Fallback to runtime info
	return runtime.GOOS
}