-- Migration: Add Session Tracking and Delta Sync Enhancement
-- Date: 2025-10-03
-- Description: Adds sync session tracking and separates delta bytes from full file size

-- ============================================
-- 1. ALTER file_transfer_logs TABLE
-- ============================================
-- Add delta tracking columns to distinguish actual transferred bytes from full file size
ALTER TABLE file_transfer_logs
ADD COLUMN IF NOT EXISTS delta_bytes_transferred BIGINT DEFAULT 0,
ADD COLUMN IF NOT EXISTS delta_bytes_completed BIGINT DEFAULT 0,
ADD COLUMN IF NOT EXISTS full_file_size BIGINT,
ADD COLUMN IF NOT EXISTS session_id VARCHAR(255),
ADD COLUMN IF NOT EXISTS compression_ratio DECIMAL(5,2);

-- Update existing records: set full_file_size from current file_size
UPDATE file_transfer_logs
SET full_file_size = file_size
WHERE full_file_size IS NULL;

-- Add index for session queries
CREATE INDEX IF NOT EXISTS idx_file_transfer_logs_session_id ON file_transfer_logs(session_id);
CREATE INDEX IF NOT EXISTS idx_file_transfer_logs_job_session ON file_transfer_logs(job_id, session_id);

-- Add comments for clarity
COMMENT ON COLUMN file_transfer_logs.delta_bytes_transferred IS 'Actual bytes transferred (delta sync efficiency)';
COMMENT ON COLUMN file_transfer_logs.delta_bytes_completed IS 'Delta bytes completed so far';
COMMENT ON COLUMN file_transfer_logs.full_file_size IS 'Full file size (reference)';
COMMENT ON COLUMN file_transfer_logs.session_id IS 'Unique session identifier for grouping transfers';
COMMENT ON COLUMN file_transfer_logs.compression_ratio IS 'Delta efficiency ratio (delta/full)';

-- ============================================
-- 2. CREATE sync_sessions TABLE
-- ============================================
CREATE TABLE IF NOT EXISTS sync_sessions (
    -- Primary identification
    session_id VARCHAR(255) PRIMARY KEY,
    job_id VARCHAR(255) NOT NULL,
    job_name VARCHAR(255),
    agent_id VARCHAR(255) NOT NULL,

    -- Session timing
    session_start_time TIMESTAMP NOT NULL,
    session_end_time TIMESTAMP,
    total_duration_seconds INTEGER,

    -- Scan statistics
    scan_start_time TIMESTAMP,
    scan_end_time TIMESTAMP,
    scan_duration_seconds INTEGER,
    files_scanned BIGINT DEFAULT 0,
    bytes_scanned BIGINT DEFAULT 0,

    -- Transfer statistics (delta-aware)
    transfer_start_time TIMESTAMP,
    transfer_end_time TIMESTAMP,
    transfer_duration_seconds INTEGER,
    files_transferred BIGINT DEFAULT 0,
    total_delta_bytes BIGINT DEFAULT 0,           -- Actual bytes transferred (delta)
    total_full_file_size BIGINT DEFAULT 0,        -- Combined file sizes (reference)
    compression_ratio DECIMAL(5,2),               -- Delta efficiency (delta/full)

    -- Transfer performance
    average_transfer_rate DECIMAL(15,2),          -- Bytes per second
    peak_transfer_rate DECIMAL(15,2),             -- Maximum rate observed

    -- Session state
    current_state VARCHAR(50),                    -- idle, scanning, syncing, completed, failed
    status VARCHAR(50) NOT NULL DEFAULT 'active', -- active, completed, failed
    error_message TEXT,

    -- Metadata
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Add indexes for performance
CREATE INDEX IF NOT EXISTS idx_sync_sessions_job_id ON sync_sessions(job_id);
CREATE INDEX IF NOT EXISTS idx_sync_sessions_agent_id ON sync_sessions(agent_id);
CREATE INDEX IF NOT EXISTS idx_sync_sessions_job_agent ON sync_sessions(job_id, agent_id);
CREATE INDEX IF NOT EXISTS idx_sync_sessions_start_time ON sync_sessions(session_start_time DESC);
CREATE INDEX IF NOT EXISTS idx_sync_sessions_status ON sync_sessions(status);

-- Add comments
COMMENT ON TABLE sync_sessions IS 'Tracks individual sync sessions with comprehensive statistics';
COMMENT ON COLUMN sync_sessions.session_id IS 'Unique identifier: {job_id}-{timestamp}-{random}';
COMMENT ON COLUMN sync_sessions.total_delta_bytes IS 'Actual bytes transferred using delta sync';
COMMENT ON COLUMN sync_sessions.total_full_file_size IS 'Sum of full file sizes for reference';
COMMENT ON COLUMN sync_sessions.compression_ratio IS 'Efficiency ratio: delta_bytes / full_file_size';

-- ============================================
-- 3. CREATE sync_session_events TABLE
-- ============================================
-- Track state changes within a session for detailed timeline
CREATE TABLE IF NOT EXISTS sync_session_events (
    id SERIAL PRIMARY KEY,
    session_id VARCHAR(255) NOT NULL REFERENCES sync_sessions(session_id) ON DELETE CASCADE,
    event_type VARCHAR(100) NOT NULL,             -- session_started, scan_started, scan_completed, transfer_started, transfer_completed, session_completed
    event_state VARCHAR(50),                      -- idle, scanning, syncing, etc.
    event_data JSONB,                             -- Additional event-specific data
    timestamp TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Add indexes
CREATE INDEX IF NOT EXISTS idx_sync_session_events_session_id ON sync_session_events(session_id);
CREATE INDEX IF NOT EXISTS idx_sync_session_events_type ON sync_session_events(event_type);
CREATE INDEX IF NOT EXISTS idx_sync_session_events_timestamp ON sync_session_events(timestamp DESC);

-- Add comments
COMMENT ON TABLE sync_session_events IS 'Detailed timeline of events within sync sessions';
COMMENT ON COLUMN sync_session_events.event_type IS 'Type of session event (lifecycle stages)';

-- ============================================
-- 4. CREATE VIEWS FOR ANALYTICS
-- ============================================

-- View: Session Summary with Delta Efficiency
CREATE OR REPLACE VIEW v_session_summary AS
SELECT
    s.session_id,
    s.job_id,
    s.job_name,
    s.agent_id,
    s.session_start_time,
    s.session_end_time,
    s.total_duration_seconds,
    s.files_transferred,
    s.total_delta_bytes,
    s.total_full_file_size,
    s.compression_ratio,
    s.average_transfer_rate,
    s.status,
    -- Calculate data saved by delta sync
    (s.total_full_file_size - s.total_delta_bytes) as bytes_saved_by_delta,
    -- Calculate efficiency percentage
    CASE
        WHEN s.total_full_file_size > 0
        THEN ROUND((1 - (s.total_delta_bytes::DECIMAL / s.total_full_file_size)) * 100, 2)
        ELSE 0
    END as efficiency_percentage
FROM sync_sessions s
ORDER BY s.session_start_time DESC;

COMMENT ON VIEW v_session_summary IS 'Session overview with delta sync efficiency metrics';

-- View: Recent Session Activity
CREATE OR REPLACE VIEW v_recent_sessions AS
SELECT
    s.session_id,
    s.job_id,
    s.job_name,
    s.agent_id,
    s.session_start_time,
    s.status,
    s.files_transferred,
    s.total_delta_bytes,
    s.average_transfer_rate,
    s.current_state,
    -- Human-readable duration
    CASE
        WHEN s.total_duration_seconds IS NOT NULL
        THEN CONCAT(
            FLOOR(s.total_duration_seconds / 3600), 'h ',
            FLOOR((s.total_duration_seconds % 3600) / 60), 'm ',
            (s.total_duration_seconds % 60), 's'
        )
        ELSE 'In Progress'
    END as duration_display
FROM sync_sessions s
WHERE s.session_start_time >= NOW() - INTERVAL '24 hours'
ORDER BY s.session_start_time DESC;

COMMENT ON VIEW v_recent_sessions IS 'Sessions from last 24 hours with readable formatting';

-- ============================================
-- 5. CREATE FUNCTIONS FOR SESSION MANAGEMENT
-- ============================================

-- Function: Calculate session statistics
CREATE OR REPLACE FUNCTION update_session_statistics(p_session_id VARCHAR)
RETURNS void AS $$
BEGIN
    UPDATE sync_sessions
    SET
        files_transferred = (
            SELECT COUNT(DISTINCT file_name)
            FROM file_transfer_logs
            WHERE session_id = p_session_id
            AND status = 'completed'
        ),
        total_delta_bytes = (
            SELECT COALESCE(SUM(delta_bytes_transferred), 0)
            FROM file_transfer_logs
            WHERE session_id = p_session_id
        ),
        total_full_file_size = (
            SELECT COALESCE(SUM(full_file_size), 0)
            FROM file_transfer_logs
            WHERE session_id = p_session_id
        ),
        compression_ratio = (
            SELECT CASE
                WHEN SUM(full_file_size) > 0
                THEN ROUND(SUM(delta_bytes_transferred)::DECIMAL / SUM(full_file_size), 4)
                ELSE 0
            END
            FROM file_transfer_logs
            WHERE session_id = p_session_id
        ),
        updated_at = NOW()
    WHERE session_id = p_session_id;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION update_session_statistics IS 'Recalculate aggregated statistics for a session';

-- Function: Auto-complete session when all transfers done
CREATE OR REPLACE FUNCTION auto_complete_session()
RETURNS TRIGGER AS $$
BEGIN
    -- If this was the last file to complete in the session
    IF NEW.status = 'completed' AND NEW.session_id IS NOT NULL THEN
        -- Check if all files in session are completed
        IF NOT EXISTS (
            SELECT 1 FROM file_transfer_logs
            WHERE session_id = NEW.session_id
            AND status NOT IN ('completed', 'failed')
        ) THEN
            -- Update session to completed
            UPDATE sync_sessions
            SET
                status = 'completed',
                session_end_time = NOW(),
                total_duration_seconds = EXTRACT(EPOCH FROM (NOW() - session_start_time))::INTEGER,
                current_state = 'idle',
                updated_at = NOW()
            WHERE session_id = NEW.session_id
            AND status = 'active';

            -- Update statistics
            PERFORM update_session_statistics(NEW.session_id);
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger for auto-completion
DROP TRIGGER IF EXISTS trg_auto_complete_session ON file_transfer_logs;
CREATE TRIGGER trg_auto_complete_session
    AFTER UPDATE OR INSERT ON file_transfer_logs
    FOR EACH ROW
    WHEN (NEW.session_id IS NOT NULL)
    EXECUTE FUNCTION auto_complete_session();

COMMENT ON FUNCTION auto_complete_session IS 'Automatically mark session as completed when all transfers finish';

-- ============================================
-- 6. SAMPLE QUERIES FOR DOCUMENTATION
-- ============================================

-- Query 1: Get all sessions for a job
-- SELECT * FROM v_session_summary WHERE job_id = 'job-123' ORDER BY session_start_time DESC;

-- Query 2: Get session details with efficiency
-- SELECT
--     session_id,
--     files_transferred,
--     pg_size_pretty(total_delta_bytes) as transferred,
--     pg_size_pretty(total_full_file_size) as full_size,
--     pg_size_pretty(bytes_saved_by_delta) as saved,
--     efficiency_percentage || '%' as efficiency
-- FROM v_session_summary
-- WHERE session_id = 'session-xxx';

-- Query 3: Get top efficient sessions
-- SELECT
--     job_name,
--     session_id,
--     efficiency_percentage,
--     pg_size_pretty(bytes_saved_by_delta) as data_saved
-- FROM v_session_summary
-- WHERE status = 'completed'
-- ORDER BY efficiency_percentage DESC
-- LIMIT 10;
