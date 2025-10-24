package embedded

import (
	"crypto/tls"
	"fmt"
	"log"
	"path/filepath"
	"time"
)

// EmbeddedSyncthing represents the embedded Syncthing instance
type EmbeddedSyncthing struct {
	// Real implementation
	real        *RealEmbeddedSyncthing
	
	// Core Syncthing components (will be actual Syncthing types later)
	dataDir     string
	deviceID    string
	cert        tls.Certificate
	
	// Event system
	eventChan   chan Event
	subscribers map[string]chan Event
	
	// Configuration
	config      *SyncthingConfig
	
	// Status
	running     bool
	initialized bool
}

// Event represents a Syncthing event
type Event struct {
	Type      EventType              `json:"type"`
	Time      time.Time             `json:"time"`
	Data      map[string]interface{} `json:"data"`
}

// FolderType represents the sync direction of a folder
type FolderType string

const (
	FolderTypeSendReceive FolderType = "sendreceive" // Two-way sync (default)
	FolderTypeSendOnly    FolderType = "sendonly"    // Only send changes, don't receive
	FolderTypeReceiveOnly FolderType = "receiveonly" // Only receive changes, don't send
)

// EventType represents different types of Syncthing events
type EventType string

const (
	EventStarting         EventType = "Starting"
	EventStartupComplete  EventType = "StartupComplete"
	EventItemStarted      EventType = "ItemStarted"
	EventItemFinished     EventType = "ItemFinished"
	EventDownloadProgress EventType = "DownloadProgress"
	EventFolderSummary        EventType = "FolderSummary"
	EventFolderCompletion     EventType = "FolderCompletion"
	EventFolderErrors         EventType = "FolderErrors"
	EventDeviceConnected      EventType = "DeviceConnected"
	EventDeviceDisconnected   EventType = "DeviceDisconnected"
	EventDeviceDiscovered     EventType = "DeviceDiscovered"
	EventDeviceRejected       EventType = "DeviceRejected"
	EventLocalChangeDetected  EventType = "LocalChangeDetected"
	EventRemoteChangeDetected EventType = "RemoteChangeDetected"
	EventFolderScanProgress   EventType = "FolderScanProgress"
	EventFolderRejected       EventType = "FolderRejected"
	EventConfigSaved          EventType = "ConfigSaved"
	EventRemoteDownloadProgress EventType = "RemoteDownloadProgress"
	EventStateChanged         EventType = "StateChanged"
	EventLocalIndexUpdated    EventType = "LocalIndexUpdated"
	EventRemoteIndexUpdated   EventType = "RemoteIndexUpdated"
)

// SyncthingConfig holds Syncthing configuration
type SyncthingConfig struct {
	DataDir         string            `yaml:"data_dir"`
	ListenAddress   string            `yaml:"listen_address"`
	AdvertiseAddress string           `yaml:"advertise_address"` // Address that other devices should use to connect to this device
	APIKey          string            `yaml:"api_key"`
	DeviceID        string            `yaml:"device_id"`
	Folders         []FolderConfig    `yaml:"folders"`
	Devices         []DeviceConfig    `yaml:"devices"`
	Options         map[string]interface{} `yaml:"options"`
	EventDebug      bool              `yaml:"event_debug"`
}

// FolderConfig represents a Syncthing folder configuration
type FolderConfig struct {
	ID              string   `yaml:"id" json:"id"`
	Label           string   `yaml:"label" json:"label"`
	Path            string   `yaml:"path" json:"path"`
	Type            string   `yaml:"type" json:"type"` // "sendreceive", "sendonly", "receiveonly"
	Devices         []string `yaml:"devices" json:"devices"`
	RescanIntervalS int      `yaml:"rescan_interval_s" json:"rescan_interval_s"`
	IgnorePerms     bool     `yaml:"ignore_perms" json:"ignore_perms"`
	FSWatcherEnabled bool    `yaml:"fs_watcher_enabled" json:"fs_watcher_enabled"` // Enable/disable real-time watching
	FSWatcherDelayS  int     `yaml:"fs_watcher_delay_s" json:"fs_watcher_delay_s"`  // Delay before processing changes
	IgnorePatterns  []string `yaml:"ignore_patterns" json:"ignore_patterns"` // Patterns to ignore (like .stignore)
}

// DeviceConfig represents a Syncthing device configuration
type DeviceConfig struct {
	DeviceID    string   `yaml:"device_id"`
	Name        string   `yaml:"name"`
	Addresses   []string `yaml:"addresses"`
	Compression string   `yaml:"compression"`
	Paused      bool     `yaml:"paused"`
}

// FolderStatus represents the status of a synced folder
type FolderStatus struct {
	ID            string    `json:"id"`
	Label         string    `json:"label"`
	Path          string    `json:"path"`
	Type          string    `json:"type"`
	State         string    `json:"state"`
	StateChanged  time.Time `json:"stateChanged"`
	GlobalFiles   int64     `json:"globalFiles"`
	GlobalBytes   int64     `json:"globalBytes"`
	LocalFiles    int64     `json:"localFiles"`
	LocalBytes    int64     `json:"localBytes"`
	NeedFiles     int64     `json:"needFiles"`
	NeedBytes     int64     `json:"needBytes"`
	InSyncFiles   int64     `json:"inSyncFiles"`
	InSyncBytes   int64     `json:"inSyncBytes"`
	Errors        []string  `json:"errors"`
	Version       int64     `json:"version"`
}

// ConnectionInfo represents device connection information
type ConnectionInfo struct {
	DeviceID     string    `json:"deviceID"`
	Address      string    `json:"address"`
	Type         string    `json:"type"`
	Connected    bool      `json:"connected"`
	Paused       bool      `json:"paused"`
	BytesSent    int64     `json:"bytesSent"`
	BytesRecv    int64     `json:"bytesRecv"`
	StartedAt    time.Time `json:"startedAt"`
	Hostname     string    `json:"hostname,omitempty"`
	IPAddress    string    `json:"ipAddress,omitempty"`
}

// NewEmbeddedSyncthing creates a new embedded Syncthing instance
func NewEmbeddedSyncthing(dataDir string, config *SyncthingConfig) (*EmbeddedSyncthing, error) {
	// For now, use the real implementation
	realSyncthing, err := NewRealEmbeddedSyncthing(dataDir, config)
	if err != nil {
		return nil, err
	}
	
	// Wrap it in our interface
	return &EmbeddedSyncthing{
		real:        realSyncthing,
		dataDir:     dataDir,
		deviceID:    realSyncthing.GetDeviceID(),
		config:      config,
		eventChan:   make(chan Event, 10000), // Increased buffer for scalability (100x increase)
		subscribers: make(map[string]chan Event),
		running:     false,
		initialized: true,
	}, nil
}


// Private methods

func (es *EmbeddedSyncthing) initDirectories() error {
	// Create necessary directories
	dirs := []string{
		es.dataDir,
		filepath.Join(es.dataDir, "config"),
		filepath.Join(es.dataDir, "data"),
	}
	
	for _, dir := range dirs {
		// TODO: Create directories
		log.Printf("Would create directory: %s", dir)
	}
	
	return nil
}

func (es *EmbeddedSyncthing) initCertificate() error {
	// TODO: Load or generate certificate
	// For now, use mock device ID
	es.deviceID = "XLKB4HO-D4E5HTH-OJKRPA4-OU2CZXX-BVMWIPT-KZNU3BP-ENAOWLA-P4BZYAU"
	log.Printf("Using device ID: %s", es.deviceID)
	return nil
}

func (es *EmbeddedSyncthing) initSyncthing() error {
	// TODO: Initialize actual Syncthing components
	// This is where we'll create the model.Model, events.Logger, etc.
	log.Println("Initializing BSync components...")
	return nil
}

// SetEventDebug sets the event debug flag at runtime
func (es *EmbeddedSyncthing) SetEventDebug(enabled bool) {
	if es.real != nil {
		es.real.SetEventDebug(enabled)
	}
}

// GetEventDebug returns the current event debug flag
func (es *EmbeddedSyncthing) GetEventDebug() bool {
	if es.real != nil {
		return es.real.GetEventDebug()
	}
	return false
}

// PauseFolder pauses the specified folder
func (es *EmbeddedSyncthing) PauseFolder(folderID string) error {
	if es.real != nil {
		return es.real.PauseFolder(folderID)
	}
	return fmt.Errorf("embedded BSync not initialized")
}

// ResumeFolder resumes the specified folder
func (es *EmbeddedSyncthing) ResumeFolder(folderID string) error {
	if es.real != nil {
		return es.real.ResumeFolder(folderID)
	}
	return fmt.Errorf("embedded BSync not initialized")
}

// GetFileInfo returns file information including size from the model
func (es *EmbeddedSyncthing) GetFileInfo(folderID, fileName string) (int64, error) {
	if es.real != nil {
		return es.real.GetFileInfo(folderID, fileName)
	}
	return 0, fmt.Errorf("embedded BSync not initialized")
}

// GetFolderStatus returns the status of a specific folder
func (es *EmbeddedSyncthing) GetFolderStatus(folderID string) (*FolderStatus, error) {
	if es.real != nil {
		return es.real.GetFolderStatus(folderID)
	}
	return nil, fmt.Errorf("embedded BSync not initialized")
}

// GetAllFolderStatuses returns status of all folders
func (es *EmbeddedSyncthing) GetAllFolderStatuses() (map[string]*FolderStatus, error) {
	if es.real != nil {
		return es.real.GetAllFolderStatuses()
	}
	return nil, fmt.Errorf("embedded BSync not initialized")
}

