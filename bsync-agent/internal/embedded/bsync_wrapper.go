package embedded

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// Update all methods to delegate to real implementation

// Start starts the embedded Syncthing instance
func (es *EmbeddedSyncthing) Start(ctx context.Context) error {
	if es.real != nil {
		err := es.real.Start(ctx)
		if err == nil {
			es.running = true
			// Bridge events from real to our subscribers
			go es.bridgeEvents()
		}
		return err
	}
	
	// Fallback to mock implementation
	return es.startMock(ctx)
}

// Stop stops the embedded Syncthing instance
func (es *EmbeddedSyncthing) Stop() error {
	es.running = false
	if es.real != nil {
		return es.real.Stop()
	}
	
	// Fallback mock cleanup
	close(es.eventChan)
	for _, ch := range es.subscribers {
		close(ch)
	}
	return nil
}


// GetConnections returns information about device connections
func (es *EmbeddedSyncthing) GetConnections() (map[string]*ConnectionInfo, error) {
	if es.real != nil {
		return es.real.GetConnections()
	}
	
	// No fallback mock - return empty if real implementation not available
	return make(map[string]*ConnectionInfo), nil
}

// ScanFolder triggers a scan of the specified folder
func (es *EmbeddedSyncthing) ScanFolder(folderID string) error {
	if es.real != nil {
		return es.real.ScanFolder(folderID)
	}
	
	// Fallback mock
	log.Printf("Mock scanning folder: %s", folderID)
	es.emitEvent(Event{
		Type: EventStateChanged,
		Time: time.Now(),
		Data: map[string]interface{}{
			"folder": folderID,
			"from":   "idle",
			"to":     "scanning",
		},
	})
	return nil
}

// RevertFolder reverts local changes in a receiveonly folder to match global state
func (es *EmbeddedSyncthing) RevertFolder(folderID string) error {
	if es.real != nil {
		return es.real.RevertFolder(folderID)
	}

	// Fallback mock
	log.Printf("Mock reverting folder: %s", folderID)
	es.emitEvent(Event{
		Type: EventStateChanged,
		Time: time.Now(),
		Data: map[string]interface{}{
			"folder": folderID,
			"from":   "idle",
			"to":     "syncing",
		},
	})
	return nil
}

// OverrideFolder uses Syncthing's native Override mechanism for receiveonly folders
func (es *EmbeddedSyncthing) OverrideFolder(folderID string) error {
	if es.real != nil {
		return es.real.OverrideFolder(folderID)
	}

	// Fallback mock
	log.Printf("Mock overriding folder: %s", folderID)
	es.emitEvent(Event{
		Type: EventStateChanged,
		Time: time.Now(),
		Data: map[string]interface{}{
			"folder": folderID,
			"from":   "idle",
			"to":     "syncing",
		},
	})
	return nil
}

// RevertFolderNative uses Syncthing's native Revert() method for receiveonly folders
func (es *EmbeddedSyncthing) RevertFolderNative(folderID string) error {
	if es.real != nil {
		return es.real.RevertFolderNative(folderID)
	}

	// Fallback mock
	log.Printf("Mock reverting folder natively: %s", folderID)
	return nil
}

// ResetFolderDatabase resets the folder database to force complete re-sync
func (es *EmbeddedSyncthing) ResetFolderDatabase(folderID string) error {
	if es.real != nil {
		return es.real.ResetFolderDatabase(folderID)
	}

	// Fallback mock
	log.Printf("Mock resetting folder database: %s", folderID)
	return nil
}

// AddDevice adds a remote device to sync with
func (es *EmbeddedSyncthing) AddDevice(deviceID, name, address string) error {
	if es.real != nil {
		return es.real.AddDevice(deviceID, name, address)
	}
	
	// Fallback mock
	log.Printf("Mock adding device: %s (%s) at %s", deviceID, name, address)
	return nil
}

// AddFolder adds a new folder to sync
func (es *EmbeddedSyncthing) AddFolder(folder FolderConfig) error {
	if es.real != nil {
		return es.real.AddFolder(folder)
	}
	
	// Fallback mock
	log.Printf("Mock adding folder: %s at %s", folder.ID, folder.Path)
	es.config.Folders = append(es.config.Folders, folder)
	return nil
}

// UpdateFolder updates an existing folder configuration
func (es *EmbeddedSyncthing) UpdateFolder(folder FolderConfig) error {
	if es.real != nil {
		return es.real.UpdateFolder(folder)
	}
	
	// Fallback mock - find and update existing folder
	log.Printf("Mock updating folder: %s at %s", folder.ID, folder.Path)
	for i, existingFolder := range es.config.Folders {
		if existingFolder.ID == folder.ID {
			es.config.Folders[i] = folder
			return nil
		}
	}
	
	// If folder doesn't exist, add it (fallback behavior)
	es.config.Folders = append(es.config.Folders, folder)
	return nil
}

// RemoveFolder removes a folder from sync
func (es *EmbeddedSyncthing) RemoveFolder(folderID string) error {
	if es.real != nil {
		return es.real.RemoveFolder(folderID)
	}
	
	// Fallback mock
	log.Printf("Mock removing folder: %s", folderID)
	for i, folder := range es.config.Folders {
		if folder.ID == folderID {
			es.config.Folders = append(es.config.Folders[:i], es.config.Folders[i+1:]...)
			break
		}
	}
	return nil
}

// GetDeviceID returns the device ID of this Syncthing instance
func (es *EmbeddedSyncthing) GetDeviceID() string {
	if es.real != nil {
		return es.real.GetDeviceID()
	}
	return es.deviceID
}

// IsRunning returns whether Syncthing is currently running
func (es *EmbeddedSyncthing) IsRunning() bool {
	if es.real != nil {
		return es.real.IsRunning()
	}
	return es.running
}

// GetGUIAddress returns the GUI address for API access
func (es *EmbeddedSyncthing) GetGUIAddress() string {
	if es.real != nil {
		return es.real.GetGUIAddress()
	}
	// Fallback - construct from config
	if es.config != nil && es.config.ListenAddress != "" {
		// Convert tcp://0.0.0.0:22101 to http://127.0.0.1:22101
		addr := es.config.ListenAddress
		if strings.HasPrefix(addr, "tcp://") {
			addr = strings.TrimPrefix(addr, "tcp://")
			if strings.HasPrefix(addr, "0.0.0.0:") {
				addr = "127.0.0.1:" + strings.TrimPrefix(addr, "0.0.0.0:")
			}
			return "http://" + addr
		}
	}
	return "http://127.0.0.1:22101"
}

// GetAPIKey returns the API key for authentication
func (es *EmbeddedSyncthing) GetAPIKey() string {
	if es.real != nil {
		return es.real.GetAPIKey()
	}
	// Fallback - get from config
	if es.config != nil {
		return es.config.APIKey
	}
	return ""
}

// Subscribe subscribes to Syncthing events
func (es *EmbeddedSyncthing) Subscribe(subscriberID string) <-chan Event {
	if es.real != nil {
		return es.real.Subscribe(subscriberID)
	}
	
	// Fallback - create channel for subscriber
	ch := make(chan Event, 5000) // Increased buffer for scalability (100x increase)
	es.subscribers[subscriberID] = ch
	return ch
}

// Unsubscribe unsubscribes from Syncthing events
func (es *EmbeddedSyncthing) Unsubscribe(subscriberID string) {
	if es.real != nil {
		es.real.Unsubscribe(subscriberID)
		return
	}
	
	// Fallback
	if ch, exists := es.subscribers[subscriberID]; exists {
		close(ch)
		delete(es.subscribers, subscriberID)
	}
}

// Private helper methods

func (es *EmbeddedSyncthing) startMock(ctx context.Context) error {
	if !es.initialized {
		return fmt.Errorf("BSync not initialized")
	}

	if es.running {
		return fmt.Errorf("BSync already running")
	}

	log.Println("Starting mock embedded BSync...")

	// Start event processing
	go es.processEvents()
	
	es.running = true
	
	// Emit startup events
	es.emitEvent(Event{
		Type: EventStarting,
		Time: time.Now(),
		Data: map[string]interface{}{
			"home": es.dataDir,
		},
	})
	
	// Simulate startup completion
	go func() {
		time.Sleep(2 * time.Second)
		es.emitEvent(Event{
			Type: EventStartupComplete,
			Time: time.Now(),
			Data: map[string]interface{}{
				"myID": es.deviceID,
			},
		})
	}()
	
	log.Println("Mock embedded BSync started successfully")
	return nil
}

func (es *EmbeddedSyncthing) bridgeEvents() {
	if es.real == nil {
		return
	}
	
	// Get events from real implementation and forward to our subscribers
	realEvents := es.real.Subscribe("wrapper")
	defer es.real.Unsubscribe("wrapper")
	
	for event := range realEvents {
		// Forward to all subscribers (removed unnecessary es.eventChan forwarding)
		for _, ch := range es.subscribers {
			select {
			case ch <- event:
			default:
				// Skip if channel is full
			}
		}
	}
}

func (es *EmbeddedSyncthing) processEvents() {
	for event := range es.eventChan {
		// Broadcast to all subscribers
		for _, ch := range es.subscribers {
			select {
			case ch <- event:
			default:
				// Skip if channel is full
			}
		}
	}
}

func (es *EmbeddedSyncthing) emitEvent(event Event) {
	if es.running {
		select {
		case es.eventChan <- event:
		default:
			// Skip if channel is full
		}
	}
}