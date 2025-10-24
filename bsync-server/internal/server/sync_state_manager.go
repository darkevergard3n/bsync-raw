package server

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"
	
	"github.com/lib/pq"
)

// SyncStateManager manages file transfer states and ensures consistency
type SyncStateManager struct {
	db *sql.DB
	
	// Event deduplication
	processedEvents map[string]time.Time // event_hash -> timestamp
	eventsMutex     sync.RWMutex
	
	// Active file transfers tracking
	activeTransfers map[string]*FileTransferState // composite_key -> state
	transfersMutex  sync.RWMutex
	
	// Configuration
	deduplicationWindow time.Duration
	stateTimeout        time.Duration
	cleanupInterval     time.Duration
	
	// Background cleanup
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	
	// Statistics
	processedCount    int64
	deduplicatedCount int64
	conflictCount     int64
	statsMutex        sync.RWMutex
}

// FileTransferState represents the current state of a file transfer
type FileTransferState struct {
	JobID       string    `json:"job_id"`
	JobName     string    `json:"job_name"`
	AgentID     string    `json:"agent_id"`
	FileName    string    `json:"file_name"`
	FilePath    string    `json:"file_path"`
	FileSize    int64     `json:"file_size"`
	Status      string    `json:"status"`      // started, in_progress, completed, failed
	Action      string    `json:"action"`      // update, delete, metadata
	Progress    float64   `json:"progress"`    // 0.0 - 100.0
	TransferRate float64  `json:"transfer_rate"`
	Duration    float64   `json:"duration"`
	ErrorMsg    string    `json:"error_message,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Version     int64     `json:"version"`     // For optimistic locking
	LastEventHash string  `json:"last_event_hash"`
}

// FileTransferEvent represents an incoming file transfer event
type FileTransferEvent struct {
	JobID        string                 `json:"job_id"`
	JobName      string                 `json:"job_name"`
	AgentID      string                 `json:"agent_id"`
	FileName     string                 `json:"file_name"`
	FilePath     string                 `json:"file_path"`
	FileSize     int64                  `json:"file_size"`
	Status       string                 `json:"status"`
	Action       string                 `json:"action"`
	Progress     float64                `json:"progress"`
	TransferRate float64                `json:"transfer_rate"`
	Duration     float64                `json:"duration"`
	ErrorMsg     string                 `json:"error_message,omitempty"`
	Timestamp    time.Time              `json:"timestamp"`
	EventType    string                 `json:"event_type"`
	RawData      map[string]interface{} `json:"raw_data,omitempty"`
}

// NewSyncStateManager creates a new sync state manager
func NewSyncStateManager(db *sql.DB) *SyncStateManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	ssm := &SyncStateManager{
		db:                  db,
		processedEvents:     make(map[string]time.Time),
		activeTransfers:     make(map[string]*FileTransferState),
		deduplicationWindow: 30 * time.Second,  // Events within 30s are considered duplicates
		stateTimeout:        10 * time.Minute,  // Clean up inactive transfers after 10 minutes
		cleanupInterval:     2 * time.Minute,   // Run cleanup every 2 minutes
		ctx:                 ctx,
		cancel:              cancel,
	}
	
	// Start background cleanup
	ssm.wg.Add(1)
	go ssm.cleanupWorker()
	
	log.Printf("🔄 SyncStateManager initialized with %v deduplication window", ssm.deduplicationWindow)
	return ssm
}

// Stop stops the sync state manager
func (ssm *SyncStateManager) Stop() {
	ssm.cancel()
	ssm.wg.Wait()
	log.Printf("🔄 SyncStateManager stopped")
}

// ProcessFileTransferEvent processes a file transfer event with deduplication and consistency checks
func (ssm *SyncStateManager) ProcessFileTransferEvent(event *FileTransferEvent) error {
	// Generate event hash for deduplication
	eventHash := ssm.generateEventHash(event)
	
	// Check for duplicate event
	if ssm.isDuplicateEvent(eventHash) {
		ssm.recordDeduplicated()
		log.Printf("🔄 Duplicate event ignored: %s/%s (hash: %s)", event.JobID, event.FileName, eventHash[:8])
		return nil
	}
	
	// Mark event as processed
	ssm.markEventProcessed(eventHash)
	
	// Get composite key for this transfer
	transferKey := ssm.getTransferKey(event.JobID, event.FileName, event.AgentID)
	
	// Process event with transaction
	return ssm.processEventWithTransaction(transferKey, event, eventHash)
}

// processEventWithTransaction processes event within a database transaction
func (ssm *SyncStateManager) processEventWithTransaction(transferKey string, event *FileTransferEvent, eventHash string) error {
	// Start transaction
	tx, err := ssm.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()
	
	// Get or create transfer state
	currentState, err := ssm.getTransferState(tx, transferKey, event)
	if err != nil {
		return fmt.Errorf("failed to get transfer state: %w", err)
	}
	
	// Apply event to state with validation
	newState, err := ssm.applyEventToState(currentState, event, eventHash)
	if err != nil {
		ssm.recordConflict()
		return fmt.Errorf("failed to apply event to state: %w", err)
	}
	
	// Persist state to database
	if err := ssm.persistTransferState(tx, transferKey, newState); err != nil {
		return fmt.Errorf("failed to persist transfer state: %w", err)
	}
	
	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	
	// Update in-memory state
	ssm.updateMemoryState(transferKey, newState)
	
	ssm.recordProcessed()
	log.Printf("✅ Processed file transfer event: %s/%s (state: %s->%s, version: %d)", 
		event.JobID, event.FileName, currentState.Status, newState.Status, newState.Version)
	
	return nil
}

// getTransferState retrieves current state from database or memory
func (ssm *SyncStateManager) getTransferState(tx *sql.Tx, transferKey string, event *FileTransferEvent) (*FileTransferState, error) {
	// Try to get from database first (source of truth)
	var state FileTransferState
	var startedAt, updatedAt time.Time
	var completedAt pq.NullTime
	
	query := `
		SELECT job_id, job_name, agent_id, file_name, file_path, file_size, 
			   status, action, progress, transfer_rate, duration, error_message,
			   started_at, updated_at, completed_at, version, last_event_hash
		FROM file_transfer_logs 
		WHERE job_id = $1 AND file_name = $2 AND agent_id = $3
		ORDER BY version DESC LIMIT 1
	`
	
	err := tx.QueryRow(query, event.JobID, event.FileName, event.AgentID).Scan(
		&state.JobID, &state.JobName, &state.AgentID, &state.FileName, &state.FilePath,
		&state.FileSize, &state.Status, &state.Action, &state.Progress, &state.TransferRate,
		&state.Duration, &state.ErrorMsg, &startedAt, &updatedAt, &completedAt,
		&state.Version, &state.LastEventHash,
	)
	
	if err != nil {
		if err == sql.ErrNoRows {
			// Create new state
			return &FileTransferState{
				JobID:         event.JobID,
				JobName:       event.JobName,
				AgentID:       event.AgentID,
				FileName:      event.FileName,
				FilePath:      event.FilePath,
				Status:        "pending",
				StartedAt:     event.Timestamp,
				UpdatedAt:     event.Timestamp,
				Version:       0,
				LastEventHash: "", // Initialize as empty string for safety
			}, nil
		}
		return nil, err
	}
	
	state.StartedAt = startedAt
	state.UpdatedAt = updatedAt
	if completedAt.Valid {
		state.CompletedAt = &completedAt.Time
	}
	
	return &state, nil
}

// applyEventToState applies an event to current state with validation
func (ssm *SyncStateManager) applyEventToState(currentState *FileTransferState, event *FileTransferEvent, eventHash string) (*FileTransferState, error) {
	// Create new state based on current
	newState := *currentState
	newState.UpdatedAt = event.Timestamp
	newState.Version++
	newState.LastEventHash = eventHash
	
	// Validate state transitions
	if !ssm.isValidStateTransition(currentState.Status, event.EventType, event.Status) {
		// Safe handling of LastEventHash for error message
		currentEventHashDisplay := "new"
		if len(currentState.LastEventHash) >= 8 {
			currentEventHashDisplay = currentState.LastEventHash[:8]
		}
		newEventHashDisplay := "unknown"
		if len(eventHash) >= 8 {
			newEventHashDisplay = eventHash[:8]
		}
		return nil, fmt.Errorf("invalid state transition: %s (%s) -> %s (%s)", 
			currentState.Status, currentEventHashDisplay, event.Status, newEventHashDisplay)
	}
	
	// Apply event based on type
	switch event.EventType {
	case "file_transfer_started":
		newState.Status = "started"
		newState.Action = event.Action
		newState.Progress = 0.0
		newState.StartedAt = event.Timestamp
		if event.FileSize > 0 {
			newState.FileSize = event.FileSize
		}
		
	case "file_transfer_progress":
		// Only update progress if in valid state
		if currentState.Status == "started" || currentState.Status == "in_progress" {
			newState.Status = "in_progress"
			newState.Progress = event.Progress
			newState.TransferRate = event.TransferRate
			if event.FileSize > 0 && event.FileSize > currentState.FileSize {
				newState.FileSize = event.FileSize
			}
		}
		
	case "file_transfer_completed":
		// Only complete if in valid state
		if currentState.Status != "completed" && currentState.Status != "failed" {
			newState.Status = event.Status // "completed" or "failed"
			newState.Progress = 100.0
			newState.Duration = event.Duration
			newState.ErrorMsg = event.ErrorMsg
			completedAt := event.Timestamp
			newState.CompletedAt = &completedAt
			if event.FileSize > 0 {
				newState.FileSize = event.FileSize
			}
		} else {
			return nil, fmt.Errorf("transfer already completed: current=%s, event=%s", 
				currentState.Status, event.Status)
		}
		
	default:
		return nil, fmt.Errorf("unknown event type: %s", event.EventType)
	}
	
	// Update job name and file path if provided
	if event.JobName != "" {
		newState.JobName = event.JobName
	}
	if event.FilePath != "" {
		newState.FilePath = event.FilePath
	}
	
	return &newState, nil
}

// isValidStateTransition validates if a state transition is allowed
func (ssm *SyncStateManager) isValidStateTransition(currentStatus, eventType, newStatus string) bool {
	// Define valid state transitions
	validTransitions := map[string]map[string][]string{
		"pending": {
			"file_transfer_started": {"started"},
		},
		"started": {
			"file_transfer_progress":  {"in_progress"},
			"file_transfer_completed": {"completed", "failed"},
		},
		"in_progress": {
			"file_transfer_progress":  {"in_progress"},
			"file_transfer_completed": {"completed", "failed"},
		},
		"completed": {
			// Completed transfers should not accept new events
		},
		"failed": {
			"file_transfer_started": {"started"}, // Allow retry
		},
	}
	
	if eventTransitions, exists := validTransitions[currentStatus]; exists {
		if allowedStatuses, exists := eventTransitions[eventType]; exists {
			for _, allowedStatus := range allowedStatuses {
				if newStatus == allowedStatus || (newStatus == "" && eventType == "file_transfer_progress") {
					return true
				}
			}
		}
	}
	
	return false
}

// persistTransferState persists the transfer state to database using UPSERT
func (ssm *SyncStateManager) persistTransferState(tx *sql.Tx, transferKey string, state *FileTransferState) error {
	query := `
		INSERT INTO file_transfer_logs (
			job_id, job_name, agent_id, file_name, file_path, file_size,
			status, action, progress, transfer_rate, duration, error_message,
			started_at, updated_at, completed_at, version, last_event_hash, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NOW())
		ON CONFLICT (job_id, file_name, agent_id, started_at) 
		DO UPDATE SET
			job_name = EXCLUDED.job_name,
			file_path = EXCLUDED.file_path,
			file_size = EXCLUDED.file_size,
			status = EXCLUDED.status,
			action = EXCLUDED.action,
			progress = EXCLUDED.progress,
			transfer_rate = EXCLUDED.transfer_rate,
			duration = EXCLUDED.duration,
			error_message = EXCLUDED.error_message,
			updated_at = EXCLUDED.updated_at,
			completed_at = EXCLUDED.completed_at,
			version = EXCLUDED.version,
			last_event_hash = EXCLUDED.last_event_hash
		WHERE file_transfer_logs.version < EXCLUDED.version
	`
	
	_, err := tx.Exec(query,
		state.JobID, state.JobName, state.AgentID, state.FileName, state.FilePath, state.FileSize,
		state.Status, state.Action, state.Progress, state.TransferRate, state.Duration, state.ErrorMsg,
		state.StartedAt, state.UpdatedAt, state.CompletedAt, state.Version, state.LastEventHash,
	)
	
	return err
}

// Helper methods for event processing
func (ssm *SyncStateManager) generateEventHash(event *FileTransferEvent) string {
	// Create hash based on key fields to identify duplicate events
	hashData := fmt.Sprintf("%s|%s|%s|%s|%s|%.2f|%d", 
		event.JobID, event.FileName, event.AgentID, event.EventType, event.Status, 
		event.Progress, event.Timestamp.Unix())
	
	hash := md5.Sum([]byte(hashData))
	return hex.EncodeToString(hash[:])
}

func (ssm *SyncStateManager) isDuplicateEvent(eventHash string) bool {
	ssm.eventsMutex.RLock()
	defer ssm.eventsMutex.RUnlock()
	
	if lastSeen, exists := ssm.processedEvents[eventHash]; exists {
		// Check if within deduplication window
		return time.Since(lastSeen) < ssm.deduplicationWindow
	}
	
	return false
}

func (ssm *SyncStateManager) markEventProcessed(eventHash string) {
	ssm.eventsMutex.Lock()
	defer ssm.eventsMutex.Unlock()
	
	ssm.processedEvents[eventHash] = time.Now()
}

func (ssm *SyncStateManager) getTransferKey(jobID, fileName, agentID string) string {
	return fmt.Sprintf("%s:%s:%s", jobID, fileName, agentID)
}

func (ssm *SyncStateManager) updateMemoryState(transferKey string, state *FileTransferState) {
	ssm.transfersMutex.Lock()
	defer ssm.transfersMutex.Unlock()
	
	ssm.activeTransfers[transferKey] = state
}

// Background cleanup worker
func (ssm *SyncStateManager) cleanupWorker() {
	defer ssm.wg.Done()
	
	ticker := time.NewTicker(ssm.cleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ssm.ctx.Done():
			return
		case <-ticker.C:
			ssm.cleanup()
		}
	}
}

func (ssm *SyncStateManager) cleanup() {
	now := time.Now()
	
	// Clean up old processed events
	ssm.eventsMutex.Lock()
	for hash, timestamp := range ssm.processedEvents {
		if now.Sub(timestamp) > ssm.deduplicationWindow*2 {
			delete(ssm.processedEvents, hash)
		}
	}
	ssm.eventsMutex.Unlock()
	
	// Clean up inactive transfers
	ssm.transfersMutex.Lock()
	for key, state := range ssm.activeTransfers {
		if now.Sub(state.UpdatedAt) > ssm.stateTimeout {
			delete(ssm.activeTransfers, key)
		}
	}
	ssm.transfersMutex.Unlock()
}

// Statistics methods
func (ssm *SyncStateManager) recordProcessed() {
	ssm.statsMutex.Lock()
	ssm.processedCount++
	ssm.statsMutex.Unlock()
}

func (ssm *SyncStateManager) recordDeduplicated() {
	ssm.statsMutex.Lock()
	ssm.deduplicatedCount++
	ssm.statsMutex.Unlock()
}

func (ssm *SyncStateManager) recordConflict() {
	ssm.statsMutex.Lock()
	ssm.conflictCount++
	ssm.statsMutex.Unlock()
}

// GetStats returns processing statistics
func (ssm *SyncStateManager) GetStats() map[string]interface{} {
	ssm.statsMutex.RLock()
	ssm.eventsMutex.RLock()
	ssm.transfersMutex.RLock()
	
	stats := map[string]interface{}{
		"processed_events":     ssm.processedCount,
		"deduplicated_events":  ssm.deduplicatedCount,
		"conflict_events":      ssm.conflictCount,
		"active_transfers":     len(ssm.activeTransfers),
		"cached_events":        len(ssm.processedEvents),
	}
	
	if ssm.processedCount > 0 {
		stats["deduplication_rate"] = float64(ssm.deduplicatedCount) / float64(ssm.processedCount) * 100
		stats["conflict_rate"] = float64(ssm.conflictCount) / float64(ssm.processedCount) * 100
	}
	
	ssm.transfersMutex.RUnlock()
	ssm.eventsMutex.RUnlock()
	ssm.statsMutex.RUnlock()
	
	return stats
}