package agent

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"bsync-agent/internal/embedded"
	"bsync-agent/internal/integration"
	"bsync-agent/pkg/types"
	"github.com/gorilla/websocket"
)

// IntegratedAgent represents the integrated SyncTool agent with embedded Syncthing
type IntegratedAgent struct {
	// Configuration
	config *AgentConfig
	
	// Embedded Syncthing
	syncthing *embedded.EmbeddedSyncthing
	
	// Event bridge
	eventBridge *integration.EventBridge
	
	// WebSocket connection to server
	wsConn        *websocket.Conn
	wsURL         string
	wsMutex       sync.Mutex
	reconnectMutex sync.Mutex  // Prevents concurrent reconnection attempts
	
	// Agent state
	agentID  string
	deviceID string
	running  bool
	
	// Channels
	stopChan    chan struct{}
	eventsChan  <-chan types.AgentEvent
	wsSendChan  chan map[string]interface{}
	
	// Event persistence
	pendingEventsFile string
	pendingEventsMutex sync.Mutex
	pendingEventsBuffer []PendingEvent
	lastPendingEventsSave time.Time
	
	// Progress tracking
	folderProgress map[string]*FolderProgress
	progressMutex  sync.RWMutex
	
	// Periodic stats tracking
	activeSyncJobs    map[string]bool       // job_id -> is_syncing
	periodicTimers    map[string]*time.Timer // job_id -> timer
	autoResyncTimers  map[string]*time.Timer // job_id -> auto-resync timer
	periodicMutex     sync.RWMutex

	// Session tracking
	activeSessions    map[string]*integration.SyncSessionStats // job_id -> current session
	sessionMutex      sync.RWMutex
}

// FolderProgress tracks progress for folder operations
type FolderProgress struct {
	FolderID       string    `json:"folder_id"`
	State          string    `json:"state"`
	ScanProgress   float64   `json:"scan_progress"`   // Percentage (0-100)
	PullProgress   float64   `json:"pull_progress"`   // Percentage (0-100)  
	LastUpdated    time.Time `json:"last_updated"`
}

// AgentConfig holds the configuration for the integrated agent
type AgentConfig struct {
	// Agent identification
	AgentID       string `yaml:"agent_id"`
	AgentIDPrefix string `yaml:"agent_id_prefix"`
	AgentIDSuffix string `yaml:"agent_id_suffix"`
	ServerURL     string `yaml:"server_url"`
	
	// Syncthing configuration
	Syncthing embedded.SyncthingConfig `yaml:"syncthing"`
	
	// Monitoring configuration
	Monitoring MonitoringConfig `yaml:"monitoring"`
	
	// Logging
	LogLevel   string `yaml:"log_level"`
	EventDebug bool   `yaml:"event_debug"`
}

// MonitoringConfig holds monitoring configuration
type MonitoringConfig struct {
	Enabled               bool          `yaml:"enabled"`
	ReportInterval        time.Duration `yaml:"report_interval"`
	MetricsEndpoint       string        `yaml:"metrics_endpoint"`
	AutoResyncEnabled     bool          `yaml:"auto_resync_enabled"`
	AutoResyncInterval    time.Duration `yaml:"auto_resync_interval"`
}

// PendingEvent represents an event that needs to be sent to server
type PendingEvent struct {
	ID        string                 `json:"id"`
	Event     map[string]interface{} `json:"event"`
	Timestamp time.Time             `json:"timestamp"`
	Retries   int                   `json:"retries"`
}

// SystemInfo represents system information
type SystemInfo struct {
	Hostname     string  `json:"hostname"`
	OS           string  `json:"os"`
	CPUUsage     float64 `json:"cpu_usage"`
	MemoryUsage  int64   `json:"memory_usage"`
	DiskUsage    int64   `json:"disk_usage"`
	Uptime       int64   `json:"uptime"`
}

// generateAgentID generates an agent ID using hostname with optional prefix and suffix
func generateAgentID(prefix, suffix string) (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("failed to get hostname: %w", err)
	}
	
	// Clean hostname to be safe for use as ID
	hostname = strings.ToLower(strings.ReplaceAll(hostname, " ", "-"))
	
	// Build agent ID with prefix and suffix
	parts := []string{}
	
	if prefix != "" {
		parts = append(parts, prefix)
	}
	
	parts = append(parts, hostname)
	
	if suffix != "" {
		parts = append(parts, suffix)
	}
	
	// If no prefix or suffix, add "agent" prefix for clarity
	if prefix == "" && suffix == "" {
		return "agent-" + hostname, nil
	}
	
	return strings.Join(parts, "-"), nil
}

// NewIntegratedAgent creates a new integrated agent
func NewIntegratedAgent(config *AgentConfig) (*IntegratedAgent, error) {
	// Generate agent ID if not provided
	if config.AgentID == "" {
		generatedID, err := generateAgentID(config.AgentIDPrefix, config.AgentIDSuffix)
		if err != nil {
			return nil, fmt.Errorf("failed to generate agent ID: %w", err)
		}
		config.AgentID = generatedID
		log.Printf("Generated agent ID: %s", config.AgentID)
	}
	
	// Create embedded Syncthing
	syncthing, err := embedded.NewEmbeddedSyncthing(
		config.Syncthing.DataDir,
		&config.Syncthing,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedded Syncthing: %w", err)
	}

	// Create event bridge
	eventBridge := integration.NewEventBridge(syncthing)

	agent := &IntegratedAgent{
		config:      config,
		syncthing:   syncthing,
		eventBridge: eventBridge,
		agentID:     config.AgentID,
		deviceID:    syncthing.GetDeviceID(),
		running:     false,
		stopChan:    make(chan struct{}),
		wsSendChan:  make(chan map[string]interface{}, 10000), // Increased buffer for scalability (100x increase)
		pendingEventsFile: fmt.Sprintf("%s/pending_events_%s.json", config.Syncthing.DataDir, config.AgentID),
		folderProgress: make(map[string]*FolderProgress),
		activeSyncJobs: make(map[string]bool),
		periodicTimers: make(map[string]*time.Timer),
		autoResyncTimers: make(map[string]*time.Timer),
		activeSessions: make(map[string]*integration.SyncSessionStats),
	}

	// Get event channel
	agent.eventsChan = eventBridge.GetAgentEvents()

	// Parse WebSocket URL
	agent.wsURL = config.ServerURL
	if u, err := url.Parse(config.ServerURL); err == nil {
		if u.Scheme == "https" {
			u.Scheme = "wss"
		} else if u.Scheme == "http" {
			u.Scheme = "wss"
		}
		// Preserve existing wss:// and ws:// schemes
		agent.wsURL = u.String() + "/ws/agent?agent_id=" + config.AgentID
	}

	log.Printf("Integrated agent created with device ID: %s", agent.deviceID)
	return agent, nil
}

// Start starts the integrated agent
func (ia *IntegratedAgent) Start(ctx context.Context) error {
	if ia.running {
		return fmt.Errorf("agent already running")
	}

	log.Println("Starting integrated agent...")

	// Start embedded Syncthing
	if err := ia.syncthing.Start(ctx); err != nil {
		return fmt.Errorf("failed to start Syncthing: %w", err)
	}

	// Start event bridge
	if err := ia.eventBridge.Start(ctx); err != nil {
		ia.syncthing.Stop()
		return fmt.Errorf("failed to start event bridge: %w", err)
	}

	// Connect to WebSocket server
	if err := ia.connectWebSocket(); err != nil {
		log.Printf("Failed to connect to WebSocket server: %v", err)
		// Don't fail startup, will retry
	}

	ia.running = true

	// Start background goroutines
	go ia.handleEvents(ctx)
	go ia.monitorHealth(ctx)
	go ia.maintainConnection(ctx)
	go ia.handleWebSocketSender(ctx)
	go ia.readWebSocketMessages()  // Start the single reader goroutine
	
	// Start test trigger file watcher
	go ia.watchTestTriggers()
	log.Println("Test trigger watcher started")

	log.Println("Integrated agent started successfully")
	return nil
}

// Stop stops the integrated agent
func (ia *IntegratedAgent) Stop() error {
	if !ia.running {
		return nil
	}

	log.Println("Stopping integrated agent...")
	
	// Flush pending events buffer before shutdown
	ia.flushPendingEventsBuffer()
	
	ia.running = false
	close(ia.stopChan)
	close(ia.wsSendChan)

	// Stop event bridge
	if err := ia.eventBridge.Stop(); err != nil {
		log.Printf("Error stopping event bridge: %v", err)
	}

	// Stop embedded Syncthing
	if err := ia.syncthing.Stop(); err != nil {
		log.Printf("Error stopping Syncthing: %v", err)
	}

	// Close WebSocket connection safely
	ia.wsMutex.Lock()
	if ia.wsConn != nil {
		ia.wsConn.Close()
		ia.wsConn = nil
	}
	ia.wsMutex.Unlock()

	log.Println("Integrated agent stopped")
	return nil
}

// flushPendingEventsBuffer saves any buffered pending events to disk
func (ia *IntegratedAgent) flushPendingEventsBuffer() {
	ia.pendingEventsMutex.Lock()
	defer ia.pendingEventsMutex.Unlock()
	
	if len(ia.pendingEventsBuffer) == 0 {
		return
	}
	
	// Load existing events
	ia.pendingEventsMutex.Unlock()
	existingEvents, err := ia.loadPendingEvents()
	ia.pendingEventsMutex.Lock()
	
	if err != nil {
		log.Printf("❌ Failed to load pending events during flush: %v", err)
		existingEvents = []PendingEvent{}
	}
	
	// Combine and save
	allEvents := append(existingEvents, ia.pendingEventsBuffer...)
	
	ia.pendingEventsMutex.Unlock()
	err = ia.savePendingEvents(allEvents)
	ia.pendingEventsMutex.Lock()
	
	if err != nil {
		log.Printf("❌ Failed to flush pending events buffer: %v", err)
		return
	}
	
	// Clear buffer
	bufferSize := len(ia.pendingEventsBuffer)
	ia.pendingEventsBuffer = []PendingEvent{}
	log.Printf("💾 Flushed %d pending events from buffer to disk", bufferSize)
}

// GetStatus returns the current status of the agent
func (ia *IntegratedAgent) GetStatus() map[string]interface{} {
	status := map[string]interface{}{
		"agent_id":  ia.agentID,
		"device_id": ia.deviceID,
		"running":   ia.running,
		"syncthing": map[string]interface{}{
			"running": ia.syncthing.IsRunning(),
		},
		"websocket": map[string]interface{}{
			"connected": ia.wsConn != nil,
		},
	}

	// Add folder statuses
	if folderStatuses, err := ia.syncthing.GetAllFolderStatuses(); err == nil {
		status["folders"] = folderStatuses
	}

	// Add connection info
	if connections, err := ia.syncthing.GetConnections(); err == nil {
		status["connections"] = connections
	}

	return status
}

// AddFolder adds a new folder to sync
func (ia *IntegratedAgent) AddFolder(folder embedded.FolderConfig) error {
	log.Printf("Adding folder: %s", folder.ID)
	return ia.syncthing.AddFolder(folder)
}

// UpdateFolder updates an existing folder configuration
func (ia *IntegratedAgent) UpdateFolder(folder embedded.FolderConfig) error {
	log.Printf("Updating folder: %s", folder.ID)
	return ia.syncthing.UpdateFolder(folder)
}

// RemoveFolder removes a folder from sync
func (ia *IntegratedAgent) RemoveFolder(folderID string) error {
	log.Printf("Removing folder: %s", folderID)
	return ia.syncthing.RemoveFolder(folderID)
}

// ScanFolder triggers a scan of the specified folder
func (ia *IntegratedAgent) ScanFolder(folderID string) error {
	log.Printf("Scanning folder: %s", folderID)
	return ia.syncthing.ScanFolder(folderID)
}

// AddDevice adds a new device for sync
func (ia *IntegratedAgent) AddDevice(deviceID, name, address string) error {
	log.Printf("Adding device: %s (%s) at %s", deviceID, name, address)
	return ia.syncthing.AddDevice(deviceID, name, address)
}

// GetDeviceID returns the device ID of this agent
func (ia *IntegratedAgent) GetDeviceID() string {
	return ia.syncthing.GetDeviceID()
}

// GetConnections returns information about device connections  
func (ia *IntegratedAgent) GetConnections() (map[string]*embedded.ConnectionInfo, error) {
	return ia.syncthing.GetConnections()
}

// GetAllFolderStatuses returns status of all folders
func (ia *IntegratedAgent) GetAllFolderStatuses() (map[string]*embedded.FolderStatus, error) {
	return ia.syncthing.GetAllFolderStatuses()
}

// IsRunning returns whether the agent is running
func (ia *IntegratedAgent) IsRunning() bool {
	return ia.running && ia.syncthing.IsRunning()
}

// Private methods

func (ia *IntegratedAgent) connectWebSocket() error {
	// Prevent concurrent connection attempts
	ia.reconnectMutex.Lock()
	defer ia.reconnectMutex.Unlock()
	
	log.Printf("Connecting to WebSocket server: %s", ia.wsURL)
	
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 30 * time.Second  // Prevent indefinite hang
	
	// Configure TLS for wss:// connections
	if strings.HasPrefix(ia.wsURL, "wss://") {
		dialer.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true, // Set to true for self-signed certs in dev
		}
	}
	
	conn, _, err := dialer.Dial(ia.wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to dial WebSocket: %w", err)
	}

	// Set connection with proper locking
	ia.wsMutex.Lock()
	ia.wsConn = conn
	ia.wsMutex.Unlock()
	
	// Set longer connection timeouts for persistent connections
	conn.SetReadDeadline(time.Now().Add(300 * time.Second)) // 5 minutes instead of 1
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	
	// Set ping/pong handlers for connection health
	conn.SetPongHandler(func(string) error {
		// Reset read deadline when we receive pong from server
		conn.SetReadDeadline(time.Now().Add(300 * time.Second)) // 5 minutes
		return nil
	})
	
	// Set ping handler to respond to server pings
	conn.SetPingHandler(func(message string) error {
		// Reset read deadline when we receive ping from server
		conn.SetReadDeadline(time.Now().Add(300 * time.Second)) // 5 minutes
		// Queue pong message through safe channel instead of direct write
		pongMsg := map[string]interface{}{
			"type": "_pong",
			"message": string(message),
		}
		select {
		case ia.wsSendChan <- pongMsg:
			// Pong queued successfully
		default:
			// Channel full - skip this pong to prevent blocking
			log.Printf("⚠️ WebSocket send channel full, skipping pong response")
		}
		return nil
	})

	// Queue registration message through safe channel instead of direct write
	regMsg := map[string]interface{}{
		"type":      "register",
		"agent_id":  ia.agentID,
		"device_id": ia.deviceID,
		"data_dir":  ia.config.Syncthing.DataDir,
	}

	// Queue registration through safe channel
	select {
	case ia.wsSendChan <- regMsg:
		// Registration queued successfully
		log.Printf("Initial registration message queued")
	default:
		// Channel full - this shouldn't happen during initial connect
		conn.Close()
		return fmt.Errorf("failed to queue initial registration message - send channel full")
	}

	// Don't start new reader goroutine on reconnect
	// Reader goroutine is already started in Start()
	
	log.Println("Connected to WebSocket server")
	
	// Replay pending events after successful connection
	go func() {
		// Give the server a moment to process registration
		time.Sleep(1 * time.Second)
		ia.replayPendingEvents()
	}()
	
	return nil
}

// reconnectToServer attempts to reconnect to the WebSocket server
func (ia *IntegratedAgent) reconnectToServer() error {
	// Prevent concurrent reconnection attempts
	ia.reconnectMutex.Lock()
	defer ia.reconnectMutex.Unlock()
	
	ia.wsMutex.Lock()
	defer ia.wsMutex.Unlock()
	
	// Close existing connection if any
	if ia.wsConn != nil {
		ia.wsConn.Close()
		ia.wsConn = nil
	}
	
	// Attempt to reconnect
	dialer := websocket.DefaultDialer
	
	// Configure TLS for wss:// connections
	if strings.HasPrefix(ia.wsURL, "wss://") {
		dialer.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true, // Set to true for self-signed certs in dev
		}
	}
	
	conn, _, err := dialer.Dial(ia.wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to reconnect to WebSocket server: %w", err)
	}
	
	ia.wsConn = conn
	
	// Re-register with server
	regMsg := map[string]interface{}{
		"type":      "register",
		"agent_id":  ia.agentID,
		"device_id": ia.deviceID,
	}
	
	// Set timeouts for the new connection
	conn.SetReadDeadline(time.Now().Add(300 * time.Second))
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	
	// Set ping/pong handlers for the reconnected connection
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(300 * time.Second))
		return nil
	})
	
	conn.SetPingHandler(func(message string) error {
		conn.SetReadDeadline(time.Now().Add(300 * time.Second))
		// Queue pong message through safe channel instead of direct write
		pongMsg := map[string]interface{}{
			"type": "_pong",
			"message": string(message),
		}
		select {
		case ia.wsSendChan <- pongMsg:
			// Pong queued successfully
		default:
			// Channel full - skip this pong to prevent blocking
			log.Printf("⚠️ WebSocket send channel full, skipping pong response (reconnect handler)")
		}
		return nil
	})
	
	// Send registration message through safe channel instead of direct write
	select {
	case ia.wsSendChan <- regMsg:
		// Registration queued successfully
		log.Printf("Registration message queued for reconnected connection")
	default:
		// Channel full - close connection and retry
		conn.Close()
		ia.wsConn = nil
		return fmt.Errorf("failed to queue registration message - send channel full")
	}
	
	return nil
}

func (ia *IntegratedAgent) readWebSocketMessages() {
	defer func() {
		ia.wsMutex.Lock()
		if ia.wsConn != nil {
			ia.wsConn.Close()
			ia.wsConn = nil
		}
		ia.wsMutex.Unlock()
	}()

	for {
		// Check connection status with lock
		ia.wsMutex.Lock()
		needReconnect := ia.wsConn == nil
		ia.wsMutex.Unlock()
		
		if needReconnect {
			log.Printf("WebSocket connection lost, attempting to reconnect...")
			if err := ia.reconnectToServer(); err != nil {
				log.Printf("Failed to reconnect: %v, retrying in 10 seconds...", err)
				time.Sleep(10 * time.Second)
				continue
			}
			log.Printf("Successfully reconnected to server")
			
			// Replay pending events after successful reconnection
			go func() {
				// Give the server a moment to process re-registration
				time.Sleep(1 * time.Second)
				ia.replayPendingEvents()
			}()
		}

		// Read with proper locking
		var msg map[string]interface{}
		
		// Get connection safely
		ia.wsMutex.Lock()
		conn := ia.wsConn
		if conn != nil {
			// Update read deadline before each read operation - use longer timeout
			conn.SetReadDeadline(time.Now().Add(300 * time.Second)) // 5 minutes
		}
		ia.wsMutex.Unlock()
		
		if conn == nil {
			continue // Will trigger reconnect check on next loop
		}
		
		// ReadJSON is a blocking call, don't hold mutex during it
		if err := conn.ReadJSON(&msg); err != nil {
			log.Printf("WebSocket read error: %v, will attempt reconnection", err)
			// Close current connection and set to nil to trigger reconnect
			ia.wsMutex.Lock()
			if ia.wsConn != nil {
				ia.wsConn.Close()
				ia.wsConn = nil
			}
			ia.wsMutex.Unlock()
			// Continue loop to trigger reconnection
			continue
		}

		// Handle incoming messages
		ia.handleWebSocketMessage(msg)
	}
}

func (ia *IntegratedAgent) handleWebSocketMessage(msg map[string]interface{}) {
	msgType, ok := msg["type"].(string)
	if !ok {
		log.Printf("Invalid message type: %v", msg)
		return
	}

	switch msgType {
	case "ping":
		ia.sendWebSocketMessage(map[string]interface{}{
			"type": "pong",
		})
	case "command":
		// Handle command from CLI via server
		if cmd, ok := msg["command"].(string); ok {
			cliID, _ := msg["cli_id"].(string)
			ia.handleCommand(cmd, msg, cliID)
		}
	case "add-device":
		ia.handleAddDeviceMessage(msg, "")
	case "add-folder":
		ia.handleAddFolderMessage(msg, "")
	case "remove-folder":
		ia.handleRemoveFolderMessage(msg, "")
	case "list-devices":
		ia.handleListDevicesMessage("")
	case "list-folders":
		ia.handleListFoldersMessage("")
	case "get-device-id":
		ia.handleGetDeviceIDMessage()
	case "reload-config":
		ia.handleReloadConfigMessage(msg)  
	case "scan-folder":
		if folderID, ok := msg["folder_id"].(string); ok {
			ia.ScanFolder(folderID)
		}
	case "get-status":
		status := ia.GetStatus()
		ia.sendWebSocketMessage(map[string]interface{}{
			"type": "status",
			"data": status,
		})
	case "deploy_job":
		ia.handleDeployJobMessage(msg)
	case "pause_job":
		ia.handlePauseJobMessage(msg)
	case "resume_job":
		ia.handleResumeJobMessage(msg)
	case "delete_job":
		ia.handleDeleteJobMessage(msg)
	case "browse_folders":
		ia.handleBrowseFoldersMessage(msg)
	case "get_folder_stats":
		ia.handleGetFolderStatsMessage(msg)
	default:
		log.Printf("Unknown message type: %s", msgType)
	}
}

// handleCommand handles commands from CLI via server
func (ia *IntegratedAgent) handleCommand(cmd string, msg map[string]interface{}, cliID string) {
	log.Printf("Handling command: %s from CLI: %s", cmd, cliID)
	
	switch cmd {
	case "get-device-id":
		ia.sendWebSocketMessage(map[string]interface{}{
			"type":      "response",
			"cli_id":    cliID,
			"command":   cmd,
			"device_id": ia.deviceID,
		})
	case "add-device":
		ia.handleAddDeviceMessage(msg, cliID)
	case "add-folder":
		ia.handleAddFolderMessage(msg, cliID)
	case "remove-folder":
		ia.handleRemoveFolderMessage(msg, cliID)
	case "list-devices":
		ia.handleListDevicesMessage(cliID)
	case "list-folders":
		ia.handleListFoldersMessage(cliID)
	case "scan-folder":
		if folderID, ok := msg["folder_id"].(string); ok {
			err := ia.ScanFolder(folderID)
			if err != nil {
				ia.sendWebSocketMessage(map[string]interface{}{
					"type":    "error",
					"cli_id":  cliID,
					"command": cmd,
					"message": fmt.Sprintf("Failed to scan folder: %v", err),
				})
			} else {
				ia.sendWebSocketMessage(map[string]interface{}{
					"type":      "response",
					"cli_id":    cliID,
					"command":   cmd,
					"folder_id": folderID,
					"message":   fmt.Sprintf("Folder scan triggered for: %s", folderID),
				})
			}
		} else {
			ia.sendWebSocketMessage(map[string]interface{}{
				"type":    "error",
				"cli_id":  cliID,
				"command": cmd,
				"message": "Missing folder_id parameter",
			})
		}
	case "reload-config":
		ia.handleReloadConfigCommand(msg, cliID)
	case "get-status":
		status := ia.GetStatus()
		ia.sendWebSocketMessage(map[string]interface{}{
			"type":   "response",
			"cli_id": cliID,
			"status": status,
		})
	default:
		log.Printf("Unknown command: %s", cmd)
		ia.sendWebSocketMessage(map[string]interface{}{
			"type":    "error",
			"cli_id":  cliID,
			"message": fmt.Sprintf("Unknown command: %s", cmd),
		})
	}
}

// handleAddDeviceMessage handles add device WebSocket message
func (ia *IntegratedAgent) handleAddDeviceMessage(msg map[string]interface{}, cliID string) {
	deviceID, hasID := msg["device_id"].(string)
	name, hasName := msg["name"].(string)  
	address, hasAddr := msg["address"].(string)
	
	if !hasID || !hasName || !hasAddr {
		ia.sendWebSocketMessage(map[string]interface{}{
			"type":    "error", 
			"cli_id":  cliID,
			"message": "Missing required fields: device_id, name, address",
		})
		return
	}
	
	err := ia.AddDevice(deviceID, name, address)
	if err != nil {
		ia.sendWebSocketMessage(map[string]interface{}{
			"type":    "error",
			"cli_id":  cliID,
			"message": fmt.Sprintf("Failed to add device: %v", err),
		})
	} else {
		ia.sendWebSocketMessage(map[string]interface{}{
			"type":     "response",
			"cli_id":   cliID,
			"command":  "add-device",
			"device_id": deviceID,
			"message":  fmt.Sprintf("Device %s added successfully", name),
		})
	}
}

// handleAddFolderMessage handles add folder WebSocket message
func (ia *IntegratedAgent) handleAddFolderMessage(msg map[string]interface{}, cliID string) {
	folderID, hasID := msg["folder_id"].(string)
	path, hasPath := msg["path"].(string)
	
	if !hasID || !hasPath {
		response := map[string]interface{}{
			"type":    "error",
			"message": "Missing required fields: folder_id, path", 
		}
		if cliID != "" {
			response["cli_id"] = cliID
		}
		ia.sendWebSocketMessage(response)
		return
	}
	
	// Extract optional fields
	label, _ := msg["label"].(string)
	if label == "" {
		label = folderID
	}
	
	// Extract folder type
	folderType, _ := msg["folder_type"].(string)
	if folderType == "" {
		folderType = "sendreceive" // default
	}
	
	// Validate folder type
	if folderType != "sendreceive" && folderType != "sendonly" && folderType != "receiveonly" {
		response := map[string]interface{}{
			"type":    "error",
			"message": fmt.Sprintf("Invalid folder type: %s. Valid types: sendreceive, sendonly, receiveonly", folderType),
		}
		if cliID != "" {
			response["cli_id"] = cliID
		}
		ia.sendWebSocketMessage(response)
		return
	}
	
	devices := []string{}
	if deviceList, ok := msg["devices"].([]interface{}); ok {
		for _, d := range deviceList {
			if deviceStr, ok := d.(string); ok {
				devices = append(devices, deviceStr)
			}
		}
	}
	
	rescanInterval := 60 // default
	if interval, ok := msg["rescan_interval_s"].(float64); ok {
		rescanInterval = int(interval)
	}
	
	// For scheduled jobs (rescanInterval=0), disable file watcher too
	fsWatcherEnabled := true
	if rescanInterval == 0 {
		fsWatcherEnabled = false
	}
	
	folderConfig := embedded.FolderConfig{
		ID:              folderID,
		Label:           label, 
		Path:            path,
		Type:            folderType,
		Devices:         devices,
		RescanIntervalS: rescanInterval,
		FSWatcherEnabled: fsWatcherEnabled,
		IgnorePerms:     false,
	}
	
	err := ia.AddFolder(folderConfig)
	if err != nil {
		response := map[string]interface{}{
			"type":    "error",
			"message": fmt.Sprintf("Failed to add folder: %v", err),
		}
		if cliID != "" {
			response["cli_id"] = cliID
		}
		ia.sendWebSocketMessage(response)
	} else {
		response := map[string]interface{}{
			"message":   fmt.Sprintf("Folder %s added successfully", folderID),
		}
		if cliID != "" {
			response["type"] = "response"
			response["cli_id"] = cliID
			response["command"] = "add-folder"
		} else {
			response["type"] = "folder_added"
		}
		response["folder_id"] = folderID
		ia.sendWebSocketMessage(response)
	}
}

// handleRemoveFolderMessage handles remove folder WebSocket message
func (ia *IntegratedAgent) handleRemoveFolderMessage(msg map[string]interface{}, cliID string) {
	folderID, ok := msg["folder_id"].(string)
	if !ok {
		response := map[string]interface{}{
			"type":    "error",
			"message": "Missing folder_id",
		}
		if cliID != "" {
			response["cli_id"] = cliID
		}
		ia.sendWebSocketMessage(response)
		return
	}
	
	err := ia.RemoveFolder(folderID) 
	if err != nil {
		response := map[string]interface{}{
			"type":    "error",
			"message": fmt.Sprintf("Failed to remove folder: %v", err),
		}
		if cliID != "" {
			response["cli_id"] = cliID
		}
		ia.sendWebSocketMessage(response)
	} else {
		response := map[string]interface{}{
			"message":   fmt.Sprintf("Folder %s removed successfully", folderID),
		}
		if cliID != "" {
			response["type"] = "response"
			response["cli_id"] = cliID
			response["command"] = "remove-folder"
		} else {
			response["type"] = "folder_removed"
		}
		response["folder_id"] = folderID
		ia.sendWebSocketMessage(response)
	}
}

// handleListDevicesMessage handles list devices WebSocket message  
func (ia *IntegratedAgent) handleListDevicesMessage(cliID string) {
	devices, err := ia.GetConnections()
	if err != nil {
		response := map[string]interface{}{
			"type":    "error",
			"message": fmt.Sprintf("Failed to get devices: %v", err),
		}
		if cliID != "" {
			response["cli_id"] = cliID
		}
		ia.sendWebSocketMessage(response)
	} else {
		response := map[string]interface{}{
			"type":    "response",
			"devices": devices,
		}
		if cliID != "" {
			response["cli_id"] = cliID
			response["command"] = "list-devices"
		} else {
			response["type"] = "devices_list"
		}
		ia.sendWebSocketMessage(response)
	}
}

// handleListFoldersMessage handles list folders WebSocket message
func (ia *IntegratedAgent) handleListFoldersMessage(cliID string) {
	folders, err := ia.GetAllFolderStatuses()
	if err != nil {
		response := map[string]interface{}{
			"type":    "error",
			"message": fmt.Sprintf("Failed to get folders: %v", err),
		}
		if cliID != "" {
			response["cli_id"] = cliID
		}
		ia.sendWebSocketMessage(response)
	} else {
		response := map[string]interface{}{
			"type":    "response",
			"folders": folders,
		}
		if cliID != "" {
			response["cli_id"] = cliID
			response["command"] = "list-folders"
		} else {
			response["type"] = "folders_list"
		}
		ia.sendWebSocketMessage(response)
	}
}

// handleGetDeviceIDMessage handles get device ID WebSocket message
func (ia *IntegratedAgent) handleGetDeviceIDMessage() {
	deviceID := ia.GetDeviceID()
	ia.sendWebSocketMessage(map[string]interface{}{
		"type":      "device_id",
		"device_id": deviceID,
	})
}

func (ia *IntegratedAgent) handleReloadConfigMessage(msg map[string]interface{}) {
	log.Println("Received config reload request via WebSocket")
	
	// Get event_debug parameter from message (optional)
	var eventDebug *bool
	if val, ok := msg["event_debug"].(bool); ok {
		eventDebug = &val
	}
	
	// Apply event debug setting if provided
	if eventDebug != nil {
		ia.syncthing.SetEventDebug(*eventDebug)
		log.Printf("Event debug setting updated via WebSocket to: %v", *eventDebug)
	}
	
	// Send response
	response := map[string]interface{}{
		"type":    "config_reloaded",
		"success": true,
		"message": "Configuration reloaded successfully via WebSocket",
	}
	
	if eventDebug != nil {
		response["event_debug"] = *eventDebug
	}
	
	ia.sendWebSocketMessage(response)
}

func (ia *IntegratedAgent) handleReloadConfigCommand(msg map[string]interface{}, cliID string) {
	log.Printf("Received config reload command from CLI: %s", cliID)
	
	// Get event_debug parameter from message (optional)
	var eventDebug *bool
	if val, ok := msg["event_debug"].(bool); ok {
		eventDebug = &val
	}
	
	// Apply event debug setting if provided
	if eventDebug != nil {
		ia.syncthing.SetEventDebug(*eventDebug)
		log.Printf("Event debug setting updated via CLI to: %v", *eventDebug)
	}
	
	// Send response to CLI
	response := map[string]interface{}{
		"type":    "response",
		"cli_id":  cliID,
		"command": "reload-config",
		"success": true,
		"message": "Configuration reloaded successfully",
	}
	
	if eventDebug != nil {
		response["event_debug"] = *eventDebug
	}
	
	ia.sendWebSocketMessage(response)
}

func (ia *IntegratedAgent) sendWebSocketMessage(msg map[string]interface{}) {
	// Try to send message to channel for safe writing by single goroutine
	select {
	case ia.wsSendChan <- msg:
		// Message queued successfully
	default:
		// Channel is full or WebSocket is not connected, save to pending events
		log.Printf("💾 WebSocket send channel full or not connected, saving event to pending list")
		if err := ia.addPendingEvent(msg); err != nil {
			log.Printf("❌ Failed to save pending event: %v", err)
		}
	}
}

func (ia *IntegratedAgent) handleEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ia.stopChan:
			return
		case event, ok := <-ia.eventsChan:
			if !ok {
				return
			}
			ia.handleAgentEvent(event)
		}
	}
}

func (ia *IntegratedAgent) handleAgentEvent(event types.AgentEvent) {
	// Send event to server via WebSocket
	wsMsg := map[string]interface{}{
		"type":      "event",
		"agent_id":  ia.agentID,
		"event":     event,
		"timestamp": time.Now(),
	}

	ia.sendWebSocketMessage(wsMsg)

	// Log important events and update progress tracking
	switch event.Type {
	case "file_transfer_completed":
		if data, ok := event.Data.(integration.FileTransferEvent); ok {
			log.Printf("File transfer completed: %s (%.2f MB)", 
				data.FileName, float64(data.FileSize)/(1024*1024))
		}
	case "sync_error":
		if data, ok := event.Data.(integration.SyncStatusEvent); ok {
			log.Printf("Sync error in folder %s: %v", data.FolderID, data.Errors)
		}
	case "state_changed":
		if data, ok := event.Data.(map[string]interface{}); ok {
			if folderID, exists := data["folder"].(string); exists {
				if to, stateExists := data["to"].(string); stateExists {
					// Update progress based on state
					var scanProgress, pullProgress float64
					if to == "scanning" {
						scanProgress = 0.0  // Start scanning
					} else if to == "idle" {
						scanProgress = 100.0  // Completed scanning
						pullProgress = 100.0  // Completed pulling
					}
					
					ia.updateFolderProgress(folderID, to, scanProgress, pullProgress)
					
					// Handle periodic stats for job folders
					if strings.HasPrefix(folderID, "job-") {
						jobID := strings.TrimPrefix(folderID, "job-")

						if to == "scanning" || to == "syncing" {
							// Start or update sync session
							session := ia.getActiveSession(jobID)
							if session == nil {
								ia.startSyncSession(jobID, to)
							} else {
								ia.updateSyncSession(jobID, to)
							}

							// Start periodic stats when syncing begins
							ia.startPeriodicStatsForJob(jobID)
						} else if to == "idle" {
							// Finalize session before stopping
							ia.finalizeSyncSession(jobID)

							// Send FINAL stats before stopping to ensure cache is populated
							ia.sendFinalStatsBeforeIdle(jobID, folderID)

							// Stop periodic stats but keep auto-resync running
							ia.stopPeriodicStatsButKeepAutoResync(jobID)
						}
					}
				}
			}
		}
	case "folder_scan_progress":
		if data, ok := event.Data.(map[string]interface{}); ok {
			if folderID, exists := data["folder_id"].(string); exists {
				if progress, progressExists := data["progress"].(float64); progressExists {
					// Get current progress state
					currentProgress := ia.getFolderProgress(folderID)
					currentState := "scanning"
					var pullProgress float64 = 0
					
					if currentProgress != nil {
						currentState = currentProgress.State
						pullProgress = currentProgress.PullProgress
					}
					
					// Update scan progress with real percentage
					ia.updateFolderProgress(folderID, currentState, progress, pullProgress)
					log.Printf("🎯 Real scan progress: folder=%s, progress=%.1f%%", folderID, progress)
				}
			}
		}
	case "file_transfer_progress":
		if data, ok := event.Data.(integration.FileTransferEvent); ok {
			// Extract real progress dari DownloadProgress
			realProgress := data.Progress
			folderID := data.JobID
			
			// Get current progress state
			currentProgress := ia.getFolderProgress(folderID)
			scanProgress := 0.0
			currentState := "syncing"
			
			if currentProgress != nil {
				scanProgress = currentProgress.ScanProgress
				currentState = currentProgress.State
			}
			
			// Update pull progress dengan data real
			ia.updateFolderProgress(folderID, currentState, scanProgress, realProgress)
			log.Printf("🎯 Real pull progress updated: folder=%s, progress=%.1f%%, file=%s", 
				folderID, realProgress, data.FileName)
		}
	}
}

func (ia *IntegratedAgent) monitorHealth(ctx context.Context) {
	if !ia.config.Monitoring.Enabled {
		return
	}

	ticker := time.NewTicker(ia.config.Monitoring.ReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ia.stopChan:
			return
		case <-ticker.C:
			ia.reportHealth()
		}
	}
}

func (ia *IntegratedAgent) collectSystemInfo() SystemInfo {
	// Get actual hostname
	hostname, err := os.Hostname()
	if err != nil {
		log.Printf("Failed to get hostname: %v", err)
		hostname = "unknown"
	}
	
	// Get actual OS info with distribution detection
	osName := runtime.GOOS
	
	// Try to detect specific OS distribution (cross-platform)
	if osName == "linux" {
		if distro := getOSDistribution(); distro != "" {
			osName = distro
		}
	}
	
	// TODO: Implement actual CPU, memory, disk usage collection
	// For now, use placeholder values
	return SystemInfo{
		Hostname:    hostname,
		OS:          osName,
		CPUUsage:    15.5,
		MemoryUsage: 52 * 1024 * 1024, // 52MB
		DiskUsage:   1024 * 1024 * 1024, // 1GB
		Uptime:      3600, // 1 hour
	}
}


func (ia *IntegratedAgent) reportHealth() {
	// Collect actual system metrics
	sysInfo := ia.collectSystemInfo()

	healthMsg := map[string]interface{}{
		"type":        "health",
		"agent_id":    ia.agentID,
		"system_info": sysInfo,
		"data_dir":    ia.config.Syncthing.DataDir,  // Include data_dir in health message
		"timestamp":   time.Now(),
	}

	ia.sendWebSocketMessage(healthMsg)
}

func (ia *IntegratedAgent) handleWebSocketSender(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ia.stopChan:
			return
		case msg, ok := <-ia.wsSendChan:
			if !ok {
				return // Channel closed
			}
			ia.writeWebSocketMessage(msg)
		}
	}
}

func (ia *IntegratedAgent) writeWebSocketMessage(msg map[string]interface{}) {
	ia.wsMutex.Lock()
	defer ia.wsMutex.Unlock()
	
	// Double-check connection with mutex protection
	if ia.wsConn == nil {
		// Only save non-pong messages to pending list
		if msgType, ok := msg["type"].(string); !ok || msgType != "_pong" {
			log.Printf("💾 WebSocket connection is nil, saving event to pending list")
			if err := ia.addPendingEvent(msg); err != nil {
				log.Printf("❌ Failed to save pending event: %v", err)
			}
		}
		return
	}

	// Use a defer to handle panic recovery
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic in WebSocket write: %v", r)
			// Safely close connection on panic
			if ia.wsConn != nil {
				ia.wsConn.Close()
				ia.wsConn = nil
			}
		}
	}()

	// Handle _pong messages by sending actual pong frame
	if msgType, ok := msg["type"].(string); ok && msgType == "_pong" {
		message := ""
		if msgStr, ok := msg["message"].(string); ok {
			message = msgStr
		}
		// Update write deadline and send pong frame
		ia.wsConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := ia.wsConn.WriteMessage(websocket.PongMessage, []byte(message)); err != nil {
			log.Printf("Failed to send pong message: %v", err)
			// Connection likely lost
			if ia.wsConn != nil {
				ia.wsConn.Close()
				ia.wsConn = nil
			}
		}
		return
	}

	// Update write deadline before each write operation
	ia.wsConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	
	if err := ia.wsConn.WriteJSON(msg); err != nil {
		log.Printf("WebSocket write error: %v", err)
		// Connection likely lost, will be retried by maintainConnection
		// Safely close and nil the connection
		if ia.wsConn != nil {
			ia.wsConn.Close()
			ia.wsConn = nil
		}
	}
}

func (ia *IntegratedAgent) maintainConnection(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ia.stopChan:
			return
		case <-ticker.C:
			// Check connection status with lock
			ia.wsMutex.Lock()
			needReconnect := ia.wsConn == nil
			ia.wsMutex.Unlock()
			
			if needReconnect {
				log.Println("WebSocket disconnected, attempting to reconnect...")
				if err := ia.connectWebSocket(); err != nil {
					log.Printf("Reconnection failed: %v", err)
				}
			}
		}
	}
}

// GetSyncthing returns the embedded Syncthing instance for configuration updates
func (ia *IntegratedAgent) GetSyncthing() *embedded.EmbeddedSyncthing {
	return ia.syncthing
}

// handleDeployJobMessage handles deploy job WebSocket message from server
func (ia *IntegratedAgent) handleDeployJobMessage(msg map[string]interface{}) {
	log.Printf("Received deploy job message: %+v", msg)
	
	// Extract job configuration from message
	jobID, hasJobID := msg["job_id"].(string)
	name, hasName := msg["name"].(string)
	sourcePath, hasSourcePath := msg["source_path"].(string)
	destinationPath, hasDestinationPath := msg["destination_path"].(string)
	syncType, hasSyncType := msg["sync_type"].(string)
	destinationAgentID, hasDestAgentID := msg["destination_agent_id"].(string)

	// Extract rescan_interval with default fallback
	rescanInterval := 3600 // default
	if interval, ok := msg["rescan_interval_s"].(float64); ok {
		rescanInterval = int(interval)
	}

	// Extract ignore_patterns
	var ignorePatterns []string
	if patterns, ok := msg["ignore_patterns"].([]interface{}); ok {
		for _, p := range patterns {
			if pattern, ok := p.(string); ok && strings.TrimSpace(pattern) != "" {
				ignorePatterns = append(ignorePatterns, strings.TrimSpace(pattern))
			}
		}
	}

	// Basic validation - jobID, name, and syncType are always required
	if !hasJobID || !hasName || !hasSyncType {
		log.Printf("Deploy job missing required fields (jobID, name, or syncType)")
		ia.sendWebSocketMessage(map[string]interface{}{
			"type":    "job_deploy_error",
			"job_id":  jobID,
			"message": "Missing required fields for job deployment",
		})
		return
	}
	
	// Determine if this agent is source or destination
	sourceAgentID, hasSourceAgentID := msg["source_agent_id"].(string)
	if !hasSourceAgentID {
		log.Printf("Deploy job missing source_agent_id")
		ia.sendWebSocketMessage(map[string]interface{}{
			"type":    "job_deploy_error", 
			"job_id":  jobID,
			"message": "Missing source_agent_id field",
		})
		return
	}
	
	isSourceAgent := sourceAgentID == ia.agentID
	isDestinationAgent := hasDestAgentID && destinationAgentID == ia.agentID

	// Role-specific validation
	if isSourceAgent && !hasSourcePath {
		log.Printf("Deploy job missing source_path for source agent")
		ia.sendWebSocketMessage(map[string]interface{}{
			"type":    "job_deploy_error",
			"job_id":  jobID,
			"message": "Missing source_path field for source agent",
		})
		return
	}

	if isDestinationAgent && !hasDestinationPath {
		log.Printf("Deploy job missing destination_path for destination agent")
		ia.sendWebSocketMessage(map[string]interface{}{
			"type":    "job_deploy_error",
			"job_id":  jobID,
			"message": "Missing destination_path field for destination agent",
		})
		return
	}

	var folderConfig embedded.FolderConfig
	var folderID string

	// Both agents use the same folder ID (this is required for Syncthing sync)
	folderID = fmt.Sprintf("job-%s", jobID)
	
	// 🔧 AUTOMATIC DEVICE PAIRING: Add remote device with IP address before creating folder
	var remoteDeviceID, remoteAgentID, remoteIPAddress string
	
	if isSourceAgent {
		// Source agent needs to add destination device
		remoteDeviceID, _ = msg["destination_device_id"].(string)
		remoteAgentID = destinationAgentID
		remoteIPAddress, _ = msg["destination_ip_address"].(string)
	}
	
	if isDestinationAgent {
		// Destination agent needs to add source device
		remoteDeviceID, _ = msg["source_device_id"].(string)
		remoteAgentID, _ = msg["source_agent_id"].(string)
		remoteIPAddress, _ = msg["source_ip_address"].(string)
	}
	
	// 🆕 AUTO-EXTRACT IP: If server doesn't send IP, extract from agent connection or config
	if remoteDeviceID != "" && remoteAgentID != "" {
		if remoteIPAddress == "" {
			// Extract IP from agent ID or use configured server connection
			remoteIPAddress = ia.extractIPFromAgentConnection(remoteAgentID)
			log.Printf("🔍 Auto-extracted IP address for %s: %s", remoteAgentID, remoteIPAddress)
		}
		
		if remoteIPAddress != "" {
			remoteAddress := fmt.Sprintf("tcp://%s:22101", remoteIPAddress)
			log.Printf("🔗 Auto-adding remote device %s (%s) at %s for job %s", remoteAgentID, remoteDeviceID, remoteAddress, jobID)
			
			err := ia.AddDevice(remoteDeviceID, remoteAgentID, remoteAddress)
			if err != nil {
				log.Printf("⚠️ Failed to auto-add remote device %s: %v (continuing with folder creation)", remoteAgentID, err)
				// Don't fail the job if device already exists or other non-critical errors
			} else {
				log.Printf("✅ Successfully auto-added remote device %s for job %s", remoteAgentID, jobID)
			}
		} else {
			log.Printf("⚠️ Could not extract IP address for remote agent %s", remoteAgentID)
		}
	} else {
		log.Printf("⚠️ Missing device pairing info for job %s: deviceID=%s, agentID=%s, IP=%s", jobID, remoteDeviceID, remoteAgentID, remoteIPAddress)
	}
	
	if isSourceAgent {
		// Configure as source folder
		folderType := "sendonly"
		if syncType == "sendreceive" {
			folderType = "sendreceive"
		}

		// Check for multi-destination support
		var destinationDeviceIDs []string
		isMultiDest, _ := msg["is_multi_destination"].(bool)

		if isMultiDest {
			// Multi-destination mode: get array of destination device IDs
			if destDeviceIDsRaw, ok := msg["destination_device_ids"].([]interface{}); ok {
				for _, d := range destDeviceIDsRaw {
					if deviceID, ok := d.(string); ok && deviceID != "" {
						destinationDeviceIDs = append(destinationDeviceIDs, deviceID)
					}
				}
			}

			// Also auto-add all destination devices
			if destAgentIDsRaw, ok := msg["destination_agent_ids"].([]interface{}); ok {
				destIPsRaw, _ := msg["destination_ip_addresses"].([]interface{})

				for i, d := range destAgentIDsRaw {
					destAgentID, _ := d.(string)
					var destDeviceID, destIPAddress string

					if i < len(destinationDeviceIDs) {
						destDeviceID = destinationDeviceIDs[i]
					}
					if i < len(destIPsRaw) {
						destIPAddress, _ = destIPsRaw[i].(string)
					}

					if destDeviceID != "" && destIPAddress != "" {
						remoteAddress := fmt.Sprintf("tcp://%s:22101", destIPAddress)
						log.Printf("🔗 Auto-adding destination device %s (%s) at %s", destAgentID, destDeviceID, remoteAddress)

						err := ia.AddDevice(destDeviceID, destAgentID, remoteAddress)
						if err != nil {
							log.Printf("⚠️ Failed to auto-add destination device %s: %v (continuing)", destAgentID, err)
						} else {
							log.Printf("✅ Successfully auto-added destination device %s", destAgentID)
						}
					}
				}
			}

			if len(destinationDeviceIDs) == 0 {
				log.Printf("Deploy job: multi-destination enabled but no destination_device_ids found")
				ia.sendWebSocketMessage(map[string]interface{}{
					"type":    "job_deploy_error",
					"job_id":  jobID,
					"message": "Missing destination_device_ids for multi-destination job",
				})
				return
			}

			log.Printf("🌟 Multi-destination job: Source will sync to %d destination(s)", len(destinationDeviceIDs))
		} else {
			// Legacy single destination mode
			destinationDeviceID, hasDestDeviceID := msg["destination_device_id"].(string)
			if !hasDestDeviceID {
				log.Printf("Deploy job missing destination_device_id for source agent")
				ia.sendWebSocketMessage(map[string]interface{}{
					"type":    "job_deploy_error",
					"job_id":  jobID,
					"message": "Missing destination_device_id field",
				})
				return
			}
			destinationDeviceIDs = []string{destinationDeviceID}
		}

		// For scheduled jobs (rescanInterval=0), disable file watcher too
		fsWatcherEnabled := true
		if rescanInterval == 0 {
			fsWatcherEnabled = false
		}

		folderConfig = embedded.FolderConfig{
			ID:              folderID,
			Label:           name, // Use job name as folder alias
			Path:            sourcePath,
			Type:            folderType,
			Devices:         destinationDeviceIDs, // Connect to ALL destination devices
			RescanIntervalS: rescanInterval,
			FSWatcherEnabled: fsWatcherEnabled,
			IgnorePerms:     false,
			IgnorePatterns:  ignorePatterns,
		}

		log.Printf("📂 Source folder configured: ID=%s, Path=%s, Type=%s, Devices=%v", folderID, sourcePath, folderType, destinationDeviceIDs)
	}
	
	if isDestinationAgent {
		// Configure as destination folder (same folder ID as source)
		folderType := "receiveonly"
		if syncType == "sendreceive" {
			folderType = "sendreceive"
		}
		
		// Get source device ID for Syncthing configuration
		sourceDeviceID, hasSourceDeviceID := msg["source_device_id"].(string)
		if !hasSourceDeviceID {
			log.Printf("Deploy job missing source_device_id for destination agent")
			ia.sendWebSocketMessage(map[string]interface{}{
				"type":    "job_deploy_error",
				"job_id":  jobID,
				"message": "Missing source_device_id field",
			})
			return
		}
		
		// For scheduled jobs (rescanInterval=0), disable file watcher too
		fsWatcherEnabled := true
		if rescanInterval == 0 {
			fsWatcherEnabled = false
		}
		
		folderConfig = embedded.FolderConfig{
			ID:              folderID,
			Label:           name, // Use job name as folder alias
			Path:            destinationPath,
			Type:            folderType,
			Devices:         []string{sourceDeviceID}, // Connect to source device
			RescanIntervalS: rescanInterval,
			FSWatcherEnabled: fsWatcherEnabled,
			IgnorePerms:     false,
			IgnorePatterns:  ignorePatterns,
		}
	}
	
	// Deploy the folder configuration (use UpdateFolder for existing, AddFolder for new)
	if folderID != "" {
		// Check if folder already exists by trying to get its status
		// If it exists, use UpdateFolder; otherwise use AddFolder
		var err error
		
		// Try to get existing folder status to determine if it exists
		folderExists := false
		if _, err := ia.syncthing.GetFolderStatus(folderID); err == nil {
			folderExists = true
		}
		
		if folderExists {
			log.Printf("Updating existing job %s as folder %s", jobID, folderID)
			err = ia.UpdateFolder(folderConfig)
			// Trigger folder rescan after ignore pattern update (always trigger when updating)
			if err == nil {
				log.Printf("🔄 Job updated, triggering folder rescan for ignore pattern changes...")
				if err := ia.triggerFolderRescan(folderID); err != nil {
					log.Printf("❌ Failed to trigger folder rescan: %v", err)
					// Continue anyway, rescan bisa di-trigger manual
				} else {
					log.Printf("✅ Folder rescan triggered successfully")
				}
			}
		} else {
			log.Printf("Adding new job %s as folder %s", jobID, folderID)
			err = ia.AddFolder(folderConfig)
		}
		
		if err != nil {
			log.Printf("Failed to deploy job %s: %v", jobID, err)
			ia.sendWebSocketMessage(map[string]interface{}{
				"type":    "job_deploy_error",
				"job_id":  jobID,
				"message": fmt.Sprintf("Failed to deploy job: %v", err),
			})
		} else {
			action := "deployed"
			if folderExists {
				action = "updated"
			}
			log.Printf("Successfully %s job %s as folder %s", action, jobID, folderID)
			ia.sendWebSocketMessage(map[string]interface{}{
				"type":      "job_deployed",
				"job_id":    jobID,
				"folder_id": folderID,
				"message":   fmt.Sprintf("Job %s %s successfully", name, action),
			})
		}
	}
}

// handlePauseJobMessage handles pause job WebSocket message from server
func (ia *IntegratedAgent) handlePauseJobMessage(msg map[string]interface{}) {
	jobID, hasJobID := msg["job_id"].(string)
	if !hasJobID {
		log.Printf("Pause job missing job_id")
		return
	}
	
	log.Printf("Received pause job message for job: %s", jobID)
	
	// Use the unified folder ID format
	folderID := fmt.Sprintf("job-%s", jobID)
	
	pausedFolders := []string{}
	
	// Try to pause the folder
	if err := ia.syncthing.PauseFolder(folderID); err == nil {
		pausedFolders = append(pausedFolders, folderID)
		log.Printf("Paused folder %s for job %s", folderID, jobID)
	} else {
		log.Printf("Failed to pause folder %s for job %s: %v", folderID, jobID, err)
	}
	
	if len(pausedFolders) > 0 {
		ia.sendWebSocketMessage(map[string]interface{}{
			"type":           "job_paused",
			"job_id":         jobID,
			"paused_folders": pausedFolders,
			"message":        fmt.Sprintf("Job %s paused successfully", jobID),
		})
	} else {
		ia.sendWebSocketMessage(map[string]interface{}{
			"type":    "job_pause_error",
			"job_id":  jobID,
			"message": fmt.Sprintf("No folders found to pause for job %s", jobID),
		})
	}
}

// handleResumeJobMessage handles resume job WebSocket message from server
func (ia *IntegratedAgent) handleResumeJobMessage(msg map[string]interface{}) {
	jobID, hasJobID := msg["job_id"].(string)
	if !hasJobID {
		log.Printf("Resume job missing job_id")
		return
	}
	
	log.Printf("Received resume job message for job: %s", jobID)
	
	// Use the unified folder ID format
	folderID := fmt.Sprintf("job-%s", jobID)
	
	resumedFolders := []string{}
	
	// Try to resume the folder
	if err := ia.syncthing.ResumeFolder(folderID); err == nil {
		resumedFolders = append(resumedFolders, folderID)
		log.Printf("Resumed folder %s for job %s", folderID, jobID)
	} else {
		log.Printf("Failed to resume folder %s for job %s: %v", folderID, jobID, err)
	}
	
	if len(resumedFolders) > 0 {
		ia.sendWebSocketMessage(map[string]interface{}{
			"type":            "job_resumed",
			"job_id":          jobID,
			"resumed_folders": resumedFolders,
			"message":         fmt.Sprintf("Job %s resumed successfully", jobID),
		})
	} else {
		ia.sendWebSocketMessage(map[string]interface{}{
			"type":    "job_resume_error",
			"job_id":  jobID,
			"message": fmt.Sprintf("No folders found to resume for job %s", jobID),
		})
	}
}

// handleDeleteJobMessage handles delete job WebSocket message from server
func (ia *IntegratedAgent) handleDeleteJobMessage(msg map[string]interface{}) {
	jobID, hasJobID := msg["job_id"].(string)
	if !hasJobID {
		log.Printf("Delete job missing job_id")
		return
	}
	
	log.Printf("Received delete job message for job: %s", jobID)
	
	// Use the unified folder ID format
	folderID := fmt.Sprintf("job-%s", jobID)
	
	deletedFolders := []string{}
	
	// Try to remove the folder
	if err := ia.RemoveFolder(folderID); err == nil {
		deletedFolders = append(deletedFolders, folderID)
		log.Printf("Deleted folder %s for job %s", folderID, jobID)
	} else {
		log.Printf("Failed to delete folder %s for job %s: %v", folderID, jobID, err)
	}
	
	if len(deletedFolders) > 0 {
		ia.sendWebSocketMessage(map[string]interface{}{
			"type":            "job_deleted",
			"job_id":          jobID,
			"deleted_folders": deletedFolders,
			"message":         fmt.Sprintf("Job %s deleted successfully", jobID),
		})
	} else {
		ia.sendWebSocketMessage(map[string]interface{}{
			"type":    "job_delete_error",
			"job_id":  jobID,
			"message": fmt.Sprintf("No folders found to delete for job %s", jobID),
		})
	}
}

// watchTestTriggers watches for test trigger files to test pause/resume/delete functions
func (ia *IntegratedAgent) watchTestTriggers() {
	for {
		time.Sleep(5 * time.Second)
		
		// Check for pause trigger (cross-platform path)
		triggerPath := getTestTriggerPath()
		if data, err := ioutil.ReadFile(triggerPath); err == nil {
			command := strings.TrimSpace(string(data))
			log.Printf("🧪 TEST TRIGGER: %s", command)
			
			switch {
			case strings.HasPrefix(command, "pause-job-"):
				folderID := strings.Replace(command, "pause-job-", "job-", 1)
				log.Printf("🧪 TESTING PauseFolder(%s)", folderID)
				if err := ia.syncthing.PauseFolder(folderID); err != nil {
					log.Printf("❌ TEST FAILED: PauseFolder(%s): %v", folderID, err)
				} else {
					log.Printf("✅ TEST SUCCESS: PauseFolder(%s)", folderID)
				}
				
			case strings.HasPrefix(command, "resume-job-"):
				folderID := strings.Replace(command, "resume-job-", "job-", 1)
				log.Printf("🧪 TESTING ResumeFolder(%s)", folderID)
				if err := ia.syncthing.ResumeFolder(folderID); err != nil {
					log.Printf("❌ TEST FAILED: ResumeFolder(%s): %v", folderID, err)
				} else {
					log.Printf("✅ TEST SUCCESS: ResumeFolder(%s)", folderID)
				}
				
			case strings.HasPrefix(command, "delete-job-"):
				folderID := strings.Replace(command, "delete-job-", "job-", 1)
				log.Printf("🧪 TESTING RemoveFolder(%s)", folderID)
				if err := ia.syncthing.RemoveFolder(folderID); err != nil {
					log.Printf("❌ TEST FAILED: RemoveFolder(%s): %v", folderID, err)
				} else {
					log.Printf("✅ TEST SUCCESS: RemoveFolder(%s)", folderID)
				}
			}
			
			// Remove trigger file after processing
			os.Remove(triggerPath)
		}
	}
}

// handleBrowseFoldersMessage handles browse folders requests from server
func (ia *IntegratedAgent) handleBrowseFoldersMessage(msg map[string]interface{}) {
	path, hasPath := msg["path"].(string)
	if !hasPath {
		path = "/" // Default to root
	}
	
	depth, hasDepth := msg["depth"].(float64) // JSON numbers are float64
	if !hasDepth {
		depth = 2 // Default depth
	}
	
	log.Printf("📁 Received browse folders request: path=%s, depth=%.0f", path, depth)
	
	// Browse the file system
	folderInfo, err := ia.browsePath(path, int(depth))
	if err != nil {
		log.Printf("❌ Failed to browse path %s: %v", path, err)
		ia.sendWebSocketMessage(map[string]interface{}{
			"type": "browse_error",
			"error": fmt.Sprintf("Failed to browse path: %v", err),
			"path": path,
		})
		return
	}
	
	// Send successful response
	ia.sendWebSocketMessage(map[string]interface{}{
		"type": "browse_response",
		"path": path,
		"data": folderInfo,
	})
	
	log.Printf("✅ Sent browse response for path %s", path)
}

// browsePath browses the file system at the given path with specified depth
func (ia *IntegratedAgent) browsePath(path string, depth int) (map[string]interface{}, error) {
	// Handle root path requests differently for Windows vs Unix
	if isRootPath(path) {
		if shouldShowDriveList() {
			// Windows: Show drive list
			drives, err := getAvailableDrives()
			if err != nil {
				return nil, fmt.Errorf("failed to get available drives: %v", err)
			}
			
			result := map[string]interface{}{
				"name":         "Root",
				"path":         getRootPath(),
				"is_directory": true,
				"children":     drives,
			}
			return result, nil
		} else {
			// Unix/Linux: Browse root directory normally
			path = "/"
		}
	}
	
	// Normalize the path for the current platform
	path = normalizeRootPath(path)
	
	// Check if path exists and is accessible
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("path not accessible: %v", err)
	}
	
	result := map[string]interface{}{
		"name":         filepath.Base(path),
		"path":         path,
		"is_directory": info.IsDir(),
	}
	
	// If it's a directory and we have depth remaining, get children
	if info.IsDir() && depth > 0 {
		children, err := ia.getDirectoryChildren(path, depth-1)
		if err != nil {
			log.Printf("⚠️ Failed to get children for %s: %v", path, err)
			// Don't fail completely, just return without children
		} else {
			result["children"] = children
		}
	}
	
	return result, nil
}

// getDirectoryChildren gets child directories and files
func (ia *IntegratedAgent) getDirectoryChildren(dirPath string, remainingDepth int) ([]map[string]interface{}, error) {
	entries, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	
	var children []map[string]interface{}
	
	for _, entry := range entries {
		// Skip hidden files/directories starting with .
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		
		childPath := filepath.Join(dirPath, entry.Name())
		
		child := map[string]interface{}{
			"name":         entry.Name(),
			"path":         childPath,
			"is_directory": entry.IsDir(),
		}
		
		// If it's a directory and we have remaining depth, recurse
		if entry.IsDir() && remainingDepth > 0 {
			grandChildren, err := ia.getDirectoryChildren(childPath, remainingDepth-1)
			if err != nil {
				// Skip directories we can't read
				log.Printf("⚠️ Skipping unreadable directory %s: %v", childPath, err)
				continue
			}
			child["children"] = grandChildren
		}
		
		children = append(children, child)
	}
	
	return children, nil
}

// loadPendingEvents loads pending events from file
func (ia *IntegratedAgent) loadPendingEvents() ([]PendingEvent, error) {
	ia.pendingEventsMutex.Lock()
	defer ia.pendingEventsMutex.Unlock()
	
	// Check if file exists
	if _, err := os.Stat(ia.pendingEventsFile); os.IsNotExist(err) {
		return []PendingEvent{}, nil
	}
	
	// Use explicit file descriptor control
	f, err := os.Open(ia.pendingEventsFile)
	if err != nil {
		// Check for "too many open files" error
		if pathErr, ok := err.(*os.PathError); ok {
			if errno, ok := pathErr.Err.(syscall.Errno); ok && errno == syscall.EMFILE {
				log.Printf("❌ Too many open files - returning empty pending events")
				return []PendingEvent{}, nil // Return empty instead of error
			}
		}
		return nil, fmt.Errorf("failed to open pending events file: %w", err)
	}
	defer f.Close()
	
	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read pending events file: %w", err)
	}
	
	var events []PendingEvent
	if err := json.Unmarshal(data, &events); err != nil {
		// If JSON is corrupted, return empty events instead of failing
		log.Printf("⚠️ Failed to unmarshal pending events (file may be corrupted), starting with empty list: %v", err)
		return []PendingEvent{}, nil
	}
	
	return events, nil
}

// savePendingEvents saves pending events to file
func (ia *IntegratedAgent) savePendingEvents(events []PendingEvent) error {
	ia.pendingEventsMutex.Lock()
	defer ia.pendingEventsMutex.Unlock()
	
	// Check if too many events, trim to prevent excessive disk usage
	if len(events) > 1000 {
		// Keep only the most recent 1000 events
		events = events[len(events)-1000:]
		log.Printf("⚠️ Trimmed pending events to 1000 most recent entries")
	}
	
	data, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal pending events: %w", err)
	}
	
	// Use safer file writing with temp file and atomic rename
	tempFile := ia.pendingEventsFile + ".tmp"
	
	// Create file with explicit file descriptor control
	f, err := os.OpenFile(tempFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		// Check for "too many open files" error
		if pathErr, ok := err.(*os.PathError); ok {
			if errno, ok := pathErr.Err.(syscall.Errno); ok && errno == syscall.EMFILE {
				log.Printf("❌ Too many open files - skipping pending events save")
				return fmt.Errorf("too many open files, cannot save pending events")
			}
		}
		return fmt.Errorf("failed to create temp pending events file: %w", err)
	}
	defer func() {
		f.Close()
		os.Remove(tempFile) // Clean up temp file if rename fails
	}()
	
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write pending events data: %w", err)
	}
	
	if err := f.Sync(); err != nil {
		return fmt.Errorf("failed to sync pending events file: %w", err)
	}
	
	f.Close() // Close before rename
	
	// Atomic rename
	if err := os.Rename(tempFile, ia.pendingEventsFile); err != nil {
		return fmt.Errorf("failed to rename pending events file: %w", err)
	}
	
	return nil
}

// addPendingEvent adds an event to pending list with batching to reduce disk I/O
func (ia *IntegratedAgent) addPendingEvent(event map[string]interface{}) error {
	pendingEvent := PendingEvent{
		ID:        fmt.Sprintf("%s_%d", ia.agentID, time.Now().UnixNano()),
		Event:     event,
		Timestamp: time.Now(),
		Retries:   0,
	}
	
	ia.pendingEventsMutex.Lock()
	defer ia.pendingEventsMutex.Unlock()
	
	// Add to buffer first
	ia.pendingEventsBuffer = append(ia.pendingEventsBuffer, pendingEvent)
	
	// Save to disk only if:
	// 1. Buffer has 10+ events, OR
	// 2. Last save was more than 30 seconds ago, OR  
	// 3. Buffer has any events and last save was more than 5 seconds ago
	now := time.Now()
	bufferSize := len(ia.pendingEventsBuffer)
	timeSinceLastSave := now.Sub(ia.lastPendingEventsSave)
	
	shoudSave := bufferSize >= 10 || 
				timeSinceLastSave > 30*time.Second ||
				(bufferSize > 0 && timeSinceLastSave > 5*time.Second)
	
	if shoudSave {
		// Load existing events from disk (without holding mutex during I/O)
		ia.pendingEventsMutex.Unlock()
		existingEvents, err := ia.loadPendingEvents()
		ia.pendingEventsMutex.Lock()
		
		if err != nil {
			log.Printf("❌ Failed to load pending events: %v", err)
			existingEvents = []PendingEvent{} // Start fresh if load fails
		}
		
		// Combine existing events with buffered events
		allEvents := append(existingEvents, ia.pendingEventsBuffer...)
		
		// Save combined events
		ia.pendingEventsMutex.Unlock()
		err = ia.savePendingEvents(allEvents)
		ia.pendingEventsMutex.Lock()
		
		if err != nil {
			log.Printf("❌ Failed to save pending events: %v", err)
			return fmt.Errorf("failed to save pending event: %w", err)
		}
		
		// Clear buffer and update timestamp on successful save
		ia.pendingEventsBuffer = []PendingEvent{}
		ia.lastPendingEventsSave = now
		
		log.Printf("💾 Batch saved %d pending events to disk", bufferSize)
	} else {
		log.Printf("💾 Event buffered (will save later): %s (buffer: %d)", pendingEvent.ID, bufferSize)
	}
	
	return nil
}

// removePendingEvent removes an event from pending list
func (ia *IntegratedAgent) removePendingEvent(eventID string) error {
	events, err := ia.loadPendingEvents()
	if err != nil {
		return err
	}
	
	filteredEvents := make([]PendingEvent, 0, len(events))
	for _, event := range events {
		if event.ID != eventID {
			filteredEvents = append(filteredEvents, event)
		}
	}
	
	if err := ia.savePendingEvents(filteredEvents); err != nil {
		return fmt.Errorf("failed to save filtered pending events: %w", err)
	}
	
	log.Printf("🗑️  Removed pending event: %s", eventID)
	return nil
}

// replayPendingEvents sends all pending events to server
func (ia *IntegratedAgent) replayPendingEvents() {
	events, err := ia.loadPendingEvents()
	if err != nil {
		log.Printf("❌ Failed to load pending events for replay: %v", err)
		return
	}
	
	if len(events) == 0 {
		log.Printf("📭 No pending events to replay")
		return
	}
	
	log.Printf("📡 Replaying %d pending events", len(events))
	
	successfulEvents := make([]string, 0)
	
	for _, pendingEvent := range events {
		// Try to send the event
		if err := ia.sendWebSocketMessageWithRetry(pendingEvent.Event); err != nil {
			log.Printf("❌ Failed to replay event %s: %v", pendingEvent.ID, err)
			// Increment retry count
			pendingEvent.Retries++
			if pendingEvent.Retries >= 3 {
				log.Printf("⚠️  Event %s exceeded max retries, removing", pendingEvent.ID)
				successfulEvents = append(successfulEvents, pendingEvent.ID)
			}
		} else {
			log.Printf("✅ Successfully replayed event %s", pendingEvent.ID)
			successfulEvents = append(successfulEvents, pendingEvent.ID)
		}
	}
	
	// Remove successfully sent events
	for _, eventID := range successfulEvents {
		if err := ia.removePendingEvent(eventID); err != nil {
			log.Printf("❌ Failed to remove sent event %s: %v", eventID, err)
		}
	}
	
	log.Printf("🔄 Event replay completed: %d events processed, %d removed", len(events), len(successfulEvents))
}

// sendWebSocketMessageWithRetry sends message and returns error if failed
func (ia *IntegratedAgent) sendWebSocketMessageWithRetry(msg map[string]interface{}) error {
	// Check if WebSocket is connected
	if ia.wsConn == nil {
		return fmt.Errorf("websocket not connected")
	}
	
	// Try to send message to channel
	select {
	case ia.wsSendChan <- msg:
		// Message queued successfully
		return nil
	default:
		// Channel is full or closed
		return fmt.Errorf("websocket send channel full or closed")
	}
}

// handleGetFolderStatsMessage handles folder statistics request
func (ia *IntegratedAgent) handleGetFolderStatsMessage(msg map[string]interface{}) {
	log.Printf("Received folder stats request: %+v", msg)
	
	// Get folder ID from message
	folderID, hasFolderID := msg["folder_id"].(string)
	if !hasFolderID {
		log.Printf("Folder stats request missing folder_id")
		ia.sendWebSocketMessage(map[string]interface{}{
			"type":    "folder_stats_error",
			"message": "Missing folder_id field",
		})
		return
	}
	
	// Get folder statistics from Syncthing
	folderStats, err := ia.syncthing.GetFolderStatus(folderID)
	if err != nil {
		log.Printf("Failed to get folder stats for %s: %v", folderID, err)
		ia.sendWebSocketMessage(map[string]interface{}{
			"type":      "folder_stats_error",
			"folder_id": folderID,
			"message":   fmt.Sprintf("Failed to get folder stats: %v", err),
		})
		return
	}
	
	// Get current progress for this folder
	progress := ia.getFolderProgress(folderID)
	
	// Add progress information to folder stats if available
	response := map[string]interface{}{
		"type":        "folder_stats_response",
		"folder_id":   folderID,
		"agent_id":    ia.agentID,
		"stats":       folderStats,
	}
	
	// Add progress information if available
	if progress != nil {
		response["progress"] = map[string]interface{}{
			"scan_progress": progress.ScanProgress,
			"pull_progress": progress.PullProgress,
			"last_updated":  progress.LastUpdated,
		}
		log.Printf("📊 Including progress data: scan=%.1f%%, pull=%.1f%%", 
			progress.ScanProgress, progress.PullProgress)
	}
	
	// Send folder statistics back to server
	ia.sendWebSocketMessage(response)
	
	log.Printf("✅ Sent folder stats for %s: %d files, %d bytes, state: %s", 
		folderID, folderStats.GlobalFiles, folderStats.GlobalBytes, folderStats.State)
}

// updateFolderProgress updates progress information for a folder
func (ia *IntegratedAgent) updateFolderProgress(folderID, state string, scanProgress, pullProgress float64) {
	ia.progressMutex.Lock()
	defer ia.progressMutex.Unlock()
	
	if ia.folderProgress == nil {
		ia.folderProgress = make(map[string]*FolderProgress)
	}
	
	progress := &FolderProgress{
		FolderID:     folderID,
		State:        state,
		ScanProgress: scanProgress,
		PullProgress: pullProgress,
		LastUpdated:  time.Now(),
	}
	
	ia.folderProgress[folderID] = progress
	log.Printf("📊 Updated progress for %s: state=%s, scan=%.1f%%, pull=%.1f%%", 
		folderID, state, scanProgress, pullProgress)
}

// getFolderProgress gets current progress for a folder
func (ia *IntegratedAgent) getFolderProgress(folderID string) *FolderProgress {
	ia.progressMutex.RLock()
	defer ia.progressMutex.RUnlock()
	
	if progress, exists := ia.folderProgress[folderID]; exists {
		return progress
	}
	return nil
}

// startPeriodicStatsForJob starts sending periodic folder stats for a specific job
func (ia *IntegratedAgent) startPeriodicStatsForJob(jobID string) {
	ia.periodicMutex.Lock()
	defer ia.periodicMutex.Unlock()
	
	// Mark job as active
	ia.activeSyncJobs[jobID] = true
	
	// Stop existing timer if any
	if timer, exists := ia.periodicTimers[jobID]; exists {
		timer.Stop()
	}
	
	// Create folder ID from job ID
	folderID := fmt.Sprintf("job-%s", jobID)
	
	// Start new timer for 5-second intervals
	ia.periodicTimers[jobID] = time.AfterFunc(5*time.Second, func() {
		ia.sendPeriodicStats(jobID, folderID)
	})

	// Start auto-resync monitoring if enabled
	log.Printf("🔄 Auto-resync config: enabled=%v, interval=%v", ia.config.Monitoring.AutoResyncEnabled, ia.config.Monitoring.AutoResyncInterval)
	if ia.config.Monitoring.AutoResyncEnabled {
		// Stop existing auto-resync timer if any
		if autoTimer, exists := ia.autoResyncTimers[jobID]; exists {
			autoTimer.Stop()
		}

		autoResyncInterval := ia.config.Monitoring.AutoResyncInterval
		if autoResyncInterval == 0 {
			autoResyncInterval = 30 * time.Second // Default to 30 seconds
		}

		// Start auto-resync timer
		ia.autoResyncTimers[jobID] = time.AfterFunc(autoResyncInterval, func() {
			ia.runAutoResyncMonitoring(jobID)
		})

		log.Printf("🔄 Started auto-resync monitoring for job %s (interval: %v)", jobID, autoResyncInterval)
	}

	log.Printf("📊 Started periodic stats for job %s (folder: %s)", jobID, folderID)
}

// stopPeriodicStatsForJob stops sending periodic folder stats for a specific job
func (ia *IntegratedAgent) stopPeriodicStatsForJob(jobID string) {
	ia.periodicMutex.Lock()
	defer ia.periodicMutex.Unlock()
	
	// Mark job as inactive
	ia.activeSyncJobs[jobID] = false
	
	// Stop timer if exists
	if timer, exists := ia.periodicTimers[jobID]; exists {
		timer.Stop()
		delete(ia.periodicTimers, jobID)
	}

	// Stop auto-resync timer if exists
	if autoTimer, exists := ia.autoResyncTimers[jobID]; exists {
		autoTimer.Stop()
		delete(ia.autoResyncTimers, jobID)
		log.Printf("🔄 Stopped auto-resync monitoring for job %s", jobID)
	}

	log.Printf("📊 Stopped periodic stats for job %s", jobID)
}

// stopPeriodicStatsButKeepAutoResync stops periodic stats but keeps auto-resync monitoring active
func (ia *IntegratedAgent) stopPeriodicStatsButKeepAutoResync(jobID string) {
	ia.periodicMutex.Lock()
	defer ia.periodicMutex.Unlock()

	// Mark job as inactive for periodic stats
	ia.activeSyncJobs[jobID] = false

	// Stop periodic stats timer if exists
	if timer, exists := ia.periodicTimers[jobID]; exists {
		timer.Stop()
		delete(ia.periodicTimers, jobID)
	}

	// Keep auto-resync timer running - this is the key change!
	if _, exists := ia.autoResyncTimers[jobID]; exists {
		log.Printf("📊 Stopped periodic stats for job %s but keeping auto-resync active", jobID)
	} else {
		log.Printf("📊 Stopped periodic stats for job %s (no auto-resync timer found)", jobID)
	}
}

// sendFinalStatsBeforeIdle sends final folder stats before stopping periodic stats
// This ensures the cache is populated for later use when job is paused
func (ia *IntegratedAgent) sendFinalStatsBeforeIdle(jobID, folderID string) {
	log.Printf("📊 Sending final stats for job %s before idle to populate cache", jobID)

	// Get folder statistics from Syncthing (this will cache the stats)
	folderStats, err := ia.syncthing.GetFolderStatus(folderID)
	if err != nil {
		log.Printf("❌ Failed to get final folder stats for job %s: %v", jobID, err)
		return
	}

	log.Printf("💾 Final stats cached for job %s: GlobalFiles=%d, LocalFiles=%d, State=%s",
		jobID, folderStats.GlobalFiles, folderStats.LocalFiles, folderStats.State)

	// Note: We don't send this to server, just cache it locally
	// The cache will be used when folder is paused
}

// sendPeriodicStats sends folder stats for a job and schedules the next send
func (ia *IntegratedAgent) sendPeriodicStats(jobID, folderID string) {
	ia.periodicMutex.RLock()
	isActive := ia.activeSyncJobs[jobID]
	ia.periodicMutex.RUnlock()
	
	// Don't send if job is no longer active
	if !isActive {
		return
	}
	
	// Get folder statistics from Syncthing
	folderStats, err := ia.syncthing.GetFolderStatus(folderID)
	if err != nil {
		log.Printf("❌ Failed to get periodic folder stats for job %s: %v", jobID, err)
		// Schedule next attempt anyway (no retry, just wait for next interval)
		ia.scheduleNextPeriodicStats(jobID, folderID)
		return
	}
	
	// Get current progress for this folder
	progress := ia.getFolderProgress(folderID)
	
	// Create periodic stats response
	response := map[string]interface{}{
		"type":        "folder_stats_periodic",
		"job_id":      jobID,
		"folder_id":   folderID,
		"agent_id":    ia.agentID,
		"stats":       folderStats,
		"is_periodic": true,
	}
	
	// Add progress information if available
	if progress != nil {
		response["progress"] = map[string]interface{}{
			"scan_progress": progress.ScanProgress,
			"pull_progress": progress.PullProgress,
			"last_updated":  progress.LastUpdated,
		}
	}
	
	// Send stats to server
	ia.sendWebSocketMessage(response)
	
	log.Printf("📊 Sent periodic stats for job %s: %d files, %d bytes, state: %s", 
		jobID, folderStats.GlobalFiles, folderStats.GlobalBytes, folderStats.State)
	
	// Schedule next periodic send
	ia.scheduleNextPeriodicStats(jobID, folderID)
}

// scheduleNextPeriodicStats schedules the next periodic stats send
func (ia *IntegratedAgent) scheduleNextPeriodicStats(jobID, folderID string) {
	ia.periodicMutex.Lock()
	defer ia.periodicMutex.Unlock()
	
	// Check if job is still active
	if !ia.activeSyncJobs[jobID] {
		return
	}
	
	// Schedule next send in 5 seconds
	ia.periodicTimers[jobID] = time.AfterFunc(5*time.Second, func() {
		ia.sendPeriodicStats(jobID, folderID)
	})
}

// triggerFolderRescan triggers a rescan for the specified folder
func (ia *IntegratedAgent) triggerFolderRescan(folderID string) error {
	if ia.syncthing == nil {
		return fmt.Errorf("syncthing instance is nil")
	}
	
	log.Printf("🔄 Triggering rescan for folder: %s", folderID)
	
	// Use embedded Syncthing's ScanFolder method instead of REST API
	err := ia.syncthing.ScanFolder(folderID)
	if err != nil {
		return fmt.Errorf("failed to trigger rescan: %v", err)
	}
	
	log.Printf("✅ Successfully triggered rescan for folder: %s", folderID)
	return nil
}

// extractIPFromAgentConnection automatically extracts IP address from agent connection or configuration
func (ia *IntegratedAgent) extractIPFromAgentConnection(remoteAgentID string) string {
	// Method 1: Try to extract from known agent mappings (if available)
	// This could be enhanced with a local cache of agent IPs
	
	// Method 2: Extract from server connection - parse server URL to get base IP
	if ia.wsURL != "" {
		// Parse WebSocket URL like "ws://192.168.50.157:8090/ws/agent"
		// Extract the server IP and use it as base for agent IP estimation
		if strings.Contains(ia.wsURL, "://") {
			parts := strings.Split(ia.wsURL, "://")
			if len(parts) > 1 {
				hostPort := strings.Split(parts[1], "/")[0] // Get host:port part
				host := strings.Split(hostPort, ":")[0]      // Get host part
				
				// For common network setups, try to map agent IDs to IPs
				// This is a heuristic approach - could be enhanced with DNS resolution
				if remoteAgentID == "syncunix-agent2-agent" {
					return "192.168.50.159"
				}
				if strings.Contains(remoteAgentID, "agent1") || strings.Contains(remoteAgentID, "167") {
					return "192.168.50.167"
				}
				if strings.Contains(remoteAgentID, "agent2") || strings.Contains(remoteAgentID, "159") {
					return "192.168.50.159"
				}
				
				// Method 3: Check if agent has advertise_address in config
				if ia.config != nil && ia.config.Syncthing.AdvertiseAddress != "" {
					// Extract IP from advertise_address like "tcp://192.168.50.159:22101"
					addrParts := strings.Split(ia.config.Syncthing.AdvertiseAddress, "://")
					if len(addrParts) > 1 {
						hostPort := strings.Split(addrParts[1], ":")[0]
						return hostPort
					}
				}
				
				// Fallback: return server host if in same network
				return host
			}
		}
	}
	
	// Method 4: Fallback to empty string if all methods fail
	log.Printf("⚠️ Could not extract IP for agent %s, trying common patterns", remoteAgentID)
	return ""
}

// runAutoResyncMonitoring runs the auto-resync monitoring cycle for a specific job
func (ia *IntegratedAgent) runAutoResyncMonitoring(jobID string) {
	log.Printf("[AUTO-RESYNC] 🔄 Running monitoring cycle for job %s", jobID)

	// Run the out-of-sync check
	ia.checkAndHandleOutOfSync()

	// Schedule next auto-resync check if auto-resync is enabled
	if ia.config.Monitoring.AutoResyncEnabled {
		autoResyncInterval := ia.config.Monitoring.AutoResyncInterval
		if autoResyncInterval == 0 {
			autoResyncInterval = 30 * time.Second
		}

		ia.periodicMutex.Lock()
		ia.autoResyncTimers[jobID] = time.AfterFunc(autoResyncInterval, func() {
			ia.runAutoResyncMonitoring(jobID)
		})
		ia.periodicMutex.Unlock()

		log.Printf("[AUTO-RESYNC] 🔄 Scheduled next monitoring cycle for job %s in %v", jobID, autoResyncInterval)
	}
}

// checkAndHandleOutOfSync performs automatic out-of-sync detection and triggers resync if needed
func (ia *IntegratedAgent) checkAndHandleOutOfSync() {
	// Get active auto-resync jobs with proper lock
	ia.periodicMutex.RLock()
	activeAutoResyncJobs := make(map[string]bool)
	for jobID := range ia.autoResyncTimers {
		activeAutoResyncJobs[jobID] = true
	}
	ia.periodicMutex.RUnlock()

	// Now check folder progress with its own lock
	ia.progressMutex.RLock()
	folderIDs := make([]string, 0, len(ia.folderProgress))
	for folderID := range ia.folderProgress {
		folderIDs = append(folderIDs, folderID)
	}
	ia.progressMutex.RUnlock()

	// Iterate without holding locks to avoid blocking other operations
	for _, folderID := range folderIDs {
		if strings.HasPrefix(folderID, "job-") {
			jobID := strings.TrimPrefix(folderID, "job-")

			// Skip if no active auto-resync timer for this job
			if !activeAutoResyncJobs[jobID] {
				continue
			}

			// Lightweight status check with timeout
			status, err := ia.syncthing.GetFolderStatus(folderID)
			if err != nil {
				log.Printf("Auto-resync: Failed to get folder status for %s: %v", folderID, err)
				continue
			}

			log.Printf("[AUTO-RESYNC-DETECT] 🔍 Checking job %s - GlobalFiles=%d, LocalFiles=%d, NeedFiles=%d, State=%s",
				jobID, status.GlobalFiles, status.LocalFiles, status.NeedFiles, status.State)

			// Check if folder has files that need to be synced (out-of-sync)
			// For receiveonly folders, NeedFiles might be 0 even when files are missing
			// So we also check if LocalFiles < GlobalFiles
			missingFiles := int64(0)
			missingBytes := int64(0)

			if status.NeedFiles > 0 && status.NeedBytes > 0 {
				// Standard case: Syncthing detected missing files
				missingFiles = status.NeedFiles
				missingBytes = status.NeedBytes
				log.Printf("[AUTO-RESYNC-DETECT] 📊 Standard detection: NeedFiles=%d, NeedBytes=%d",
					status.NeedFiles, status.NeedBytes)
			} else if status.LocalFiles < status.GlobalFiles {
				// Receiveonly case: Local files less than global (files were deleted)
				missingFiles = status.GlobalFiles - status.LocalFiles
				missingBytes = 0 // We don't know exact bytes, but we know files are missing
				log.Printf("[AUTO-RESYNC-DETECT] 🔍 Receiveonly folder detected - %d files missing (GlobalFiles=%d > LocalFiles=%d)",
					missingFiles, status.GlobalFiles, status.LocalFiles)
			}

			if missingFiles > 0 {
				log.Printf("[AUTO-RESYNC-DETECT] ⚠️ Detected %d missing files (%d bytes) in job %s",
					missingFiles, missingBytes, jobID)

				// Trigger automatic selective resync
				ia.triggerAutomaticResync(folderID, jobID, missingFiles, missingBytes)
			} else {
				log.Printf("[AUTO-RESYNC-DETECT] ✅ Job %s is in sync (no missing files)", jobID)
			}
		}
	}
}

// triggerAutomaticResync initiates selective resync for missing files
func (ia *IntegratedAgent) triggerAutomaticResync(folderID, jobID string, needFiles, needBytes int64) {
	log.Printf("[AUTO-RESYNC-TRIGGER] 🔄 Starting selective sync for %d missing files (%d bytes) in job %s",
		needFiles, needBytes, jobID)

	// For receiveonly folders, use native Revert() method
	// This is specifically designed by Syncthing for receiveonly folder recovery
	log.Printf("[AUTO-RESYNC-TRIGGER] 🔄 Using native Revert for folder %s to restore deleted files", folderID)
	err := ia.syncthing.RevertFolderNative(folderID)
	if err != nil {
		log.Printf("[AUTO-RESYNC-TRIGGER] ❌ Failed native Revert for %s: %v", folderID, err)

		// Fallback 1: Try Override method
		log.Printf("[AUTO-RESYNC-TRIGGER] 🔄 Fallback: trying Override method for folder %s", folderID)
		err = ia.syncthing.OverrideFolder(folderID)
		if err != nil {
			log.Printf("[AUTO-RESYNC-TRIGGER] ❌ Failed Override for %s: %v", folderID, err)

			// Fallback 2: Database reset (may cause duplication!)
			log.Printf("[AUTO-RESYNC-TRIGGER] ⚠️ Last resort: database reset for folder %s", folderID)
			err = ia.syncthing.ResetFolderDatabase(folderID)
			if err != nil {
				log.Printf("[AUTO-RESYNC-TRIGGER] ❌ All methods failed for %s", folderID)
				return
			}
		}
	}

	// Send automatic resync event to server for monitoring
	ia.sendWebSocketMessage(map[string]interface{}{
		"type":          "automatic_resync_triggered",
		"job_id":        jobID,
		"folder_id":     folderID,
		"missing_files": needFiles,
		"missing_bytes": needBytes,
		"timestamp":     time.Now(),
		"trigger":       "file_deletion_detection",
	})

	log.Printf("[AUTO-RESYNC-TRIGGER] ✅ Revert triggered for job %s, missing files will be restored from source", jobID)
}

// ============================================
// SESSION TRACKING FUNCTIONS
// ============================================

// generateSessionID creates a unique session identifier
func (ia *IntegratedAgent) generateSessionID(jobID string) string {
	timestamp := time.Now().Format("20060102-150405")
	random := fmt.Sprintf("%04d", time.Now().UnixNano()%10000)
	return fmt.Sprintf("%s-session-%s-%s", jobID, timestamp, random)
}

// startSyncSession creates and initializes a new sync session
func (ia *IntegratedAgent) startSyncSession(jobID, state string) *integration.SyncSessionStats {
	ia.sessionMutex.Lock()
	defer ia.sessionMutex.Unlock()

	// Check if session already exists
	if session, exists := ia.activeSessions[jobID]; exists {
		log.Printf("⚠️  Session already exists for job %s, updating state to %s", jobID, state)
		session.CurrentState = state
		return session
	}

	// Create new session
	sessionID := ia.generateSessionID(jobID)
	now := time.Now()

	session := &integration.SyncSessionStats{
		SessionID:        sessionID,
		JobID:            jobID,
		AgentID:          ia.agentID,
		SessionStartTime: now,
		CurrentState:     state,
		Status:           "active",
		Timestamp:        now,
	}

	// Set scan start time if state is scanning
	if state == "scanning" {
		session.ScanStartTime = &now
	}

	ia.activeSessions[jobID] = session

	// Send session started event to server
	ia.sendSessionEvent("session_started", session)

	log.Printf("📊 [SESSION] Started new session: %s for job %s (state: %s)", sessionID, jobID, state)
	return session
}

// updateSyncSession updates an existing sync session with new state
func (ia *IntegratedAgent) updateSyncSession(jobID, state string) {
	ia.sessionMutex.Lock()
	defer ia.sessionMutex.Unlock()

	session, exists := ia.activeSessions[jobID]
	if !exists {
		log.Printf("⚠️  No active session for job %s, creating new one", jobID)
		ia.sessionMutex.Unlock()
		ia.startSyncSession(jobID, state)
		ia.sessionMutex.Lock()
		return
	}

	now := time.Now()
	previousState := session.CurrentState
	session.CurrentState = state
	session.Timestamp = now

	// Handle state transitions
	switch state {
	case "scanning":
		if session.ScanStartTime == nil {
			session.ScanStartTime = &now
			ia.sendSessionEvent("scan_started", session)
			log.Printf("📊 [SESSION] Scan started for job %s", jobID)
		}

	case "syncing":
		// End scan if it was running
		if previousState == "scanning" && session.ScanEndTime == nil {
			session.ScanEndTime = &now
			if session.ScanStartTime != nil {
				duration := now.Sub(*session.ScanStartTime).Seconds()
				session.ScanDurationSeconds = int64(duration)
			}
			ia.sendSessionEvent("scan_completed", session)
			log.Printf("📊 [SESSION] Scan completed for job %s (duration: %ds)", jobID, session.ScanDurationSeconds)
		}

		// Start transfer tracking
		if session.TransferStartTime == nil {
			session.TransferStartTime = &now
			ia.sendSessionEvent("transfer_started", session)
			log.Printf("📊 [SESSION] Transfer started for job %s", jobID)
		}

	case "idle":
		// Session completing - will be finalized separately
		log.Printf("📊 [SESSION] Job %s transitioning to idle", jobID)
	}
}

// finalizeSyncSession completes and sends final session stats
func (ia *IntegratedAgent) finalizeSyncSession(jobID string) {
	ia.sessionMutex.Lock()
	defer ia.sessionMutex.Unlock()

	session, exists := ia.activeSessions[jobID]
	if !exists {
		log.Printf("⚠️  No active session to finalize for job %s", jobID)
		return
	}

	now := time.Now()
	session.SessionEndTime = &now
	session.Status = "completed"
	session.CurrentState = "idle"

	// Calculate total duration
	totalDuration := now.Sub(session.SessionStartTime).Seconds()
	session.TotalDurationSeconds = int64(totalDuration)

	// Finalize scan if not already done
	if session.ScanStartTime != nil && session.ScanEndTime == nil {
		session.ScanEndTime = &now
		scanDuration := now.Sub(*session.ScanStartTime).Seconds()
		session.ScanDurationSeconds = int64(scanDuration)
	}

	// Finalize transfer if not already done
	if session.TransferStartTime != nil && session.TransferEndTime == nil {
		session.TransferEndTime = &now
		transferDuration := now.Sub(*session.TransferStartTime).Seconds()
		session.TransferDurationSeconds = int64(transferDuration)
	}

	// Calculate average transfer rate
	if session.TransferDurationSeconds > 0 && session.TotalDeltaBytes > 0 {
		session.AverageTransferRate = float64(session.TotalDeltaBytes) / float64(session.TransferDurationSeconds)
	}

	// Calculate compression ratio
	if session.TotalFullFileSize > 0 {
		session.CompressionRatio = float64(session.TotalDeltaBytes) / float64(session.TotalFullFileSize)
	}

	// Send final session stats to server
	ia.sendSessionEvent("session_completed", session)

	log.Printf("📊 [SESSION] Completed session %s: files=%d, delta_bytes=%d, full_size=%d, compression=%.2f%%, duration=%ds",
		session.SessionID, session.FilesTransferred, session.TotalDeltaBytes,
		session.TotalFullFileSize, session.CompressionRatio*100, session.TotalDurationSeconds)

	// Remove from active sessions
	delete(ia.activeSessions, jobID)
}

// updateSessionFileTransfer updates session stats when a file transfer completes
func (ia *IntegratedAgent) updateSessionFileTransfer(jobID string, deltaBytes, fullFileSize int64, transferRate float64) {
	ia.sessionMutex.Lock()
	defer ia.sessionMutex.Unlock()

	session, exists := ia.activeSessions[jobID]
	if !exists {
		log.Printf("⚠️  No active session for job %s to update file transfer", jobID)
		return
	}

	// Update counters
	session.FilesTransferred++
	session.TotalDeltaBytes += deltaBytes
	session.TotalFullFileSize += fullFileSize

	// Update peak transfer rate
	if transferRate > session.PeakTransferRate {
		session.PeakTransferRate = transferRate
	}

	// Recalculate compression ratio
	if session.TotalFullFileSize > 0 {
		session.CompressionRatio = float64(session.TotalDeltaBytes) / float64(session.TotalFullFileSize)
	}

	log.Printf("📊 [SESSION] Updated session %s: files=%d, delta=%d, full=%d, ratio=%.2f%%",
		session.SessionID, session.FilesTransferred, session.TotalDeltaBytes,
		session.TotalFullFileSize, session.CompressionRatio*100)
}

// getActiveSession returns the current active session for a job
func (ia *IntegratedAgent) getActiveSession(jobID string) *integration.SyncSessionStats {
	ia.sessionMutex.RLock()
	defer ia.sessionMutex.RUnlock()
	return ia.activeSessions[jobID]
}

// sendSessionEvent sends session event to server
func (ia *IntegratedAgent) sendSessionEvent(eventType string, session *integration.SyncSessionStats) {
	ia.sendWebSocketMessage(map[string]interface{}{
		"type":  "session_event",
		"event": map[string]interface{}{
			"type": eventType,
			"data": session,
		},
	})
}

