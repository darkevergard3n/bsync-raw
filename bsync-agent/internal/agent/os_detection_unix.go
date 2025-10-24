//go:build !windows
// +build !windows

package agent

import (
	"io/ioutil"
	"os"
	"strings"
)

// getOSDistribution detects the specific OS distribution for Unix systems
// This function wraps the existing getLinuxDistribution() to maintain compatibility
func getOSDistribution() string {
	return getLinuxDistribution()
}

// getTestTriggerPath returns the test trigger file path for Unix systems  
// This maintains the existing behavior of using /tmp/
func getTestTriggerPath() string {
	return "/tmp/test-pause-trigger.txt"
}

// Cross-platform wrapper functions that maintain existing Unix behavior

// getLinuxDistribution detects the Linux distribution
// NOTE: This is the ORIGINAL function moved here to maintain exact same behavior
func getLinuxDistribution() string {
	// Try /etc/os-release first (most modern distributions)
	if data, err := ioutil.ReadFile("/etc/os-release"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				name := strings.TrimPrefix(line, "PRETTY_NAME=")
				name = strings.Trim(name, "\"")
				return name
			}
		}
	}
	
	// Fallback to checking specific files
	if _, err := os.Stat("/etc/ubuntu_version"); err == nil {
		return "Ubuntu"
	}
	if _, err := os.Stat("/etc/centos-release"); err == nil {
		return "CentOS"
	}
	if _, err := os.Stat("/etc/debian_version"); err == nil {
		return "Debian"
	}
	if _, err := os.Stat("/etc/redhat-release"); err == nil {
		return "RedHat"
	}
	
	// Default fallback
	return "Linux"
}