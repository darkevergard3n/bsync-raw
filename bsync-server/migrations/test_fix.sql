-- Quick Test Script
-- Run this to test if the views work after applying migration 007

-- 1. Check current view ownership
SELECT viewname, viewowner
FROM pg_views
WHERE viewname IN ('v_top_jobs_by_file_count', 'v_top_jobs_by_data_size')
ORDER BY viewname;

-- 2. Try to query the views
SELECT 'Testing v_top_jobs_by_file_count' as test;
SELECT * FROM v_top_jobs_by_file_count LIMIT 1;

SELECT 'Testing v_top_jobs_by_data_size' as test;
SELECT * FROM v_top_jobs_by_data_size LIMIT 1;

-- 3. Check if there are any jobs in sync_jobs table
SELECT 'Checking sync_jobs table' as test;
SELECT id, name, source_agent_id, target_agent_id, is_multi_destination, status
FROM sync_jobs
LIMIT 5;

-- 4. Check if there are any file transfer logs
SELECT 'Checking file_transfer_logs' as test;
SELECT COUNT(*) as total_transfers,
       COUNT(*) FILTER (WHERE status = 'completed') as completed_transfers
FROM file_transfer_logs;
