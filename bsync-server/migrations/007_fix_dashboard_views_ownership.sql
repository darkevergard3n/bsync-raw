-- Migration: Fix Dashboard Views Ownership and Multi-Destination Support
-- Date: 2025-10-22
-- Description: Recreate views with correct ownership to fix "permission denied for table sync_jobs" error
--              Root cause: Views were created by a different user and don't have access to underlying tables
--              Solution: Drop and recreate views, ensuring they're created by the correct user (bsync)

-- ============================================
-- IMPORTANT: Run this migration as user 'bsync' or as postgres then change ownership
-- ============================================

-- ============================================
-- 1. FIX v_top_jobs_by_file_count
-- ============================================

DROP VIEW IF EXISTS v_top_jobs_by_file_count CASCADE;

CREATE OR REPLACE VIEW v_top_jobs_by_file_count AS
WITH job_destinations AS (
    -- For multi-destination jobs, get first destination
    SELECT DISTINCT ON (job_id)
        job_id,
        destination_agent_id as target_agent_id
    FROM sync_job_destinations
    ORDER BY job_id, id
)
SELECT
    sj.id as job_id,
    sj.name as job_name,
    sj.source_agent_id,
    -- Use target_agent_id for legacy jobs, or first destination for multi-dest jobs
    COALESCE(sj.target_agent_id, jd.target_agent_id) as target_agent_id,
    sj.status as job_status,
    COUNT(ftl.id) as total_files_transferred,
    COALESCE(SUM(ftl.file_size), 0) as total_bytes_transferred,
    MAX(ftl.completed_at) as last_transfer_at
FROM sync_jobs sj
LEFT JOIN job_destinations jd ON sj.id = jd.job_id
LEFT JOIN file_transfer_logs ftl ON sj.name = ftl.job_name AND ftl.status = 'completed'
GROUP BY sj.id, sj.name, sj.source_agent_id, sj.target_agent_id, jd.target_agent_id, sj.status
ORDER BY total_files_transferred DESC
LIMIT 5;

-- Change ownership to bsync user
ALTER VIEW v_top_jobs_by_file_count OWNER TO bsync;

COMMENT ON VIEW v_top_jobs_by_file_count IS 'Top 5 jobs by number of files transferred (supports both legacy and multi-destination models)';

-- ============================================
-- 2. FIX v_top_jobs_by_data_size
-- ============================================

DROP VIEW IF EXISTS v_top_jobs_by_data_size CASCADE;

CREATE OR REPLACE VIEW v_top_jobs_by_data_size AS
WITH job_destinations AS (
    -- For multi-destination jobs, get first destination
    SELECT DISTINCT ON (job_id)
        job_id,
        destination_agent_id as target_agent_id
    FROM sync_job_destinations
    ORDER BY job_id, id
)
SELECT
    sj.id as job_id,
    sj.name as job_name,
    sj.source_agent_id,
    -- Use target_agent_id for legacy jobs, or first destination for multi-dest jobs
    COALESCE(sj.target_agent_id, jd.target_agent_id) as target_agent_id,
    sj.status as job_status,
    COUNT(ftl.id) as total_files_transferred,
    COALESCE(SUM(ftl.file_size), 0) as total_bytes_transferred,
    MAX(ftl.completed_at) as last_transfer_at
FROM sync_jobs sj
LEFT JOIN job_destinations jd ON sj.id = jd.job_id
LEFT JOIN file_transfer_logs ftl ON sj.name = ftl.job_name AND ftl.status = 'completed'
GROUP BY sj.id, sj.name, sj.source_agent_id, sj.target_agent_id, jd.target_agent_id, sj.status
ORDER BY total_bytes_transferred DESC
LIMIT 5;

-- Change ownership to bsync user
ALTER VIEW v_top_jobs_by_data_size OWNER TO bsync;

COMMENT ON VIEW v_top_jobs_by_data_size IS 'Top 5 jobs by total data size transferred (supports both legacy and multi-destination models)';

-- ============================================
-- 3. RECREATE OTHER DASHBOARD VIEWS WITH CORRECT OWNERSHIP
-- ============================================

-- Fix v_daily_file_transfer_stats
DROP VIEW IF EXISTS v_daily_file_transfer_stats CASCADE;

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
    TO_CHAR(ds.transfer_date, 'DD Mon') as date_label,
    TO_CHAR(ds.transfer_date, 'Dy') as day_name
FROM date_series ds
LEFT JOIN daily_stats dst ON ds.transfer_date = dst.transfer_date
ORDER BY ds.transfer_date;

ALTER VIEW v_daily_file_transfer_stats OWNER TO bsync;
COMMENT ON VIEW v_daily_file_transfer_stats IS 'Daily file transfer statistics for last 7 days';

-- Fix v_recent_file_transfer_events
DROP VIEW IF EXISTS v_recent_file_transfer_events CASCADE;

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

ALTER VIEW v_recent_file_transfer_events OWNER TO bsync;
COMMENT ON VIEW v_recent_file_transfer_events IS 'Last 5 completed file transfer events';

-- Fix v_licensed_agents
DROP VIEW IF EXISTS v_licensed_agents CASCADE;

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

ALTER VIEW v_licensed_agents OWNER TO bsync;
COMMENT ON VIEW v_licensed_agents IS 'List of agents that have been licensed';

-- ============================================
-- 4. GRANT PERMISSIONS
-- ============================================

GRANT SELECT ON v_top_jobs_by_file_count TO PUBLIC;
GRANT SELECT ON v_top_jobs_by_data_size TO PUBLIC;
GRANT SELECT ON v_daily_file_transfer_stats TO PUBLIC;
GRANT SELECT ON v_recent_file_transfer_events TO PUBLIC;
GRANT SELECT ON v_licensed_agents TO PUBLIC;

-- ============================================
-- 5. VERIFICATION QUERIES
-- ============================================

-- Verify view ownership
-- SELECT viewname, viewowner FROM pg_views WHERE viewname LIKE 'v_%' ORDER BY viewname;

-- Test queries
-- SELECT * FROM v_top_jobs_by_file_count;
-- SELECT * FROM v_top_jobs_by_data_size;
