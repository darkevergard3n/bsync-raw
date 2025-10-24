package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"bsync-server/internal/models"
)

// UserRepository handles database operations for users
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository creates a new user repository
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// CreateUser creates a new user
func (r *UserRepository) CreateUser(user *models.User) error {
	query := `
		INSERT INTO users (username, email, fullname, password_hash, role, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`
	err := r.db.QueryRow(
		query,
		user.Username,
		user.Email,
		user.Fullname,
		user.PasswordHash,
		user.Role,
		user.Status,
		user.CreatedBy,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)

	return err
}

// GetUserByID retrieves a user by ID
func (r *UserRepository) GetUserByID(id int) (*models.User, error) {
	query := `
		SELECT id, username, email, fullname, password_hash, role, status,
		       last_login, created_at, created_by, updated_at, updated_by, deleted_at
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
	`
	user := &models.User{}
	err := r.db.QueryRow(query, id).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.Fullname,
		&user.PasswordHash,
		&user.Role,
		&user.Status,
		&user.LastLogin,
		&user.CreatedAt,
		&user.CreatedBy,
		&user.UpdatedAt,
		&user.UpdatedBy,
		&user.DeletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	return user, err
}

// GetUserByUsername retrieves a user by username
func (r *UserRepository) GetUserByUsername(username string) (*models.User, error) {
	query := `
		SELECT id, username, email, fullname, password_hash, role, status,
		       last_login, created_at, created_by, updated_at, updated_by, deleted_at
		FROM users
		WHERE username = $1 AND deleted_at IS NULL
	`
	user := &models.User{}
	err := r.db.QueryRow(query, username).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.Fullname,
		&user.PasswordHash,
		&user.Role,
		&user.Status,
		&user.LastLogin,
		&user.CreatedAt,
		&user.CreatedBy,
		&user.UpdatedAt,
		&user.UpdatedBy,
		&user.DeletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	return user, err
}

// GetUserByEmail retrieves a user by email
func (r *UserRepository) GetUserByEmail(email string) (*models.User, error) {
	query := `
		SELECT id, username, email, fullname, password_hash, role, status,
		       last_login, created_at, created_by, updated_at, updated_by, deleted_at
		FROM users
		WHERE email = $1 AND deleted_at IS NULL
	`
	user := &models.User{}
	err := r.db.QueryRow(query, email).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.Fullname,
		&user.PasswordHash,
		&user.Role,
		&user.Status,
		&user.LastLogin,
		&user.CreatedAt,
		&user.CreatedBy,
		&user.UpdatedAt,
		&user.UpdatedBy,
		&user.DeletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	return user, err
}

// GetUserWithAgents retrieves a user with their assigned agents
func (r *UserRepository) GetUserWithAgents(id int) (*models.UserWithAgents, error) {
	query := `
		SELECT
			u.id, u.username, u.email, u.fullname, u.role, u.status,
			u.last_login, u.created_at, u.updated_at, u.deleted_at,
			COALESCE(creator.username, '') as created_by_username,
			COALESCE(updater.username, '') as updated_by_username,
			COALESCE(
				json_agg(
					json_build_object(
						'agent_id', uaa.agent_id,
						'agent_name', ia.name,
						'assigned_at', to_char(uaa.assigned_at, 'YYYY-MM-DD"T"HH24:MI:SS.USTZH:TZM'),
						'is_active', uaa.is_active
					) ORDER BY ia.name
				) FILTER (WHERE uaa.agent_id IS NOT NULL AND uaa.is_active = true),
				'[]'::json
			) as assigned_agents,
			COUNT(uaa.agent_id) FILTER (WHERE uaa.is_active = true) as assigned_agent_count
		FROM users u
		LEFT JOIN user_agent_assignments uaa ON u.id = uaa.user_id AND uaa.is_active = true
		LEFT JOIN integrated_agents ia ON uaa.agent_id = ia.agent_id
		LEFT JOIN users creator ON u.created_by = creator.id
		LEFT JOIN users updater ON u.updated_by = updater.id
		WHERE u.id = $1 AND u.deleted_at IS NULL
		GROUP BY u.id, creator.username, updater.username
	`

	userWithAgents := &models.UserWithAgents{}
	var assignedAgentsJSON string

	err := r.db.QueryRow(query, id).Scan(
		&userWithAgents.ID,
		&userWithAgents.Username,
		&userWithAgents.Email,
		&userWithAgents.Fullname,
		&userWithAgents.Role,
		&userWithAgents.Status,
		&userWithAgents.LastLogin,
		&userWithAgents.CreatedAt,
		&userWithAgents.UpdatedAt,
		&userWithAgents.DeletedAt,
		&userWithAgents.CreatedByUsername,
		&userWithAgents.UpdatedByUsername,
		&assignedAgentsJSON,
		&userWithAgents.AssignedAgentCount,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, err
	}

	// Parse JSON for assigned agents
	if err := json.Unmarshal([]byte(assignedAgentsJSON), &userWithAgents.AssignedAgents); err != nil {
		return nil, err
	}

	return userWithAgents, nil
}

// ListUsers retrieves users with optional filters
func (r *UserRepository) ListUsers(filter models.UserListFilter) ([]*models.UserWithAgents, int, error) {
	// Build WHERE clause
	var whereClauses []string
	var args []interface{}
	argIndex := 1

	whereClauses = append(whereClauses, "u.deleted_at IS NULL")

	if filter.Role != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("u.role = $%d", argIndex))
		args = append(args, filter.Role)
		argIndex++
	}

	if filter.Status != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("u.status = $%d", argIndex))
		args = append(args, filter.Status)
		argIndex++
	}

	if filter.Search != "" {
		searchPattern := "%" + filter.Search + "%"
		whereClauses = append(whereClauses, fmt.Sprintf(
			"(u.username ILIKE $%d OR u.email ILIKE $%d OR u.fullname ILIKE $%d)",
			argIndex, argIndex, argIndex,
		))
		args = append(args, searchPattern)
		argIndex++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM users u WHERE %s", whereClause)
	var total int
	err := r.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Pagination
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 1 {
		filter.Limit = 20
	}
	offset := (filter.Page - 1) * filter.Limit

	// Get users with agents
	query := fmt.Sprintf(`
		SELECT
			u.id, u.username, u.email, u.fullname, u.role, u.status,
			u.last_login, u.created_at, u.updated_at, u.deleted_at,
			COALESCE(creator.username, '') as created_by_username,
			COALESCE(updater.username, '') as updated_by_username,
			COALESCE(
				json_agg(
					json_build_object(
						'agent_id', uaa.agent_id,
						'agent_name', ia.name,
						'assigned_at', to_char(uaa.assigned_at, 'YYYY-MM-DD"T"HH24:MI:SS.USTZH:TZM'),
						'is_active', uaa.is_active
					) ORDER BY ia.name
				) FILTER (WHERE uaa.agent_id IS NOT NULL AND uaa.is_active = true),
				'[]'::json
			) as assigned_agents,
			COUNT(uaa.agent_id) FILTER (WHERE uaa.is_active = true) as assigned_agent_count
		FROM users u
		LEFT JOIN user_agent_assignments uaa ON u.id = uaa.user_id AND uaa.is_active = true
		LEFT JOIN integrated_agents ia ON uaa.agent_id = ia.agent_id
		LEFT JOIN users creator ON u.created_by = creator.id
		LEFT JOIN users updater ON u.updated_by = updater.id
		WHERE %s
		GROUP BY u.id, creator.username, updater.username
		ORDER BY u.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, filter.Limit, offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []*models.UserWithAgents
	for rows.Next() {
		user := &models.UserWithAgents{}
		var assignedAgentsJSON string

		err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.Email,
			&user.Fullname,
			&user.Role,
			&user.Status,
			&user.LastLogin,
			&user.CreatedAt,
			&user.UpdatedAt,
			&user.DeletedAt,
			&user.CreatedByUsername,
			&user.UpdatedByUsername,
			&assignedAgentsJSON,
			&user.AssignedAgentCount,
		)
		if err != nil {
			return nil, 0, err
		}

		// Parse JSON for assigned agents
		if err := json.Unmarshal([]byte(assignedAgentsJSON), &user.AssignedAgents); err != nil {
			return nil, 0, err
		}

		users = append(users, user)
	}

	return users, total, nil
}

// UpdateUser updates user information
func (r *UserRepository) UpdateUser(id int, updates map[string]interface{}, updatedBy int) error {
	if len(updates) == 0 {
		return fmt.Errorf("no fields to update")
	}

	// Build SET clause
	var setClauses []string
	var args []interface{}
	argIndex := 1

	for field, value := range updates {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", field, argIndex))
		args = append(args, value)
		argIndex++
	}

	// Add updated_by and updated_at
	setClauses = append(setClauses, fmt.Sprintf("updated_by = $%d", argIndex))
	args = append(args, updatedBy)
	argIndex++

	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIndex))
	args = append(args, time.Now())
	argIndex++

	// Add WHERE clause
	args = append(args, id)

	query := fmt.Sprintf(
		"UPDATE users SET %s WHERE id = $%d AND deleted_at IS NULL",
		strings.Join(setClauses, ", "),
		argIndex,
	)

	result, err := r.db.Exec(query, args...)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// UpdateLastLogin updates user's last login timestamp
func (r *UserRepository) UpdateLastLogin(id int) error {
	query := `UPDATE users SET last_login = $1 WHERE id = $2 AND deleted_at IS NULL`
	_, err := r.db.Exec(query, time.Now(), id)
	return err
}

// SoftDeleteUser soft deletes a user
func (r *UserRepository) SoftDeleteUser(id int, deletedBy int) error {
	query := `
		UPDATE users
		SET deleted_at = $1, updated_by = $2, updated_at = $3
		WHERE id = $4 AND deleted_at IS NULL
	`
	result, err := r.db.Exec(query, time.Now(), deletedBy, time.Now(), id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// GetUserAgents retrieves all agent IDs assigned to a user
func (r *UserRepository) GetUserAgents(userID int) ([]string, error) {
	query := `
		SELECT agent_id
		FROM user_agent_assignments
		WHERE user_id = $1 AND is_active = true
		ORDER BY assigned_at
	`
	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agentIDs []string
	for rows.Next() {
		var agentID string
		if err := rows.Scan(&agentID); err != nil {
			return nil, err
		}
		agentIDs = append(agentIDs, agentID)
	}

	return agentIDs, nil
}

// AssignAgentsToUser assigns multiple agents to a user
func (r *UserRepository) AssignAgentsToUser(userID int, agentIDs []string, assignedBy int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Deactivate all current assignments
	_, err = tx.Exec(
		`UPDATE user_agent_assignments SET is_active = false WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return err
	}

	// Insert or reactivate assignments
	for _, agentID := range agentIDs {
		query := `
			INSERT INTO user_agent_assignments (user_id, agent_id, assigned_by, is_active)
			VALUES ($1, $2, $3, true)
			ON CONFLICT (user_id, agent_id)
			DO UPDATE SET is_active = true, assigned_by = $3, assigned_at = NOW(), updated_at = NOW()
		`
		_, err = tx.Exec(query, userID, agentID, assignedBy)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// RemoveAgentAssignment removes a specific agent assignment
func (r *UserRepository) RemoveAgentAssignment(userID int, agentID string) error {
	query := `
		UPDATE user_agent_assignments
		SET is_active = false
		WHERE user_id = $1 AND agent_id = $2
	`
	result, err := r.db.Exec(query, userID, agentID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("assignment not found")
	}

	return nil
}

// HasAgentAccess checks if user has access to an agent
func (r *UserRepository) HasAgentAccess(userID int, agentID string) (bool, error) {
	query := `SELECT user_has_agent_access($1, $2)`
	var hasAccess bool
	err := r.db.QueryRow(query, userID, agentID).Scan(&hasAccess)
	return hasAccess, err
}

// LogActivity logs user activity for audit trail
func (r *UserRepository) LogActivity(log *models.UserActivityLog) error {
	query := `
		INSERT INTO user_activity_logs
		(user_id, username, action, resource_type, resource_id, ip_address, user_agent, details)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.db.Exec(
		query,
		log.UserID,
		log.Username,
		log.Action,
		log.ResourceType,
		log.ResourceID,
		log.IPAddress,
		log.UserAgent,
		log.Details,
	)
	return err
}
