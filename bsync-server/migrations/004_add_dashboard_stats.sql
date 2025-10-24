-- Migration: Add Dashboard Statistics Views and Functions
-- Date: 2025-10-15
-- Description: Create views and functions for dashboard statistics and analytics

-- ============================================
-- 1. CREATE VIEW FOR LICENSED AGENTS
-- ============================================
CREATE OR REPLACE VIEW v_licensed_agents AS
SELECT DISTINCT
    ia.agent_id,
    ia.name,
    ia.hostname,
    ia.status,
    ia.last_seen,
    al.license_id,
    al.created_at as licensed_at
FROM integrated_agents ia
INNER JOIN agent_licenses al ON ia.agent_id = al.agent_id
WHERE ia.approval_status = 'approved'
ORDER BY ia.name;

COMMENT ON VIEW v_licensed_agents IS 'List of agents that have been licensed';

-- ============================================
-- 2. CREATE FUNCTION FOR USER STATS
-- ============================================
CREATE OR REPLACE FUNCTION get_user_stats()
RETURNS TABLE(
    total_users BIGINT,
    total_admin BIGINT,
    total_operator BIGINT,
    active_users BIGINT
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        COUNT(*)::BIGINT as total_users,
        COUNT(*) FILTER (WHERE role = 'admin')::BIGINT as total_admin,
        COUNT(*) FILTER (WHERE role = 'operator')::BIGINT as total_operator,
        COUNT(*) FILTER (WHERE status = 'active')::BIGINT as active_users
    FROM users
    WHERE deleted_at IS NULL;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION get_user_stats IS 'Get user statistics for dashboard card';

-- ============================================
-- 3. CREATE FUNCTION FOR DASHBOARD STATS
-- ============================================
CREATE OR REPLACE FUNCTION get_dashboard_stats()
RETURNS TABLE(
    total_agents BIGINT,
    total_active_jobs BIGINT,
    total_users BIGINT,
    total_files BIGINT,
    total_data_transferred BIGINT
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        -- Total agents (approved)
        (SELECT COUNT(*)::BIGINT
         FROM integrated_agents
         WHERE approval_status = 'approved') as total_agents,

        -- Total active jobs
        (SELECT COUNT(*)::BIGINT
         FROM sync_jobs
         WHERE status = 'active') as total_active_jobs,

        -- Total users
        (SELECT COUNT(*)::BIGINT
         FROM users
         WHERE deleted_at IS NULL) as total_users,

        -- Total files transferred (completed)
        (SELECT COUNT(*)::BIGINT
         FROM file_transfer_logs
         WHERE status = 'completed') as total_files,

        -- Total data transferred (sum of file sizes for completed transfers)
        (SELECT COALESCE(SUM(file_size), 0)::BIGINT
         FROM file_transfer_logs
         WHERE status = 'completed') as total_data_transferred;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION get_dashboard_stats IS 'Get main dashboard statistics';

-- ============================================
-- 4. CREATE VIEW FOR DAILY FILE TRANSFER STATS (Last 7 days)
-- ============================================
CREATE OR REPLACE VIEW v_daily_file_transfer_stats AS
WITH date_series AS (
    SELECT generate_series(
        CURRENT_DATE - INTERVAL '6 days',
        CURRENT_DATE,
        '1 day'::interval
    )::date as transfer_date
),
daily_stats AS (
    SELECT
        DATE(completed_at) as transfer_date,
        COUNT(*) as file_count,
        COALESCE(SUM(COALESCE(delta_bytes_transferred, file_size)), 0) as total_bytes
    FROM file_transfer_logs
    WHERE status = 'completed'
        AND completed_at >= CURRENT_DATE - INTERVAL '6 days'
        AND completed_at < CURRENT_DATE + INTERVAL '1 day'
    GROUP BY DATE(completed_at)
)
SELECT
    ds.transfer_date,
    COALESCE(dst.file_count, 0) as file_count,
    COALESCE(dst.total_bytes, 0) as total_bytes,
    TO_CHAR(ds.transfer_date, 'YYYY-MM-DD') as date_label,
    TO_CHAR(ds.transfer_date, 'Dy') as day_name
FROM date_series ds
LEFT JOIN daily_stats dst ON ds.transfer_date = dst.transfer_date
ORDER BY ds.transfer_date;

COMMENT ON VIEW v_daily_file_transfer_stats IS 'Daily file transfer statistics for last 7 days';

-- ============================================
-- 5. CREATE VIEW FOR TOP JOBS PERFORMANCE
-- ============================================
CREATE OR REPLACE VIEW v_top_jobs_by_file_count AS
SELECT
    sj.id as job_id,
    sj.name as job_name,
    sj.source_agent_id,
    sj.target_agent_id,
    sj.status as job_status,
    COUNT(ftl.id) as total_files_transferred,
    COALESCE(SUM(ftl.file_size), 0) as total_bytes_transferred,
    MAX(ftl.completed_at) as last_transfer_at
FROM sync_jobs sj
LEFT JOIN file_transfer_logs ftl ON sj.name = ftl.job_name AND ftl.status = 'completed'
GROUP BY sj.id, sj.name, sj.source_agent_id, sj.target_agent_id, sj.status
ORDER BY total_files_transferred DESC
LIMIT 5;

COMMENT ON VIEW v_top_jobs_by_file_count IS 'Top 5 jobs by number of files transferred';

CREATE OR REPLACE VIEW v_top_jobs_by_data_size AS
SELECT
    sj.id as job_id,
    sj.name as job_name,
    sj.source_agent_id,
    sj.target_agent_id,
    sj.status as job_status,
    COUNT(ftl.id) as total_files_transferred,
    COALESCE(SUM(ftl.file_size), 0) as total_bytes_transferred,
    MAX(ftl.completed_at) as last_transfer_at
FROM sync_jobs sj
LEFT JOIN file_transfer_logs ftl ON sj.name = ftl.job_name AND ftl.status = 'completed'
GROUP BY sj.id, sj.name, sj.source_agent_id, sj.target_agent_id, sj.status
ORDER BY total_bytes_transferred DESC
LIMIT 5;

COMMENT ON VIEW v_top_jobs_by_data_size IS 'Top 5 jobs by total data size transferred';

-- ============================================
-- 6. CREATE VIEW FOR RECENT FILE TRANSFER EVENTS
-- ============================================
CREATE OR REPLACE VIEW v_recent_file_transfer_events AS
SELECT
    ftl.id,
    ftl.job_id,
    ftl.job_name,
    ftl.agent_id,
    ftl.file_name,
    ftl.file_path,
    COALESCE(ftl.delta_bytes_transferred, ftl.file_size, 0) as bytes_transferred,
    ftl.file_size,
    ftl.status,
    ftl.action,
    ftl.source_agent_name,
    ftl.destination_agent_name,
    ftl.sync_mode,
    ftl.started_at,
    ftl.completed_at,
    ftl.duration,
    ftl.transfer_rate,
    ftl.error_message,
    ftl.session_id
FROM file_transfer_logs ftl
WHERE ftl.status = 'completed'
ORDER BY ftl.completed_at DESC
LIMIT 5;

COMMENT ON VIEW v_recent_file_transfer_events IS 'Last 5 completed file transfer events';

-- ============================================
-- 7. CREATE INDEXES FOR PERFORMANCE
-- ============================================
-- Index for completed_at queries (if not exists)
CREATE INDEX IF NOT EXISTS idx_file_transfer_logs_completed_at
    ON file_transfer_logs(completed_at DESC)
    WHERE status = 'completed';

-- Index for job_name queries
CREATE INDEX IF NOT EXISTS idx_file_transfer_logs_job_name
    ON file_transfer_logs(job_name)
    WHERE status = 'completed';

-- Index for status queries
CREATE INDEX IF NOT EXISTS idx_file_transfer_logs_status
    ON file_transfer_logs(status);

-- Index for active jobs
CREATE INDEX IF NOT EXISTS idx_sync_jobs_status
    ON sync_jobs(status);

-- Index for approved agents
CREATE INDEX IF NOT EXISTS idx_integrated_agents_approval
    ON integrated_agents(approval_status);

-- ============================================
-- 8. CREATE FUNCTION TO GET ROLE LIST
-- ============================================
CREATE OR REPLACE FUNCTION get_role_list()
RETURNS TABLE(
    role_code VARCHAR,
    role_label VARCHAR,
    role_description TEXT
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        'admin'::VARCHAR as role_code,
        'Administrator'::VARCHAR as role_label,
        'Full access to all features and agents'::TEXT as role_description
    UNION ALL
    SELECT
        'operator'::VARCHAR as role_code,
        'Operator'::VARCHAR as role_label,
        'Limited access to assigned agents only'::TEXT as role_description;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION get_role_list IS 'Get list of available user roles with descriptions';

-- ============================================
-- 9. SAMPLE QUERIES FOR TESTING
-- ============================================

-- Query 1: Get user stats
-- SELECT * FROM get_user_stats();

-- Query 2: Get dashboard stats
-- SELECT * FROM get_dashboard_stats();

-- Query 3: Get daily file transfer stats (last 7 days)
-- SELECT * FROM v_daily_file_transfer_stats;

-- Query 4: Get top jobs by file count
-- SELECT * FROM v_top_jobs_by_file_count;

-- Query 5: Get top jobs by data size
-- SELECT * FROM v_top_jobs_by_data_size;

-- Query 6: Get recent file transfer events
-- SELECT * FROM v_recent_file_transfer_events;

-- Query 7: Get role list
-- SELECT * FROM get_role_list();

-- Query 8: Check licensed agents
-- SELECT * FROM v_licensed_agents;
