package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// EventStore interface for different storage implementations
type EventStore interface {
	Store(event *Event) error
	GetEvents(agentID string, since int64, limit int) ([]*Event, error)
	GetEventsByType(agentID string, eventType string, limit int) ([]*Event, error)
	GetEventsInTimeRange(agentID string, from, to time.Time) ([]*Event, error)
	GetEventStats(agentID string) (*EventStats, error)
	Close() error
}

// Event represents a stored event with ID for querying
type Event struct {
	ID        int64           `json:"id"`
	AgentID   string          `json:"agent_id"`
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
	Processed bool            `json:"processed"`
	CreatedAt time.Time       `json:"created_at"`
}

// EventStats provides analytics on events
type EventStats struct {
	TotalEvents      int64                  `json:"total_events"`
	EventsByType     map[string]int64       `json:"events_by_type"`
	EventsPerHour    []int64                `json:"events_per_hour"`
	LastEventTime    time.Time              `json:"last_event_time"`
	ProcessingRate   float64                `json:"processing_rate"`
}

// MemoryEventStore implements in-memory circular buffer storage
type MemoryEventStore struct {
	events    []*Event
	size      int
	next      int
	lastID    int64
	mutex     sync.RWMutex
	indexByID map[int64]int
}

// NewMemoryEventStore creates a new in-memory event store
func NewMemoryEventStore(bufferSize int) *MemoryEventStore {
	return &MemoryEventStore{
		events:    make([]*Event, bufferSize),
		size:      bufferSize,
		indexByID: make(map[int64]int),
	}
}

// Store adds an event to the circular buffer
func (m *MemoryEventStore) Store(event *Event) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.lastID++
	event.ID = m.lastID
	event.CreatedAt = time.Now()

	// Remove old event from index if buffer is full
	if oldEvent := m.events[m.next]; oldEvent != nil {
		delete(m.indexByID, oldEvent.ID)
	}

	// Store new event
	m.events[m.next] = event
	m.indexByID[event.ID] = m.next
	m.next = (m.next + 1) % m.size

	return nil
}

// GetEvents retrieves events since a given ID
func (m *MemoryEventStore) GetEvents(agentID string, since int64, limit int) ([]*Event, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var result []*Event
	count := 0

	// Iterate through buffer in reverse order (newest first)
	for i := 0; i < m.size && count < limit; i++ {
		idx := (m.next - 1 - i + m.size) % m.size
		event := m.events[idx]

		if event == nil {
			continue
		}

		if event.ID > since && (agentID == "" || event.AgentID == agentID) {
			result = append([]*Event{event}, result...)
			count++
		}
	}

	return result, nil
}

// GetEventsByType retrieves events by type
func (m *MemoryEventStore) GetEventsByType(agentID string, eventType string, limit int) ([]*Event, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var result []*Event
	count := 0

	for i := 0; i < m.size && count < limit; i++ {
		idx := (m.next - 1 - i + m.size) % m.size
		event := m.events[idx]

		if event == nil {
			continue
		}

		if event.Type == eventType && (agentID == "" || event.AgentID == agentID) {
			result = append(result, event)
			count++
		}
	}

	return result, nil
}

// GetEventsInTimeRange retrieves events within a time range
func (m *MemoryEventStore) GetEventsInTimeRange(agentID string, from, to time.Time) ([]*Event, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var result []*Event

	for i := 0; i < m.size; i++ {
		event := m.events[i]
		if event == nil {
			continue
		}

		if event.Timestamp.After(from) && event.Timestamp.Before(to) && 
		   (agentID == "" || event.AgentID == agentID) {
			result = append(result, event)
		}
	}

	return result, nil
}

// GetEventStats returns statistics about stored events
func (m *MemoryEventStore) GetEventStats(agentID string) (*EventStats, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stats := &EventStats{
		EventsByType:  make(map[string]int64),
		EventsPerHour: make([]int64, 24),
	}

	now := time.Now()
	var lastEventTime time.Time

	for i := 0; i < m.size; i++ {
		event := m.events[i]
		if event == nil {
			continue
		}

		if agentID == "" || event.AgentID == agentID {
			stats.TotalEvents++
			stats.EventsByType[event.Type]++

			// Track events per hour for last 24 hours
			hoursSince := int(now.Sub(event.Timestamp).Hours())
			if hoursSince < 24 {
				stats.EventsPerHour[23-hoursSince]++
			}

			if event.Timestamp.After(lastEventTime) {
				lastEventTime = event.Timestamp
			}
		}
	}

	stats.LastEventTime = lastEventTime
	if stats.TotalEvents > 0 {
		duration := now.Sub(lastEventTime).Seconds()
		if duration > 0 {
			stats.ProcessingRate = float64(stats.TotalEvents) / duration
		}
	}

	return stats, nil
}

// Close closes the event store
func (m *MemoryEventStore) Close() error {
	return nil
}

// EventProcessor handles incoming events from agents
type EventProcessor struct {
	store            EventStore
	db               *sql.DB
	syncStateManager *SyncStateManager
	mutex            sync.RWMutex
}

// NewEventProcessor creates a new event processor
func NewEventProcessor(store EventStore, db *sql.DB) *EventProcessor {
	ep := &EventProcessor{
		store: store,
		db:    db,
	}
	
	// Initialize SyncStateManager if database is available
	if db != nil {
		ep.syncStateManager = NewSyncStateManager(db)
		log.Printf("✅ SyncStateManager initialized for EventProcessor")
	}
	
	return ep
}

// ProcessEvent processes and stores an incoming event
func (p *EventProcessor) ProcessEvent(agentID string, eventData []byte) error {
	var rawEvent map[string]interface{}
	if err := json.Unmarshal(eventData, &rawEvent); err != nil {
		return err
	}

	eventType, _ := rawEvent["type"].(string)
	timestamp, _ := time.Parse(time.RFC3339, rawEvent["timestamp"].(string))

	event := &Event{
		AgentID:   agentID,
		Type:      eventType,
		Timestamp: timestamp,
		Data:      eventData,
		Processed: false,
	}

	// Store in memory
	if err := p.store.Store(event); err != nil {
		return err
	}

	// Also persist specific events to database if database is available
	if p.db != nil {
		if err := p.persistEventToDatabase(agentID, eventType, rawEvent, timestamp); err != nil {
			log.Printf("❌ Failed to persist event to database: %v", err)
			// Don't fail the entire operation if database write fails
		}
	}

	// Mark as processed after successful storage
	event.Processed = true

	return nil
}

// GetEvents retrieves events from the store
func (p *EventProcessor) GetEvents(agentID string, since int64, limit int) ([]*Event, error) {
	return p.store.GetEvents(agentID, since, limit)
}

// GetEventStats returns event statistics
func (p *EventProcessor) GetEventStats(agentID string) (*EventStats, error) {
	return p.store.GetEventStats(agentID)
}

// persistEventToDatabase persists specific event types to database tables
func (p *EventProcessor) persistEventToDatabase(agentID, eventType string, rawEvent map[string]interface{}, timestamp time.Time) error {
	// Handle nested event structure from agents
	var actualEventType string
	var eventData interface{}
	
	if eventType == "event" {
		// Extract nested event type and data
		if eventObj, ok := rawEvent["event"]; ok {
			if eventMap, ok := eventObj.(map[string]interface{}); ok {
				actualEventType, _ = eventMap["type"].(string)
				eventData = eventMap["data"]
			}
		}
	} else {
		actualEventType = eventType
		eventData = rawEvent["data"]
	}
	
	log.Printf("🔍 Processing event: type=%s, actualType=%s", eventType, actualEventType)
	
	switch actualEventType {
	case "file_transfer_started", "file_transfer_completed", "file_transfer_progress":
		return p.persistFileTransferEvent(agentID, actualEventType, eventData, timestamp)
	case "sync_status", "state_changed", "device_connected", "device_disconnected":
		return p.persistSyncEvent(agentID, actualEventType, eventData, timestamp)
	default:
		// For other events, just store in sync_events table
		return p.persistGeneralEvent(agentID, actualEventType, rawEvent, timestamp)
	}
}

// persistFileTransferEvent persists file transfer events using SyncStateManager
func (p *EventProcessor) persistFileTransferEvent(agentID, eventType string, eventData interface{}, timestamp time.Time) error {
	if eventData == nil {
		return nil // Skip if no data
	}

	// Use SyncStateManager if available for better consistency
	if p.syncStateManager != nil {
		return p.persistFileTransferEventWithStateManager(agentID, eventType, eventData, timestamp)
	}

	// Fallback to legacy method if SyncStateManager not available
	log.Printf("⚠️ SyncStateManager not available, using legacy persistence method")
	return p.legacyPersistFileTransferEvent(agentID, eventType, eventData, timestamp)
}

// persistFileTransferEventWithStateManager uses the new SyncStateManager for consistency
func (p *EventProcessor) persistFileTransferEventWithStateManager(agentID, eventType string, eventData interface{}, timestamp time.Time) error {
	// Parse event data into FileTransferEvent struct
	event, err := p.parseFileTransferEvent(agentID, eventType, eventData, timestamp)
	if err != nil {
		log.Printf("❌ Failed to parse file transfer event: %v", err)
		return err
	}

	// Process event through SyncStateManager
	if err := p.syncStateManager.ProcessFileTransferEvent(event); err != nil {
		log.Printf("❌ SyncStateManager failed to process event: %v", err)
		// Don't fail completely, log the error and continue
		return p.legacyPersistFileTransferEvent(agentID, eventType, eventData, timestamp)
	}

	log.Printf("✅ File transfer event processed through SyncStateManager: %s/%s - %s", 
		event.JobID, event.FileName, eventType)
	return nil
}

// parseFileTransferEvent converts raw event data to structured FileTransferEvent
func (p *EventProcessor) parseFileTransferEvent(agentID, eventType string, eventData interface{}, timestamp time.Time) (*FileTransferEvent, error) {
	event := &FileTransferEvent{
		AgentID:   agentID,
		EventType: eventType,
		Timestamp: timestamp,
	}

	// Parse data from the event
	if dataMap, ok := eventData.(map[string]interface{}); ok {
		// Store raw data for debugging
		event.RawData = dataMap

		// Extract job_id
		event.JobID, _ = dataMap["job_id"].(string)
		if event.JobID == "" {
			event.JobID, _ = dataMap["JobID"].(string)
		}

		// Extract file_name
		event.FileName, _ = dataMap["file_name"].(string)
		if event.FileName == "" {
			event.FileName, _ = dataMap["FileName"].(string)
		}

		// Extract status
		event.Status, _ = dataMap["status"].(string)
		if event.Status == "" {
			event.Status, _ = dataMap["Status"].(string)
		}

		// Extract action
		event.Action, _ = dataMap["action"].(string)
		if event.Action == "" {
			event.Action, _ = dataMap["Action"].(string)
		}

		// Extract error message
		event.ErrorMsg, _ = dataMap["error"].(string)
		if event.ErrorMsg == "" {
			event.ErrorMsg, _ = dataMap["Error"].(string)
		}

		// Extract file_path
		event.FilePath, _ = dataMap["file_path"].(string)
		if event.FilePath == "" {
			event.FilePath, _ = dataMap["FilePath"].(string)
		}

		// Extract numeric values with type conversion
		if size, ok := dataMap["file_size"]; ok {
			if sizeFloat, ok := size.(float64); ok {
				event.FileSize = int64(sizeFloat)
			}
		} else if size, ok := dataMap["FileSize"]; ok {
			if sizeFloat, ok := size.(float64); ok {
				event.FileSize = int64(sizeFloat)
			}
		}

		if prog, ok := dataMap["progress"]; ok {
			event.Progress, _ = prog.(float64)
		} else if prog, ok := dataMap["Progress"]; ok {
			event.Progress, _ = prog.(float64)
		}

		if rate, ok := dataMap["transfer_rate"]; ok {
			event.TransferRate, _ = rate.(float64)
		} else if rate, ok := dataMap["TransferRate"]; ok {
			event.TransferRate, _ = rate.(float64)
		}

		if dur, ok := dataMap["duration"]; ok {
			event.Duration, _ = dur.(float64)
		} else if dur, ok := dataMap["Duration"]; ok {
			event.Duration, _ = dur.(float64)
		}
	}

	// Resolve job name from job_id if available
	if event.JobID != "" {
		event.JobName = p.getJobNameFromJobID(event.JobID)
	}

	// Set default status based on event type if not provided
	if event.Status == "" {
		switch eventType {
		case "file_transfer_started":
			event.Status = "started"
		case "file_transfer_completed":
			if event.ErrorMsg != "" {
				event.Status = "failed"
			} else {
				event.Status = "completed"
			}
		case "file_transfer_progress":
			event.Status = "in_progress"
		}
	}

	// Validate required fields
	if event.JobID == "" || event.FileName == "" {
		return nil, fmt.Errorf("missing required fields: job_id=%s, file_name=%s", event.JobID, event.FileName)
	}

	return event, nil
}

// legacyPersistFileTransferEvent is the original implementation as fallback
func (p *EventProcessor) legacyPersistFileTransferEvent(agentID, eventType string, eventData interface{}, timestamp time.Time) error {
	// Original implementation code (shortened for brevity - keeping the core logic)
	if eventData == nil {
		return nil
	}

	var jobID, jobName, fileName, status, action, errorMessage string
	var fileSize int64
	var progress, transferRate, duration float64
	var startedAt, completedAt *time.Time

	if dataMap, ok := eventData.(map[string]interface{}); ok {
		jobID, _ = dataMap["job_id"].(string)
		fileName, _ = dataMap["file_name"].(string)
		status, _ = dataMap["status"].(string)
		action, _ = dataMap["action"].(string)
		errorMessage, _ = dataMap["error"].(string)

		if size, ok := dataMap["file_size"]; ok {
			if sizeFloat, ok := size.(float64); ok {
				fileSize = int64(sizeFloat)
			}
		}
		if prog, ok := dataMap["progress"]; ok {
			progress, _ = prog.(float64)
		}
		if rate, ok := dataMap["transfer_rate"]; ok {
			transferRate, _ = rate.(float64)
		}
		if dur, ok := dataMap["duration"]; ok {
			duration, _ = dur.(float64)
		}
	}

	if eventType == "file_transfer_started" {
		startedAt = &timestamp
	} else if eventType == "file_transfer_completed" {
		completedAt = &timestamp
	}

	if jobID != "" {
		row := p.db.QueryRow("SELECT name FROM sync_jobs WHERE id = $1", jobID)
		row.Scan(&jobName)
	}

	// Legacy UPDATE then INSERT pattern (with race condition risk)
	if eventType == "file_transfer_completed" && jobID != "" && fileName != "" {
		result, err := p.db.Exec(`
			UPDATE file_transfer_logs 
			SET status = $1, action = $2, file_size = $3, duration = $4, 
				error_message = $5, completed_at = $6, progress = $7, version = version + 1
			WHERE job_id = $8 AND file_name = $9 AND agent_id = $10 
				AND status IN ('started', 'in_progress') AND completed_at IS NULL
		`, status, action, fileSize, duration, errorMessage, completedAt, progress, 
		   jobID, fileName, agentID)
		
		if err == nil {
			rowsAffected, _ := result.RowsAffected()
			if rowsAffected > 0 {
				log.Printf("✅ Legacy: Updated existing file transfer record: %s - %s", fileName, status)
				return nil
			}
		}
	}

	// Insert new record
	_, err := p.db.Exec(`
		INSERT INTO file_transfer_logs (
			job_id, job_name, agent_id, file_name, file_size, 
			status, action, progress, transfer_rate, duration, error_message, 
			started_at, completed_at, version, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, 1, NOW())
	`, jobID, jobName, agentID, fileName, fileSize, status, action, progress, transferRate, duration, errorMessage, startedAt, completedAt)

	if err != nil {
		log.Printf("❌ Legacy: Failed to insert file transfer log: %v", err)
		return err
	}

	log.Printf("✅ Legacy: File transfer event persisted: %s - %s", eventType, fileName)
	return nil
}

// persistSyncEvent persists sync-related events to sync_events table
func (p *EventProcessor) persistSyncEvent(agentID, eventType string, eventData interface{}, timestamp time.Time) error {
	if eventData == nil {
		return nil // Skip if no data
	}

	// Convert data to JSON for storage
	dataJSON, err := json.Marshal(eventData)
	if err != nil {
		return err
	}

	var jobID, folderID, deviceID string
	if dataMap, ok := eventData.(map[string]interface{}); ok {
		jobID, _ = dataMap["job_id"].(string)
		folderID, _ = dataMap["folder_id"].(string) 
		if folderID == "" {
			folderID, _ = dataMap["folder"].(string)
		}
		deviceID, _ = dataMap["device_id"].(string)
		if deviceID == "" {
			deviceID, _ = dataMap["device"].(string)
		}
	}

	// Insert into sync_events table
	_, err = p.db.Exec(`
		INSERT INTO sync_events (
			agent_id, job_id, event_type, folder_id, device_id, 
			event_data, timestamp, processed, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, false, NOW())
	`, agentID, jobID, eventType, folderID, deviceID, string(dataJSON), timestamp)

	if err != nil {
		log.Printf("❌ Failed to insert sync event: %v", err)
		return err
	}

	log.Printf("✅ Sync event persisted: %s - %s", eventType, folderID)
	return nil
}

// getJobNameFromJobID resolves job name from job_id (e.g., "job-14" -> "Two Way")
func (p *EventProcessor) getJobNameFromJobID(jobID string) string {
	// Extract numeric ID from job_id (e.g., "job-14" -> 14)
	if len(jobID) > 4 && jobID[:4] == "job-" {
		numericID := jobID[4:] // Remove "job-" prefix
		
		// Query the sync_jobs table to get the job name
		var jobName string
		err := p.db.QueryRow("SELECT name FROM sync_jobs WHERE id = $1", numericID).Scan(&jobName)
		if err == nil {
			log.Printf("🔗 Resolved job name: %s -> %s", jobID, jobName)
			return jobName
		} else {
			log.Printf("⚠️  Failed to resolve job name for %s: %v", jobID, err)
		}
	}
	
	// Return original jobID if resolution fails
	return jobID
}

// persistGeneralEvent persists general events to sync_events table
func (p *EventProcessor) persistGeneralEvent(agentID, eventType string, rawEvent map[string]interface{}, timestamp time.Time) error {
	// Convert entire event to JSON for storage
	dataJSON, err := json.Marshal(rawEvent)
	if err != nil {
		return err
	}

	// Insert into sync_events table
	_, err = p.db.Exec(`
		INSERT INTO sync_events (
			agent_id, job_id, event_type, folder_id, device_id, 
			event_data, timestamp, processed, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, false, NOW())
	`, agentID, "", eventType, "", "", string(dataJSON), timestamp)

	if err != nil {
		log.Printf("❌ Failed to insert general event: %v", err)
		return err
	}

	return nil
}

// Stop stops the EventProcessor and cleans up resources
func (p *EventProcessor) Stop() {
	if p.syncStateManager != nil {
		p.syncStateManager.Stop()
		log.Printf("✅ EventProcessor: SyncStateManager stopped")
	}
}