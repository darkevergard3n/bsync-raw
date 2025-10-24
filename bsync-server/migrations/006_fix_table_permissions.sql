-- Migration: Fix Table Permissions for Dashboard Views
-- Date: 2025-10-22
-- Description: Grant necessary permissions on tables and views to fix permission denied errors

-- ============================================
-- 1. GRANT PERMISSIONS ON CORE TABLES
-- ============================================

-- Grant SELECT permission on sync_jobs table
-- This is required for dashboard views that reference sync_jobs
GRANT SELECT ON sync_jobs TO PUBLIC;

-- Grant permissions on related tables used by dashboard
GRANT SELECT ON integrated_agents TO PUBLIC;
GRANT SELECT ON file_transfer_logs TO PUBLIC;
GRANT SELECT ON sync_job_destinations TO PUBLIC;
GRANT SELECT ON agent_licenses TO PUBLIC;
GRANT SELECT ON users TO PUBLIC;
GRANT SELECT ON user_agent_assignments TO PUBLIC;

-- ============================================
-- 2. GRANT PERMISSIONS ON VIEWS
-- ============================================

-- Grant SELECT on dashboard views
GRANT SELECT ON v_licensed_agents TO PUBLIC;
GRANT SELECT ON v_daily_file_transfer_stats TO PUBLIC;
GRANT SELECT ON v_top_jobs_by_file_count TO PUBLIC;
GRANT SELECT ON v_top_jobs_by_data_size TO PUBLIC;
GRANT SELECT ON v_recent_file_transfer_events TO PUBLIC;
GRANT SELECT ON v_sync_jobs_with_destinations TO PUBLIC;
GRANT SELECT ON v_sync_jobs_summary TO PUBLIC;

-- ============================================
-- 3. GRANT EXECUTE ON FUNCTIONS
-- ============================================

-- Grant EXECUTE permission on dashboard functions
GRANT EXECUTE ON FUNCTION get_user_stats() TO PUBLIC;
GRANT EXECUTE ON FUNCTION get_dashboard_stats() TO PUBLIC;
GRANT EXECUTE ON FUNCTION get_role_list() TO PUBLIC;
GRANT EXECUTE ON FUNCTION update_destination_stats(INTEGER, VARCHAR, BIGINT, BIGINT) TO PUBLIC;
GRANT EXECUTE ON FUNCTION set_destination_error(INTEGER, VARCHAR, TEXT) TO PUBLIC;

-- ============================================
-- 4. GRANT USAGE ON SEQUENCES
-- ============================================

-- Grant USAGE and SELECT on sequences (if needed for INSERT operations)
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO PUBLIC;

-- ============================================
-- 5. SET DEFAULT PRIVILEGES FOR FUTURE OBJECTS
-- ============================================

-- Ensure future tables created in public schema are accessible
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO PUBLIC;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT EXECUTE ON FUNCTIONS TO PUBLIC;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO PUBLIC;

-- ============================================
-- NOTES
-- ============================================
-- This migration fixes the "permission denied for table sync_jobs" error
-- by granting necessary SELECT permissions to PUBLIC.
--
-- In production, you may want to create a specific application user
-- and grant permissions only to that user instead of PUBLIC.
--
-- Example for specific user:
-- GRANT SELECT ON sync_jobs TO bsync_app_user;
-- GRANT SELECT ON integrated_agents TO bsync_app_user;
-- etc.
