-- Migration: Add Multi-Destination Support for Sync Jobs
-- Date: 2025-10-20
-- Description: Adds support for single source to multiple destinations in sync jobs

-- ============================================
-- 1. CREATE sync_job_destinations TABLE
-- ============================================
CREATE TABLE IF NOT EXISTS sync_job_destinations (
    id SERIAL PRIMARY KEY,
    job_id INTEGER NOT NULL,
    destination_agent_id VARCHAR(255) NOT NULL,
    destination_path TEXT NOT NULL,
    destination_device_id VARCHAR(255),
    destination_ip_address VARCHAR(255),

    -- Status tracking per destination
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    last_sync_status VARCHAR(50),

    -- Sync statistics per destination
    last_sync_time TIMESTAMP,
    files_synced BIGINT DEFAULT 0,
    bytes_synced BIGINT DEFAULT 0,

    -- Error tracking
    last_error TEXT,
    error_count INTEGER DEFAULT 0,

    -- Metadata
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),

    -- Foreign key constraint (will be added if sync_jobs table exists)
    CONSTRAINT fk_sync_job_destinations_job_id
        FOREIGN KEY (job_id)
        REFERENCES sync_jobs(id)
        ON DELETE CASCADE
);

-- Add comments
COMMENT ON TABLE sync_job_destinations IS 'Stores multiple destinations for a single sync job (1-to-N relationship)';
COMMENT ON COLUMN sync_job_destinations.job_id IS 'Reference to parent sync job';
COMMENT ON COLUMN sync_job_destinations.destination_agent_id IS 'Agent ID that will receive synced data';
COMMENT ON COLUMN sync_job_destinations.destination_path IS 'Path on destination agent where files will be synced';
COMMENT ON COLUMN sync_job_destinations.destination_device_id IS 'Syncthing device ID of destination agent';
COMMENT ON COLUMN sync_job_destinations.destination_ip_address IS 'IP address of destination agent for P2P connection';
COMMENT ON COLUMN sync_job_destinations.status IS 'Status: active, paused, failed';
COMMENT ON COLUMN sync_job_destinations.last_sync_status IS 'Last sync result: idle, scanning, syncing, completed, error';

-- ============================================
-- 2. CREATE INDEXES
-- ============================================
CREATE INDEX IF NOT EXISTS idx_sync_job_destinations_job_id ON sync_job_destinations(job_id);
CREATE INDEX IF NOT EXISTS idx_sync_job_destinations_agent_id ON sync_job_destinations(destination_agent_id);
CREATE INDEX IF NOT EXISTS idx_sync_job_destinations_status ON sync_job_destinations(status);
CREATE INDEX IF NOT EXISTS idx_sync_job_destinations_job_status ON sync_job_destinations(job_id, status);

-- ============================================
-- 3. MIGRATE EXISTING DATA (if sync_jobs table exists)
-- ============================================
-- Copy existing single destination to new table
INSERT INTO sync_job_destinations (
    job_id,
    destination_agent_id,
    destination_path,
    status,
    created_at,
    updated_at
)
SELECT
    id,
    target_agent_id,
    target_path,
    status,
    created_at,
    updated_at
FROM sync_jobs
WHERE target_agent_id IS NOT NULL
ON CONFLICT DO NOTHING;

-- ============================================
-- 4. MAKE target_agent_id and target_path NULLABLE
-- ============================================
-- For multi-destination jobs, these fields will be NULL
-- and destinations will be stored in sync_job_destinations table
ALTER TABLE sync_jobs
ALTER COLUMN target_agent_id DROP NOT NULL;

ALTER TABLE sync_jobs
ALTER COLUMN target_path DROP NOT NULL;

COMMENT ON COLUMN sync_jobs.target_agent_id IS 'Legacy single destination agent ID (NULL for multi-destination jobs)';
COMMENT ON COLUMN sync_jobs.target_path IS 'Legacy single destination path (NULL for multi-destination jobs)';

-- ============================================
-- 5. ADD COLUMN FOR MULTI-DESTINATION FLAG
-- ============================================
-- Add flag to indicate if job uses multi-destination model
ALTER TABLE sync_jobs
ADD COLUMN IF NOT EXISTS is_multi_destination BOOLEAN DEFAULT false;

COMMENT ON COLUMN sync_jobs.is_multi_destination IS 'True if job uses sync_job_destinations table, false if using legacy target_agent_id';

-- Mark migrated jobs as multi-destination
UPDATE sync_jobs
SET is_multi_destination = true
WHERE id IN (SELECT DISTINCT job_id FROM sync_job_destinations);

-- ============================================
-- 6. CREATE VIEW FOR EASY QUERYING
-- ============================================
CREATE OR REPLACE VIEW v_sync_jobs_with_destinations AS
SELECT
    sj.id as job_id,
    sj.name as job_name,
    sj.source_agent_id,
    sj.source_path,
    sj.sync_type,
    sj.status as job_status,
    sj.schedule_type,
    sj.is_multi_destination,
    sjd.id as destination_id,
    sjd.destination_agent_id,
    sjd.destination_path,
    sjd.destination_device_id,
    sjd.destination_ip_address,
    sjd.status as destination_status,
    sjd.last_sync_status,
    sjd.last_sync_time,
    sjd.files_synced,
    sjd.bytes_synced,
    sjd.last_error,
    sj.created_at,
    sj.updated_at
FROM sync_jobs sj
LEFT JOIN sync_job_destinations sjd ON sj.id = sjd.job_id
WHERE sj.is_multi_destination = true;

COMMENT ON VIEW v_sync_jobs_with_destinations IS 'Flattened view of jobs with their destinations';

-- ============================================
-- 7. CREATE VIEW FOR JOB SUMMARY
-- ============================================
CREATE OR REPLACE VIEW v_sync_jobs_summary AS
SELECT
    sj.id as job_id,
    sj.name as job_name,
    sj.source_agent_id,
    sj.source_path,
    sj.sync_type,
    sj.status as job_status,
    sj.schedule_type,
    sj.is_multi_destination,
    COUNT(sjd.id) as destination_count,
    COUNT(CASE WHEN sjd.status = 'active' THEN 1 END) as active_destinations,
    COUNT(CASE WHEN sjd.status = 'paused' THEN 1 END) as paused_destinations,
    COUNT(CASE WHEN sjd.status = 'failed' THEN 1 END) as failed_destinations,
    SUM(sjd.files_synced) as total_files_synced,
    SUM(sjd.bytes_synced) as total_bytes_synced,
    MAX(sjd.last_sync_time) as last_sync_time,
    sj.created_at,
    sj.updated_at
FROM sync_jobs sj
LEFT JOIN sync_job_destinations sjd ON sj.id = sjd.job_id
WHERE sj.is_multi_destination = true
GROUP BY sj.id, sj.name, sj.source_agent_id, sj.source_path,
         sj.sync_type, sj.status, sj.schedule_type, sj.is_multi_destination,
         sj.created_at, sj.updated_at;

COMMENT ON VIEW v_sync_jobs_summary IS 'Aggregated summary of multi-destination jobs';

-- ============================================
-- 8. CREATE FUNCTION TO UPDATE DESTINATION STATS
-- ============================================
CREATE OR REPLACE FUNCTION update_destination_stats(
    p_job_id INTEGER,
    p_destination_agent_id VARCHAR,
    p_files_synced BIGINT,
    p_bytes_synced BIGINT
)
RETURNS void AS $$
BEGIN
    UPDATE sync_job_destinations
    SET
        files_synced = p_files_synced,
        bytes_synced = p_bytes_synced,
        last_sync_time = NOW(),
        last_sync_status = 'completed',
        updated_at = NOW()
    WHERE job_id = p_job_id
      AND destination_agent_id = p_destination_agent_id;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION update_destination_stats IS 'Update sync statistics for a specific destination';

-- ============================================
-- 9. CREATE FUNCTION TO SET DESTINATION ERROR
-- ============================================
CREATE OR REPLACE FUNCTION set_destination_error(
    p_job_id INTEGER,
    p_destination_agent_id VARCHAR,
    p_error_message TEXT
)
RETURNS void AS $$
BEGIN
    UPDATE sync_job_destinations
    SET
        last_error = p_error_message,
        error_count = error_count + 1,
        last_sync_status = 'error',
        updated_at = NOW()
    WHERE job_id = p_job_id
      AND destination_agent_id = p_destination_agent_id;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION set_destination_error IS 'Record an error for a specific destination';

-- ============================================
-- 10. CREATE TRIGGER FOR UPDATED_AT
-- ============================================
CREATE OR REPLACE FUNCTION update_modified_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_sync_job_destinations_updated_at ON sync_job_destinations;
CREATE TRIGGER trg_sync_job_destinations_updated_at
    BEFORE UPDATE ON sync_job_destinations
    FOR EACH ROW
    EXECUTE FUNCTION update_modified_column();

-- ============================================
-- 11. SAMPLE QUERIES FOR DOCUMENTATION
-- ============================================

-- Query 1: Get all destinations for a job
-- SELECT * FROM v_sync_jobs_with_destinations WHERE job_id = 123;

-- Query 2: Get job summary with destination counts
-- SELECT * FROM v_sync_jobs_summary WHERE job_id = 123;

-- Query 3: Get jobs with failed destinations
-- SELECT job_id, job_name, failed_destinations
-- FROM v_sync_jobs_summary
-- WHERE failed_destinations > 0;

-- Query 4: Get total sync statistics across all destinations
-- SELECT
--     job_name,
--     destination_count,
--     total_files_synced,
--     pg_size_pretty(total_bytes_synced) as total_data_synced
-- FROM v_sync_jobs_summary
-- ORDER BY total_bytes_synced DESC;

-- Query 5: Add destination to existing job
-- INSERT INTO sync_job_destinations (job_id, destination_agent_id, destination_path)
-- VALUES (123, 'agent-456', '/data/backup3');

-- Query 6: Remove destination from job
-- DELETE FROM sync_job_destinations
-- WHERE job_id = 123 AND destination_agent_id = 'agent-456';
