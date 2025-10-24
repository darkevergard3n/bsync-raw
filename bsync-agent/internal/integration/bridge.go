package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"bsync-agent/internal/embedded"
	"bsync-agent/pkg/types"
)

// EventBridge bridges events between embedded Syncthing and SyncTool agent
// FileStartInfo stores information from ItemStarted events for duration calculation
type FileStartInfo struct {
	StartTime time.Time
	Action    string
}

type EventBridge struct {
	syncthing      *embedded.EmbeddedSyncthing
	eventChan      <-chan embedded.Event
	agentEvents    chan types.AgentEvent
	running        bool
	fileStartTimes map[string]FileStartInfo // Track ItemStarted info for duration calculation
	
	// Event batching for high-volume operations
	batchBuffer    []types.AgentEvent
	batchSize      int
	batchTimer     *time.Timer
	batchMutex     sync.RWMutex
	droppedEvents  int64 // Counter for dropped events
	
	// Circular buffer for overflow handling
	circularBuffer *CircularBuffer
}

// AgentEvent represents events sent to SyncTool server
type AgentEvent struct {
	Type      string                 `json:"type"`
	Timestamp time.Time             `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// FileTransferEvent represents a file transfer event
type FileTransferEvent struct {
	JobID                 string    `json:"job_id"`
	SessionID             string    `json:"session_id,omitempty"`          // Session identifier for grouping transfers
	FileName              string    `json:"file_name"`
	FullFileSize          int64     `json:"full_file_size"`                // Full file size (reference)
	DeltaBytesTransferred int64     `json:"delta_bytes_transferred"`       // Actual bytes transferred (delta sync)
	DeltaBytesCompleted   int64     `json:"delta_bytes_completed"`         // Delta bytes completed so far
	FileSize              int64     `json:"file_size"`                     // DEPRECATED: Use FullFileSize instead (kept for backward compatibility)
	Action                string    `json:"action"`                        // "update", "delete", etc.
	Status                string    `json:"status"`
	Progress              float64   `json:"progress"`                      // Progress percentage (0-100)
	TransferRate          float64   `json:"transfer_rate"`                 // Bytes per second
	Duration              float64   `json:"duration"`                      // Duration in seconds
	CompressionRatio      float64   `json:"compression_ratio,omitempty"`   // Delta efficiency (delta/full)
	Error                 string    `json:"error,omitempty"`
	Timestamp             time.Time `json:"timestamp"`
}

// SyncSessionStats represents aggregated statistics for a sync session
type SyncSessionStats struct {
	SessionID             string    `json:"session_id"`
	JobID                 string    `json:"job_id"`
	AgentID               string    `json:"agent_id,omitempty"`

	// Session timing
	SessionStartTime      time.Time `json:"session_start_time"`
	SessionEndTime        *time.Time `json:"session_end_time,omitempty"`
	TotalDurationSeconds  int64     `json:"total_duration_seconds,omitempty"`

	// Scan statistics
	ScanStartTime         *time.Time `json:"scan_start_time,omitempty"`
	ScanEndTime           *time.Time `json:"scan_end_time,omitempty"`
	ScanDurationSeconds   int64     `json:"scan_duration_seconds,omitempty"`
	FilesScanned          int64     `json:"files_scanned"`
	BytesScanned          int64     `json:"bytes_scanned"`

	// Transfer statistics (delta-aware)
	TransferStartTime     *time.Time `json:"transfer_start_time,omitempty"`
	TransferEndTime       *time.Time `json:"transfer_end_time,omitempty"`
	TransferDurationSeconds int64   `json:"transfer_duration_seconds,omitempty"`
	FilesTransferred      int64     `json:"files_transferred"`
	TotalDeltaBytes       int64     `json:"total_delta_bytes"`           // Actual bytes transferred
	TotalFullFileSize     int64     `json:"total_full_file_size"`        // Combined file sizes
	CompressionRatio      float64   `json:"compression_ratio,omitempty"` // Efficiency (delta/full)

	// Transfer performance
	AverageTransferRate   float64   `json:"average_transfer_rate,omitempty"` // Bytes per second
	PeakTransferRate      float64   `json:"peak_transfer_rate,omitempty"`    // Maximum rate observed

	// Session state
	CurrentState          string    `json:"current_state"`               // idle, scanning, syncing, completed, failed
	Status                string    `json:"status"`                      // active, completed, failed
	ErrorMessage          string    `json:"error_message,omitempty"`

	Timestamp             time.Time `json:"timestamp"`
}

// SyncStatusEvent represents a sync status event
type SyncStatusEvent struct {
	FolderID    string    `json:"folder_id"`
	State       string    `json:"state"`
	Progress    float64   `json:"progress"`
	GlobalFiles int64     `json:"global_files"`
	LocalFiles  int64     `json:"local_files"`
	NeedFiles   int64     `json:"need_files"`
	Errors      []string  `json:"errors"`
	Timestamp   time.Time `json:"timestamp"`
}

// CircularBuffer implements a thread-safe circular buffer for high-volume events
type CircularBuffer struct {
	buffer []types.AgentEvent
	head   int
	tail   int
	size   int
	maxSize int
	mutex  sync.RWMutex
	full   bool
}

// NewCircularBuffer creates a new circular buffer
func NewCircularBuffer(maxSize int) *CircularBuffer {
	return &CircularBuffer{
		buffer:  make([]types.AgentEvent, maxSize),
		maxSize: maxSize,
		head:    0,
		tail:    0,
		size:    0,
		full:    false,
	}
}

// Push adds an event to the buffer (overwrites oldest if full)
func (cb *CircularBuffer) Push(event types.AgentEvent) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	
	cb.buffer[cb.head] = event
	
	if cb.full {
		// Buffer full, move tail forward (overwrite oldest)
		cb.tail = (cb.tail + 1) % cb.maxSize
	} else {
		// Buffer not full yet
		cb.size++
		if cb.size == cb.maxSize {
			cb.full = true
		}
	}
	
	cb.head = (cb.head + 1) % cb.maxSize
}

// Pop removes and returns the oldest event
func (cb *CircularBuffer) Pop() (types.AgentEvent, bool) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	
	if cb.size == 0 {
		return types.AgentEvent{}, false
	}
	
	event := cb.buffer[cb.tail]
	cb.tail = (cb.tail + 1) % cb.maxSize
	cb.size--
	cb.full = false
	
	return event, true
}

// Size returns current buffer size
func (cb *CircularBuffer) Size() int {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.size
}

// IsFull returns true if buffer is full
func (cb *CircularBuffer) IsFull() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.full
}

// PopBatch returns multiple events at once
func (cb *CircularBuffer) PopBatch(maxEvents int) []types.AgentEvent {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	
	if cb.size == 0 {
		return nil
	}
	
	batchSize := maxEvents
	if batchSize > cb.size {
		batchSize = cb.size
	}
	
	events := make([]types.AgentEvent, batchSize)
	for i := 0; i < batchSize; i++ {
		events[i] = cb.buffer[cb.tail]
		cb.tail = (cb.tail + 1) % cb.maxSize
		cb.size--
	}
	
	cb.full = false
	return events
}

// NewEventBridge creates a new event bridge
func NewEventBridge(syncthing *embedded.EmbeddedSyncthing) *EventBridge {
	return &EventBridge{
		syncthing:      syncthing,
		agentEvents:    make(chan types.AgentEvent, 60000), // Increased buffer for 300k+ file sync
		running:        false,
		fileStartTimes: make(map[string]FileStartInfo),
		batchBuffer:    make([]types.AgentEvent, 0, 600),
		batchSize:      600, // Batch 600 events together for better throughput
		droppedEvents:  0,
		circularBuffer: NewCircularBuffer(10000), // 10k circular buffer for overflow
	}
}

// Start starts the event bridge
func (eb *EventBridge) Start(ctx context.Context) error {
	if eb.running {
		return fmt.Errorf("event bridge already running")
	}

	// Subscribe to Syncthing events
	eb.eventChan = eb.syncthing.Subscribe("event-bridge")
	eb.running = true

	// Start processing events
	go eb.processEvents(ctx)
	
	// Start circular buffer drain process
	go eb.drainCircularBuffer(ctx)

	log.Println("Event bridge started with enhanced buffering")
	return nil
}

// Stop stops the event bridge
func (eb *EventBridge) Stop() error {
	if !eb.running {
		return nil
	}

	eb.running = false
	eb.syncthing.Unsubscribe("event-bridge")
	close(eb.agentEvents)

	log.Println("Event bridge stopped")
	return nil
}

// GetAgentEvents returns the channel for agent events
func (eb *EventBridge) GetAgentEvents() <-chan types.AgentEvent {
	return eb.agentEvents
}

// processEvents processes Syncthing events and converts them to agent events
func (eb *EventBridge) processEvents(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Event processing panic: %v", r)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-eb.eventChan:
			if !ok {
				return
			}
			eb.handleSyncthingEvent(event)
		}
	}
}

// handleSyncthingEvent handles a single Syncthing event
func (eb *EventBridge) handleSyncthingEvent(event embedded.Event) {
	// Debug logging for tracking events
	if event.Type == embedded.EventItemStarted || event.Type == embedded.EventItemFinished {
		log.Printf("DEBUG Bridge: Processing %s event", event.Type)
	}
	
	switch event.Type {
	case embedded.EventItemStarted:
		eb.handleItemStarted(event)
	case embedded.EventItemFinished:
		eb.handleItemFinished(event)
	case embedded.EventDownloadProgress:
		eb.handleDownloadProgress(event)
	case embedded.EventFolderSummary:
		eb.handleFolderSummary(event)
	case embedded.EventFolderCompletion:
		eb.handleFolderCompletion(event)
	case embedded.EventFolderErrors:
		eb.handleFolderErrors(event)
	case embedded.EventDeviceConnected:
		eb.handleDeviceConnected(event)
	case embedded.EventDeviceDisconnected:
		eb.handleDeviceDisconnected(event)
	case embedded.EventStateChanged:
		eb.handleStateChanged(event)
	case embedded.EventDeviceDiscovered:
		eb.handleDeviceDiscovered(event)
	case embedded.EventDeviceRejected:
		eb.handleDeviceRejected(event)
	case embedded.EventLocalChangeDetected:
		eb.handleLocalChangeDetected(event)
	case embedded.EventRemoteChangeDetected:
		eb.handleRemoteChangeDetected(event)
	case embedded.EventFolderScanProgress:
		log.Printf("DEBUG Bridge: Received FolderScanProgress event")
		eb.handleFolderScanProgress(event)
	case embedded.EventFolderRejected:
		eb.handleFolderRejected(event)
	case embedded.EventConfigSaved:
		eb.handleConfigSaved(event)
	case embedded.EventRemoteDownloadProgress:
		eb.handleRemoteDownloadProgress(event)
	case embedded.EventLocalIndexUpdated:
		eb.handleLocalIndexUpdated(event)
	case embedded.EventRemoteIndexUpdated:
		eb.handleRemoteIndexUpdated(event)
	default:
		// Log unknown events
		log.Printf("Unknown BSync event: %s", event.Type)
	}
}

func (eb *EventBridge) handleItemStarted(event embedded.Event) {
	data := event.Data
	
	// DEBUG: Log raw Syncthing data
	dataJSON, _ := json.Marshal(data)
	log.Printf("🐛 DEBUG ItemStarted raw data: %s", string(dataJSON))
	
	// Based on Syncthing source code analysis:
	// ItemStarted events use map[string]string and DO NOT contain file size
	// File size is only available in ItemFinished events
	var jobID, fileName, action string
	
	// Parse nested data structure: data["data"] contains the actual event data
	if nestedData, ok := data["data"]; ok {
		log.Printf("🔍 DEBUG ItemStarted: found data['data'] = %+v", nestedData)
		// Try map[string]interface{} first
		if dataMap, ok := nestedData.(map[string]interface{}); ok {
			log.Printf("🔍 DEBUG ItemStarted: dataMap = %+v", dataMap)
			jobID = getStringFromData(dataMap, "folder")
			fileName = getStringFromData(dataMap, "item")
			action = getStringFromData(dataMap, "action")
			log.Printf("🔍 DEBUG ItemStarted: extracted jobID=%s, fileName=%s, action=%s", jobID, fileName, action)
		} else if dataMapStr, ok := nestedData.(map[string]string); ok {
			// Handle map[string]string case (which Syncthing ItemStarted uses)
			log.Printf("🔍 DEBUG ItemStarted: dataMapStr = %+v", dataMapStr)
			jobID = dataMapStr["folder"]
			fileName = dataMapStr["item"]
			action = dataMapStr["action"]
			log.Printf("🔍 DEBUG ItemStarted: extracted from string map - jobID=%s, fileName=%s, action=%s", jobID, fileName, action)
		} else {
			log.Printf("❌ DEBUG ItemStarted: nestedData is neither map[string]interface{} nor map[string]string, type is %T", nestedData)
		}
	} else {
		log.Printf("❌ DEBUG ItemStarted: data['data'] not found, keys available: %+v", getMapKeys(data))
	}
	
	// If nested structure didn't work, try direct access
	if jobID == "" {
		jobID = getStringFromData(data, "folder")
	}
	if fileName == "" {
		fileName = getStringFromData(data, "item")
	}
	if action == "" {
		action = getStringFromData(data, "action")
	}
	
	fileTransfer := FileTransferEvent{
		JobID:     jobID,
		FileName:  fileName,
		FileSize:  0, // ItemStarted events don't contain file size per Syncthing source code
		Action:    action,
		Status:    "started",
		Progress:  0.0,
		Timestamp: event.Time,
	}

	// Store start info for duration calculation
	if jobID != "" && fileName != "" {
		fileKey := fmt.Sprintf("%s/%s", jobID, fileName)
		eb.fileStartTimes[fileKey] = FileStartInfo{
			StartTime: event.Time,
			Action:    action,
		}
		log.Printf("📅 Stored start info for %s: time=%v, action=%s", fileKey, event.Time, action)
	}

	// DEBUG: Log extracted data
	log.Printf("🐛 DEBUG ItemStarted extracted - JobID: %s, FileName: %s, Action: %s (FileSize not available in ItemStarted)", 
		fileTransfer.JobID, fileTransfer.FileName, action)

	eb.sendAgentEvent("file_transfer_started", fileTransfer)
	log.Printf("File transfer started: %s", fileTransfer.FileName)
}

func (eb *EventBridge) handleItemFinished(event embedded.Event) {
	data := event.Data
	
	// DEBUG: Log raw Syncthing data
	dataJSON, _ := json.Marshal(data)
	log.Printf("🐛 DEBUG ItemFinished raw data: %s", string(dataJSON))
	
	status := "completed"
	errorMsg := getStringFromData(data, "error")
	if errorMsg != "" {
		status = "failed"
	}

	folderID := getStringFromData(data, "folder")
	fileName := getStringFromData(data, "item")
	action := getStringFromData(data, "action")
	
	// Try to get file size from Syncthing events first (may not be available)
	fileSize := getInt64FromData(data, "size")
	
	// For delete actions, don't try to get file size from model (file is deleted)
	// For update actions, try to get file size from model if not in event
	if action != "delete" && fileSize == 0 && folderID != "" && fileName != "" {
		if modelSize, err := eb.syncthing.GetFileInfo(folderID, fileName); err == nil {
			fileSize = modelSize
			log.Printf("🔍 Retrieved file size from model: %s/%s = %d bytes", folderID, fileName, modelSize)
		} else {
			log.Printf("⚠️  Failed to get file size from model for %s/%s: %v", folderID, fileName, err)
		}
	}

	fileTransfer := FileTransferEvent{
		JobID:     folderID,
		FileName:  fileName,
		FileSize:  fileSize,
		Action:    action,
		Status:    status,
		Progress:  100.0,
		Error:     errorMsg,
		Timestamp: event.Time,
	}

	// Calculate duration using stored start info
	if folderID != "" && fileName != "" {
		fileKey := fmt.Sprintf("%s/%s", folderID, fileName)
		if startInfo, exists := eb.fileStartTimes[fileKey]; exists {
			fileTransfer.Duration = event.Time.Sub(startInfo.StartTime).Seconds()
			log.Printf("⏱️  Calculated duration for %s: %.3f seconds (started: %v, finished: %v, action: %s)", 
				fileKey, fileTransfer.Duration, startInfo.StartTime, event.Time, startInfo.Action)
			// Clean up stored start info
			delete(eb.fileStartTimes, fileKey)
		} else {
			log.Printf("⚠️  No start info found for %s, duration will be 0", fileKey)
		}
	}

	// DEBUG: Log extracted data
	log.Printf("🐛 DEBUG ItemFinished extracted - JobID: %s, FileName: %s, FileSize: %d, Action: %s, Duration: %.3f", 
		fileTransfer.JobID, fileTransfer.FileName, fileTransfer.FileSize, fileTransfer.Action, fileTransfer.Duration)

	eb.sendAgentEvent("file_transfer_completed", fileTransfer)
	log.Printf("File transfer completed: %s (status: %s)", fileTransfer.FileName, status)
}

func (eb *EventBridge) handleDownloadProgress(event embedded.Event) {
	data := event.Data

	log.Printf("DEBUG Bridge: handleDownloadProgress called with data: %+v", data)

	// Extract progress from nested DownloadProgress structure
	// Structure: data["data"][folderID][fileName] -> progress data

	// Level 1: Extract data["data"] field
	dataFieldRaw, exists := data["data"]
	if !exists {
		log.Printf("❌ Bridge: No 'data' field in DownloadProgress event")
		return
	}

	log.Printf("DEBUG Bridge: dataField type: %T", dataFieldRaw)

	// Level 2: Use reflection to handle any map type (not just map[string]interface{})
	dataFieldValue := reflect.ValueOf(dataFieldRaw)
	if dataFieldValue.Kind() != reflect.Map {
		log.Printf("❌ Bridge: dataField is not a map, kind: %v", dataFieldValue.Kind())
		return
	}

	log.Printf("DEBUG Bridge: dataField is a map with %d folders", dataFieldValue.Len())

	// Level 3: Iterate over folders in the data map
	for _, folderKey := range dataFieldValue.MapKeys() {
		folderID := folderKey.String()
		filesRaw := dataFieldValue.MapIndex(folderKey).Interface()

		log.Printf("DEBUG Bridge: Processing folder %s, files type: %T", folderID, filesRaw)

		// Handle file progress data (could be map[string]interface{} or map of pointers)
		totalBytes := int64(0)
		completedBytes := int64(0)
		var currentFileName string

		// Level 4: Process files in the folder using reflection to handle both map and pointer types
		filesValue := reflect.ValueOf(filesRaw)
		log.Printf("DEBUG Bridge: files reflection - Kind: %v, Type: %v", filesValue.Kind(), filesValue.Type())

		if filesValue.Kind() == reflect.Map {
			// Level 5: Iterate over map keys (file names)
			for _, fileKey := range filesValue.MapKeys() {
				fileName := fileKey.String()
				currentFileName = fileName

				// Get progress data for this file
				progressData := filesValue.MapIndex(fileKey).Interface()
				log.Printf("DEBUG Bridge: Processing file %s, progress data type: %T", fileName, progressData)

				// Try to extract BytesTotal and BytesDone from progress data
				bytesTotal, bytesDone := eb.extractProgressData(progressData)
				if bytesTotal > 0 {
					totalBytes += bytesTotal
					completedBytes += bytesDone
					log.Printf("DEBUG Bridge: File %s - %d/%d bytes (%.1f%%)",
						fileName, bytesDone, bytesTotal, float64(bytesDone)*100.0/float64(bytesTotal))
				} else {
					log.Printf("⚠️  Bridge: File %s - extraction returned 0 bytes (might be completed or no data)", fileName)
				}
			}
		} else {
			log.Printf("❌ Bridge: files is not a map, kind: %v", filesValue.Kind())
			continue
		}

		// Calculate overall progress percentage
		var progressPct float64 = 0
		if totalBytes > 0 {
			progressPct = float64(completedBytes) * 100.0 / float64(totalBytes)
		}

		// Get full file size from Syncthing DB (global file info)
		fullFileSize := eb.getFullFileSize(folderID, currentFileName)
		if fullFileSize == 0 {
			fullFileSize = totalBytes // Fallback to delta bytes if we can't get full size
		}

		// Calculate compression ratio
		var compressionRatio float64
		if fullFileSize > 0 {
			compressionRatio = float64(totalBytes) / float64(fullFileSize)
		}

		// Only send event if we have actual progress data
		if totalBytes > 0 {
			// Create file transfer event with delta-aware fields
			fileTransfer := FileTransferEvent{
				JobID:                 folderID,
				FileName:              currentFileName,
				FullFileSize:          fullFileSize,                // Full file size (reference)
				DeltaBytesTransferred: totalBytes,                  // Actual bytes to transfer (delta)
				DeltaBytesCompleted:   completedBytes,              // Delta bytes completed
				FileSize:              totalBytes,                  // DEPRECATED: kept for backward compatibility
				Action:                "update",
				Status:                "downloading",
				Progress:              progressPct,
				TransferRate:          0, // Calculate if needed
				CompressionRatio:      compressionRatio,
				Timestamp:             event.Time,
			}

			log.Printf("🎯 Bridge: Calculated real progress - folder=%s, file=%s, progress=%.1f%% (%d/%d bytes)",
				folderID, currentFileName, progressPct, completedBytes, totalBytes)

			eb.sendAgentEvent("file_transfer_progress", fileTransfer)
		} else {
			log.Printf("⚠️  Bridge: Skipping event for folder %s - no bytes extracted", folderID)
		}
	}
}

func (eb *EventBridge) handleFolderSummary(event embedded.Event) {
	data := event.Data
	summary := getMapFromData(data, "summary")
	
	syncStatus := SyncStatusEvent{
		FolderID:    getStringFromData(data, "folder"),
		State:       "idle", // Summary usually means idle
		GlobalFiles: getInt64FromData(summary, "globalFiles"),
		LocalFiles:  getInt64FromData(summary, "localFiles"),
		NeedFiles:   getInt64FromData(summary, "needFiles"),
		Timestamp:   event.Time,
	}

	// Calculate progress
	if syncStatus.GlobalFiles > 0 {
		syncStatus.Progress = float64(syncStatus.LocalFiles) / float64(syncStatus.GlobalFiles) * 100.0
	}

	eb.sendAgentEvent("sync_status", syncStatus)
}

func (eb *EventBridge) handleFolderCompletion(event embedded.Event) {
	data := event.Data
	
	syncStatus := SyncStatusEvent{
		FolderID:  getStringFromData(data, "folder"),
		State:     "syncing",
		Progress:  getFloat64FromData(data, "completion"),
		Timestamp: event.Time,
	}

	eb.sendAgentEvent("sync_status", syncStatus)
}

func (eb *EventBridge) handleFolderErrors(event embedded.Event) {
	data := event.Data
	errorsData := getSliceFromData(data, "errors")
	
	var errors []string
	for _, err := range errorsData {
		if errStr, ok := err.(string); ok {
			errors = append(errors, errStr)
		}
	}
	
	syncStatus := SyncStatusEvent{
		FolderID:  getStringFromData(data, "folder"),
		State:     "error",
		Errors:    errors,
		Timestamp: event.Time,
	}

	eb.sendAgentEvent("sync_error", syncStatus)
	log.Printf("Sync errors in folder %s: %v", syncStatus.FolderID, errors)
}

func (eb *EventBridge) handleDeviceConnected(event embedded.Event) {
	data := event.Data
	
	deviceEvent := map[string]interface{}{
		"device_id": getStringFromData(data, "id"),
		"address":   getStringFromData(data, "addr"),
		"status":    "connected",
		"timestamp": event.Time,
	}

	eb.sendAgentEvent("device_connected", deviceEvent)
	log.Printf("Device connected: %s", deviceEvent["device_id"])
}

func (eb *EventBridge) handleDeviceDisconnected(event embedded.Event) {
	data := event.Data
	
	deviceEvent := map[string]interface{}{
		"device_id": getStringFromData(data, "id"),
		"status":    "disconnected", 
		"timestamp": event.Time,
		"error":     getStringFromData(data, "error"),
	}

	eb.sendAgentEvent("device_disconnected", deviceEvent)
	log.Printf("Device disconnected: %s", deviceEvent["device_id"])
}

func (eb *EventBridge) handleStateChanged(event embedded.Event) {
	data := event.Data
	
	stateEvent := map[string]interface{}{
		"folder":    getStringFromData(data, "folder"),
		"from":      getStringFromData(data, "from"),
		"to":        getStringFromData(data, "to"),
		"timestamp": event.Time,
	}

	eb.sendAgentEvent("state_changed", stateEvent)
	log.Printf("State changed for folder %s: %s -> %s", 
		stateEvent["folder"], stateEvent["from"], stateEvent["to"])
}

func (eb *EventBridge) sendAgentEvent(eventType string, data interface{}) {
	if !eb.running {
		return
	}

	agentEvent := types.AgentEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
	}

	// Try to send to main channel first
	select {
	case eb.agentEvents <- agentEvent:
		return
	default:
		// Channel full, try circular buffer for high-priority events
		if eb.isHighPriorityEvent(eventType) {
			eb.circularBuffer.Push(agentEvent)
			log.Printf("High-priority event moved to circular buffer: %s", eventType)
			return
		}
	}

	// Add to batch buffer for less critical events or when both channels full
	eb.addToBatch(agentEvent)
}

// isHighPriorityEvent determines if an event should be sent immediately
func (eb *EventBridge) isHighPriorityEvent(eventType string) bool {
	switch eventType {
	case "sync_error", "folder_error", "connection_error":
		return true
	case "sync_completed", "folder_sync_completed":
		return true
	default:
		return false
	}
}

// addToBatch adds event to batch buffer with smart batching logic
func (eb *EventBridge) addToBatch(event types.AgentEvent) {
	eb.batchMutex.Lock()
	defer eb.batchMutex.Unlock()

	// Add to batch buffer
	eb.batchBuffer = append(eb.batchBuffer, event)
	
	// Dynamic batch sizing based on current load
	dynamicBatchSize := eb.calculateDynamicBatchSize()
	flushInterval := eb.calculateFlushInterval()
	
	if len(eb.batchBuffer) >= dynamicBatchSize {
		eb.flushBatch()
	} else if eb.batchTimer == nil {
		// Start timer with adaptive flush interval
		eb.batchTimer = time.AfterFunc(flushInterval, func() {
			eb.batchMutex.Lock()
			eb.flushBatch()
			eb.batchMutex.Unlock()
		})
	}
}

// calculateDynamicBatchSize returns optimal batch size based on system load
func (eb *EventBridge) calculateDynamicBatchSize() int {
	channelLoad := float64(len(eb.agentEvents)) / float64(cap(eb.agentEvents))
	circularLoad := float64(eb.circularBuffer.Size()) / float64(eb.circularBuffer.maxSize)
	
	// Increase batch size when system is under heavy load
	if channelLoad > 0.8 || circularLoad > 0.5 {
		return 1000 // High-load batch size
	} else if channelLoad > 0.5 || circularLoad > 0.2 {
		return 500  // Medium-load batch size
	} else {
		return 100  // Low-load batch size
	}
}

// calculateFlushInterval returns optimal flush interval based on load
func (eb *EventBridge) calculateFlushInterval() time.Duration {
	channelLoad := float64(len(eb.agentEvents)) / float64(cap(eb.agentEvents))
	
	// Reduce flush interval under high load for faster processing
	if channelLoad > 0.8 {
		return 50 * time.Millisecond   // High-load: 50ms
	} else if channelLoad > 0.5 {
		return 200 * time.Millisecond  // Medium-load: 200ms
	} else {
		return 1 * time.Second         // Low-load: 1s
	}
}

// drainCircularBuffer periodically moves events from circular buffer to main channel
func (eb *EventBridge) drainCircularBuffer(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond) // Check every 100ms
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Try to drain some events from circular buffer
			if eb.circularBuffer.Size() > 0 {
				events := eb.circularBuffer.PopBatch(50) // Drain 50 events at a time
				for _, event := range events {
					select {
					case eb.agentEvents <- event:
						// Successfully moved to main channel
					default:
						// Main channel still full, put back to circular buffer
						eb.circularBuffer.Push(event)
						break // Stop trying for now
					}
				}
			}
		}
	}
}

// flushBatch sends batched events or summarizes them if channel is still full
func (eb *EventBridge) flushBatch() {
	if len(eb.batchBuffer) == 0 {
		return
	}

	// Try to send individual events first
	sent := 0
	for i, event := range eb.batchBuffer {
		select {
		case eb.agentEvents <- event:
			sent++
		default:
			// Channel still full, create summary event for remaining
			if i < len(eb.batchBuffer) {
				eb.createSummaryEvent(eb.batchBuffer[i:])
			}
			break
		}
	}

	// Update dropped events counter if any were lost
	dropped := len(eb.batchBuffer) - sent
	if dropped > 0 {
		atomic.AddInt64(&eb.droppedEvents, int64(dropped))
		log.Printf("Batched %d events, dropped %d due to full channel (total dropped: %d)", 
			sent, dropped, atomic.LoadInt64(&eb.droppedEvents))
	}

	// Clear buffer and reset timer
	eb.batchBuffer = eb.batchBuffer[:0]
	if eb.batchTimer != nil {
		eb.batchTimer.Stop()
		eb.batchTimer = nil
	}
}

// createSummaryEvent creates a single summary event for multiple file operations
func (eb *EventBridge) createSummaryEvent(events []types.AgentEvent) {
	if len(events) == 0 {
		return
	}

	// Count events by type
	eventCounts := make(map[string]int)
	totalFiles := 0
	
	for _, event := range events {
		eventCounts[event.Type]++
		totalFiles++
	}

	summaryData := map[string]interface{}{
		"summary_type":     "batched_file_operations",
		"total_files":      totalFiles,
		"event_breakdown":  eventCounts,
		"dropped_count":    len(events),
		"timestamp":        time.Now(),
	}

	summaryEvent := types.AgentEvent{
		Type:      "file_operations_summary",
		Timestamp: time.Now(),
		Data:      summaryData,
	}

	// Force send summary (blocking if necessary to prevent complete data loss)
	select {
	case eb.agentEvents <- summaryEvent:
	case <-time.After(5 * time.Second):
		log.Printf("Failed to send summary event after 5s timeout")
	}
}

// Helper functions to safely extract data from event data maps

func getStringFromData(data map[string]interface{}, key string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getInt64FromData(data map[string]interface{}, key string) int64 {
	if val, ok := data[key]; ok {
		switch v := val.(type) {
		case int64:
			return v
		case int:
			return int64(v)
		case float64:
			return int64(v)
		}
	}
	return 0
}

func getFloat64FromData(data map[string]interface{}, key string) float64 {
	if val, ok := data[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return 0.0
}

func getMapFromData(data map[string]interface{}, key string) map[string]interface{} {
	if val, ok := data[key]; ok {
		if m, ok := val.(map[string]interface{}); ok {
			return m
		}
	}
	return make(map[string]interface{})
}

func getSliceFromData(data map[string]interface{}, key string) []interface{} {
	if val, ok := data[key]; ok {
		if slice, ok := val.([]interface{}); ok {
			return slice
		}
	}
	return []interface{}{}
}

// getMapKeys returns all keys in a map for debugging
func getMapKeys(data map[string]interface{}) []string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	return keys
}

// handleDeviceDiscovered handles device discovery events
func (eb *EventBridge) handleDeviceDiscovered(event embedded.Event) {
	data := event.Data
	
	deviceEvent := map[string]interface{}{
		"type":       "device_discovered",
		"device_id":  getStringFromData(data, "device"),
		"address":    getStringFromData(data, "address"),
		"timestamp":  event.Time,
	}
	
	eb.sendAgentEvent("device_discovered", deviceEvent)
	log.Printf("Device discovered: %s at %s", getStringFromData(data, "device"), getStringFromData(data, "address"))
}

// handleDeviceRejected handles device rejection events
func (eb *EventBridge) handleDeviceRejected(event embedded.Event) {
	data := event.Data
	
	deviceEvent := map[string]interface{}{
		"type":       "device_rejected",
		"device_id":  getStringFromData(data, "device"),
		"reason":     getStringFromData(data, "reason"),
		"timestamp":  event.Time,
	}
	
	eb.sendAgentEvent("device_rejected", deviceEvent)
	log.Printf("Device rejected: %s (reason: %s)", getStringFromData(data, "device"), getStringFromData(data, "reason"))
}

// handleLocalChangeDetected handles local file change events
func (eb *EventBridge) handleLocalChangeDetected(event embedded.Event) {
	data := event.Data
	
	changeEvent := map[string]interface{}{
		"type":        "local_change_detected",
		"folder_id":   getStringFromData(data, "folderID"),
		"path":        getStringFromData(data, "path"),
		"action":      getStringFromData(data, "action"), // added, modified, deleted
		"size":        getInt64FromData(data, "size"),
		"timestamp":   event.Time,
	}
	
	eb.sendAgentEvent("local_change_detected", changeEvent)
}

// handleRemoteChangeDetected handles remote file change events  
func (eb *EventBridge) handleRemoteChangeDetected(event embedded.Event) {
	data := event.Data
	
	changeEvent := map[string]interface{}{
		"type":        "remote_change_detected",
		"folder_id":   getStringFromData(data, "folderID"),
		"path":        getStringFromData(data, "path"),
		"device_id":   getStringFromData(data, "device"),
		"action":      getStringFromData(data, "action"),
		"size":        getInt64FromData(data, "size"),
		"timestamp":   event.Time,
	}
	
	eb.sendAgentEvent("remote_change_detected", changeEvent)
}

// handleFolderScanProgress handles folder scan progress events
func (eb *EventBridge) handleFolderScanProgress(event embedded.Event) {
	data := event.Data
	
	log.Printf("DEBUG Bridge: handleFolderScanProgress called with data: %+v", data)
	
	// Calculate progress percentage from current and total
	current := getInt64FromData(data, "current")
	total := getInt64FromData(data, "total")
	progress := float64(0)
	if total > 0 {
		progress = float64(current) * 100.0 / float64(total)
	}
	
	log.Printf("DEBUG Bridge: Calculated progress: %.1f%% (%d/%d)", progress, current, total)
	
	scanEvent := map[string]interface{}{
		"type":       "folder_scan_progress",
		"folder_id":  getStringFromData(data, "folder"),
		"progress":   progress,
		"current":    current,
		"total":      total,
		"timestamp":  event.Time,
	}
	
	log.Printf("DEBUG Bridge: About to send agent event: %+v", scanEvent)
	eb.sendAgentEvent("folder_scan_progress", scanEvent)
	log.Printf("📊 Bridge: Folder scan progress for %s: %.1f%% (%d/%d)", 
		getStringFromData(data, "folder"), progress, current, total)
}

// handleFolderRejected handles folder rejection events
func (eb *EventBridge) handleFolderRejected(event embedded.Event) {
	data := event.Data
	
	rejectEvent := map[string]interface{}{
		"type":       "folder_rejected",
		"folder_id":  getStringFromData(data, "folderID"),
		"device_id":  getStringFromData(data, "device"),
		"reason":     getStringFromData(data, "reason"),
		"timestamp":  event.Time,
	}
	
	eb.sendAgentEvent("folder_rejected", rejectEvent)
	log.Printf("Folder rejected: %s by %s (reason: %s)", 
		getStringFromData(data, "folderID"), 
		getStringFromData(data, "device"), 
		getStringFromData(data, "reason"))
}

// handleConfigSaved handles configuration save events
func (eb *EventBridge) handleConfigSaved(event embedded.Event) {
	configEvent := map[string]interface{}{
		"type":      "config_saved",
		"timestamp": event.Time,
	}
	
	eb.sendAgentEvent("config_saved", configEvent)
	log.Println("Configuration saved")
}

// handleRemoteDownloadProgress handles remote download progress events
func (eb *EventBridge) handleRemoteDownloadProgress(event embedded.Event) {
	data := event.Data
	
	progressEvent := map[string]interface{}{
		"type":         "remote_download_progress",
		"device_id":    getStringFromData(data, "device"),
		"folder_id":    getStringFromData(data, "folder"),
		"file_name":    getStringFromData(data, "item"),
		"bytes_done":   getInt64FromData(data, "bytesDone"),
		"bytes_total":  getInt64FromData(data, "bytesTotal"),
		"timestamp":    event.Time,
	}
	
	eb.sendAgentEvent("remote_download_progress", progressEvent)
}

// handleLocalIndexUpdated handles local index update events
func (eb *EventBridge) handleLocalIndexUpdated(event embedded.Event) {
	data := event.Data
	
	indexEvent := map[string]interface{}{
		"type":       "local_index_updated",
		"folder_id":  getStringFromData(data, "folder"),
		"items":      getInt64FromData(data, "items"),
		"size":       getInt64FromData(data, "size"),
		"version":    getInt64FromData(data, "version"),
		"timestamp":  event.Time,
	}
	
	eb.sendAgentEvent("local_index_updated", indexEvent)
}

// handleRemoteIndexUpdated handles remote index update events
func (eb *EventBridge) handleRemoteIndexUpdated(event embedded.Event) {
	data := event.Data
	
	indexEvent := map[string]interface{}{
		"type":       "remote_index_updated",
		"device_id":  getStringFromData(data, "device"),
		"folder_id":  getStringFromData(data, "folder"),
		"items":      getInt64FromData(data, "items"),
		"size":       getInt64FromData(data, "size"),
		"version":    getInt64FromData(data, "version"),
		"timestamp":  event.Time,
	}
	
	eb.sendAgentEvent("remote_index_updated", indexEvent)
}

// extractProgressData extracts BytesTotal and BytesDone from Syncthing progress data
func (eb *EventBridge) extractProgressData(progressData interface{}) (bytesTotal int64, bytesDone int64) {
	// Handle different types of progress data
	switch data := progressData.(type) {
	case map[string]interface{}:
		// Direct map access
		if total, ok := data["bytesTotal"].(int64); ok {
			bytesTotal = total
		}
		if done, ok := data["bytesDone"].(int64); ok {
			bytesDone = done
		}
		log.Printf("DEBUG Bridge: Map extraction - bytesTotal=%d, bytesDone=%d", bytesTotal, bytesDone)
		
	default:
		// Try pointer extraction using reflection (from existing code in syncthing_real.go)
		bytesTotal, bytesDone = eb.extractProgressFromPointer(progressData)
		if bytesTotal > 0 || bytesDone > 0 {
			log.Printf("DEBUG Bridge: Pointer extraction - bytesTotal=%d, bytesDone=%d", bytesTotal, bytesDone)
		}
	}
	
	return bytesTotal, bytesDone
}

// extractProgressFromPointer uses reflection to extract progress from pointer (from syncthing_real.go)
func (eb *EventBridge) extractProgressFromPointer(progressData interface{}) (bytesTotal int64, bytesDone int64) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("DEBUG Bridge: Panic in extractProgressFromPointer: %v", r)
			bytesTotal = 0
			bytesDone = 0
		}
	}()
	
	if progressData == nil {
		return 0, 0
	}
	
	// Use reflection to inspect the pointer (same logic as syncthing_real.go)
	val := reflect.ValueOf(progressData)
	log.Printf("DEBUG Bridge: Reflection - Type: %v, Kind: %v, Value: %+v", val.Type(), val.Kind(), progressData)
	
	// Check if it's a pointer
	if val.Kind() == reflect.Ptr && !val.IsNil() {
		// Dereference the pointer
		elem := val.Elem()
		log.Printf("DEBUG Bridge: Dereferenced - Type: %v, Kind: %v", elem.Type(), elem.Kind())
		
		// Check if it's a struct
		if elem.Kind() == reflect.Struct {
			// Look for BytesTotal and BytesDone fields
			bytesTotalField := elem.FieldByName("BytesTotal")
			bytesDoneField := elem.FieldByName("BytesDone")
			
			log.Printf("DEBUG Bridge: Fields - BytesTotal valid: %v, BytesDone valid: %v", 
				bytesTotalField.IsValid(), bytesDoneField.IsValid())
			
			if bytesTotalField.IsValid() && bytesTotalField.CanInterface() {
				if totalVal, ok := bytesTotalField.Interface().(int64); ok {
					bytesTotal = totalVal
					log.Printf("DEBUG Bridge: Extracted BytesTotal: %d", bytesTotal)
				} else {
					log.Printf("DEBUG Bridge: BytesTotal not int64: %T = %v", bytesTotalField.Interface(), bytesTotalField.Interface())
				}
			}
			
			if bytesDoneField.IsValid() && bytesDoneField.CanInterface() {
				if doneVal, ok := bytesDoneField.Interface().(int64); ok {
					bytesDone = doneVal
					log.Printf("DEBUG Bridge: Extracted BytesDone: %d", bytesDone)
				} else {
					log.Printf("DEBUG Bridge: BytesDone not int64: %T = %v", bytesDoneField.Interface(), bytesDoneField.Interface())
				}
			}
		}
	}

	return bytesTotal, bytesDone
}

// getFullFileSize retrieves the full file size from Syncthing's global database
func (eb *EventBridge) getFullFileSize(folderID, fileName string) int64 {
	// Use embedded Syncthing's direct model access (much faster than HTTP API)
	fileSize, err := eb.syncthing.GetFileInfo(folderID, fileName)
	if err != nil {
		log.Printf("⚠️  Failed to get file info from model: %v", err)
		return 0
	}

	log.Printf("🔍 Retrieved file size from model: %s/%s = %d bytes", folderID, fileName, fileSize)
	return fileSize
}