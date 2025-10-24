//go:build windows
// +build windows

package main

import (
	"os"
	"path/filepath"
)

// getTempDir returns the temporary directory for Windows systems
func getTempDir() string {
	if tempDir := os.Getenv("TEMP"); tempDir != "" {
		return tempDir
	}
	if tempDir := os.Getenv("TMP"); tempDir != "" {
		return tempDir
	}
	return `C:\Windows\Temp`
}

// getTestTriggerPath returns the test trigger file path for Windows systems
func getTestTriggerPath() string {
	return filepath.Join(getTempDir(), "test-pause-trigger.txt")
}

// getCertPaths returns certificate file paths for Windows systems
func getCertPaths() (certFile, keyFile string) {
	tempDir := getTempDir()
	return filepath.Join(tempDir, "synctool-cert.pem"), 
		   filepath.Join(tempDir, "synctool-key.pem")
}

// getDefaultDataDir returns default data directory for Windows systems
func getDefaultDataDir() string {
	if appData := os.Getenv("PROGRAMDATA"); appData != "" {
		return filepath.Join(appData, "SyncTool", "data")
	}
	return `C:\ProgramData\SyncTool\data`
}

// createConfigDirs creates necessary configuration directories for Windows
func createConfigDirs(baseDir string) error {
	dirs := []string{
		baseDir,
		filepath.Join(baseDir, "config"),
		filepath.Join(baseDir, "data"),
		filepath.Join(baseDir, "logs"),
	}
	
	for _, dir := range dirs {
		// Windows doesn't use Unix permissions - use 0666 for compatibility
		if err := os.MkdirAll(dir, 0666); err != nil {
			return err
		}
	}
	return nil
}