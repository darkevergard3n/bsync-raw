package server

import (
	"encoding/json"
	"log"
	"time"
)

// FolderProgressEvent represents real-time folder progress updates
type FolderProgressEvent struct {
	AgentID       string    `json:"agent_id"`
	FolderID      string    `json:"folder_id"`
	State         string    `json:"state"`
	ScanProgress  float64   `json:"scan_progress"`
	PullProgress  float64   `json:"pull_progress"`
	GlobalFiles   int64     `json:"global_files"`
	LocalFiles    int64     `json:"local_files"`
	NeedFiles     int64     `json:"need_files"`
	Error         string    `json:"error,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

// FolderStateChangeEvent represents folder state transitions
type FolderStateChangeEvent struct {
	AgentID   string    `json:"agent_id"`
	FolderID  string    `json:"folder_id"`
	State     string    `json:"state"`
	FromState string    `json:"from_state,omitempty"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// BroadcastFolderProgress broadcasts folder progress to all connected dashboard clients
func (s *SyncToolServer) BroadcastFolderProgress(event FolderProgressEvent) {
	if s.hub == nil {
		return
	}

	message := map[string]interface{}{
		"type": "folder_progress",
		"data": event,
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling folder progress event: %v", err)
		return
	}

	// Broadcast to dashboard clients (not agents)
	s.hub.Broadcast(Message{
		Type: "folder_progress",
		Data: messageBytes,
	})
	
	log.Printf("📊 Broadcasted folder progress: agent=%s, folder=%s, state=%s, scan=%.1f%%, pull=%.1f%%", 
		event.AgentID, event.FolderID, event.State, event.ScanProgress, event.PullProgress)
}

// BroadcastFolderStateChange broadcasts folder state changes to dashboard clients
func (s *SyncToolServer) BroadcastFolderStateChange(event FolderStateChangeEvent) {
	if s.hub == nil {
		return
	}

	message := map[string]interface{}{
		"type": "folder_state_change",
		"data": event,
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling folder state change event: %v", err)
		return
	}

	// Broadcast to dashboard clients
	s.hub.Broadcast(Message{
		Type: "folder_state_change",
		Data: messageBytes,
	})
	
	log.Printf("🔄 Broadcasted state change: agent=%s, folder=%s, %s → %s", 
		event.AgentID, event.FolderID, event.FromState, event.State)
}

// HandleAgentFolderEvent processes folder events from agents and broadcasts to dashboard
func (s *SyncToolServer) HandleAgentFolderEvent(agentID string, eventData map[string]interface{}) {
	eventType, ok := eventData["type"].(string)
	if !ok {
		return
	}

	switch eventType {
	case "folder_scan_progress":
		log.Printf("DEBUG Server: Received folder_scan_progress event from agent %s", agentID)
		s.handleFolderScanProgress(agentID, eventData)
	case "folder_state_changed":
		s.handleFolderStateChanged(agentID, eventData)
	case "folder_completion":
		s.handleFolderCompletion(agentID, eventData)
	case "sync_status_update":
		s.handleSyncStatusUpdate(agentID, eventData)
	}
}

func (s *SyncToolServer) handleFolderScanProgress(agentID string, eventData map[string]interface{}) {
	log.Printf("DEBUG Server: handleFolderScanProgress called with data: %+v", eventData)
	
	folderID, _ := eventData["folder_id"].(string)
	progress, _ := eventData["progress"].(float64)
	state, _ := eventData["state"].(string)
	
	log.Printf("DEBUG Server: Extracted - folderID: %s, progress: %.1f%%, state: %s", folderID, progress, state)
	
	if state == "" {
		state = "scanning" // Default state for scan progress
	}

	event := FolderProgressEvent{
		AgentID:      agentID,
		FolderID:     folderID,
		State:        state,
		ScanProgress: progress,
		PullProgress: 0, // Scan progress only
		Timestamp:    time.Now(),
	}

	log.Printf("DEBUG Server: About to broadcast progress event: %+v", event)
	s.BroadcastFolderProgress(event)
}

func (s *SyncToolServer) handleFolderStateChanged(agentID string, eventData map[string]interface{}) {
	folderID, _ := eventData["folder_id"].(string)
	newState, _ := eventData["state"].(string)
	oldState, _ := eventData["from_state"].(string)
	errorMsg, _ := eventData["error"].(string)

	stateEvent := FolderStateChangeEvent{
		AgentID:   agentID,
		FolderID:  folderID,
		State:     newState,
		FromState: oldState,
		Error:     errorMsg,
		Timestamp: time.Now(),
	}

	s.BroadcastFolderStateChange(stateEvent)

	// Also send as progress event with appropriate progress values
	var scanProgress, pullProgress float64
	switch newState {
	case "idle":
		scanProgress, pullProgress = 100, 100
	case "scanning":
		scanProgress, pullProgress = 0, 0
	case "syncing":
		scanProgress, pullProgress = 100, 0
	case "error":
		scanProgress, pullProgress = 0, 0
	}

	progressEvent := FolderProgressEvent{
		AgentID:      agentID,
		FolderID:     folderID,
		State:        newState,
		ScanProgress: scanProgress,
		PullProgress: pullProgress,
		Error:        errorMsg,
		Timestamp:    time.Now(),
	}

	s.BroadcastFolderProgress(progressEvent)
}

func (s *SyncToolServer) handleFolderCompletion(agentID string, eventData map[string]interface{}) {
	folderID, _ := eventData["folder_id"].(string)
	completion, _ := eventData["completion"].(float64)
	
	// Completion percentage maps to pull progress
	event := FolderProgressEvent{
		AgentID:      agentID,
		FolderID:     folderID,
		State:        "syncing",
		ScanProgress: 100, // Scan is complete when we have completion data
		PullProgress: completion,
		Timestamp:    time.Now(),
	}

	s.BroadcastFolderProgress(event)
}

func (s *SyncToolServer) handleSyncStatusUpdate(agentID string, eventData map[string]interface{}) {
	folderID, _ := eventData["folder_id"].(string)
	state, _ := eventData["state"].(string)
	progress, _ := eventData["progress"].(float64)
	globalFiles, _ := eventData["global_files"].(float64)
	localFiles, _ := eventData["local_files"].(float64)
	needFiles, _ := eventData["need_files"].(float64)

	var scanProgress, pullProgress float64
	switch state {
	case "scanning":
		scanProgress = progress
		pullProgress = 0
	case "syncing":
		scanProgress = 100
		pullProgress = progress
	case "idle":
		scanProgress = 100
		pullProgress = 100
	default:
		scanProgress = progress
		pullProgress = 0
	}

	event := FolderProgressEvent{
		AgentID:      agentID,
		FolderID:     folderID,
		State:        state,
		ScanProgress: scanProgress,
		PullProgress: pullProgress,
		GlobalFiles:  int64(globalFiles),
		LocalFiles:   int64(localFiles),
		NeedFiles:    int64(needFiles),
		Timestamp:    time.Now(),
	}

	s.BroadcastFolderProgress(event)
}