-- Migration: Add User Management and Authentication
-- Date: 2025-10-13
-- Description: Create user management tables with role-based access control and agent assignment

-- ============================================
-- 1. CREATE users TABLE
-- ============================================
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(100) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    fullname VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL CHECK (role IN ('admin', 'operator')),
    status VARCHAR(50) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'suspended')),
    last_login TIMESTAMP,

    -- Audit fields
    created_at TIMESTAMP DEFAULT NOW(),
    created_by INTEGER REFERENCES users(id),
    updated_at TIMESTAMP DEFAULT NOW(),
    updated_by INTEGER REFERENCES users(id),
    deleted_at TIMESTAMP
);

-- Add indexes for performance
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at);

-- Add comments
COMMENT ON TABLE users IS 'User accounts with role-based access control';
COMMENT ON COLUMN users.role IS 'User role: admin (full access) or operator (limited to assigned agents)';
COMMENT ON COLUMN users.status IS 'User account status: active, inactive, or suspended';
COMMENT ON COLUMN users.deleted_at IS 'Soft delete timestamp (NULL if not deleted)';

-- ============================================
-- 2. CREATE user_agent_assignments TABLE
-- ============================================
CREATE TABLE IF NOT EXISTS user_agent_assignments (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    agent_id VARCHAR(255) NOT NULL REFERENCES integrated_agents(agent_id) ON DELETE CASCADE,

    -- Assignment metadata
    assigned_at TIMESTAMP DEFAULT NOW(),
    assigned_by INTEGER REFERENCES users(id),
    is_active BOOLEAN NOT NULL DEFAULT true,

    -- Audit fields
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),

    -- Ensure no duplicate assignments
    UNIQUE(user_id, agent_id)
);

-- Add indexes for performance
CREATE INDEX IF NOT EXISTS idx_user_agent_assignments_user_id ON user_agent_assignments(user_id);
CREATE INDEX IF NOT EXISTS idx_user_agent_assignments_agent_id ON user_agent_assignments(agent_id);
CREATE INDEX IF NOT EXISTS idx_user_agent_assignments_active ON user_agent_assignments(is_active);
CREATE INDEX IF NOT EXISTS idx_user_agent_assignments_user_active ON user_agent_assignments(user_id, is_active);

-- Add comments
COMMENT ON TABLE user_agent_assignments IS 'Maps operators to their assigned agents for access control';
COMMENT ON COLUMN user_agent_assignments.is_active IS 'Whether assignment is currently active (allows soft disable)';

-- ============================================
-- 3. CREATE user_activity_logs TABLE (Simple Audit)
-- ============================================
CREATE TABLE IF NOT EXISTS user_activity_logs (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
    username VARCHAR(100),
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(50),
    resource_id VARCHAR(255),
    ip_address VARCHAR(45),
    user_agent TEXT,
    details JSONB,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Add indexes
CREATE INDEX IF NOT EXISTS idx_user_activity_logs_user_id ON user_activity_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_user_activity_logs_action ON user_activity_logs(action);
CREATE INDEX IF NOT EXISTS idx_user_activity_logs_created_at ON user_activity_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_activity_logs_resource ON user_activity_logs(resource_type, resource_id);

-- Add comments
COMMENT ON TABLE user_activity_logs IS 'Simple audit trail for user actions';
COMMENT ON COLUMN user_activity_logs.action IS 'Action performed (e.g., login, create_user, create_job, etc.)';
COMMENT ON COLUMN user_activity_logs.resource_type IS 'Type of resource affected (e.g., user, job, agent)';
COMMENT ON COLUMN user_activity_logs.resource_id IS 'ID of the affected resource';

-- ============================================
-- 4. CREATE VIEWS FOR USER MANAGEMENT
-- ============================================

-- View: Users with their assigned agents
CREATE OR REPLACE VIEW v_users_with_agents AS
SELECT
    u.id,
    u.username,
    u.email,
    u.fullname,
    u.role,
    u.status,
    u.last_login,
    u.created_at,
    u.updated_at,
    u.deleted_at,
    creator.username as created_by_username,
    updater.username as updated_by_username,
    COALESCE(
        json_agg(
            json_build_object(
                'agent_id', uaa.agent_id,
                'agent_name', ia.name,
                'assigned_at', uaa.assigned_at,
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
WHERE u.deleted_at IS NULL
GROUP BY u.id, creator.username, updater.username;

COMMENT ON VIEW v_users_with_agents IS 'User list with assigned agents and metadata';

-- View: Agent assignments with user details
CREATE OR REPLACE VIEW v_agent_assignments AS
SELECT
    uaa.id,
    uaa.user_id,
    u.username,
    u.fullname,
    u.role,
    u.status as user_status,
    uaa.agent_id,
    ia.name as agent_name,
    ia.status as agent_status,
    ia.hostname,
    uaa.assigned_at,
    uaa.is_active,
    assigner.username as assigned_by_username
FROM user_agent_assignments uaa
JOIN users u ON uaa.user_id = u.id
LEFT JOIN integrated_agents ia ON uaa.agent_id = ia.agent_id
LEFT JOIN users assigner ON uaa.assigned_by = assigner.id
WHERE u.deleted_at IS NULL
ORDER BY uaa.assigned_at DESC;

COMMENT ON VIEW v_agent_assignments IS 'All agent assignments with full context';

-- View: Recent user activity (last 30 days)
CREATE OR REPLACE VIEW v_recent_user_activity AS
SELECT
    ual.id,
    ual.user_id,
    ual.username,
    ual.action,
    ual.resource_type,
    ual.resource_id,
    ual.ip_address,
    ual.created_at,
    u.fullname,
    u.role
FROM user_activity_logs ual
LEFT JOIN users u ON ual.user_id = u.id
WHERE ual.created_at >= NOW() - INTERVAL '30 days'
ORDER BY ual.created_at DESC;

COMMENT ON VIEW v_recent_user_activity IS 'User activity logs from the last 30 days';

-- ============================================
-- 5. CREATE FUNCTIONS FOR USER MANAGEMENT
-- ============================================

-- Function: Check if user has access to agent
CREATE OR REPLACE FUNCTION user_has_agent_access(p_user_id INTEGER, p_agent_id VARCHAR)
RETURNS BOOLEAN AS $$
DECLARE
    v_role VARCHAR(50);
    v_has_access BOOLEAN;
BEGIN
    -- Get user role
    SELECT role INTO v_role FROM users WHERE id = p_user_id AND deleted_at IS NULL;

    -- Admin has access to all agents
    IF v_role = 'admin' THEN
        RETURN TRUE;
    END IF;

    -- Check if operator has assignment
    SELECT EXISTS(
        SELECT 1 FROM user_agent_assignments
        WHERE user_id = p_user_id
        AND agent_id = p_agent_id
        AND is_active = true
    ) INTO v_has_access;

    RETURN v_has_access;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION user_has_agent_access IS 'Check if user has permission to access a specific agent';

-- Function: Get user's accessible agent IDs
CREATE OR REPLACE FUNCTION get_user_agent_ids(p_user_id INTEGER)
RETURNS TABLE(agent_id VARCHAR) AS $$
DECLARE
    v_role VARCHAR(50);
BEGIN
    -- Get user role
    SELECT role INTO v_role FROM users WHERE id = p_user_id AND deleted_at IS NULL;

    -- Admin gets all agents
    IF v_role = 'admin' THEN
        RETURN QUERY SELECT ia.agent_id FROM integrated_agents ia;
    ELSE
        -- Operator gets only assigned agents
        RETURN QUERY
        SELECT uaa.agent_id
        FROM user_agent_assignments uaa
        WHERE uaa.user_id = p_user_id AND uaa.is_active = true;
    END IF;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION get_user_agent_ids IS 'Get list of agent IDs accessible by user';

-- Function: Log user activity
CREATE OR REPLACE FUNCTION log_user_activity(
    p_user_id INTEGER,
    p_username VARCHAR,
    p_action VARCHAR,
    p_resource_type VARCHAR DEFAULT NULL,
    p_resource_id VARCHAR DEFAULT NULL,
    p_ip_address VARCHAR DEFAULT NULL,
    p_user_agent TEXT DEFAULT NULL,
    p_details JSONB DEFAULT NULL
)
RETURNS void AS $$
BEGIN
    INSERT INTO user_activity_logs (
        user_id, username, action, resource_type, resource_id,
        ip_address, user_agent, details
    ) VALUES (
        p_user_id, p_username, p_action, p_resource_type, p_resource_id,
        p_ip_address, p_user_agent, p_details
    );
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION log_user_activity IS 'Log user action to audit trail';

-- Function: Update user's updated_at timestamp
CREATE OR REPLACE FUNCTION update_user_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create triggers for updated_at
DROP TRIGGER IF EXISTS trg_users_updated_at ON users;
CREATE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_user_updated_at();

DROP TRIGGER IF EXISTS trg_user_agent_assignments_updated_at ON user_agent_assignments;
CREATE TRIGGER trg_user_agent_assignments_updated_at
    BEFORE UPDATE ON user_agent_assignments
    FOR EACH ROW
    EXECUTE FUNCTION update_user_updated_at();

-- ============================================
-- 6. CREATE DEFAULT ADMIN USER
-- ============================================
-- Password: Admin@123 (bcrypt hash with cost 10)
-- IMPORTANT: Change this password after first login!
INSERT INTO users (username, email, fullname, password_hash, role, status)
VALUES (
    'admin',
    'admin@bsync.local',
    'System Administrator',
    '$2a$10$q/LJk90VEd8BmnfHCkcGvuU2TCKHvEPtPBYpASdhAetaNQz9zDCuS', -- Admin@123
    'admin',
    'active'
)
ON CONFLICT (username) DO NOTHING;

-- Log the admin creation
INSERT INTO user_activity_logs (username, action, resource_type, details)
VALUES (
    'system',
    'create_default_admin',
    'user',
    '{"message": "Default admin user created. Please change password immediately!", "username": "admin"}'::jsonb
)
ON CONFLICT DO NOTHING;

-- ============================================
-- 7. SAMPLE QUERIES FOR DOCUMENTATION
-- ============================================

-- Query 1: Get all operators with their assigned agents
-- SELECT * FROM v_users_with_agents WHERE role = 'operator' ORDER BY username;

-- Query 2: Check if user can access an agent
-- SELECT user_has_agent_access(1, 'agent-a');

-- Query 3: Get all agents accessible by user
-- SELECT * FROM get_user_agent_ids(1);

-- Query 4: Get recent activity for a user
-- SELECT * FROM user_activity_logs WHERE user_id = 1 ORDER BY created_at DESC LIMIT 20;

-- Query 5: Get all assignments for an agent
-- SELECT * FROM v_agent_assignments WHERE agent_id = 'agent-a' AND is_active = true;

-- Query 6: Count operators per agent
-- SELECT
--     agent_id,
--     agent_name,
--     COUNT(*) as operator_count
-- FROM v_agent_assignments
-- WHERE role = 'operator' AND is_active = true
-- GROUP BY agent_id, agent_name
-- ORDER BY operator_count DESC;
