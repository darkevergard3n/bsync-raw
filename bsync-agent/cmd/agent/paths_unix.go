//go:build !windows
// +build !windows

package main

import (
	"os"
	"path/filepath"
)

// getTempDir returns the temporary directory for Unix systems
func getTempDir() string {
	return "/tmp"
}

// getTestTriggerPath returns the test trigger file path for Unix systems
func getTestTriggerPath() string {
	return "/tmp/test-pause-trigger.txt"
}

// getCertPaths returns certificate file paths for Unix systems
func getCertPaths() (certFile, keyFile string) {
	tempDir := getTempDir()
	return filepath.Join(tempDir, "synctool-cert.pem"), 
		   filepath.Join(tempDir, "synctool-key.pem")
}

// getDefaultDataDir returns default data directory for Unix systems
func getDefaultDataDir() string {
	return "./data"
}

// createConfigDirs creates necessary configuration directories for Unix
func createConfigDirs(baseDir string) error {
	dirs := []string{
		baseDir,
		filepath.Join(baseDir, "config"),
		filepath.Join(baseDir, "data"),
		filepath.Join(baseDir, "logs"),
	}
	
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}