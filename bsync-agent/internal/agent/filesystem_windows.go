//go:build windows
// +build windows

package agent

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// getRootPath returns the root path for Windows systems (list of drives)
func getRootPath() string {
	return "drives://"
}

// isRootPath checks if the given path is the root path (empty, "/", or drives request)
func isRootPath(path string) bool {
	return path == "" || path == "/" || path == "." || path == "drives://"
}

// normalizeRootPath normalizes empty or root paths to the system root
func normalizeRootPath(path string) string {
	if isRootPath(path) {
		return "drives://"
	}
	// Convert forward slashes to backslashes for Windows
	path = strings.ReplaceAll(path, "/", "\\")
	return filepath.Clean(path)
}

// getAvailableDrives returns available drives on Windows
func getAvailableDrives() ([]map[string]interface{}, error) {
	drives, err := getLogicalDrives()
	if err != nil {
		return nil, err
	}
	
	var result []map[string]interface{}
	for _, drive := range drives {
		// Get drive type and info
		driveType := getDriveType(drive)
		
		driveInfo := map[string]interface{}{
			"name":         drive,
			"path":         drive,
			"is_directory": true,
			"is_drive":     true,
			"drive_type":   driveType,
		}
		
		// Try to get volume information
		if volumeLabel, err := getVolumeLabel(drive); err == nil {
			driveInfo["volume_label"] = volumeLabel
		}
		
		result = append(result, driveInfo)
	}
	
	return result, nil
}

// getLogicalDrives returns a list of logical drives on Windows
func getLogicalDrives() ([]string, error) {
	kernel32, err := syscall.LoadLibrary("kernel32.dll")
	if err != nil {
		return nil, err
	}
	defer syscall.FreeLibrary(kernel32)
	
	getLogicalDrivesProc, err := syscall.GetProcAddress(kernel32, "GetLogicalDrives")
	if err != nil {
		return nil, err
	}
	
	ret, _, _ := syscall.Syscall(getLogicalDrivesProc, 0, 0, 0, 0)
	
	var drives []string
	for i := 0; i < 26; i++ {
		if ret&(1<<uint(i)) != 0 {
			drives = append(drives, string(rune('A'+i))+":\\")
		}
	}
	
	return drives, nil
}

// getDriveType returns the type of drive (fixed, removable, network, etc.)
func getDriveType(drive string) string {
	kernel32, err := syscall.LoadLibrary("kernel32.dll")
	if err != nil {
		return "unknown"
	}
	defer syscall.FreeLibrary(kernel32)
	
	getDriveTypeProc, err := syscall.GetProcAddress(kernel32, "GetDriveTypeW")
	if err != nil {
		return "unknown"
	}
	
	utf16Drive, _ := syscall.UTF16PtrFromString(drive)
	ret, _, _ := syscall.Syscall(getDriveTypeProc, 1, uintptr(unsafe.Pointer(utf16Drive)), 0, 0)
	
	switch ret {
	case 2:
		return "removable"
	case 3:
		return "fixed"
	case 4:
		return "network"
	case 5:
		return "cdrom"
	case 6:
		return "ramdisk"
	default:
		return "unknown"
	}
}

// getVolumeLabel returns the volume label for a drive
func getVolumeLabel(drive string) (string, error) {
	kernel32, err := syscall.LoadLibrary("kernel32.dll")
	if err != nil {
		return "", err
	}
	defer syscall.FreeLibrary(kernel32)
	
	getVolumeInformationProc, err := syscall.GetProcAddress(kernel32, "GetVolumeInformationW")
	if err != nil {
		return "", err
	}
	
	utf16Drive, _ := syscall.UTF16PtrFromString(drive)
	volumeNameBuffer := make([]uint16, 256)
	
	ret, _, _ := syscall.Syscall9(getVolumeInformationProc, 8,
		uintptr(unsafe.Pointer(utf16Drive)),
		uintptr(unsafe.Pointer(&volumeNameBuffer[0])),
		uintptr(len(volumeNameBuffer)),
		0, 0, 0, 0, 0, 0)
	
	if ret == 0 {
		return "", syscall.GetLastError()
	}
	
	return syscall.UTF16ToString(volumeNameBuffer), nil
}

// shouldShowDriveList returns true if we should show drive list instead of directory contents  
func shouldShowDriveList() bool {
	return true // Windows systems should show drive list for root
}