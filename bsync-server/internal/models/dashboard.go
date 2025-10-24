package models

import "time"

// RoleInfo represents a user role with description
type RoleInfo struct {
	RoleCode        string `json:"role_code" db:"role_code"`
	RoleLabel       string `json:"role_label" db:"role_label"`
	RoleDescription string `json:"role_description" db:"role_description"`
}

// UserStats represents user statistics for dashboard
type UserStats struct {
	TotalUsers     int64 `json:"total_users" db:"total_users"`
	TotalAdmin     int64 `json:"total_admin" db:"total_admin"`
	TotalOperator  int64 `json:"total_operator" db:"total_operator"`
	ActiveUsers    int64 `json:"active_users" db:"active_users"`
}

// LicensedAgent represents an agent with license information
type LicensedAgent struct {
	AgentID    string     `json:"agent_id" db:"agent_id"`
	Name       string     `json:"name" db:"name"`
	Hostname   string     `json:"hostname" db:"hostname"`
	Status     string     `json:"status" db:"status"`
	LastSeen   *time.Time `json:"last_seen,omitempty" db:"last_seen"`
	LicenseID  int        `json:"license_id" db:"license_id"`
	LicensedAt *time.Time `json:"licensed_at,omitempty" db:"licensed_at"`
}

// DashboardStats represents main dashboard statistics
type DashboardStats struct {
	TotalAgents          int64 `json:"total_agents" db:"total_agents"`
	TotalActiveJobs      int64 `json:"total_active_jobs" db:"total_active_jobs"`
	TotalUsers           int64 `json:"total_users" db:"total_users"`
	TotalFiles           int64 `json:"total_files" db:"total_files"`
	TotalDataTransferred int64 `json:"total_data_transferred" db:"total_data_transferred"`
}

// DailyFileTransferStats represents daily file transfer statistics
type DailyFileTransferStats struct {
	TransferDate time.Time `json:"transfer_date" db:"transfer_date"`
	FileCount    int64     `json:"file_count" db:"file_count"`
	TotalBytes   int64     `json:"total_bytes" db:"total_bytes"`
	DateLabel    string    `json:"date_label" db:"date_label"`
	DayName      string    `json:"day_name" db:"day_name"`
}

// JobPerformance represents job performance statistics
type JobPerformance struct {
	JobID                  int        `json:"job_id" db:"job_id"`
	JobName                string     `json:"job_name" db:"job_name"`
	SourceAgentID          string     `json:"source_agent_id" db:"source_agent_id"`
	TargetAgentID          string     `json:"target_agent_id" db:"target_agent_id"`
	JobStatus              string     `json:"job_status" db:"job_status"`
	TotalFilesTransferred  int64      `json:"total_files_transferred" db:"total_files_transferred"`
	TotalBytesTransferred  int64      `json:"total_bytes_transferred" db:"total_bytes_transferred"`
	LastTransferAt         *time.Time `json:"last_transfer_at,omitempty" db:"last_transfer_at"`
}

// TopJobsPerformance represents combined top jobs performance
type TopJobsPerformance struct {
	TopJobsByFileCount []JobPerformance `json:"top_jobs_by_file_count"`
	TopJobsByDataSize  []JobPerformance `json:"top_jobs_by_data_size"`
}

// RecentFileTransferEvent represents recent file transfer event
type RecentFileTransferEvent struct {
	ID                    int        `json:"id" db:"id"`
	JobID                 *string    `json:"job_id,omitempty" db:"job_id"`
	JobName               *string    `json:"job_name,omitempty" db:"job_name"`
	AgentID               *string    `json:"agent_id,omitempty" db:"agent_id"`
	FileName              *string    `json:"file_name,omitempty" db:"file_name"`
	FilePath              *string    `json:"file_path,omitempty" db:"file_path"`
	BytesTransferred      int64      `json:"bytes_transferred" db:"bytes_transferred"`
	FileSize              *int64     `json:"file_size,omitempty" db:"file_size"`
	Status                *string    `json:"status,omitempty" db:"status"`
	Action                *string    `json:"action,omitempty" db:"action"`
	SourceAgentName       *string    `json:"source_agent_name,omitempty" db:"source_agent_name"`
	DestinationAgentName  *string    `json:"destination_agent_name,omitempty" db:"destination_agent_name"`
	SyncMode              *string    `json:"sync_mode,omitempty" db:"sync_mode"`
	StartedAt             *time.Time `json:"started_at,omitempty" db:"started_at"`
	CompletedAt           *time.Time `json:"completed_at,omitempty" db:"completed_at"`
	Duration              *float64   `json:"duration,omitempty" db:"duration"`
	TransferRate          *float64   `json:"transfer_rate,omitempty" db:"transfer_rate"`
	ErrorMessage          *string    `json:"error_message,omitempty" db:"error_message"`
	SessionID             *string    `json:"session_id,omitempty" db:"session_id"`
}

// DashboardResponse represents complete dashboard data
type DashboardResponse struct {
	Stats                DashboardStats               `json:"stats"`
	UserStats            UserStats                    `json:"user_stats"`
	DailyTransferStats   []DailyFileTransferStats     `json:"daily_transfer_stats"`
	TopJobsPerformance   TopJobsPerformance           `json:"top_jobs_performance"`
	RecentEvents         []RecentFileTransferEvent    `json:"recent_events"`
}
