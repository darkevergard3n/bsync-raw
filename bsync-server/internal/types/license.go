package types

import "time"

// License represents a license in the system
type License struct {
	ID         int       `json:"id" db:"id"`
	LicenseKey string    `json:"license_key" db:"license_key"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}

// AgentLicense represents the mapping between an agent and a license
type AgentLicense struct {
	ID        int       `json:"id" db:"id"`
	AgentID   string    `json:"agent_id" db:"agent_id"`
	LicenseID int       `json:"license_id" db:"license_id"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// AgentWithLicense represents an agent with its license information
type AgentWithLicense struct {
	// Agent fields
	ID             string    `json:"id"`
	AgentID        string    `json:"agent_id"`
	DeviceID       string    `json:"device_id"`
	Hostname       string    `json:"hostname"`
	IPAddress      string    `json:"ip_address"`
	OS             string    `json:"os"`
	Architecture   string    `json:"architecture"`
	Version        string    `json:"version"`
	Status         string    `json:"status"`
	ApprovalStatus string    `json:"approval_status"`
	LastHeartbeat  string    `json:"last_heartbeat"`
	CreatedAt      string    `json:"created_at"`
	UpdatedAt      string    `json:"updated_at"`
	DataDir        string    `json:"data_dir"`
	
	// License fields (nullable)
	LicenseID   *int    `json:"license_id,omitempty"`
	LicenseKey  *string `json:"license_key,omitempty"`
	LicensedAt  *string `json:"licensed_at,omitempty"`
}

// CreateLicenseRequest represents the request to create a new license
type CreateLicenseRequest struct {
	LicenseKey string `json:"license_key" validate:"required"`
}

// CreateAgentLicenseRequest represents the request to map an agent to a license
type CreateAgentLicenseRequest struct {
	AgentID   string `json:"agent_id" validate:"required"`
	LicenseID int    `json:"license_id" validate:"required"`
}

// LicenseDeleteResponse represents the response when deleting a license
type LicenseDeleteResponse struct {
	Success       bool              `json:"success"`
	Message       string            `json:"message"`
	AffectedAgent *string           `json:"affected_agent,omitempty"`
	DeletedJobs   []DeletedJobInfo  `json:"deleted_jobs,omitempty"`
}

// DeletedJobInfo represents information about a deleted job
type DeletedJobInfo struct {
	JobID   string `json:"job_id"`
	JobName string `json:"job_name"`
}

// AgentLicenseMappingResponse represents the response when creating/deleting agent-license mapping
type AgentLicenseMappingResponse struct {
	Success       bool             `json:"success"`
	Message       string           `json:"message"`
	AgentID       string           `json:"agent_id"`
	LicenseID     *int             `json:"license_id,omitempty"`
	DeletedJobs   []DeletedJobInfo `json:"deleted_jobs,omitempty"`
}