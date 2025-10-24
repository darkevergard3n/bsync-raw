//go:build !windows
// +build !windows

package server

// getTempCertPaths returns platform-specific temporary certificate paths for Unix/Linux systems
func getTempCertPaths() (string, string) {
	return "/tmp/synctool-cert.pem", "/tmp/synctool-key.pem"
}