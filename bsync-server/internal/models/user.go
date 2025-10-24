package models

import (
	"database/sql"
	"time"
)

// User represents a system user with role-based access
type User struct {
	ID           int            `json:"id"`
	Username     string         `json:"username"`
	Email        string         `json:"email"`
	Fullname     string         `json:"fullname"`
	PasswordHash string         `json:"-"` // Never expose in JSON
	Role         string         `json:"role"` // "admin" or "operator"
	Status       string         `json:"status"` // "active", "inactive", "suspended"
	LastLogin    *time.Time     `json:"last_login,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	CreatedBy    sql.NullInt64  `json:"created_by,omitempty"`
	UpdatedAt    time.Time      `json:"updated_at"`
	UpdatedBy    sql.NullInt64  `json:"updated_by,omitempty"`
	DeletedAt    *time.Time     `json:"deleted_at,omitempty"`
}

// UserWithAgents represents a user with their assigned agents
type UserWithAgents struct {
	User
	AssignedAgents     []AssignedAgent `json:"assigned_agents"`
	AssignedAgentCount int             `json:"assigned_agent_count"`
	CreatedByUsername  string          `json:"created_by_username,omitempty"`
	UpdatedByUsername  string          `json:"updated_by_username,omitempty"`
}

// AssignedAgent represents an agent assigned to a user
type AssignedAgent struct {
	AgentID    string `json:"agent_id"`
	AgentName  string `json:"agent_name"`
	AssignedAt string `json:"assigned_at"` // ISO 8601 string from database
	IsActive   bool   `json:"is_active"`
}

// UserAgentAssignment represents the assignment table
type UserAgentAssignment struct {
	ID         int           `json:"id"`
	UserID     int           `json:"user_id"`
	AgentID    string        `json:"agent_id"`
	AssignedAt time.Time     `json:"assigned_at"`
	AssignedBy sql.NullInt64 `json:"assigned_by,omitempty"`
	IsActive   bool          `json:"is_active"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
}

// UserActivityLog represents audit trail entry
type UserActivityLog struct {
	ID           int            `json:"id"`
	UserID       sql.NullInt64  `json:"user_id,omitempty"`
	Username     string         `json:"username"`
	Action       string         `json:"action"`
	ResourceType sql.NullString `json:"resource_type,omitempty"`
	ResourceID   sql.NullString `json:"resource_id,omitempty"`
	IPAddress    sql.NullString `json:"ip_address,omitempty"`
	UserAgent    sql.NullString `json:"user_agent,omitempty"`
	Details      interface{}    `json:"details,omitempty"` // JSONB field
	CreatedAt    time.Time      `json:"created_at"`
}

// CreateUserRequest represents the request to create a new user
type CreateUserRequest struct {
	Username       string   `json:"username" binding:"required,min=3,max=100"`
	Email          string   `json:"email" binding:"required,email,max=255"`
	Fullname       string   `json:"fullname" binding:"required,max=255"`
	Password       string   `json:"password" binding:"omitempty,min=8"` // Optional - will be auto-generated if not provided
	Role           string   `json:"role" binding:"required,oneof=admin operator"`
	Status         string   `json:"status" binding:"omitempty,oneof=active inactive suspended"`
	AssignedAgents []string `json:"assigned_agents,omitempty"`
}

// UpdateUserRequest represents the request to update user
type UpdateUserRequest struct {
	Email          *string  `json:"email" binding:"omitempty,email,max=255"`
	Fullname       *string  `json:"fullname" binding:"omitempty,max=255"`
	Password       *string  `json:"password" binding:"omitempty,min=8"`
	Role           *string  `json:"role" binding:"omitempty,oneof=admin operator"`
	Status         *string  `json:"status" binding:"omitempty,oneof=active inactive suspended"`
	AssignedAgents []string `json:"assigned_agents,omitempty"`
}

// LoginRequest represents login credentials
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse represents the response after successful login
type LoginResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresIn   int64     `json:"expires_in"` // seconds
	User        UserInfo  `json:"user"`
}

// UserInfo represents safe user info (no sensitive data)
type UserInfo struct {
	ID             int      `json:"id"`
	Username       string   `json:"username"`
	Email          string   `json:"email"`
	Fullname       string   `json:"fullname"`
	Role           string   `json:"role"`
	Status         string   `json:"status"`
	AssignedAgents []string `json:"assigned_agents,omitempty"`
	LastLogin      *time.Time `json:"last_login,omitempty"`
}

// JWTClaims represents the JWT token claims
type JWTClaims struct {
	UserID         int      `json:"user_id"`
	Username       string   `json:"username"`
	Email          string   `json:"email"`
	Fullname       string   `json:"fullname"`
	Role           string   `json:"role"`
	AssignedAgents []string `json:"assigned_agents,omitempty"`
	ExpiresAt      int64    `json:"exp"`
	IssuedAt       int64    `json:"iat"`
}

// ChangePasswordRequest represents password change request
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

// AssignAgentsRequest represents agent assignment request
type AssignAgentsRequest struct {
	AgentIDs []string `json:"agent_ids" binding:"required,min=1"`
}

// UserListFilter represents filter options for listing users
type UserListFilter struct {
	Role   string `form:"role" binding:"omitempty,oneof=admin operator"`
	Status string `form:"status" binding:"omitempty,oneof=active inactive suspended"`
	Search string `form:"search"` // Search in username, email, fullname
	Page   int    `form:"page" binding:"omitempty,min=1"`
	Limit  int    `form:"limit" binding:"omitempty,min=1,max=100"`
}

// Role constants
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
)

// Status constants
const (
	StatusActive    = "active"
	StatusInactive  = "inactive"
	StatusSuspended = "suspended"
)

// Action constants for audit log
const (
	ActionLogin              = "login"
	ActionLogout             = "logout"
	ActionLoginFailed        = "login_failed"
	ActionCreateUser         = "create_user"
	ActionUpdateUser         = "update_user"
	ActionDeleteUser         = "delete_user"
	ActionChangePassword     = "change_password"
	ActionAssignAgents       = "assign_agents"
	ActionUnassignAgent      = "unassign_agent"
	ActionCreateJob          = "create_job"
	ActionUpdateJob          = "update_job"
	ActionDeleteJob          = "delete_job"
	ActionAccessDenied       = "access_denied"
)
