//go:build !windows
// +build !windows

package agent

import (
	"path/filepath"
)

// getRootPath returns the root path for Unix/Linux systems
func getRootPath() string {
	return "/"
}

// isRootPath checks if the given path is the root path
func isRootPath(path string) bool {
	return path == "" || path == "/" || path == "."
}

// normalizeRootPath normalizes empty or root paths to the system root
func normalizeRootPath(path string) string {
	if isRootPath(path) {
		return "/"
	}
	return filepath.Clean(path)
}

// getAvailableDrives returns available drives (for Unix, this is not used as we browse root directly)
func getAvailableDrives() ([]map[string]interface{}, error) {
	// For Unix/Linux, we don't have the concept of drives like Windows
	// This function shouldn't be called for Unix systems when browsing root
	// Instead, we should directly list the contents of "/"
	return nil, nil
}

// shouldShowDriveList returns true if we should show drive list instead of directory contents
func shouldShowDriveList() bool {
	return false // Unix systems don't have drive lists
}