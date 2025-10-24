//go:build windows
// +build windows

package server

import (
	"os"
	"path/filepath"
)

// getTempCertPaths returns platform-specific temporary certificate paths for Windows
func getTempCertPaths() (string, string) {
	tempDir := os.Getenv("TEMP")
	if tempDir == "" {
		tempDir = os.Getenv("TMP")
		if tempDir == "" {
			tempDir = "C:\\Windows\\Temp"
		}
	}
	
	return filepath.Join(tempDir, "synctool-cert.pem"),
		filepath.Join(tempDir, "synctool-key.pem")
}