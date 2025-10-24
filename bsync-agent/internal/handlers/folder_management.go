package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"bsync-agent/internal/agent"
	"bsync-agent/internal/embedded"
)

// FolderManagementHandler handles folder-related API requests
type FolderManagementHandler struct {
	agent *agent.IntegratedAgent
}

// NewFolderManagementHandler creates a new folder management handler
func NewFolderManagementHandler(agent *agent.IntegratedAgent) *FolderManagementHandler {
	return &FolderManagementHandler{
		agent: agent,
	}
}

// AddFolderRequest represents a request to add a new folder
type AddFolderRequest struct {
	ID               string   `json:"id"`
	Label            string   `json:"label"`
	Path             string   `json:"path"`
	Type             string   `json:"type"`      // "sendreceive", "sendonly", "receiveonly"
	Devices          []string `json:"devices"`
	RescanIntervalS  int      `json:"rescan_interval_s"`
	IgnorePerms      bool     `json:"ignore_perms"`
	FSWatcherEnabled *bool    `json:"fs_watcher_enabled"` // Pointer to distinguish between not set and false
	FSWatcherDelayS  int      `json:"fs_watcher_delay_s"`
	IgnorePatterns   []string `json:"ignore_patterns"`    // Patterns to ignore
}

// AddFolderResponse represents the response after adding a folder
type AddFolderResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ScanFolderResponse represents the response after scanning a folder
type ScanFolderResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// FolderStatusResponse represents folder status information
type FolderStatusResponse struct {
	Folders map[string]*embedded.FolderStatus `json:"folders"`
}

// HandleAddFolder handles POST /api/v1/folders
func (h *FolderManagementHandler) HandleAddFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AddFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.ID == "" || req.Path == "" {
		http.Error(w, "Missing required fields: id, path", http.StatusBadRequest)
		return
	}

	// Set defaults
	if req.Label == "" {
		req.Label = req.ID
	}
	if req.RescanIntervalS == 0 {
		req.RescanIntervalS = 60 // Default 60 seconds
	}
	if req.Type == "" {
		req.Type = "sendreceive" // Default to two-way sync
	}
	
	// Handle FSWatcherEnabled default (true if not specified)
	watcherEnabled := true
	if req.FSWatcherEnabled != nil {
		watcherEnabled = *req.FSWatcherEnabled
	}
	
	// Set default watcher delay if not specified
	if req.FSWatcherDelayS == 0 {
		req.FSWatcherDelayS = 10 // Default 10 seconds
	}

	// Create folder config
	folderConfig := embedded.FolderConfig{
		ID:               req.ID,
		Label:            req.Label,
		Path:             req.Path,
		Type:             req.Type,
		Devices:          req.Devices,
		RescanIntervalS:  req.RescanIntervalS,
		IgnorePerms:      req.IgnorePerms,
		FSWatcherEnabled: watcherEnabled,
		FSWatcherDelayS:  req.FSWatcherDelayS,
		IgnorePatterns:   req.IgnorePatterns,
	}

	// Add folder using internal method
	err := h.agent.AddFolder(folderConfig)
	
	response := AddFolderResponse{
		Success: err == nil,
	}
	
	if err != nil {
		response.Message = fmt.Sprintf("Failed to add folder: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		response.Message = fmt.Sprintf("Folder %s added successfully", req.ID)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleScanFolder handles POST /api/v1/folders/{id}/scan
func (h *FolderManagementHandler) HandleScanFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract folder ID from URL path
	// Simple implementation - in production would use a proper router
	folderID := r.URL.Query().Get("id")
	if folderID == "" {
		http.Error(w, "Missing folder ID", http.StatusBadRequest)
		return
	}

	// Trigger folder scan using internal method
	err := h.agent.ScanFolder(folderID)
	
	response := ScanFolderResponse{
		Success: err == nil,
	}
	
	if err != nil {
		response.Message = fmt.Sprintf("Failed to scan folder: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		response.Message = fmt.Sprintf("Folder %s scan triggered", folderID)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleListFolders handles GET /api/v1/folders
func (h *FolderManagementHandler) HandleListFolders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	folders, err := h.agent.GetAllFolderStatuses()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get folders: %v", err), http.StatusInternalServerError)
		return
	}

	response := FolderStatusResponse{
		Folders: folders,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}