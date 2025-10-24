package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"bsync-server/internal/models"
)

// DashboardRepository handles database operations for dashboard statistics
type DashboardRepository struct {
	db *sql.DB
}

// NewDashboardRepository creates a new dashboard repository
func NewDashboardRepository(db *sql.DB) *DashboardRepository {
	return &DashboardRepository{db: db}
}

// GetRoleList retrieves list of available roles
func (r *DashboardRepository) GetRoleList() ([]models.RoleInfo, error) {
	query := `SELECT * FROM get_role_list()`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get role list: %w", err)
	}
	defer rows.Close()

	var roles []models.RoleInfo
	for rows.Next() {
		var role models.RoleInfo
		err := rows.Scan(
			&role.RoleCode,
			&role.RoleLabel,
			&role.RoleDescription,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan role: %w", err)
		}
		roles = append(roles, role)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating roles: %w", err)
	}

	return roles, nil
}

// GetUserStats retrieves user statistics
func (r *DashboardRepository) GetUserStats() (*models.UserStats, error) {
	query := `SELECT * FROM get_user_stats()`

	var stats models.UserStats
	err := r.db.QueryRow(query).Scan(
		&stats.TotalUsers,
		&stats.TotalAdmin,
		&stats.TotalOperator,
		&stats.ActiveUsers,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get user stats: %w", err)
	}

	return &stats, nil
}

// GetLicensedAgents retrieves list of licensed agents
func (r *DashboardRepository) GetLicensedAgents() ([]models.LicensedAgent, error) {
	query := `
		SELECT
			agent_id, name, hostname, status,
			last_seen, license_id, licensed_at
		FROM v_licensed_agents
		ORDER BY name
	`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get licensed agents: %w", err)
	}
	defer rows.Close()

	var agents []models.LicensedAgent
	for rows.Next() {
		var agent models.LicensedAgent
		err := rows.Scan(
			&agent.AgentID,
			&agent.Name,
			&agent.Hostname,
			&agent.Status,
			&agent.LastSeen,
			&agent.LicenseID,
			&agent.LicensedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan licensed agent: %w", err)
		}
		agents = append(agents, agent)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating licensed agents: %w", err)
	}

	return agents, nil
}

// GetDashboardStats retrieves main dashboard statistics
func (r *DashboardRepository) GetDashboardStats(assignedAgents []string) (*models.DashboardStats, error) {
	var stats models.DashboardStats

	if len(assignedAgents) > 0 {
		// Operator: custom query filtered by assigned agents
		placeholders := make([]string, len(assignedAgents))
		args := make([]interface{}, len(assignedAgents))
		for i, agentID := range assignedAgents {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = agentID
		}

		query := fmt.Sprintf(`
			SELECT
				(SELECT COUNT(DISTINCT agent_id) FROM integrated_agents
				 WHERE agent_id IN (%s)) as total_agents,
				(SELECT COUNT(*) FROM sync_jobs
				 WHERE status = 'active'
				 AND (source_agent_id IN (%s) OR dest_agent_id IN (%s))) as total_active_jobs,
				0 as total_users,
				(SELECT COALESCE(SUM(total_files), 0) FROM sync_jobs
				 WHERE source_agent_id IN (%s) OR dest_agent_id IN (%s)) as total_files,
				(SELECT COALESCE(SUM(total_bytes), 0) FROM sync_jobs
				 WHERE source_agent_id IN (%s) OR dest_agent_id IN (%s)) as total_data_transferred
		`,
			strings.Join(placeholders, ", "),
			strings.Join(placeholders, ", "),
			strings.Join(placeholders, ", "),
			strings.Join(placeholders, ", "),
			strings.Join(placeholders, ", "),
			strings.Join(placeholders, ", "),
			strings.Join(placeholders, ", "),
		)

		// Duplicate args for each IN clause
		allArgs := []interface{}{}
		for i := 0; i < 7; i++ {
			allArgs = append(allArgs, args...)
		}

		err := r.db.QueryRow(query, allArgs...).Scan(
			&stats.TotalAgents,
			&stats.TotalActiveJobs,
			&stats.TotalUsers,
			&stats.TotalFiles,
			&stats.TotalDataTransferred,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to get dashboard stats for operator: %w", err)
		}
	} else {
		// Admin: use stored function
		query := `SELECT * FROM get_dashboard_stats()`
		err := r.db.QueryRow(query).Scan(
			&stats.TotalAgents,
			&stats.TotalActiveJobs,
			&stats.TotalUsers,
			&stats.TotalFiles,
			&stats.TotalDataTransferred,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to get dashboard stats: %w", err)
		}
	}

	return &stats, nil
}

// GetDailyFileTransferStats retrieves daily file transfer statistics for last 7 days
func (r *DashboardRepository) GetDailyFileTransferStats(assignedAgents []string) ([]models.DailyFileTransferStats, error) {
	var query string
	var args []interface{}

	if len(assignedAgents) > 0 {
		// Operator: filter by assigned agents
		placeholders := make([]string, len(assignedAgents))
		args = make([]interface{}, len(assignedAgents))
		for i, agentID := range assignedAgents {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = agentID
		}

		query = fmt.Sprintf(`
			WITH date_series AS (
				SELECT generate_series(
					CURRENT_DATE - INTERVAL '6 days',
					CURRENT_DATE,
					'1 day'::interval
				)::date as transfer_date
			),
			daily_stats AS (
				SELECT
					DATE(ftl.completed_at) as transfer_date,
					COUNT(*) as file_count,
					COALESCE(SUM(COALESCE(ftl.delta_bytes_transferred, ftl.file_size)), 0) as total_bytes
				FROM file_transfer_logs ftl
				INNER JOIN sync_jobs sj ON ftl.job_id = sj.id
				WHERE ftl.status = 'completed'
					AND ftl.completed_at >= CURRENT_DATE - INTERVAL '6 days'
					AND ftl.completed_at < CURRENT_DATE + INTERVAL '1 day'
					AND (sj.source_agent_id IN (%s) OR sj.dest_agent_id IN (%s))
				GROUP BY DATE(ftl.completed_at)
			)
			SELECT
				ds.transfer_date,
				COALESCE(dst.file_count, 0) as file_count,
				COALESCE(dst.total_bytes, 0) as total_bytes,
				TO_CHAR(ds.transfer_date, 'DD Mon') as date_label,
				TO_CHAR(ds.transfer_date, 'Dy') as day_name
			FROM date_series ds
			LEFT JOIN daily_stats dst ON ds.transfer_date = dst.transfer_date
			ORDER BY ds.transfer_date
		`, strings.Join(placeholders, ", "), strings.Join(placeholders, ", "))
		// Duplicate args for second IN clause
		args = append(args, args...)
	} else {
		// Admin: use view
		query = `
			SELECT
				transfer_date, file_count, total_bytes,
				date_label, day_name
			FROM v_daily_file_transfer_stats
			ORDER BY transfer_date
		`
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily file transfer stats: %w", err)
	}
	defer rows.Close()

	var stats []models.DailyFileTransferStats
	for rows.Next() {
		var stat models.DailyFileTransferStats
		err := rows.Scan(
			&stat.TransferDate,
			&stat.FileCount,
			&stat.TotalBytes,
			&stat.DateLabel,
			&stat.DayName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan daily stats: %w", err)
		}
		stats = append(stats, stat)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating daily stats: %w", err)
	}

	return stats, nil
}

// GetTopJobsByFileCount retrieves top 5 jobs by file count
func (r *DashboardRepository) GetTopJobsByFileCount(assignedAgents []string) ([]models.JobPerformance, error) {
	var query string
	var args []interface{}

	if len(assignedAgents) > 0 {
		// Operator: filter by assigned agents
		placeholders := make([]string, len(assignedAgents))
		args = make([]interface{}, len(assignedAgents))
		for i, agentID := range assignedAgents {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = agentID
		}

		query = fmt.Sprintf(`
			SELECT
				job_id, job_name, source_agent_id, target_agent_id,
				job_status, total_files_transferred, total_bytes_transferred,
				last_transfer_at
			FROM v_top_jobs_by_file_count
			WHERE source_agent_id IN (%s) OR target_agent_id IN (%s)
		`, strings.Join(placeholders, ", "), strings.Join(placeholders, ", "))
		// Duplicate args for second IN clause
		args = append(args, args...)
	} else {
		// Admin: get all
		query = `
			SELECT
				job_id, job_name, source_agent_id, target_agent_id,
				job_status, total_files_transferred, total_bytes_transferred,
				last_transfer_at
			FROM v_top_jobs_by_file_count
		`
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get top jobs by file count: %w", err)
	}
	defer rows.Close()

	var jobs []models.JobPerformance
	for rows.Next() {
		var job models.JobPerformance
		err := rows.Scan(
			&job.JobID,
			&job.JobName,
			&job.SourceAgentID,
			&job.TargetAgentID,
			&job.JobStatus,
			&job.TotalFilesTransferred,
			&job.TotalBytesTransferred,
			&job.LastTransferAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan job performance: %w", err)
		}
		jobs = append(jobs, job)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating job performance: %w", err)
	}

	return jobs, nil
}

// GetTopJobsByDataSize retrieves top 5 jobs by data size
func (r *DashboardRepository) GetTopJobsByDataSize(assignedAgents []string) ([]models.JobPerformance, error) {
	var query string
	var args []interface{}

	if len(assignedAgents) > 0 {
		// Operator: filter by assigned agents
		placeholders := make([]string, len(assignedAgents))
		args = make([]interface{}, len(assignedAgents))
		for i, agentID := range assignedAgents {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = agentID
		}

		query = fmt.Sprintf(`
			SELECT
				job_id, job_name, source_agent_id, target_agent_id,
				job_status, total_files_transferred, total_bytes_transferred,
				last_transfer_at
			FROM v_top_jobs_by_data_size
			WHERE source_agent_id IN (%s) OR target_agent_id IN (%s)
		`, strings.Join(placeholders, ", "), strings.Join(placeholders, ", "))
		// Duplicate args for second IN clause
		args = append(args, args...)
	} else {
		// Admin: get all
		query = `
			SELECT
				job_id, job_name, source_agent_id, target_agent_id,
				job_status, total_files_transferred, total_bytes_transferred,
				last_transfer_at
			FROM v_top_jobs_by_data_size
		`
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get top jobs by data size: %w", err)
	}
	defer rows.Close()

	var jobs []models.JobPerformance
	for rows.Next() {
		var job models.JobPerformance
		err := rows.Scan(
			&job.JobID,
			&job.JobName,
			&job.SourceAgentID,
			&job.TargetAgentID,
			&job.JobStatus,
			&job.TotalFilesTransferred,
			&job.TotalBytesTransferred,
			&job.LastTransferAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan job performance: %w", err)
		}
		jobs = append(jobs, job)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating job performance: %w", err)
	}

	return jobs, nil
}

// GetRecentFileTransferEvents retrieves last 5 completed file transfer events
func (r *DashboardRepository) GetRecentFileTransferEvents(assignedAgents []string) ([]models.RecentFileTransferEvent, error) {
	var query string
	var args []interface{}

	if len(assignedAgents) > 0 {
		// Operator: filter by assigned agents
		placeholders := make([]string, len(assignedAgents))
		args = make([]interface{}, len(assignedAgents))
		for i, agentID := range assignedAgents {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = agentID
		}

		query = fmt.Sprintf(`
			SELECT
				ftl.id, ftl.job_id, ftl.job_name, ftl.agent_id, ftl.file_name, ftl.file_path,
				COALESCE(ftl.delta_bytes_transferred, ftl.file_size, 0) as bytes_transferred,
				ftl.file_size, ftl.status, ftl.action,
				ftl.source_agent_name, ftl.destination_agent_name, ftl.sync_mode,
				ftl.started_at, ftl.completed_at, ftl.duration, ftl.transfer_rate,
				ftl.error_message, ftl.session_id
			FROM file_transfer_logs ftl
			INNER JOIN sync_jobs sj ON ftl.job_id = sj.id
			WHERE ftl.status = 'completed'
				AND (sj.source_agent_id IN (%s) OR sj.dest_agent_id IN (%s))
			ORDER BY ftl.completed_at DESC
			LIMIT 5
		`, strings.Join(placeholders, ", "), strings.Join(placeholders, ", "))
		// Duplicate args for second IN clause
		args = append(args, args...)
	} else {
		// Admin: use view
		query = `
			SELECT
				id, job_id, job_name, agent_id, file_name, file_path,
				bytes_transferred, file_size, status, action,
				source_agent_name, destination_agent_name, sync_mode,
				started_at, completed_at, duration, transfer_rate,
				error_message, session_id
			FROM v_recent_file_transfer_events
		`
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent file transfer events: %w", err)
	}
	defer rows.Close()

	var events []models.RecentFileTransferEvent
	for rows.Next() {
		var event models.RecentFileTransferEvent
		err := rows.Scan(
			&event.ID,
			&event.JobID,
			&event.JobName,
			&event.AgentID,
			&event.FileName,
			&event.FilePath,
			&event.BytesTransferred,
			&event.FileSize,
			&event.Status,
			&event.Action,
			&event.SourceAgentName,
			&event.DestinationAgentName,
			&event.SyncMode,
			&event.StartedAt,
			&event.CompletedAt,
			&event.Duration,
			&event.TransferRate,
			&event.ErrorMessage,
			&event.SessionID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file transfer event: %w", err)
		}
		events = append(events, event)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating file transfer events: %w", err)
	}

	return events, nil
}

// GetCompleteDashboard retrieves all dashboard data in one call
func (r *DashboardRepository) GetCompleteDashboard(assignedAgents []string) (*models.DashboardResponse, error) {
	// Get dashboard stats
	dashboardStats, err := r.GetDashboardStats(assignedAgents)
	if err != nil {
		return nil, fmt.Errorf("failed to get dashboard stats: %w", err)
	}

	// Get user stats
	userStats, err := r.GetUserStats()
	if err != nil {
		return nil, fmt.Errorf("failed to get user stats: %w", err)
	}

	// Get daily transfer stats
	dailyStats, err := r.GetDailyFileTransferStats(assignedAgents)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily transfer stats: %w", err)
	}

	// Get top jobs by file count
	topJobsByFileCount, err := r.GetTopJobsByFileCount(assignedAgents)
	if err != nil {
		return nil, fmt.Errorf("failed to get top jobs by file count: %w", err)
	}

	// Get top jobs by data size
	topJobsByDataSize, err := r.GetTopJobsByDataSize(assignedAgents)
	if err != nil {
		return nil, fmt.Errorf("failed to get top jobs by data size: %w", err)
	}

	// Get recent events
	recentEvents, err := r.GetRecentFileTransferEvents(assignedAgents)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent events: %w", err)
	}

	return &models.DashboardResponse{
		Stats:      *dashboardStats,
		UserStats:  *userStats,
		DailyTransferStats: dailyStats,
		TopJobsPerformance: models.TopJobsPerformance{
			TopJobsByFileCount: topJobsByFileCount,
			TopJobsByDataSize:  topJobsByDataSize,
		},
		RecentEvents: recentEvents,
	}, nil
}
