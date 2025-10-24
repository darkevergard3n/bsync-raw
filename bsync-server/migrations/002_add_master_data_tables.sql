-- Migration: Add Master Data Tables
-- Date: 2025-10-09
-- Description: Create master data tables for filter options (sync_status, etc.)

-- ============================================
-- 1. CREATE sync_status_master TABLE
-- ============================================
CREATE TABLE IF NOT EXISTS sync_status_master (
    id SERIAL PRIMARY KEY,
    code VARCHAR(50) UNIQUE NOT NULL,
    label VARCHAR(100) NOT NULL,
    description TEXT,
    display_order INTEGER NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Add comments
COMMENT ON TABLE sync_status_master IS 'Master data for sync status filter options';
COMMENT ON COLUMN sync_status_master.code IS 'Internal code used in API (e.g., "Complete", "Pending", "Partial")';
COMMENT ON COLUMN sync_status_master.label IS 'Display label for UI';
COMMENT ON COLUMN sync_status_master.description IS 'Description of the status';
COMMENT ON COLUMN sync_status_master.display_order IS 'Order for displaying in UI dropdown';

-- ============================================
-- 2. INSERT MASTER DATA
-- ============================================
INSERT INTO sync_status_master (code, label, description, display_order, is_active) VALUES
('Complete', 'Complete', 'All files have been successfully synchronized', 1, true),
('Partial', 'Partial', 'Some files have been synchronized, sync in progress', 2, true),
('Pending', 'Pending', 'Synchronization has not started or no files transferred yet', 3, true)
ON CONFLICT (code) DO NOTHING;

-- ============================================
-- 3. CREATE INDEXES
-- ============================================
CREATE INDEX IF NOT EXISTS idx_sync_status_master_code ON sync_status_master(code);
CREATE INDEX IF NOT EXISTS idx_sync_status_master_active ON sync_status_master(is_active);
CREATE INDEX IF NOT EXISTS idx_sync_status_master_order ON sync_status_master(display_order);

-- ============================================
-- 4. CREATE job_status_master TABLE (for future use)
-- ============================================
CREATE TABLE IF NOT EXISTS job_status_master (
    id SERIAL PRIMARY KEY,
    code VARCHAR(50) UNIQUE NOT NULL,
    label VARCHAR(100) NOT NULL,
    description TEXT,
    display_order INTEGER NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

COMMENT ON TABLE job_status_master IS 'Master data for job status (active, paused, completed, failed)';

INSERT INTO job_status_master (code, label, description, display_order, is_active) VALUES
('active', 'Active', 'Job is currently active and running', 1, true),
('paused', 'Paused', 'Job is temporarily paused', 2, true),
('completed', 'Completed', 'Job has completed successfully', 3, true),
('failed', 'Failed', 'Job has failed', 4, true),
('pending', 'Pending', 'Job is pending to start', 5, true)
ON CONFLICT (code) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_job_status_master_code ON job_status_master(code);
CREATE INDEX IF NOT EXISTS idx_job_status_master_active ON job_status_master(is_active);
