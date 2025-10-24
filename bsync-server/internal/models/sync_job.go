package models

import "time"

// SyncJobDestination represents a destination for a sync job
// Supports multiple destinations per job (1-to-N relationship)
type SyncJobDestination struct {
	ID                   int        `json:"id"`
	JobID                int        `json:"job_id"`
	DestinationAgentID   string     `json:"destination_agent_id"`
	DestinationPath      string     `json:"destination_path"`
	DestinationDeviceID  *string    `json:"destination_device_id,omitempty"`
	DestinationIPAddress *string    `json:"destination_ip_address,omitempty"`

	// Status tracking
	Status         string     `json:"status"`          // active, paused, failed
	LastSyncStatus *string    `json:"last_sync_status,omitempty"` // idle, scanning, syncing, completed, error

	// Sync statistics
	LastSyncTime *time.Time `json:"last_sync_time,omitempty"`
	FilesSynced  int64      `json:"files_synced"`
	BytesSynced  int64      `json:"bytes_synced"`

	// Error tracking
	LastError  *string `json:"last_error,omitempty"`
	ErrorCount int     `json:"error_count"`

	// Metadata
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SyncJobWithDestinations represents a sync job with multiple destinations
type SyncJobWithDestinations struct {
	ID                 int                    `json:"id"`
	Name               string                 `json:"name"`
	SourceAgentID      string                 `json:"source_agent_id"`
	SourcePath         string                 `json:"source_path"`
	SyncType           string                 `json:"sync_type"`
	Status             string                 `json:"status"`
	ScheduleType       string                 `json:"schedule_type"`
	RescanInterval     int                    `json:"rescan_interval"`
	IgnorePatterns     []string               `json:"ignore_patterns,omitempty"`
	IsMultiDestination bool                   `json:"is_multi_destination"`
	Destinations       []SyncJobDestination   `json:"destinations"`

	// Legacy fields (for backward compatibility)
	TargetAgentID *string `json:"target_agent_id,omitempty"`
	TargetPath    *string `json:"target_path,omitempty"`

	// Metadata
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	LastScheduledRun *time.Time `json:"last_scheduled_run,omitempty"`
	NextScheduledRun *time.Time `json:"next_scheduled_run,omitempty"`
}

// SyncJobSummary represents aggregated summary of a multi-destination job
type SyncJobSummary struct {
	JobID              int        `json:"job_id"`
	JobName            string     `json:"job_name"`
	SourceAgentID      string     `json:"source_agent_id"`
	SourcePath         string     `json:"source_path"`
	SyncType           string     `json:"sync_type"`
	JobStatus          string     `json:"job_status"`
	ScheduleType       string     `json:"schedule_type"`
	IsMultiDestination bool       `json:"is_multi_destination"`

	// Aggregated stats
	DestinationCount    int        `json:"destination_count"`
	ActiveDestinations  int        `json:"active_destinations"`
	PausedDestinations  int        `json:"paused_destinations"`
	FailedDestinations  int        `json:"failed_destinations"`
	TotalFilesSynced    int64      `json:"total_files_synced"`
	TotalBytesSynced    int64      `json:"total_bytes_synced"`
	LastSyncTime        *time.Time `json:"last_sync_time,omitempty"`

	// Metadata
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateSyncJobRequest represents the request body for creating a new sync job
type CreateSyncJobRequest struct {
	Name           string                          `json:"name"`
	SourceAgentID  string                          `json:"source_agent_id"`
	SourcePath     string                          `json:"source_path"`
	SyncType       string                          `json:"sync_type"`
	ScheduleType   string                          `json:"schedule_type"`
	RescanInterval int                             `json:"rescan_interval"`
	IgnorePatterns []string                        `json:"ignore_patterns,omitempty"`

	// Multi-destination support
	Destinations []CreateDestinationRequest `json:"destinations,omitempty"`

	// Legacy single destination (for backward compatibility)
	DestinationAgentID *string `json:"destination_agent_id,omitempty"`
	DestinationPath    *string `json:"destination_path,omitempty"`
}

// CreateDestinationRequest represents a destination in job creation request
type CreateDestinationRequest struct {
	AgentID string `json:"agent_id"`
	Path    string `json:"path"`
}

// UpdateSyncJobDestinationsRequest represents the request to update job destinations
type UpdateSyncJobDestinationsRequest struct {
	Destinations []CreateDestinationRequest `json:"destinations"`
}

// Validate validates the create job request
func (req *CreateSyncJobRequest) Validate() error {
	if req.Name == "" {
		return ErrInvalidJobName
	}
	if req.SourceAgentID == "" {
		return ErrInvalidSourceAgent
	}
	if req.SourcePath == "" {
		return ErrInvalidSourcePath
	}
	if req.SyncType == "" {
		return ErrInvalidSyncType
	}

	// Check if at least one destination is specified
	hasMultiDest := len(req.Destinations) > 0
	hasLegacyDest := req.DestinationAgentID != nil && *req.DestinationAgentID != ""

	if !hasMultiDest && !hasLegacyDest {
		return ErrNoDestinationSpecified
	}

	// If using multi-destination, validate each destination
	if hasMultiDest {
		for i, dest := range req.Destinations {
			if dest.AgentID == "" {
				return &ValidationError{Field: "destinations", Index: i, Reason: "agent_id is required"}
			}
			if dest.Path == "" {
				return &ValidationError{Field: "destinations", Index: i, Reason: "path is required"}
			}
		}
	}

	return nil
}

// IsMultiDestination checks if this is a multi-destination job
func (req *CreateSyncJobRequest) IsMultiDestination() bool {
	return len(req.Destinations) > 0
}

// Custom errors
var (
	ErrInvalidJobName         = &ValidationError{Field: "name", Reason: "name is required"}
	ErrInvalidSourceAgent     = &ValidationError{Field: "source_agent_id", Reason: "source_agent_id is required"}
	ErrInvalidSourcePath      = &ValidationError{Field: "source_path", Reason: "source_path is required"}
	ErrInvalidSyncType        = &ValidationError{Field: "sync_type", Reason: "sync_type is required"}
	ErrNoDestinationSpecified = &ValidationError{Field: "destinations", Reason: "at least one destination is required"}
)

// ValidationError represents a validation error
type ValidationError struct {
	Field  string
	Index  int
	Reason string
}

func (e *ValidationError) Error() string {
	if e.Index >= 0 {
		return e.Field + "[" + string(rune(e.Index)) + "]: " + e.Reason
	}
	return e.Field + ": " + e.Reason
}
