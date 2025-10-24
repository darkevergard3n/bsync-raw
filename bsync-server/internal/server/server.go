package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lib/pq"
	_ "github.com/lib/pq"

	// User management imports
	"bsync-server/config"
	"bsync-server/internal/auth"
	"bsync-server/internal/models"
	"bsync-server/internal/repository"
	"bsync-server/utils"
)

type Config struct {
	Host     string
	Port     int
	LogLevel string
}

func (c *Config) LoadFromFile(path string) error {
	return nil
}

type SyncToolServer struct {
	config         *Config
	hub            *Hub
	server         *http.Server
	eventProcessor *EventProcessor
	scheduler      *JobScheduler
	shutdown       chan struct{}
	db             *sql.DB
	folderStats    map[string]map[string]interface{} // "agentID:folderID" -> folder_stats
	folderStatsMu  sync.RWMutex
	activeSyncJobs map[string]bool                   // agent_id -> is_syncing
	syncJobsMu     sync.RWMutex

	// User management
	userRepo    *repository.UserRepository
	authService *auth.AuthService
}

// FileTransferLogParams holds all query parameters for file transfer logs
type FileTransferLogParams struct {
	Page           int
	Limit          int
	Offset         int
	Cursor         string
	Search         string
	Status         []string
	JobName        []string
	Action         []string
	AgentID        string
	DateFrom       string
	DateTo         string
	UserRole       string   // For operator filtering
	AssignedAgents []string // For operator filtering
}

func (p *FileTransferLogParams) HasFilters() bool {
	return p.Search != "" || len(p.Status) > 0 || len(p.JobName) > 0 || 
		   len(p.Action) > 0 || p.AgentID != "" || p.DateFrom != "" || p.DateTo != ""
}

func (p *FileTransferLogParams) GetAppliedFilters() map[string]interface{} {
	filters := make(map[string]interface{})
	if p.Search != "" {
		filters["search"] = p.Search
	}
	if len(p.Status) > 0 {
		filters["status"] = p.Status
	}
	if len(p.JobName) > 0 {
		filters["job_name"] = p.JobName
	}
	if len(p.Action) > 0 {
		filters["action"] = p.Action
	}
	if p.AgentID != "" {
		filters["agent_id"] = p.AgentID
	}
	if p.DateFrom != "" {
		filters["date_from"] = p.DateFrom
	}
	if p.DateTo != "" {
		filters["date_to"] = p.DateTo
	}
	return filters
}

// FileTransferLogQueryBuilder builds optimized queries with cursor pagination
type FileTransferLogQueryBuilder struct {
	params *FileTransferLogParams
	db     *sql.DB
}

func NewSyncToolServer(config *Config) (*SyncToolServer, error) {
	// Create event store (default to memory store with 1000 events buffer)
	eventStore := NewMemoryEventStore(10000)
	
	// Connect to database
	db, err := sql.Open("postgres", "host=localhost user=bsync password=bsync_password dbname=bsync sslmode=disable")
	if err != nil {
		log.Printf("⚠️  Failed to connect to database: %v", err)
	} else {
		if err := db.Ping(); err != nil {
			log.Printf("⚠️  Failed to ping database: %v", err)
			db = nil
		} else {
			log.Printf("✅ Connected to database successfully")
		}
	}
	
	// Initialize user management if database is available
	var userRepo *repository.UserRepository
	var authService *auth.AuthService

	if db != nil {
		userRepo = repository.NewUserRepository(db)

		// Get JWT secret from environment
		jwtSecret := os.Getenv("JWT_SECRET")
		if jwtSecret == "" {
			jwtSecret = "dev-secret-change-in-production-min-32-chars"
			log.Println("⚠️  Using default JWT secret! Set JWT_SECRET environment variable for production")
		}

		// Token duration: 24 hours
		tokenDuration := 24 * time.Hour

		authService = auth.NewAuthService(userRepo, jwtSecret, tokenDuration)

		log.Println("✅ User management initialized")
	}

	s := &SyncToolServer{
		config:         config,
		hub:            NewHub(),
		eventProcessor: NewEventProcessor(eventStore, db),
		shutdown:       make(chan struct{}),
		db:             db,
		folderStats:    make(map[string]map[string]interface{}),
		activeSyncJobs: make(map[string]bool),
		userRepo:       userRepo,
		authService:    authService,
	}

	// Set event processor in hub for event handling
	s.hub.eventProcessor = s.eventProcessor

	// Initialize scheduler if database is available
	if db != nil {
		s.scheduler = NewJobScheduler(s)
		log.Println("✅ Job scheduler initialized")
	} else {
		log.Println("⚠️ Job scheduler disabled - no database connection")
	}
	
	// Set server reference in hub for database access
	s.hub.server = s

	// Start database sync if database is available
	if s.db != nil {
		go s.startDatabaseSync()
	}

	return s, nil
}

// ============================================
// CORS Middleware
// ============================================

// corsMiddleware handles CORS headers for all requests
func (s *SyncToolServer) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get allowed origin from environment variable, default to allow all in development
		allowedOrigin := os.Getenv("CORS_ALLOWED_ORIGIN")
		if allowedOrigin == "" {
			allowedOrigin = "*" // Allow all origins in development
		}

		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "3600")

		// Handle preflight OPTIONS request
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Continue to next handler
		next.ServeHTTP(w, r)
	})
}

// ============================================
// Authentication Middleware Helpers
// ============================================

// withAuth wraps a handler with JWT authentication check
func (s *SyncToolServer) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Skip auth if user management not initialized
		if s.authService == nil {
			http.Error(w, "User management not available", http.StatusServiceUnavailable)
			return
		}

		// Get token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			s.writeJSONError(w, http.StatusUnauthorized, "Authorization header required")
			return
		}

		// Parse "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			s.writeJSONError(w, http.StatusUnauthorized, "Invalid authorization header format. Use: Bearer <token>")
			return
		}

		tokenString := parts[1]

		// Validate token
		claims, err := s.authService.ValidateToken(tokenString)
		if err != nil {
			s.writeJSONError(w, http.StatusUnauthorized, "Invalid or expired token")
			return
		}

		// Store claims in context
		ctx := context.WithValue(r.Context(), "user_claims", claims)
		next(w, r.WithContext(ctx))
	}
}

// withAdminRole wraps a handler to require admin role
func (s *SyncToolServer) withAdminRole(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value("user_claims").(*models.JWTClaims)
		if !ok {
			s.writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		if claims.Role != models.RoleAdmin {
			s.writeJSONError(w, http.StatusForbidden, "Access denied: Admin only")
			return
		}

		next(w, r)
	}
}

// withAdminRoleForMutations wraps a handler to require admin role only for POST, PUT, DELETE
// GET requests are allowed for all authenticated users
func (s *SyncToolServer) withAdminRoleForMutations(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// For GET requests, allow all authenticated users (already checked by withAuth)
		if r.Method == http.MethodGet {
			next(w, r)
			return
		}

		// For POST, PUT, DELETE - require admin role
		claims, ok := r.Context().Value("user_claims").(*models.JWTClaims)
		if !ok {
			s.writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		if claims.Role != models.RoleAdmin {
			s.writeJSONError(w, http.StatusForbidden, "Access denied: Admin only")
			return
		}

		next(w, r)
	}
}

// withAgentPermission wraps a handler to check agent access for operators
func (s *SyncToolServer) withAgentPermission(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value("user_claims").(*models.JWTClaims)
		if !ok {
			// If no claims in context, let it pass (will be handled by withAuth)
			next(w, r)
			return
		}

		// Admin has access to everything
		if claims.Role == models.RoleAdmin {
			next(w, r)
			return
		}

		// For operators, the filtering will be done in the handler
		// based on claims.AssignedAgents
		next(w, r)
	}
}

// getUserClaims helper to get user claims from context
func (s *SyncToolServer) getUserClaims(r *http.Request) (*models.JWTClaims, bool) {
	claims, ok := r.Context().Value("user_claims").(*models.JWTClaims)
	return claims, ok
}

// writeJSONError helper to write JSON error response
func (s *SyncToolServer) writeJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"error":   message,
	})
}

// ============================================
// Server Start & Routes
// ============================================

func (s *SyncToolServer) Start() error {
	go s.hub.Run()
	
	// Start job scheduler if available
	if s.scheduler != nil {
		s.scheduler.Start()
	}

	mux := http.NewServeMux()

	// ========================================
	// PUBLIC ROUTES (No authentication)
	// ========================================
	mux.HandleFunc("/health", s.handleHealth)

	// WebSocket endpoints (agents use their own auth)
	mux.HandleFunc("/ws/agent", s.handleAgentWebSocket)
	mux.HandleFunc("/ws/cli", s.handleCLIWebSocket)

	// Authentication endpoints (public)
	if s.authService != nil {
		mux.HandleFunc("/api/v1/auth/login", s.handleUserLogin)
	}

	// ========================================
	// PROTECTED ROUTES (Require authentication)
	// ========================================

	// User management routes (auth required)
	if s.authService != nil {
		// Auth endpoints
		mux.HandleFunc("/api/v1/auth/logout", s.withAuth(s.handleUserLogout))
		mux.HandleFunc("/api/v1/auth/me", s.withAuth(s.handleUserMe))

		// User management (GET allowed for all authenticated users, mutations admin only)
		mux.HandleFunc("/api/v1/users", s.withAuth(s.withAdminRoleForMutations(s.handleUsers)))
		mux.HandleFunc("/api/v1/users/", s.withAuth(s.withAdminRoleForMutations(s.handleUserActions)))
		mux.HandleFunc("/api/v1/users/change-password", s.withAuth(s.handleUserChangePassword))
	}

	// Business API routes (all require authentication)
	if s.authService != nil {
		mux.HandleFunc("/api/status", s.withAuth(s.handleStatus))
		mux.HandleFunc("/api/agents", s.withAuth(s.handleAgents))
		mux.HandleFunc("/api/integrated-agents", s.withAuth(s.handleIntegratedAgents))
		mux.HandleFunc("/api/agents/", s.withAuth(s.handleAgentActions))
		mux.HandleFunc("/api/events", s.withAuth(s.handleEvents))
		mux.HandleFunc("/api/events/stats", s.withAuth(s.handleEventStats))
		mux.HandleFunc("/api/v1/sync-jobs", s.withAuth(s.handleSyncJobs))
		mux.HandleFunc("/api/v1/sync-jobs/", s.withAuth(s.handleSyncJobActions))
		mux.HandleFunc("/api/v1/file-transfer-logs", s.withAuth(s.handleFileTransferLogs))
		mux.HandleFunc("/api/file-transfers", s.withAuth(s.handleFileTransferLogs)) // Alias for dashboard
		mux.HandleFunc("/api/v1/reports/transfer-stats", s.withAuth(s.handleTransferStats))
		mux.HandleFunc("/api/v1/reports/filter-options", s.withAuth(s.handleFilterOptions))
		mux.HandleFunc("/api/v1/reports/jobs", s.withAuth(s.handleJobsList))
		mux.HandleFunc("/api/trigger-scan", s.withAuth(s.handleTriggerScan)) // Manual scan trigger for testing
		mux.HandleFunc("/api/folder-stats", s.withAuth(s.handleFolderStats)) // Get folder statistics from agent
		mux.HandleFunc("/api/v1/folder-stats/stats", s.withAuth(s.handleFolderStatsOverall)) // Dashboard statistics
		mux.HandleFunc("/api/v1/scheduler/status", s.withAuth(s.handleSchedulerStatus)) // Scheduler status
		mux.HandleFunc("/api/v1/licenses", s.withAuth(s.handleLicenses))               // License CRUD
		mux.HandleFunc("/api/v1/licenses/", s.withAuth(s.handleLicenseActions))        // License actions
		mux.HandleFunc("/api/v1/agent-licenses", s.withAuth(s.handleAgentLicenses))    // Agent-license mapping
		mux.HandleFunc("/api/v1/agent-licenses/", s.withAuth(s.handleAgentLicenseActions)) // Agent-license actions
		mux.HandleFunc("/api/v1/agents/unlicensed", s.withAuth(s.handleUnlicensedAgents)) // Unlicensed agents
		mux.HandleFunc("/api/v1/sessions", s.withAuth(s.handleSessions))                  // Session tracking endpoints
		mux.HandleFunc("/api/v1/sessions/", s.withAuth(s.handleSessionDetails))           // Session details and actions

		// Master data endpoints for filters
		mux.HandleFunc("/api/v1/master/sync-status", s.withAuth(s.handleGetSyncStatusMaster)) // Sync status filter options
		mux.HandleFunc("/api/v1/master/job-status", s.withAuth(s.handleGetJobStatusMaster))   // Job status filter options

		// Dashboard API endpoints
		mux.HandleFunc("/api/v1/roles", s.withAuth(s.handleGetRoleList))                             // Get role list
		mux.HandleFunc("/api/v1/dashboard/user-stats", s.withAuth(s.handleGetUserStats))             // Get user statistics
		mux.HandleFunc("/api/v1/agents/licensed", s.withAuth(s.handleGetLicensedAgents))             // Get licensed agents
		mux.HandleFunc("/api/v1/dashboard/stats", s.withAuth(s.handleGetDashboardStats))             // Get dashboard stats
		mux.HandleFunc("/api/v1/dashboard/daily-transfer-stats", s.withAuth(s.handleGetDailyFileTransferStats)) // Get daily transfer stats
		mux.HandleFunc("/api/v1/dashboard/top-jobs-performance", s.withAuth(s.handleGetTopJobsPerformance))     // Get top jobs performance
		mux.HandleFunc("/api/v1/dashboard/recent-events", s.withAuth(s.handleGetRecentFileTransferEvents))      // Get recent events
		mux.HandleFunc("/api/v1/dashboard/complete", s.withAuth(s.handleGetCompleteDashboard))       // Get complete dashboard data
	} else {
		// Fallback: if auth service not available, routes work without authentication
		log.Println("WARNING: Authentication service not initialized. API endpoints are unprotected!")
		mux.HandleFunc("/api/status", s.handleStatus)
		mux.HandleFunc("/api/agents", s.handleAgents)
		mux.HandleFunc("/api/integrated-agents", s.handleIntegratedAgents)
		mux.HandleFunc("/api/agents/", s.handleAgentActions)
		mux.HandleFunc("/api/events", s.handleEvents)
		mux.HandleFunc("/api/events/stats", s.handleEventStats)
		mux.HandleFunc("/api/v1/sync-jobs", s.handleSyncJobs)
		mux.HandleFunc("/api/v1/sync-jobs/", s.handleSyncJobActions)
		mux.HandleFunc("/api/v1/file-transfer-logs", s.handleFileTransferLogs)
		mux.HandleFunc("/api/file-transfers", s.handleFileTransferLogs)
		mux.HandleFunc("/api/v1/reports/transfer-stats", s.handleTransferStats)
		mux.HandleFunc("/api/v1/reports/filter-options", s.handleFilterOptions)
		mux.HandleFunc("/api/v1/reports/jobs", s.handleJobsList)
		mux.HandleFunc("/api/trigger-scan", s.handleTriggerScan)
		mux.HandleFunc("/api/folder-stats", s.handleFolderStats)
		mux.HandleFunc("/api/v1/folder-stats/stats", s.handleFolderStatsOverall)
		mux.HandleFunc("/api/v1/scheduler/status", s.handleSchedulerStatus)
		mux.HandleFunc("/api/v1/licenses", s.handleLicenses)
		mux.HandleFunc("/api/v1/licenses/", s.handleLicenseActions)
		mux.HandleFunc("/api/v1/agent-licenses", s.handleAgentLicenses)
		mux.HandleFunc("/api/v1/agent-licenses/", s.handleAgentLicenseActions)
		mux.HandleFunc("/api/v1/agents/unlicensed", s.handleUnlicensedAgents)
		mux.HandleFunc("/api/v1/sessions", s.handleSessions)
		mux.HandleFunc("/api/v1/sessions/", s.handleSessionDetails)
		mux.HandleFunc("/api/v1/master/sync-status", s.handleGetSyncStatusMaster)
		mux.HandleFunc("/api/v1/master/job-status", s.handleGetJobStatusMaster)
	}

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	// Wrap mux with CORS middleware
	handler := s.corsMiddleware(mux)

	s.server = &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	log.Printf("🌐 BSync Server listening on %s", addr)
	log.Printf("📡 WebSocket endpoints:")
	log.Printf("  ws://%s/ws/agent - Agent connections", addr)
	log.Printf("  ws://%s/ws/cli   - CLI connections", addr)
	log.Printf("🔧 REST endpoints:")
	log.Printf("  http://%s/api/status - Server status", addr)
	log.Printf("  http://%s/api/agents - List connected agents", addr)
	log.Printf("  http://%s/api/events - Query events (params: agent_id, since, limit)", addr)
	log.Printf("  http://%s/api/events/stats - Event statistics", addr)
	log.Printf("  http://%s/health     - Health check", addr)

	return s.server.ListenAndServe()
}

func (s *SyncToolServer) Shutdown() {
	log.Println("📛 Shutting down server...")
	
	close(s.shutdown)
	
	// Stop event processor
	if s.eventProcessor != nil {
		s.eventProcessor.Stop()
	}
	
	s.hub.Shutdown()
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := s.server.Shutdown(ctx); err != nil {
		log.Printf("⚠️  Server shutdown error: %v", err)
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (s *SyncToolServer) handleAgentWebSocket(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		http.Error(w, "agent_id required", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("❌ WebSocket upgrade failed: %v", err)
		return
	}

	log.Printf("✅ Agent connected: %s", agentID)
	
	// Try to load existing agent data from database
	var hostname, os, architecture, deviceID, dataDir string
	hostname = agentID // Default to agent ID
	os = "Unknown"
	architecture = "Unknown"
	
	if s.db != nil {
		var dbDeviceID, dbHostname, dbOS, dbArch sql.NullString
		var dbDataDir sql.NullString
		err := s.db.QueryRow(`
			SELECT device_id, hostname, os, architecture, data_dir 
			FROM integrated_agents 
			WHERE agent_id = $1
		`, agentID).Scan(&dbDeviceID, &dbHostname, &dbOS, &dbArch, &dbDataDir)
		
		if err == nil {
			// Found existing agent, load its data
			if dbDeviceID.Valid {
				deviceID = dbDeviceID.String
			}
			if dbHostname.Valid && dbHostname.String != "" {
				hostname = dbHostname.String
			}
			if dbOS.Valid && dbOS.String != "" {
				os = dbOS.String
			}
			if dbArch.Valid && dbArch.String != "" {
				architecture = dbArch.String
			}
			if dbDataDir.Valid {
				dataDir = dbDataDir.String
			}
			log.Printf("📋 Loaded existing agent data from database: hostname=%s, os=%s, arch=%s, device=%s", 
				hostname, os, architecture, deviceID)
		}
	}
	
	client := &AgentClient{
		ID:           agentID,
		conn:         conn,
		send:         make(chan []byte, 256),
		hub:          s.hub,
		isAgent:      true,
		lastSeen:     time.Now(),
		remoteAddr:   r.RemoteAddr,
		hostname:     hostname,
		os:           os,
		architecture: architecture,
		deviceID:     deviceID,
		dataDir:      dataDir,
	}

	s.hub.Register(client)

	go client.WritePump()
	go client.ReadPump()
}

func (s *SyncToolServer) handleCLIWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("❌ WebSocket upgrade failed: %v", err)
		return
	}

	clientID := fmt.Sprintf("cli-%d", time.Now().UnixNano())
	log.Printf("✅ CLI client connected: %s", clientID)

	client := &AgentClient{
		ID:       clientID,
		conn:     conn,
		send:     make(chan []byte, 256),
		hub:      s.hub,
		isAgent:  false,
	}

	s.hub.Register(client)

	go client.WritePump()
	go client.ReadPump()
}

func (s *SyncToolServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	status := map[string]interface{}{
		"server": "running",
		"agents": s.hub.GetConnectedAgents(),
		"uptime": time.Since(s.hub.startTime).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *SyncToolServer) handleAgents(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Get user claims from context for role-based filtering
	var userRole string
	var assignedAgents []string
	claims, ok := r.Context().Value("user_claims").(*models.JWTClaims)
	if ok {
		userRole = claims.Role
		assignedAgents = claims.AssignedAgents
	}

	allAgents := s.hub.GetAgentDetails()

	// Filter agents based on role
	if userRole == models.RoleOperator {
		if len(assignedAgents) == 0 {
			// Operator with no assigned agents = return empty array
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]interface{}{})
			return
		}

		// Filter to only assigned agents
		assignedAgentMap := make(map[string]bool)
		for _, agentID := range assignedAgents {
			assignedAgentMap[agentID] = true
		}

		filteredAgents := []interface{}{}
		for _, agent := range allAgents {
			if agentMap, ok := agent.(map[string]interface{}); ok {
				if agentIDVal, ok := agentMap["agent_id"].(string); ok {
					if assignedAgentMap[agentIDVal] {
						filteredAgents = append(filteredAgents, agent)
					}
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(filteredAgents)
		return
	}

	// Admin: return all agents
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(allAgents)
}

func (s *SyncToolServer) handleIntegratedAgents(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if s.db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	// Get user claims from context for role-based filtering
	var userRole string
	var assignedAgents []string
	claims, ok := r.Context().Value("user_claims").(*models.JWTClaims)
	if ok {
		userRole = claims.Role
		assignedAgents = claims.AssignedAgents
		log.Printf("🔐 User %s (role=%s) requesting agents, assigned to %d agents", claims.Username, userRole, len(assignedAgents))
	}

	// Build query based on user role
	var query string
	var args []interface{}

	if userRole == models.RoleOperator {
		// Operator: only show assigned agents
		if len(assignedAgents) == 0 {
			// Operator with no assigned agents = return empty array
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{})
			log.Printf("⚠️  Operator %s has no assigned agents, returning empty list", claims.Username)
			return
		}

		placeholders := make([]string, len(assignedAgents))
		args = make([]interface{}, len(assignedAgents))
		for i, agentID := range assignedAgents {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = agentID
		}

		query = fmt.Sprintf(`
			SELECT ia.id, ia.agent_id, ia.device_id, ia.hostname, ia.ip_address, ia.os,
			       ia.architecture, ia.version, ia.status, ia.approval_status,
			       ia.last_heartbeat, ia.created_at, ia.updated_at, ia.data_dir,
			       al.license_id, l.license_key, al.created_at as licensed_at
			FROM integrated_agents ia
			JOIN agent_licenses al ON ia.agent_id = al.agent_id
			JOIN licenses l ON al.license_id = l.id
			WHERE ia.agent_id IN (%s)
			ORDER BY ia.created_at DESC
		`, strings.Join(placeholders, ", "))
	} else {
		// Admin or viewer: show all agents
		query = `
			SELECT ia.id, ia.agent_id, ia.device_id, ia.hostname, ia.ip_address, ia.os,
			       ia.architecture, ia.version, ia.status, ia.approval_status,
			       ia.last_heartbeat, ia.created_at, ia.updated_at, ia.data_dir,
			       al.license_id, l.license_key, al.created_at as licensed_at
			FROM integrated_agents ia
			JOIN agent_licenses al ON ia.agent_id = al.agent_id
			JOIN licenses l ON al.license_id = l.id
			ORDER BY ia.created_at DESC
		`
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		log.Printf("❌ Failed to query integrated agents: %v", err)
		http.Error(w, "Failed to fetch agents", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	agents := []map[string]interface{}{}
	
	for rows.Next() {
		var id, agentID, deviceID, hostname, ipAddress, os, architecture, version, status, approvalStatus string
		var dataDir sql.NullString
		var lastHeartbeat, createdAt, updatedAt time.Time
		var licenseID sql.NullInt64
		var licenseKey sql.NullString
		var licensedAt pq.NullTime
		
		if err := rows.Scan(&id, &agentID, &deviceID, &hostname, &ipAddress, &os, &architecture, &version, &status, &approvalStatus, &lastHeartbeat, &createdAt, &updatedAt, &dataDir, &licenseID, &licenseKey, &licensedAt); err != nil {
			log.Printf("❌ Failed to scan agent row: %v", err)
			continue
		}
		
		agent := map[string]interface{}{
			"id":               id,
			"agent_id":         agentID,
			"device_id":        deviceID,
			"hostname":         hostname,
			"ip_address":       ipAddress,
			"os":               os,
			"architecture":     architecture,
			"version":          version,
			"status":           status,
			"approval_status":  approvalStatus,
			"last_heartbeat":   lastHeartbeat.Format(time.RFC3339),
			"created_at":       createdAt.Format(time.RFC3339),
			"updated_at":       updatedAt.Format(time.RFC3339),
			"data_dir":         dataDir.String,
			"has_license":      licenseID.Valid,
		}
		
		// Add license information if available
		if licenseID.Valid {
			agent["license_info"] = map[string]interface{}{
				"license_id":   licenseID.Int64,
				"license_key":  licenseKey.String,
				"licensed_at":  licensedAt.Time.Format(time.RFC3339),
			}
		}
		
		agents = append(agents, agent)
	}

	response := map[string]interface{}{
		"data":  agents,
		"total": len(agents),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *SyncToolServer) startDatabaseSync() {
	ticker := time.NewTicker(30 * time.Second) // Sync every 30 seconds
	defer ticker.Stop()
	
	log.Printf("🔄 Starting database sync - syncing every 30 seconds")
	
	// Initial sync
	s.syncAgentsToDatabase()
	
	for {
		select {
		case <-ticker.C:
			s.syncAgentsToDatabase()
		case <-s.shutdown:
			log.Printf("📛 Database sync shutdown requested")
			return
		}
	}
}

func (s *SyncToolServer) syncAgentsToDatabase() {
	if s.db == nil {
		return
	}
	
	// Get current live agents
	liveAgents := s.hub.GetAgentDetails()
	
	log.Printf("🔄 Syncing %d live agents to database", len(liveAgents))
	
	for agentID, agentData := range liveAgents {
		agent, ok := agentData.(map[string]interface{})
		if !ok {
			log.Printf("⚠️  Invalid agent data format for %s", agentID)
			continue
		}
		
		// Extract agent information
		deviceID, _ := agent["device_id"].(string)
		hostname, _ := agent["hostname"].(string)
		if hostname == "" {
			hostname = agentID
		}
		os, _ := agent["os"].(string)
		if os == "" {
			os = "Unknown"
		}
		architecture, _ := agent["architecture"].(string)
		if architecture == "" {
			architecture = "Unknown"
		}
		connected, _ := agent["connected"].(bool)
		remoteAddr, _ := agent["remote_addr"].(string)
		
		// Extract IP address from remote_addr
		ipAddress := "Unknown"
		if remoteAddr != "" {
			parts := strings.Split(remoteAddr, ":")
			if len(parts) > 0 {
				ipAddress = parts[0]
			}
		}
		
		status := "offline"
		if connected {
			status = "online"
		}
		
		lastSeenStr, _ := agent["last_seen"].(string)
		var lastSeen time.Time
		if lastSeenStr != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, lastSeenStr); err == nil {
				lastSeen = parsed
			} else {
				lastSeen = time.Now()
			}
		} else {
			lastSeen = time.Now()
		}
		
		// Upsert agent into database - preserve data_dir
		_, err := s.db.Exec(`
			INSERT INTO integrated_agents (agent_id, name, device_id, hostname, ip_address, os, architecture, version, status, approval_status, connected, last_heartbeat, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			ON CONFLICT (agent_id) DO UPDATE SET
				device_id = EXCLUDED.device_id,
				hostname = EXCLUDED.hostname,
				ip_address = EXCLUDED.ip_address,
				os = EXCLUDED.os,
				architecture = EXCLUDED.architecture,
				status = EXCLUDED.status,
				connected = EXCLUDED.connected,
				last_heartbeat = EXCLUDED.last_heartbeat,
				updated_at = EXCLUDED.updated_at
				-- Don't update approval_status and data_dir for existing agents
		`, agentID, agentID, deviceID, hostname, ipAddress, os, architecture, "1.0.0", status, "pending", connected, lastSeen, time.Now())
		
		if err != nil {
			log.Printf("❌ Failed to sync agent %s to database: %v", agentID, err)
		} else {
			log.Printf("✅ Synced agent %s to database (status: %s)", agentID, status)
		}
	}
	
	// Mark agents as offline if they haven't been seen recently
	_, err := s.db.Exec(`
		UPDATE integrated_agents 
		SET status = 'offline', connected = false, updated_at = $1
		WHERE last_heartbeat < $2 AND status = 'online'
	`, time.Now(), time.Now().Add(-2*time.Minute))
	
	if err != nil {
		log.Printf("❌ Failed to mark offline agents: %v", err)
	}
}

func (s *SyncToolServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status": "healthy", "service": "bsync-server"}`)
}

func (s *SyncToolServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	agentID := r.URL.Query().Get("agent_id")
	sinceStr := r.URL.Query().Get("since")
	limitStr := r.URL.Query().Get("limit")
	
	var since int64 = 0
	var limit int = 100
	
	if sinceStr != "" {
		fmt.Sscanf(sinceStr, "%d", &since)
	}
	if limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}
	
	events, err := s.eventProcessor.GetEvents(agentID, since, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

func (s *SyncToolServer) handleEventStats(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	agentID := r.URL.Query().Get("agent_id")
	
	stats, err := s.eventProcessor.GetEventStats(agentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

type Hub struct {
	agents         map[string]*AgentClient
	cliClients     map[string]*AgentClient
	register       chan *AgentClient
	unregister     chan *AgentClient
	broadcast      chan Message
	mutex          sync.RWMutex
	startTime      time.Time
	eventProcessor *EventProcessor
	browseRequests map[string]chan map[string]interface{} // Track pending browse requests
	server         *SyncToolServer // Reference to main server for database access
}

func NewHub() *Hub {
	return &Hub{
		agents:         make(map[string]*AgentClient),
		cliClients:     make(map[string]*AgentClient),
		register:       make(chan *AgentClient),
		unregister:     make(chan *AgentClient),
		broadcast:      make(chan Message),
		startTime:      time.Now(),
		browseRequests: make(map[string]chan map[string]interface{}),
	}
}

func (h *Hub) Run() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("💀 PANIC in Hub.Run(): %v", r)
			// Log stack trace for debugging
			log.Printf("💀 PANIC Stack trace available in log")
		}
	}()
	
	log.Printf("🏃 Hub started running")
	
	// Start periodic lastSeen sync for scalability
	go h.syncLastSeenPeriodically()
	
	for {
		select {
		case client := <-h.register:
			h.mutex.Lock()
			if client.isAgent {
				// Check if agent already exists (reconnection)
				if existingAgent, exists := h.agents[client.ID]; exists {
					// Update existing agent with new connection
					existingAgent.conn = client.conn
					existingAgent.send = client.send
					existingAgent.isOnline = true
					existingAgent.lastSeen = time.Now()
					existingAgent.remoteAddr = client.remoteAddr
					log.Printf("📊 Agent reconnected (marked online): %s (Total agents: %d)", client.ID, len(h.agents))
				} else {
					// New agent registration
					client.isOnline = true
					client.lastSeen = time.Now()
					h.agents[client.ID] = client
					log.Printf("📊 Agent registered: %s (Total agents: %d)", client.ID, len(h.agents))
				}
			} else {
				h.cliClients[client.ID] = client
				log.Printf("📊 CLI client registered: %s", client.ID)
			}
			h.mutex.Unlock()

		case client := <-h.unregister:
			h.mutex.Lock()
			if client.isAgent {
				if existingAgent, ok := h.agents[client.ID]; ok {
					// Don't delete agent, just mark as offline
					existingAgent.isOnline = false
					existingAgent.conn = nil
					existingAgent.lastSeen = time.Now()
					if existingAgent.send != nil {
						close(existingAgent.send)
						existingAgent.send = nil
					}
					
					// Update database to mark agent as disconnected
					if h.server != nil && h.server.db != nil {
						_, err := h.server.db.Exec(`
							UPDATE integrated_agents 
							SET connected = false, 
							    last_heartbeat = $1,
							    updated_at = $1
							WHERE agent_id = $2
						`, time.Now(), client.ID)
						if err != nil {
							log.Printf("❌ Failed to mark agent %s as disconnected in database: %v", client.ID, err)
						}
					}
					
					log.Printf("📊 Agent disconnected (marked offline): %s (Total agents: %d)", client.ID, len(h.agents))
				}
			} else {
				// CLI clients can be removed since they're temporary
				if _, ok := h.cliClients[client.ID]; ok {
					delete(h.cliClients, client.ID)
					close(client.send)
					log.Printf("📊 CLI client unregistered: %s", client.ID)
				}
			}
			h.mutex.Unlock()

		case msg := <-h.broadcast:
			log.Printf("🔄 Hub processing message: Type=%s, From=%s, To=%s", msg.Type, msg.From, msg.To)
			h.handleMessage(msg)
		}
	}
}

func (h *Hub) handleMessage(msg Message) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("💀 PANIC in handleMessage(): %v, Message: %+v", r, msg)
		}
	}()
	
	log.Printf("📨 Processing message: Type=%s, From=%s, To=%s", msg.Type, msg.From, msg.To)

	switch msg.Type {
	case "command":
		if targetAgent, ok := h.agents[msg.To]; ok {
			// Check if agent is online and channel is not closed
			if targetAgent.isOnline && targetAgent.send != nil {
				select {
				case targetAgent.send <- msg.Data:
					log.Printf("✅ Command forwarded to agent: %s", msg.To)
				default:
					log.Printf("⚠️  Agent %s channel full, dropping message", msg.To)
				}
			} else {
				log.Printf("⚠️  Agent %s is offline or channel closed", msg.To)
				h.sendErrorToCLI(msg.From, fmt.Sprintf("Agent %s not connected", msg.To))
			}
		} else {
			log.Printf("⚠️  Target agent not found: %s", msg.To)
			h.sendErrorToCLI(msg.From, fmt.Sprintf("Agent %s not connected", msg.To))
		}

	case "response", "event", "error":
		if cliClient, ok := h.cliClients[msg.To]; ok {
			// Check if CLI client channel is not closed
			if cliClient.send != nil {
				select {
				case cliClient.send <- msg.Data:
					log.Printf("✅ Response sent to CLI: %s", msg.To)
				default:
					log.Printf("⚠️  CLI client %s channel full", msg.To)
				}
			} else {
				log.Printf("⚠️  CLI client %s channel is closed", msg.To)
			}
		}

	case "broadcast_event":
		h.mutex.RLock()
		for id, cli := range h.cliClients {
			// Check if CLI client channel is not closed before sending
			if cli.send != nil {
				select {
				case cli.send <- msg.Data:
					log.Printf("📢 Event broadcast to CLI: %s", id)
				default:
					log.Printf("⚠️  CLI %s channel full during broadcast", id)
				}
			}
		}
		h.mutex.RUnlock()
	}
}

func (h *Hub) sendErrorToCLI(cliID string, errorMsg string) {
	h.mutex.RLock()
	cli, ok := h.cliClients[cliID]
	h.mutex.RUnlock()

	if ok {
		errData := map[string]string{
			"type":  "error",
			"error": errorMsg,
		}
		data, _ := json.Marshal(errData)
		select {
		case cli.send <- data:
		default:
		}
	}
}

// handleBrowseResponse handles browse folder response from agent
func (h *Hub) handleBrowseResponse(agentID string, msgData map[string]interface{}) {
	requestID := fmt.Sprintf("%s-browse", agentID)
	
	h.mutex.Lock()
	responseChannel, exists := h.browseRequests[requestID]
	if exists {
		delete(h.browseRequests, requestID)
	}
	h.mutex.Unlock()
	
	if exists {
		select {
		case responseChannel <- msgData:
			log.Printf("✅ Browse response sent to waiting request for agent %s", agentID)
		default:
			log.Printf("⚠️  Browse response channel closed for agent %s", agentID)
		}
	} else {
		log.Printf("⚠️  No pending browse request for agent %s", agentID)
	}
}

// handleBrowseError handles browse folder error from agent
func (h *Hub) handleBrowseError(agentID string, msgData map[string]interface{}) {
	requestID := fmt.Sprintf("%s-browse", agentID)
	
	h.mutex.Lock()
	responseChannel, exists := h.browseRequests[requestID]
	if exists {
		delete(h.browseRequests, requestID)
	}
	h.mutex.Unlock()
	
	if exists {
		select {
		case responseChannel <- msgData:
			log.Printf("✅ Browse error sent to waiting request for agent %s", agentID)
		default:
			log.Printf("⚠️  Browse error channel closed for agent %s", agentID)
		}
	} else {
		log.Printf("⚠️  No pending browse request for agent %s", agentID)
	}
}


func (h *Hub) Register(client *AgentClient) {
	h.register <- client
}

func (h *Hub) Unregister(client *AgentClient) {
	h.unregister <- client
}

func (h *Hub) Broadcast(msg Message) {
	h.broadcast <- msg
}

func (h *Hub) GetConnectedAgents() []string {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	agents := make([]string, 0, len(h.agents))
	for id := range h.agents {
		agents = append(agents, id)
	}
	return agents
}

func (h *Hub) GetAgentDetails() map[string]interface{} {
	details := make(map[string]interface{})

	// Fallback to memory-only if database not available
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	
	for id, agent := range h.agents {
		hostname := id
		if agent.hostname != "" && agent.hostname != id {
			hostname = agent.hostname
		}
		
		details[id] = map[string]interface{}{
			"id":         id,
			"connected":  agent.isOnline,
			"last_seen":  agent.lastSeen,
			"remote_addr": agent.remoteAddr,
			"hostname":   hostname,
			"os":         agent.os,
			"device_id":  agent.deviceID,
			"data_dir":   agent.dataDir,
		}
	}
	
	// If database is available, fetch from database for persistence
	// if h.server != nil && h.server.db != nil {
	// 	rows, err := h.server.db.Query(`
	// 		SELECT agent_id, device_id, hostname, ip_address, os, architecture, 
	// 		       version, connected, last_heartbeat, data_dir
	// 		FROM integrated_agents
	// 		-- WHERE approval_status = 'approved'
	// 		ORDER BY agent_id
	// 	`)
	// 	if err == nil {
	// 		defer rows.Close()
	// 		for rows.Next() {
	// 			var agentID, deviceID, hostname, ipAddress, os, arch, version string
	// 			var dataDir sql.NullString
	// 			var connected bool
	// 			var lastHeartbeat time.Time
				
	// 			if err := rows.Scan(&agentID, &deviceID, &hostname, &ipAddress, &os, &arch, 
	// 			                   &version, &connected, &lastHeartbeat, &dataDir); err == nil {
	// 				// Check if agent is currently connected in memory
	// 				h.mutex.RLock()
	// 				if memAgent, exists := h.agents[agentID]; exists && memAgent.isOnline {
	// 					connected = true
	// 					lastHeartbeat = memAgent.lastSeen
	// 					ipAddress = memAgent.remoteAddr
	// 				}
	// 				h.mutex.RUnlock()
					
	// 				details[agentID] = map[string]interface{}{
	// 					"id":         agentID,
	// 					"connected":  connected,
	// 					"last_seen":  lastHeartbeat,
	// 					"remote_addr": ipAddress,
	// 					"hostname":   hostname,
	// 					"os":         os,
	// 					"device_id":  deviceID,
	// 					"data_dir":   dataDir.String,
	// 				}
	// 			}
	// 		}
	// 		return details
	// 	}
	// }
	
	return details
}

// GetAgentDeviceID returns the device ID for a specific agent
func (h *Hub) GetAgentDeviceID(agentID string) string {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	
	if agent, exists := h.agents[agentID]; exists {
		return agent.deviceID
	}
	return ""
}

// GetAgentIPAddress returns the IP address for a specific agent
func (h *Hub) GetAgentIPAddress(agentID string) string {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	
	if agent, exists := h.agents[agentID]; exists {
		// Extract IP from remoteAddr (format: "IP:PORT")
		if agent.remoteAddr != "" {
			parts := strings.Split(agent.remoteAddr, ":")
			if len(parts) > 0 {
				return parts[0]
			}
		}
	}
	return ""
}

// Periodic lastSeen sync for scalability - runs every 5 seconds
func (h *Hub) syncLastSeenPeriodically() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		h.mutex.Lock()
		// Quick sync: update hub agents lastSeen from client objects
		for id, agent := range h.agents {
			// agent here is the same reference as the client, so lastSeen is already updated
			_ = id // Prevent unused variable
			_ = agent
		}
		h.mutex.Unlock()
	}
}

func (h *Hub) Shutdown() {
	log.Println("Shutting down hub...")
	h.mutex.Lock()
	for _, client := range h.agents {
		if client.send != nil {
			close(client.send)
		}
	}
	for _, client := range h.cliClients {
		if client.send != nil {
			close(client.send)
		}
	}
	h.mutex.Unlock()
}

type Message struct {
	Type string
	From string
	To   string
	Data []byte
}

type AgentClient struct {
	ID           string
	conn         *websocket.Conn
	send         chan []byte
	hub          *Hub
	isAgent      bool
	lastSeen     time.Time
	remoteAddr   string
	hostname     string
	os           string
	architecture string
	isOnline     bool  // Track online/offline status
	deviceID     string
	dataDir      string  // Agent data directory
}

func (c *AgentClient) ReadPump() {
	defer func() {
		c.hub.Unregister(c)
		if c.conn != nil {
			c.conn.Close()
		}
	}()

	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("❌ WebSocket error: %v", err)
			}
			break
		}

		c.lastSeen = time.Now()

		var msgData map[string]interface{}
		if err := json.Unmarshal(message, &msgData); err != nil {
			log.Printf("⚠️  Invalid JSON from %s: %v", c.ID, err)
			continue
		}

		if c.isAgent {
			c.handleAgentMessage(msgData, message)
		} else {
			c.handleCLIMessage(msgData, message)
		}
	}
}

func (c *AgentClient) handleAgentMessage(msgData map[string]interface{}, rawMessage []byte) {
	msgType, _ := msgData["type"].(string)
	
	switch msgType {
	case "event":
		// Store event if event processor is available
		if c.hub.eventProcessor != nil {
			c.hub.eventProcessor.ProcessEvent(c.ID, rawMessage)
		}
		
		// Check for state changes to track sync status
		if eventData, ok := msgData["event"].(map[string]interface{}); ok {
			if eventType, ok := eventData["type"].(string); ok && eventType == "state_changed" {
				if data, ok := eventData["data"].(map[string]interface{}); ok {
					if to, ok := data["to"].(string); ok {
						if folderID, ok := data["folder"].(string); ok {
							// If folder becomes idle, mark agent as not syncing
							if to == "idle" && strings.HasPrefix(folderID, "job-") {
								c.hub.server.markAgentSyncingStatus(c.ID, false)
							}
						}
					}
				}
			}
		}
		
		c.hub.Broadcast(Message{
			Type: "broadcast_event",
			From: c.ID,
			Data: rawMessage,
		})
	case "response", "error":
		if targetCLI, ok := msgData["cli_id"].(string); ok {
			messageType := "response"
			if msgType == "error" {
				messageType = "error"
			}
			c.hub.Broadcast(Message{
				Type: messageType,
				From: c.ID,
				To:   targetCLI,
				Data: rawMessage,
			})
		}
	case "folder_stats_response":
		// Handle folder stats response from agent
		log.Printf("📊 Received folder stats response from agent %s: %s", c.ID, string(rawMessage))
		
		// Store the response in server
		if c.hub.server != nil {
			c.hub.server.storeFolderStatsResponse(c.ID, msgData)
		}
	case "folder_stats_periodic":
		// Handle periodic folder stats from agent during syncing
		log.Printf("📊 Received periodic folder stats from agent %s: %s", c.ID, string(rawMessage))
		
		// Mark agent as actively syncing and store the response
		if c.hub.server != nil {
			c.hub.server.markAgentSyncingStatus(c.ID, true)
			c.hub.server.storeFolderStatsResponse(c.ID, msgData)
		}
	case "folder_stats_error":
		// Handle folder stats error response from agent
		log.Printf("❌ Received folder stats error from agent %s: %s", c.ID, string(rawMessage))
	case "session_event":
		// Handle session tracking events from agent
		if c.hub.server != nil {
			c.hub.server.handleSessionEvent(c.ID, msgData)
		}
	case "register":
		// Handle agent registration with device ID validation
		newDeviceID, _ := msgData["device_id"].(string)
		newDataDir, _ := msgData["data_dir"].(string)
		
		// Check if Device ID has changed (indicating a potentially different agent)
		needsApproval := false
		if c.deviceID != "" && newDeviceID != "" && c.deviceID != newDeviceID {
			log.Printf("⚠️  Device ID mismatch for agent %s: expected %s, got %s", c.ID, c.deviceID, newDeviceID)
			needsApproval = true
		}
		
		// Update with new values
		c.deviceID = newDeviceID
		c.dataDir = newDataDir
		
		// Update the agent registry
		c.hub.mutex.Lock()
		if existingAgent, exists := c.hub.agents[c.ID]; exists {
			existingAgent.deviceID = c.deviceID
			existingAgent.dataDir = c.dataDir
		}
		c.hub.mutex.Unlock()
		
		// Persist to database
		if c.hub.server != nil && c.hub.server.db != nil {
			// Get system info that might have been set earlier
			hostname := c.hostname
			if hostname == "" || hostname == c.ID {
				hostname = c.ID
			}
			
			// Determine approval status
			approvalStatus := "approved"
			if needsApproval {
				approvalStatus = "pending"
				log.Printf("🔒 Agent %s requires re-approval due to device ID change", c.ID)
			}
			
			// Check if this is a new agent or an existing one
			var existingDeviceID sql.NullString
			err := c.hub.server.db.QueryRow(`
				SELECT device_id FROM integrated_agents WHERE agent_id = $1
			`, c.ID).Scan(&existingDeviceID)
			
			if err == sql.ErrNoRows {
				// New agent - always needs approval
				approvalStatus = "pending"
				log.Printf("🆕 New agent %s requires approval", c.ID)
			} else if err == nil && existingDeviceID.Valid && existingDeviceID.String != newDeviceID {
				// Existing agent with different device ID - needs re-approval
				approvalStatus = "pending"
				log.Printf("🔄 Agent %s device ID changed from %s to %s - requires re-approval", 
					c.ID, existingDeviceID.String, newDeviceID)
			}
			
			// Insert or update agent record
			_, err = c.hub.server.db.Exec(`
				INSERT INTO integrated_agents (agent_id, name, device_id, hostname, ip_address, os, architecture, version, status, approval_status, connected, last_heartbeat, updated_at, data_dir)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
				ON CONFLICT (agent_id) DO UPDATE SET
					device_id = CASE 
						WHEN integrated_agents.device_id != EXCLUDED.device_id THEN EXCLUDED.device_id
						ELSE integrated_agents.device_id
					END,
					hostname = CASE 
						WHEN EXCLUDED.hostname != '' AND EXCLUDED.hostname != integrated_agents.agent_id THEN EXCLUDED.hostname 
						ELSE integrated_agents.hostname 
					END,
					ip_address = EXCLUDED.ip_address,
					os = CASE 
						WHEN integrated_agents.os = 'Unknown' OR integrated_agents.os IS NULL THEN EXCLUDED.os 
						ELSE integrated_agents.os 
					END,
					architecture = CASE 
						WHEN integrated_agents.architecture = 'Unknown' OR integrated_agents.architecture IS NULL THEN EXCLUDED.architecture 
						ELSE integrated_agents.architecture 
					END,
					status = EXCLUDED.status,
					connected = EXCLUDED.connected,
					last_heartbeat = EXCLUDED.last_heartbeat,
					updated_at = EXCLUDED.updated_at,
					data_dir = CASE 
						WHEN EXCLUDED.data_dir != '' THEN EXCLUDED.data_dir 
						ELSE integrated_agents.data_dir 
					END,
					approval_status = CASE 
						WHEN integrated_agents.device_id != EXCLUDED.device_id THEN 'pending'
						ELSE integrated_agents.approval_status
					END
			`, c.ID, c.ID, c.deviceID, hostname, c.remoteAddr, c.os, c.architecture, "1.0.0", "online", approvalStatus, true, time.Now(), time.Now(), c.dataDir)
			
			if err != nil {
				log.Printf("❌ Failed to persist agent %s to database: %v", c.ID, err)
			} else {
				log.Printf("✅ Agent %s persisted to database", c.ID)
			}
		}
		
		log.Printf("📋 Agent %s registered with device ID: %s, data dir: %s", c.ID, c.deviceID, c.dataDir)
		log.Printf("📨 Agent message: %s", string(rawMessage))
	case "health":
		// Update last seen time for heartbeat
		c.lastSeen = time.Now()
		
		// Check if data_dir is included in health message
		if dataDir, ok := msgData["data_dir"].(string); ok && dataDir != "" {
			c.dataDir = dataDir
		}
		
		// Handle health message with system info
		if systemInfo, ok := msgData["system_info"].(map[string]interface{}); ok {
			if hostname, ok := systemInfo["hostname"].(string); ok {
				c.hostname = hostname
			}
			if osInfo, ok := systemInfo["os"].(string); ok {
				c.os = osInfo
			}
			if arch, ok := systemInfo["architecture"].(string); ok {
				c.architecture = arch
			}
			// Also update the system info in the hub's agent registry
			c.hub.mutex.Lock()
			if existingAgent, exists := c.hub.agents[c.ID]; exists {
				existingAgent.hostname = c.hostname
				existingAgent.os = c.os
				existingAgent.architecture = c.architecture
				existingAgent.lastSeen = c.lastSeen  // Update lastSeen in memory
				existingAgent.dataDir = c.dataDir    // Update dataDir in memory
			}
			c.hub.mutex.Unlock()
			
			// Update database with health info including data_dir
			if c.hub.server.db != nil {
				query := `
					UPDATE integrated_agents 
					SET hostname = $2, os = $3, architecture = $4, last_heartbeat = NOW(), updated_at = NOW()`
				
				// Only update data_dir if it's not empty
				if c.dataDir != "" {
					query += `, data_dir = $5 WHERE agent_id = $1`
					_, err := c.hub.server.db.Exec(query, c.ID, c.hostname, c.os, c.architecture, c.dataDir)
					if err != nil {
						log.Printf("❌ Failed to update agent health in database: %v", err)
					}
				} else {
					query += ` WHERE agent_id = $1`
					_, err := c.hub.server.db.Exec(query, c.ID, c.hostname, c.os, c.architecture)
					if err != nil {
						log.Printf("❌ Failed to update agent health in database: %v", err)
					}
				}
			}
			
			log.Printf("🏥 Agent %s health update: hostname=%s, os=%s, arch=%s, data_dir=%s", 
				c.ID, c.hostname, c.os, c.architecture, c.dataDir)
		}
		log.Printf("📨 Agent message: %s", string(rawMessage))
	case "browse_response":
		// Handle browse folders response from agent
		c.hub.handleBrowseResponse(c.ID, msgData)
	case "browse_error":
		// Handle browse folders error from agent
		c.hub.handleBrowseError(c.ID, msgData)
	default:
		log.Printf("📨 Agent message: %s", string(rawMessage))
	}
}

func (c *AgentClient) handleCLIMessage(msgData map[string]interface{}, rawMessage []byte) {
	if agentID, ok := msgData["agent_id"].(string); ok {
		msgData["cli_id"] = c.ID
		modifiedMessage, _ := json.Marshal(msgData)
		
		c.hub.Broadcast(Message{
			Type: "command",
			From: c.ID,
			To:   agentID,
			Data: modifiedMessage,
		})
	} else {
		log.Printf("⚠️  CLI message missing agent_id: %s", string(rawMessage))
	}
}

func (c *AgentClient) WritePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		if c.conn != nil {
			c.conn.Close()
		}
	}()

	for {
		select {
		case message, ok := <-c.send:
			if c.conn == nil {
				return
			}
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			if c.conn == nil {
				return
			}
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Handle agent actions like approve/reject
func (s *SyncToolServer) handleAgentActions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if s.db == nil {
		http.Error(w, `{"error": "Database not connected"}`, http.StatusServiceUnavailable)
		return
	}

	// Parse URL path: /api/agents/{agentId}/{action}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/agents/"), "/")
	if len(pathParts) != 2 {
		http.Error(w, `{"error": "Invalid URL format. Expected: /api/agents/{agentId}/{action}"}`, http.StatusBadRequest)
		return
	}

	agentID := pathParts[0]
	action := pathParts[1]

	// Allow GET method for browse action
	if r.Method != "POST" && r.Method != "GET" {
		http.Error(w, `{"error": "Only POST and GET methods allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	switch action {
	case "approve":
		err := s.updateAgentApprovalStatus(agentID, "approved")
		if err != nil {
			log.Printf("❌ Failed to approve agent %s: %v", agentID, err)
			http.Error(w, fmt.Sprintf(`{"error": "Failed to approve agent: %v"}`, err), http.StatusInternalServerError)
			return
		}
		log.Printf("✅ Agent %s approved successfully", agentID)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("Agent %s approved successfully", agentID),
		})

	case "reject":
		err := s.updateAgentApprovalStatus(agentID, "rejected")
		if err != nil {
			log.Printf("❌ Failed to reject agent %s: %v", agentID, err)
			http.Error(w, fmt.Sprintf(`{"error": "Failed to reject agent: %v"}`, err), http.StatusInternalServerError)
			return
		}
		log.Printf("✅ Agent %s rejected successfully", agentID)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("Agent %s rejected successfully", agentID),
		})

	case "browse":
		if r.Method != "GET" {
			http.Error(w, `{"error": "Browse action requires GET method"}`, http.StatusMethodNotAllowed)
			return
		}
		s.handleBrowseFolders(w, r, agentID)

	case "delete":
		err := s.deleteAgent(agentID)
		if err != nil {
			log.Printf("❌ Failed to delete agent %s: %v", agentID, err)
			http.Error(w, fmt.Sprintf(`{"error": "Failed to delete agent: %v"}`, err), http.StatusInternalServerError)
			return
		}
		log.Printf("✅ Agent %s deleted successfully", agentID)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("Agent %s deleted successfully", agentID),
		})
		
	default:
		http.Error(w, fmt.Sprintf(`{"error": "Unknown action: %s"}`, action), http.StatusBadRequest)
	}
}

// Update agent approval status in database
func (s *SyncToolServer) updateAgentApprovalStatus(agentID, status string) error {
	if s.db == nil {
		return fmt.Errorf("database not connected")
	}

	_, err := s.db.Exec(`
		UPDATE integrated_agents 
		SET approval_status = $1, updated_at = $2 
		WHERE agent_id = $3
	`, status, time.Now(), agentID)

	return err
}

// Delete agent from database and memory
func (s *SyncToolServer) deleteAgent(agentID string) error {
	if s.db == nil {
		return fmt.Errorf("database not connected")
	}

	// First, delete any sync jobs that reference this agent
	_, err := s.db.Exec(`
		DELETE FROM sync_jobs 
		WHERE source_agent_id = $1 OR target_agent_id = $1
	`, agentID)
	if err != nil {
		log.Printf("⚠️ Failed to delete sync jobs for agent %s: %v", agentID, err)
		// Continue with agent deletion even if sync job deletion fails
	}

	// Delete the agent from database
	result, err := s.db.Exec(`
		DELETE FROM integrated_agents 
		WHERE agent_id = $1
	`, agentID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("agent not found")
	}

	// Remove from memory if currently connected
	s.hub.mutex.Lock()
	if agent, exists := s.hub.agents[agentID]; exists {
		// Close connection if active
		if agent.conn != nil {
			agent.conn.Close()
		}
		// Remove from hub
		delete(s.hub.agents, agentID)
	}
	s.hub.mutex.Unlock()

	return nil
}

// Handle sync jobs API
func (s *SyncToolServer) handleSyncJobs(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Content-Type", "application/json")
	
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if s.db == nil {
		http.Error(w, `{"error": "Database not available"}`, http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case "GET":
		s.handleGetSyncJobs(w, r)
	case "POST":
		s.handleCreateSyncJob(w, r)
	default:
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// Handle sync job actions (update, delete, pause, resume)
func (s *SyncToolServer) handleSyncJobActions(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Content-Type", "application/json")
	
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if s.db == nil {
		http.Error(w, `{"error": "Database not available"}`, http.StatusServiceUnavailable)
		return
	}

	// Parse URL path: /api/v1/sync-jobs/{id}/{action}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/sync-jobs/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		http.Error(w, `{"error": "Missing job ID"}`, http.StatusBadRequest)
		return
	}

	jobID := pathParts[0]
	
	if len(pathParts) == 1 {
		// Single job operations: GET, PUT, DELETE
		switch r.Method {
		case "GET":
			s.handleGetSyncJob(w, r, jobID)
		case "PUT":
			s.handleUpdateSyncJob(w, r, jobID)
		case "DELETE":
			s.handleDeleteSyncJob(w, r, jobID)
		default:
			http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		}
	} else if len(pathParts) == 2 {
		// Job actions: pause, resume
		action := pathParts[1]
		if r.Method != "POST" {
			http.Error(w, `{"error": "Only POST method allowed for actions"}`, http.StatusMethodNotAllowed)
			return
		}
		
		switch action {
		case "pause":
			s.handlePauseSyncJob(w, r, jobID)
		case "resume":
			s.handleResumeSyncJob(w, r, jobID)
		default:
			http.Error(w, fmt.Sprintf(`{"error": "Unknown action: %s"}`, action), http.StatusBadRequest)
		}
	} else {
		http.Error(w, `{"error": "Invalid URL format"}`, http.StatusBadRequest)
	}
}

// Get all sync jobs
// calculateSyncProgress calculates sync progress from folder stats cache
func (s *SyncToolServer) calculateSyncProgress(agentID string, jobID int) map[string]interface{} {
	// Try to get folder stats from cache
	// Note: folderID format is "job-{id}" and cache key is "agentID:folderID"
	folderID := fmt.Sprintf("job-%d", jobID)
	stats, exists := s.getFolderStatsResponse(agentID, folderID)

	// Default values when stats not available
	result := map[string]interface{}{
		"progress_file":       "0 / 0",
		"progress_percentage": 0.0,
		"sync_status":         "Pending",
		"last_synced":         nil,
	}

	if !exists {
		return result
	}

	// Extract stats data from response
	statsData, ok := stats["stats"].(map[string]interface{})
	if !ok {
		return result
	}

	// Get values from stats
	globalBytes, _ := statsData["globalBytes"].(float64)
	globalFiles, _ := statsData["globalFiles"].(float64)
	inSyncBytes, _ := statsData["inSyncBytes"].(float64)
	inSyncFiles, _ := statsData["inSyncFiles"].(float64)
	needBytes, _ := statsData["needBytes"].(float64)
	needFiles, _ := statsData["needFiles"].(float64)

	// Calculate progress percentage
	var progressPercentage float64
	if globalBytes > 0 {
		progressPercentage = (inSyncBytes / globalBytes) * 100
	}

	// Determine sync status
	syncStatus := "Pending"
	if progressPercentage >= 100 || (needBytes == 0 && needFiles == 0 && globalFiles > 0) {
		syncStatus = "Complete"
	} else if progressPercentage > 0 {
		syncStatus = "Partial"
	}

	// Get last_synced from progress data
	var lastSynced interface{} = nil
	if progressData, ok := stats["progress"].(map[string]interface{}); ok {
		if lastUpdated, ok := progressData["last_updated"].(string); ok && lastUpdated != "" {
			lastSynced = lastUpdated
		}
	}

	// Build result
	result = map[string]interface{}{
		"progress_file":       fmt.Sprintf("%.0f / %.0f", inSyncFiles, globalFiles),
		"progress_percentage": math.Round(progressPercentage*100) / 100, // Round to 2 decimal places
		"sync_status":         syncStatus,
		"last_synced":         lastSynced,
	}

	return result
}

// calculateSyncProgressMulti aggregates sync progress from multiple destinations
func (s *SyncToolServer) calculateSyncProgressMulti(jobID int, destinations []map[string]interface{}) map[string]interface{} {
	folderID := fmt.Sprintf("job-%d", jobID)

	var totalGlobalBytes, totalGlobalFiles float64
	var totalInSyncBytes, totalInSyncFiles float64
	var totalNeedBytes, totalNeedFiles float64
	var hasAnyStats bool
	var latestLastSynced interface{} = nil

	// Aggregate stats from all destinations
	for _, dest := range destinations {
		destAgentID, ok := dest["agent_id"].(string)
		if !ok {
			continue
		}

		stats, exists := s.getFolderStatsResponse(destAgentID, folderID)
		if !exists {
			continue
		}

		hasAnyStats = true

		// Extract stats data
		statsData, ok := stats["stats"].(map[string]interface{})
		if !ok {
			continue
		}

		// Aggregate values
		if globalBytes, ok := statsData["globalBytes"].(float64); ok {
			totalGlobalBytes += globalBytes
		}
		if globalFiles, ok := statsData["globalFiles"].(float64); ok {
			totalGlobalFiles += globalFiles
		}
		if inSyncBytes, ok := statsData["inSyncBytes"].(float64); ok {
			totalInSyncBytes += inSyncBytes
		}
		if inSyncFiles, ok := statsData["inSyncFiles"].(float64); ok {
			totalInSyncFiles += inSyncFiles
		}
		if needBytes, ok := statsData["needBytes"].(float64); ok {
			totalNeedBytes += needBytes
		}
		if needFiles, ok := statsData["needFiles"].(float64); ok {
			totalNeedFiles += needFiles
		}

		// Track latest last_synced
		if progressData, ok := stats["progress"].(map[string]interface{}); ok {
			if lastUpdated, ok := progressData["last_updated"].(string); ok && lastUpdated != "" {
				latestLastSynced = lastUpdated
			}
		}
	}

	// Default result when no stats available
	if !hasAnyStats {
		return map[string]interface{}{
			"progress_file":       "0 / 0",
			"progress_percentage": 0.0,
			"sync_status":         "Pending",
			"last_synced":         nil,
		}
	}

	// Calculate aggregate progress percentage
	var progressPercentage float64
	if totalGlobalBytes > 0 {
		progressPercentage = (totalInSyncBytes / totalGlobalBytes) * 100
	}

	// Determine aggregate sync status
	syncStatus := "Pending"
	if progressPercentage >= 100 || (totalNeedBytes == 0 && totalNeedFiles == 0 && totalGlobalFiles > 0) {
		syncStatus = "Complete"
	} else if progressPercentage > 0 {
		syncStatus = "Partial"
	}

	return map[string]interface{}{
		"progress_file":       fmt.Sprintf("%.0f / %.0f", totalInSyncFiles, totalGlobalFiles),
		"progress_percentage": math.Round(progressPercentage*100) / 100, // Round to 2 decimal places
		"sync_status":         syncStatus,
		"last_synced":         latestLastSynced,
		"total_bytes":         int64(totalGlobalBytes),
		"synced_bytes":        int64(totalInSyncBytes),
		"need_bytes":          int64(totalNeedBytes),
	}
}

func (s *SyncToolServer) handleGetSyncJobs(w http.ResponseWriter, r *http.Request) {
	// Get filter parameters
	searchKeyword := r.URL.Query().Get("search")
	syncStatusFilter := r.URL.Query().Get("sync_status") // Complete, Pending, Partial

	// Get user claims from context for role-based filtering
	var userRole string
	var assignedAgents []string
	claims, ok := r.Context().Value("user_claims").(*models.JWTClaims)
	if ok {
		userRole = claims.Role
		assignedAgents = claims.AssignedAgents
		log.Printf("🔐 User %s (role=%s) requesting sync jobs, assigned to %d agents", claims.Username, userRole, len(assignedAgents))
	}

	// Build SQL query with filters - now includes is_multi_destination
	query := `
		SELECT
			sj.id, sj.name, sj.source_agent_id, sj.target_agent_id,
			sj.source_path, sj.target_path, sj.sync_type,
			sj.rescan_interval, sj.ignore_patterns, sj.schedule_type,
			sj.status, sj.created_at, sj.updated_at,
			COALESCE(sj.is_multi_destination, false) as is_multi_destination,
			sa.hostname as source_agent_name, da.hostname as destination_agent_name
		FROM sync_jobs sj
		LEFT JOIN integrated_agents sa ON sj.source_agent_id = sa.agent_id
		LEFT JOIN integrated_agents da ON sj.target_agent_id = da.agent_id
	`

	// Build WHERE clause for search keyword and role-based filtering
	var whereConditions []string
	var args []interface{}
	argIndex := 1

	// Role-based filtering for operators
	if userRole == models.RoleOperator && len(assignedAgents) > 0 {
		// Operator: only show jobs involving assigned agents (as source OR destination)
		// For multi-destination jobs, also check sync_job_destinations table
		placeholders := make([]string, len(assignedAgents))
		for i, agentID := range assignedAgents {
			placeholders[i] = fmt.Sprintf("$%d", i+argIndex)
			args = append(args, agentID)
		}
		argIndex += len(assignedAgents)

		whereConditions = append(whereConditions, fmt.Sprintf(`(
			sj.source_agent_id IN (%s) OR
			sj.target_agent_id IN (%s) OR
			EXISTS (
				SELECT 1 FROM sync_job_destinations sjd
				WHERE sjd.job_id = sj.id
				AND sjd.destination_agent_id IN (%s)
			)
		)`, strings.Join(placeholders, ", "), strings.Join(placeholders, ", "), strings.Join(placeholders, ", ")))
	}

	if searchKeyword != "" {
		// Search in job name, source path, destination path, agent names
		whereConditions = append(whereConditions, fmt.Sprintf(`(
			sj.name ILIKE $%d OR
			sj.source_path ILIKE $%d OR
			sj.target_path ILIKE $%d OR
			sa.hostname ILIKE $%d OR
			da.hostname ILIKE $%d
		)`, argIndex, argIndex, argIndex, argIndex, argIndex))
		args = append(args, "%"+searchKeyword+"%")
		argIndex++
	}

	if len(whereConditions) > 0 {
		query += " WHERE " + strings.Join(whereConditions, " AND ")
	}

	query += " ORDER BY sj.created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		log.Printf("❌ Failed to query sync jobs: %v", err)
		http.Error(w, `{"error": "Failed to fetch sync jobs"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	syncJobs := []map[string]interface{}{}

	for rows.Next() {
		var id int
		var name, sourceAgentID, sourcePath, syncType, scheduleType, status string
		var targetAgentID, targetPath *string
		var sourceAgentName, destinationAgentName *string
		var createdAt, updatedAt time.Time
		var rescanInterval int
		var ignorePatterns pq.StringArray
		var isMultiDest bool

		if err := rows.Scan(&id, &name, &sourceAgentID, &targetAgentID, &sourcePath, &targetPath, &syncType, &rescanInterval, &ignorePatterns, &scheduleType, &status, &createdAt, &updatedAt, &isMultiDest, &sourceAgentName, &destinationAgentName); err != nil {
			log.Printf("❌ Failed to scan sync job row: %v", err)
			continue
		}

		// Map sync_type to sync_mode for frontend compatibility
		syncMode := "two-way"
		if syncType == "sendonly" || syncType == "receiveonly" {
			syncMode = "one-way"
		}

		// Map status to is_paused for frontend compatibility
		isPaused := status == "paused"

		// Build base job object
		syncJob := map[string]interface{}{
			"id":                   id,
			"name":                 name,
			"source_agent_id":      sourceAgentID,
			"source_path":          sourcePath,
			"sync_mode":            syncMode,
			"sync_type":            syncType,
			"schedule":             scheduleType,
			"rescan_interval":      rescanInterval,
			"max_file_size":        104857600, // Default 100MB
			"ignore_patterns":      []string(ignorePatterns),
			"auto_accept":          true,
			"is_paused":            isPaused,
			"status":               status,
			"source_agent_name":    getStringValue(sourceAgentName),
			"created_at":           createdAt.Format(time.RFC3339),
			"updated_at":           updatedAt.Format(time.RFC3339),
			"is_multi_destination": isMultiDest,
		}

		var syncProgress map[string]interface{}

		if isMultiDest {
			// Multi-destination mode: query all destinations
			destRows, err := s.db.Query(`
				SELECT
					sjd.destination_agent_id, sjd.destination_path,
					sjd.status, sjd.last_sync_status,
					sjd.files_synced, sjd.bytes_synced, sjd.last_sync_time,
					ia.hostname as agent_name
				FROM sync_job_destinations sjd
				LEFT JOIN integrated_agents ia ON sjd.destination_agent_id = ia.agent_id
				WHERE sjd.job_id = $1
				ORDER BY sjd.id
			`, id)

			if err != nil {
				log.Printf("❌ Failed to query destinations for job %d: %v", id, err)
			} else {
				destinations := []map[string]interface{}{}

				for destRows.Next() {
					var destAgentID, destPath, destStatus string
					var destLastSyncStatus *string
					var destFilesSynced, destBytesSynced int64
					var destLastSyncTime *time.Time
					var destAgentName *string

					if err := destRows.Scan(&destAgentID, &destPath, &destStatus, &destLastSyncStatus, &destFilesSynced, &destBytesSynced, &destLastSyncTime, &destAgentName); err != nil {
						log.Printf("❌ Failed to scan destination: %v", err)
						continue
					}

					dest := map[string]interface{}{
						"agent_id":         destAgentID,
						"path":             destPath,
						"status":           destStatus,
						"last_sync_status": getStringValue(destLastSyncStatus),
						"files_synced":     destFilesSynced,
						"bytes_synced":     destBytesSynced,
						"agent_name":       getStringValue(destAgentName),
					}

					if destLastSyncTime != nil {
						dest["last_sync_time"] = destLastSyncTime.Format(time.RFC3339)
					}

					destinations = append(destinations, dest)
				}
				destRows.Close()

				syncJob["destinations"] = destinations
				syncJob["destination_count"] = len(destinations)

				// Calculate aggregate progress for multi-destination
				syncProgress = s.calculateSyncProgressMulti(id, destinations)
			}
		} else {
			// Legacy single destination mode
			if targetAgentID != nil && targetPath != nil {
				syncJob["destination_agent_id"] = *targetAgentID
				syncJob["destination_path"] = *targetPath
				syncJob["destination_agent_name"] = getStringValue(destinationAgentName)

				// Calculate progress for single destination
				syncProgress = s.calculateSyncProgress(*targetAgentID, id)
			} else {
				// No destination configured
				syncProgress = map[string]interface{}{
					"progress_file":       "0/0 files",
					"progress_percentage": 0.0,
					"sync_status":         "Pending",
					"last_synced":         "",
				}
			}
		}

		// Get sync_status from progress
		calculatedSyncStatus := syncProgress["sync_status"].(string)

		// Apply sync_status filter if specified
		if syncStatusFilter != "" && calculatedSyncStatus != syncStatusFilter {
			continue // Skip this job if it doesn't match the filter
		}

		// Add progress fields to job
		syncJob["progress_file"] = syncProgress["progress_file"]
		syncJob["progress_percentage"] = syncProgress["progress_percentage"]
		syncJob["sync_status"] = calculatedSyncStatus
		syncJob["last_synced"] = syncProgress["last_synced"]

		syncJobs = append(syncJobs, syncJob)
	}

	response := map[string]interface{}{
		"sync_jobs": syncJobs,
		"total":     len(syncJobs),
	}

	json.NewEncoder(w).Encode(response)
}

// Create new sync job
func (s *SyncToolServer) handleCreateSyncJob(w http.ResponseWriter, r *http.Request) {
	var jobData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&jobData); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// Extract and validate required fields
	name, ok := jobData["name"].(string)
	if !ok || name == "" {
		http.Error(w, `{"error": "Missing or invalid name"}`, http.StatusBadRequest)
		return
	}

	sourceAgentID, ok := jobData["source_agent_id"].(string)
	if !ok || sourceAgentID == "" {
		http.Error(w, `{"error": "Missing or invalid source_agent_id"}`, http.StatusBadRequest)
		return
	}

	sourcePath, ok := jobData["source_path"].(string)
	if !ok || sourcePath == "" {
		http.Error(w, `{"error": "Missing or invalid source_path"}`, http.StatusBadRequest)
		return
	}

	// Check for multi-destination format (new) or single destination (legacy)
	var destinations []map[string]interface{}
	isMultiDestination := false

	if destArray, ok := jobData["destinations"].([]interface{}); ok && len(destArray) > 0 {
		// Multi-destination mode (new format)
		isMultiDestination = true
		for _, d := range destArray {
			if destMap, ok := d.(map[string]interface{}); ok {
				destinations = append(destinations, destMap)
			}
		}
	} else {
		// Single destination mode (legacy format for backward compatibility)
		destinationAgentID, ok := jobData["destination_agent_id"].(string)
		if !ok || destinationAgentID == "" {
			http.Error(w, `{"error": "Missing or invalid destination: must provide either 'destinations' array or 'destination_agent_id'"}`, http.StatusBadRequest)
			return
		}

		destinationPath, ok := jobData["destination_path"].(string)
		if !ok || destinationPath == "" {
			http.Error(w, `{"error": "Missing or invalid destination_path"}`, http.StatusBadRequest)
			return
		}

		// Convert single destination to array format
		destinations = []map[string]interface{}{
			{
				"agent_id": destinationAgentID,
				"path":     destinationPath,
			},
		}
	}

	// Validate destinations
	if len(destinations) == 0 {
		http.Error(w, `{"error": "At least one destination is required"}`, http.StatusBadRequest)
		return
	}

	// Role-based validation: operators can only create jobs with assigned agents
	claims, ok := r.Context().Value("user_claims").(*models.JWTClaims)
	if ok && claims.Role == models.RoleOperator && len(claims.AssignedAgents) > 0 {
		// Check source agent
		if !contains(claims.AssignedAgents, sourceAgentID) {
			http.Error(w, `{"error": "Access denied: You are not assigned to the source agent"}`, http.StatusForbidden)
			return
		}

		// Check all destination agents
		for i, dest := range destinations {
			destAgentID, ok := dest["agent_id"].(string)
			if !ok || destAgentID == "" {
				http.Error(w, fmt.Sprintf(`{"error": "Invalid agent_id in destination %d"}`, i+1), http.StatusBadRequest)
				return
			}

			if !contains(claims.AssignedAgents, destAgentID) {
				http.Error(w, fmt.Sprintf(`{"error": "Access denied: You are not assigned to destination agent %s"}`, destAgentID), http.StatusForbidden)
				return
			}
		}

		log.Printf("✅ Operator %s validated: has access to source and all %d destination(s)", claims.Username, len(destinations))
	}

	// Get sync_type - support both sync_type (direct) and sync_mode (legacy)
	syncType := "sendreceive" // Default two-way

	// Check if sync_type is provided directly (new API)
	if st, ok := jobData["sync_type"].(string); ok && st != "" {
		syncType = st
	} else if syncMode, ok := jobData["sync_mode"].(string); ok && syncMode != "" {
		// Legacy: map sync_mode to sync_type
		if syncMode == "one-way" {
			syncType = "sendonly"
		} else {
			syncType = "sendreceive"
		}
	}

	// Extract rescan_interval from frontend data (default to 3600)
	rescanInterval := 3600 // default
	if interval, ok := jobData["rescan_interval"].(float64); ok {
		rescanInterval = int(interval)
	}

	// Extract ignore_patterns from frontend data
	var ignorePatterns []string
	if patterns, ok := jobData["ignore_patterns"].([]interface{}); ok {
		for _, p := range patterns {
			if pattern, ok := p.(string); ok && strings.TrimSpace(pattern) != "" {
				ignorePatterns = append(ignorePatterns, strings.TrimSpace(pattern))
			}
		}
	}

	// Extract schedule - support both "schedule" and "schedule_type"
	schedule := "continuous" // default
	if sched, ok := jobData["schedule_type"].(string); ok && sched != "" {
		schedule = sched
	} else if sched, ok := jobData["schedule"].(string); ok && sched != "" {
		schedule = sched
	}

	// Start transaction for atomic job creation
	tx, err := s.db.Begin()
	if err != nil {
		log.Printf("❌ Failed to begin transaction: %v", err)
		http.Error(w, `{"error": "Failed to begin transaction"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback() // Rollback if not committed

	// Save to database to get the real job ID
	// For legacy single destination, still populate target_agent_id and target_path
	var targetAgentID, targetPath interface{}
	if !isMultiDestination && len(destinations) == 1 {
		targetAgentID = destinations[0]["agent_id"]
		targetPath = destinations[0]["path"]
	}

	var jobID int
	err = tx.QueryRow(`
		INSERT INTO sync_jobs (name, source_agent_id, target_agent_id, source_path, target_path, sync_type, status, rescan_interval, ignore_patterns, schedule_type, is_multi_destination)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id
	`, name, sourceAgentID, targetAgentID, sourcePath, targetPath, syncType, "active", rescanInterval, pq.Array(ignorePatterns), schedule, isMultiDestination).Scan(&jobID)

	if err != nil {
		log.Printf("❌ Failed to save sync job to database: %v", err)
		http.Error(w, `{"error": "Failed to save sync job to database"}`, http.StatusInternalServerError)
		return
	}

	// Insert destinations into sync_job_destinations table
	for _, dest := range destinations {
		agentID, ok := dest["agent_id"].(string)
		if !ok || agentID == "" {
			log.Printf("❌ Invalid destination agent_id")
			http.Error(w, `{"error": "Invalid destination agent_id"}`, http.StatusBadRequest)
			return
		}

		destPath, ok := dest["path"].(string)
		if !ok || destPath == "" {
			log.Printf("❌ Invalid destination path")
			http.Error(w, `{"error": "Invalid destination path"}`, http.StatusBadRequest)
			return
		}

		_, err = tx.Exec(`
			INSERT INTO sync_job_destinations (job_id, destination_agent_id, destination_path, status)
			VALUES ($1, $2, $3, $4)
		`, jobID, agentID, destPath, "active")

		if err != nil {
			log.Printf("❌ Failed to save destination to database: %v", err)
			http.Error(w, `{"error": "Failed to save destination to database"}`, http.StatusInternalServerError)
			return
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		log.Printf("❌ Failed to commit transaction: %v", err)
		http.Error(w, `{"error": "Failed to commit transaction"}`, http.StatusInternalServerError)
		return
	}

	// Now deploy to agents with the real job ID
	log.Printf("🚀 Deploying job %d to %d destination(s)...", jobID, len(destinations))
	deployErr := s.deployJobToAgentsSyncMulti(fmt.Sprintf("%d", jobID), name, sourceAgentID, sourcePath, destinations, syncType, rescanInterval, ignorePatterns)

	if deployErr != nil {
		log.Printf("❌ Failed to deploy job to agents: %v", deployErr)
		// Rollback: delete the job from database since agent deployment failed
		s.db.Exec("DELETE FROM sync_jobs WHERE id = $1", jobID)
		http.Error(w, fmt.Sprintf(`{"error": "Failed to deploy job to agents: %v"}`, deployErr), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success":              true,
		"message":              "Sync job created and deployed successfully",
		"job_id":               jobID,
		"is_multi_destination": isMultiDestination,
		"destination_count":    len(destinations),
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// Update sync job
func (s *SyncToolServer) handleUpdateSyncJob(w http.ResponseWriter, r *http.Request, jobID string) {
	var jobData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&jobData); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// Map sync_mode to sync_type for database storage
	syncMode, _ := jobData["sync_mode"].(string)
	syncType := "sendreceive" // Default two-way
	if syncMode == "one-way" {
		syncType = "sendonly" // Default to sendonly for one-way
	}

	// Extract rescan_interval from frontend data (default to 3600)  
	rescanInterval := 3600 // default
	if interval, ok := jobData["rescan_interval"].(float64); ok {
		rescanInterval = int(interval)
	}
	
	// Extract ignore_patterns from frontend data
	var ignorePatterns []string
	if patterns, ok := jobData["ignore_patterns"].([]interface{}); ok {
		for _, p := range patterns {
			if pattern, ok := p.(string); ok && strings.TrimSpace(pattern) != "" {
				ignorePatterns = append(ignorePatterns, strings.TrimSpace(pattern))
			}
		}
	}

	// Extract schedule from frontend data
	schedule := "continuous" // default
	if sched, ok := jobData["schedule"].(string); ok && sched != "" {
		schedule = sched
	}

	_, err := s.db.Exec(`
		UPDATE sync_jobs 
		SET name = $1, source_agent_id = $2, target_agent_id = $3, 
		    source_path = $4, target_path = $5, sync_type = $6, 
		    rescan_interval = $7, ignore_patterns = $8, schedule_type = $9, updated_at = $10
		WHERE id = $11
	`, jobData["name"], jobData["source_agent_id"], jobData["destination_agent_id"],
	   jobData["source_path"], jobData["destination_path"], syncType, 
	   rescanInterval, pq.Array(ignorePatterns), schedule, time.Now(), jobID)
	
	if err != nil {
		log.Printf("❌ Failed to update sync job: %v", err)
		http.Error(w, `{"error": "Failed to update sync job"}`, http.StatusInternalServerError)
		return
	}

	// Re-deploy job configuration to agents after update
	sourceAgentID := jobData["source_agent_id"].(string)
	destinationAgentID := jobData["destination_agent_id"].(string)
	sourcePath := jobData["source_path"].(string)
	destinationPath := jobData["destination_path"].(string)
	name := jobData["name"].(string)
	
	log.Printf("🔄 Re-deploying updated job %s to agents...", jobID)
	deployErr := s.deployJobToAgentsSync(jobID, name, sourceAgentID, destinationAgentID, sourcePath, destinationPath, syncType, rescanInterval, ignorePatterns)
	
	if deployErr != nil {
		log.Printf("❌ Failed to re-deploy updated job to agents: %v", deployErr)
		http.Error(w, fmt.Sprintf(`{"error": "Job updated in database but failed to re-deploy to agents: %v"}`, deployErr), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success": true,
		"message": "Sync job updated successfully",
	}
	
	json.NewEncoder(w).Encode(response)
}

// Delete sync job
func (s *SyncToolServer) handleDeleteSyncJob(w http.ResponseWriter, r *http.Request, jobID string) {
	// Get job details before deletion to clear cache
	var sourceAgentID, destinationAgentID string
	err := s.db.QueryRow(`
		SELECT source_agent_id, target_agent_id FROM sync_jobs WHERE id = $1
	`, jobID).Scan(&sourceAgentID, &destinationAgentID)

	if err != nil && err != sql.ErrNoRows {
		log.Printf("❌ Failed to get job details: %v", err)
	}

	// First, try to delete from agents
	deleteErr := s.deleteJobOnAgentsSync(jobID)
	if deleteErr != nil {
		log.Printf("❌ Failed to delete job from agents: %v", deleteErr)
		http.Error(w, fmt.Sprintf(`{"error": "Failed to delete job from agents: %v"}`, deleteErr), http.StatusInternalServerError)
		return
	}

	// Only delete from database if agent deletion was successful
	_, err = s.db.Exec("DELETE FROM sync_jobs WHERE id = $1", jobID)
	if err != nil {
		log.Printf("❌ Failed to delete sync job from database: %v", err)
		http.Error(w, `{"error": "Failed to delete sync job from database"}`, http.StatusInternalServerError)
		return
	}

	// Clear folder stats cache for both agents since job is deleted
	if sourceAgentID != "" {
		s.clearFolderStatsResponse(sourceAgentID)
		log.Printf("🗑️ Cleared folder stats cache for source agent %s", sourceAgentID)
	}
	if destinationAgentID != "" {
		s.clearFolderStatsResponse(destinationAgentID)
		log.Printf("🗑️ Cleared folder stats cache for destination agent %s", destinationAgentID)
	}

	response := map[string]interface{}{
		"success": true,
		"message": "Sync job deleted successfully",
	}

	json.NewEncoder(w).Encode(response)
}

// Pause sync job
func (s *SyncToolServer) handlePauseSyncJob(w http.ResponseWriter, r *http.Request, jobID string) {
	// First, try to pause on agents
	pauseErr := s.pauseJobOnAgentsSync(jobID)
	if pauseErr != nil {
		log.Printf("❌ Failed to pause job on agents: %v", pauseErr)
		http.Error(w, fmt.Sprintf(`{"error": "Failed to pause job on agents: %v"}`, pauseErr), http.StatusInternalServerError)
		return
	}

	// Only update database if agent pause was successful
	_, err := s.db.Exec("UPDATE sync_jobs SET status = 'paused', updated_at = $1 WHERE id = $2", time.Now(), jobID)
	if err != nil {
		log.Printf("❌ Failed to update sync job status in database: %v", err)
		// Rollback: resume the job on agents since DB update failed
		go s.resumeJobOnAgents(jobID)
		http.Error(w, `{"error": "Failed to update sync job status"}`, http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success": true,
		"message": "Sync job paused successfully",
	}
	
	json.NewEncoder(w).Encode(response)
}

// Resume sync job
func (s *SyncToolServer) handleResumeSyncJob(w http.ResponseWriter, r *http.Request, jobID string) {
	// First, try to resume on agents
	resumeErr := s.resumeJobOnAgentsSync(jobID)
	if resumeErr != nil {
		log.Printf("❌ Failed to resume job on agents: %v", resumeErr)
		http.Error(w, fmt.Sprintf(`{"error": "Failed to resume job on agents: %v"}`, resumeErr), http.StatusInternalServerError)
		return
	}

	// Only update database if agent resume was successful
	_, err := s.db.Exec("UPDATE sync_jobs SET status = 'active', updated_at = $1 WHERE id = $2", time.Now(), jobID)
	if err != nil {
		log.Printf("❌ Failed to update sync job status in database: %v", err)
		// Rollback: pause the job on agents since DB update failed
		go s.pauseJobOnAgents(jobID)
		http.Error(w, `{"error": "Failed to update sync job status"}`, http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success": true,
		"message": "Sync job resumed successfully",
	}
	
	json.NewEncoder(w).Encode(response)
}

// Get single sync job
func (s *SyncToolServer) handleGetSyncJob(w http.ResponseWriter, r *http.Request, jobID string) {
	var id int
	var name, sourceAgentID, destinationAgentID, sourcePath, destinationPath, syncType, status string
	var createdAt, updatedAt time.Time
	
	err := s.db.QueryRow(`
		SELECT id, name, source_agent_id, target_agent_id, source_path, target_path, sync_type, status, created_at, updated_at
		FROM sync_jobs WHERE id = $1
	`, jobID).Scan(&id, &name, &sourceAgentID, &destinationAgentID, &sourcePath, &destinationPath, &syncType, &status, &createdAt, &updatedAt)
	
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, `{"error": "Sync job not found"}`, http.StatusNotFound)
		} else {
			log.Printf("❌ Failed to get sync job: %v", err)
			http.Error(w, `{"error": "Failed to get sync job"}`, http.StatusInternalServerError)
		}
		return
	}

	// Map sync_type to sync_mode for frontend compatibility
	syncMode := "two-way"
	if syncType == "sendonly" || syncType == "receiveonly" {
		syncMode = "one-way"
	}
	
	// Map status to is_paused for frontend compatibility
	isPaused := status == "paused"
	
	syncJob := map[string]interface{}{
		"id":                   id,
		"name":                 name,
		"source_agent_id":      sourceAgentID,
		"destination_agent_id": destinationAgentID,
		"source_path":          sourcePath,
		"destination_path":     destinationPath,
		"sync_mode":            syncMode,
		"sync_type":            syncType,
		"schedule":             "continuous",
		"rescan_interval":      3600,
		"max_file_size":        104857600,
		"ignore_patterns":      []string{},
		"auto_accept":          true,
		"is_paused":            isPaused,
		"status":               status,
		"created_at":           createdAt.Format(time.RFC3339),
		"updated_at":           updatedAt.Format(time.RFC3339),
	}
	
	json.NewEncoder(w).Encode(syncJob)
}

// Helper function to get string value from nullable string pointer
func getStringValue(s *string) string {
	if s == nil {
		return "N/A"
	}
	return *s
}

// Helper function to check if string exists in slice
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Deploy job to agents via WebSocket
func (s *SyncToolServer) deployJobToAgents(jobID int, name, sourceAgentID, destinationAgentID, sourcePath, destinationPath, syncType string) {
	log.Printf("🚀 Deploying sync job %d to agents: %s → %s", jobID, sourceAgentID, destinationAgentID)
	
	// Get source and destination agents device IDs
	sourceDeviceID, destDeviceID, err := s.getAgentDeviceIDs(sourceAgentID, destinationAgentID)
	if err != nil {
		log.Printf("❌ Failed to get device IDs for job %d: %v", jobID, err)
		return
	}
	
	folderID := fmt.Sprintf("job-%d", jobID)
	
	// Create job configuration for source agent (sender)
	sourceJobConfig := map[string]interface{}{
		"type":         "deploy_job",
		"job_id":       jobID,
		"folder_id":    folderID,
		"folder_name":  name,
		"folder_path":  sourcePath,
		"folder_type":  syncType,
		"device_id":    destDeviceID,
		"device_name":  destinationAgentID,
		"role":         "source",
	}
	
	// Create job configuration for destination agent (receiver)  
	destJobConfig := map[string]interface{}{
		"type":         "deploy_job",
		"job_id":       jobID,
		"folder_id":    folderID,
		"folder_name":  name,
		"folder_path":  destinationPath,
		"folder_type":  syncType,
		"device_id":    sourceDeviceID,
		"device_name":  sourceAgentID,
		"role":         "destination",
	}
	
	// Send to source agent
	if err := s.sendJobToAgent(sourceAgentID, sourceJobConfig); err != nil {
		log.Printf("❌ Failed to send job to source agent %s: %v", sourceAgentID, err)
	}
	
	// Send to destination agent
	if err := s.sendJobToAgent(destinationAgentID, destJobConfig); err != nil {
		log.Printf("❌ Failed to send job to destination agent %s: %v", destinationAgentID, err)
	}
}

// Re-deploy job when resumed
func (s *SyncToolServer) redeployJobToAgents(jobIDStr string) {
	jobID := jobIDStr // String conversion if needed
	
	// Get job details from database
	var id int
	var name, sourceAgentID, destinationAgentID, sourcePath, destinationPath, syncType string
	
	err := s.db.QueryRow(`
		SELECT id, name, source_agent_id, target_agent_id, source_path, target_path, sync_type
		FROM sync_jobs WHERE id = $1
	`, jobID).Scan(&id, &name, &sourceAgentID, &destinationAgentID, &sourcePath, &destinationPath, &syncType)
	
	if err != nil {
		log.Printf("❌ Failed to get job details for redeploy: %v", err)
		return
	}
	
	log.Printf("🔄 Re-deploying job %d after resume", id)
	s.deployJobToAgents(id, name, sourceAgentID, destinationAgentID, sourcePath, destinationPath, syncType)
}

// Get device IDs for source and destination agents
func (s *SyncToolServer) getAgentDeviceIDs(sourceAgentID, destinationAgentID string) (string, string, error) {
	var sourceDeviceID, destDeviceID string
	
	// Get source agent device ID
	err := s.db.QueryRow("SELECT device_id FROM integrated_agents WHERE agent_id = $1", sourceAgentID).Scan(&sourceDeviceID)
	if err != nil {
		return "", "", fmt.Errorf("failed to get source agent device ID: %w", err)
	}
	
	// Get destination agent device ID
	err = s.db.QueryRow("SELECT device_id FROM integrated_agents WHERE agent_id = $1", destinationAgentID).Scan(&destDeviceID)
	if err != nil {
		return "", "", fmt.Errorf("failed to get destination agent device ID: %w", err)
	}
	
	return sourceDeviceID, destDeviceID, nil
}

// Send job configuration to specific agent via WebSocket
func (s *SyncToolServer) sendJobToAgent(agentID string, jobConfig map[string]interface{}) error {
	s.hub.mutex.RLock()
	agent, exists := s.hub.agents[agentID]
	s.hub.mutex.RUnlock()
	
	if !exists {
		return fmt.Errorf("agent %s not connected", agentID)
	}
	
	if !agent.isOnline || agent.send == nil {
		return fmt.Errorf("agent %s is offline or channel closed", agentID)
	}
	
	// Serialize job config to JSON
	jobData, err := json.Marshal(jobConfig)
	if err != nil {
		return fmt.Errorf("failed to serialize job config: %w", err)
	}
	
	// Send to agent
	select {
	case agent.send <- jobData:
		log.Printf("✅ Job config sent to agent %s", agentID)
		return nil
	default:
		return fmt.Errorf("agent %s channel is full", agentID)
	}
}

// Send pause command to agents
func (s *SyncToolServer) pauseJobOnAgents(jobID string) {
	// Get job details
	var sourceAgentID, destinationAgentID string
	err := s.db.QueryRow(`
		SELECT source_agent_id, target_agent_id FROM sync_jobs WHERE id = $1
	`, jobID).Scan(&sourceAgentID, &destinationAgentID)
	
	if err != nil {
		log.Printf("❌ Failed to get job details for pause: %v", err)
		return
	}
	
	folderID := fmt.Sprintf("job-%s", jobID)
	
	pauseConfig := map[string]interface{}{
		"type":      "pause_job",
		"job_id":    jobID,
		"folder_id": folderID,
	}
	
	// Send pause command to both agents
	s.sendJobToAgent(sourceAgentID, pauseConfig)
	s.sendJobToAgent(destinationAgentID, pauseConfig)
	
	log.Printf("📄 Pause command sent for job %s", jobID)
}

// Send resume command to agents  
func (s *SyncToolServer) resumeJobOnAgents(jobID string) {
	// Get job details
	var sourceAgentID, destinationAgentID string
	err := s.db.QueryRow(`
		SELECT source_agent_id, target_agent_id FROM sync_jobs WHERE id = $1
	`, jobID).Scan(&sourceAgentID, &destinationAgentID)
	
	if err != nil {
		log.Printf("❌ Failed to get job details for resume: %v", err)
		return
	}
	
	folderID := fmt.Sprintf("job-%s", jobID)
	
	resumeConfig := map[string]interface{}{
		"type":      "resume_job", 
		"job_id":    jobID,
		"folder_id": folderID,
	}
	
	// Send resume command to both agents
	s.sendJobToAgent(sourceAgentID, resumeConfig)
	s.sendJobToAgent(destinationAgentID, resumeConfig)
	
	log.Printf("▶️ Resume command sent for job %s", jobID)
}

// Send delete command to agents
func (s *SyncToolServer) deleteJobOnAgents(jobID string) {
	// Get job details before deletion
	var sourceAgentID, destinationAgentID string
	err := s.db.QueryRow(`
		SELECT source_agent_id, target_agent_id FROM sync_jobs WHERE id = $1
	`, jobID).Scan(&sourceAgentID, &destinationAgentID)
	
	if err != nil {
		log.Printf("❌ Failed to get job details for deletion: %v", err)
		return
	}
	
	folderID := fmt.Sprintf("job-%s", jobID)
	
	deleteConfig := map[string]interface{}{
		"type":      "delete_job",
		"job_id":    jobID,
		"folder_id": folderID,
	}
	
	// Send delete command to both agents
	s.sendJobToAgent(sourceAgentID, deleteConfig)
	s.sendJobToAgent(destinationAgentID, deleteConfig)
	
	log.Printf("🗑️ Delete command sent for job %s", jobID)
}

// Synchronous job deployment - waits for confirmation from agents
func (s *SyncToolServer) deployJobToAgentsSync(jobID, name, sourceAgentID, destinationAgentID, sourcePath, destinationPath, syncType string, rescanInterval int, ignorePatterns []string) error {
	log.Printf("🚀 Deploying job %s to agents synchronously...", jobID)
	
	// Get device IDs from hub
	sourceDeviceID := s.hub.GetAgentDeviceID(sourceAgentID)
	destinationDeviceID := s.hub.GetAgentDeviceID(destinationAgentID)
	
	if sourceDeviceID == "" {
		return fmt.Errorf("source agent %s not connected or device ID not available", sourceAgentID)
	}
	
	if destinationDeviceID == "" {
		return fmt.Errorf("destination agent %s not connected or device ID not available", destinationAgentID)
	}
	
	// 🔧 NEW: Get IP addresses for automatic device pairing
	sourceIPAddress := s.hub.GetAgentIPAddress(sourceAgentID)
	destinationIPAddress := s.hub.GetAgentIPAddress(destinationAgentID)
	
	if sourceIPAddress == "" {
		return fmt.Errorf("source agent %s IP address not available", sourceAgentID)
	}
	
	if destinationIPAddress == "" {
		return fmt.Errorf("destination agent %s IP address not available", destinationAgentID)
	}
	
	log.Printf("📋 Job deployment details: source=%s(%s), dest=%s(%s)", sourceAgentID, sourceIPAddress, destinationAgentID, destinationIPAddress)
	
	// Create job configuration with agent IDs for role identification and device IDs for Syncthing
	jobConfig := map[string]interface{}{
		"type":                  "deploy_job",
		"job_id":                jobID,
		"name":                  name,
		"source_agent_id":       sourceAgentID,         // Use agent ID for role identification
		"destination_agent_id":  destinationAgentID,    // Use agent ID for role identification
		"source_device_id":      sourceDeviceID,        // Use device ID for Syncthing configuration
		"destination_device_id": destinationDeviceID,   // Use device ID for Syncthing configuration
		"source_ip_address":     sourceIPAddress,       // 🆕 IP address for automatic device pairing
		"destination_ip_address": destinationIPAddress, // 🆕 IP address for automatic device pairing
		"source_path":           sourcePath,
		"destination_path":      destinationPath,
		"sync_type":             syncType,
		"rescan_interval_s":     rescanInterval,        // Add rescan interval support
		"ignore_patterns":       ignorePatterns,        // Add ignore patterns support
	}
	
	// Send to both agents and wait for confirmation
	sourceErr := s.sendJobToAgentSync(sourceAgentID, jobConfig)
	if sourceErr != nil {
		return fmt.Errorf("failed to deploy to source agent %s: %v", sourceAgentID, sourceErr)
	}
	
	destErr := s.sendJobToAgentSync(destinationAgentID, jobConfig)
	if destErr != nil {
		// Rollback source deployment if destination fails
		rollbackConfig := map[string]interface{}{
			"type":   "delete_job",
			"job_id": jobID,
		}
		s.sendJobToAgent(sourceAgentID, rollbackConfig)
		return fmt.Errorf("failed to deploy to destination agent %s: %v", destinationAgentID, destErr)
	}
	
	log.Printf("✅ Job %s deployed successfully to both agents", jobID)
	return nil
}

// Deploy job to multiple destinations (new multi-destination support)
func (s *SyncToolServer) deployJobToAgentsSyncMulti(jobID, name, sourceAgentID, sourcePath string, destinations []map[string]interface{}, syncType string, rescanInterval int, ignorePatterns []string) error {
	log.Printf("🚀 Deploying job %s to source and %d destination(s)...", jobID, len(destinations))

	// Get source device ID and IP
	sourceDeviceID := s.hub.GetAgentDeviceID(sourceAgentID)
	if sourceDeviceID == "" {
		return fmt.Errorf("source agent %s not connected or device ID not available", sourceAgentID)
	}

	sourceIPAddress := s.hub.GetAgentIPAddress(sourceAgentID)
	if sourceIPAddress == "" {
		return fmt.Errorf("source agent %s IP address not available", sourceAgentID)
	}

	// Collect all destination device IDs and IPs
	var destDeviceIDs []string
	var destAgentIDs []string
	var destIPAddresses []string
	destConfigs := make(map[string]map[string]interface{})

	for i, dest := range destinations {
		destAgentID := dest["agent_id"].(string)
		destPath := dest["path"].(string)

		destDeviceID := s.hub.GetAgentDeviceID(destAgentID)
		if destDeviceID == "" {
			return fmt.Errorf("destination agent %s not connected or device ID not available", destAgentID)
		}

		destIPAddress := s.hub.GetAgentIPAddress(destAgentID)
		if destIPAddress == "" {
			return fmt.Errorf("destination agent %s IP address not available", destAgentID)
		}

		destDeviceIDs = append(destDeviceIDs, destDeviceID)
		destAgentIDs = append(destAgentIDs, destAgentID)
		destIPAddresses = append(destIPAddresses, destIPAddress)

		destConfigs[destAgentID] = map[string]interface{}{
			"agent_id":   destAgentID,
			"device_id":  destDeviceID,
			"ip_address": destIPAddress,
			"path":       destPath,
		}

		log.Printf("📋 Destination %d: agent=%s(%s), device=%s", i+1, destAgentID, destIPAddress, destDeviceID)
	}

	// Deploy to source agent
	// Source folder will sync to ALL destination devices
	sourceJobConfig := map[string]interface{}{
		"type":                    "deploy_job",
		"job_id":                  jobID,
		"name":                    name,
		"source_agent_id":         sourceAgentID,
		"destination_agent_ids":   destAgentIDs,      // Array of all dest agent IDs
		"destination_device_ids":  destDeviceIDs,     // Array of all dest device IDs
		"destination_ip_addresses": destIPAddresses,  // Array of all dest IPs
		"source_path":             sourcePath,
		"sync_type":               syncType,
		"rescan_interval_s":       rescanInterval,
		"ignore_patterns":         ignorePatterns,
		"is_multi_destination":    true,
	}

	sourceErr := s.sendJobToAgentSync(sourceAgentID, sourceJobConfig)
	if sourceErr != nil {
		return fmt.Errorf("failed to deploy to source agent %s: %v", sourceAgentID, sourceErr)
	}

	log.Printf("✅ Source agent %s deployed successfully", sourceAgentID)

	// Deploy to each destination agent
	var deployedDests []string
	for destAgentID, destConfig := range destConfigs {
		destJobConfig := map[string]interface{}{
			"type":                  "deploy_job",
			"job_id":                jobID,
			"name":                  name,
			"source_agent_id":       sourceAgentID,
			"destination_agent_id":  destAgentID, // This dest's agent ID
			"source_device_id":      sourceDeviceID,
			"destination_device_id": destConfig["device_id"].(string),
			"source_ip_address":     sourceIPAddress,
			"destination_ip_address": destConfig["ip_address"].(string),
			"destination_path":      destConfig["path"].(string),
			"sync_type":             syncType,
			"rescan_interval_s":     rescanInterval,
			"ignore_patterns":       ignorePatterns,
			"is_multi_destination":  true,
		}

		destErr := s.sendJobToAgentSync(destAgentID, destJobConfig)
		if destErr != nil {
			// Rollback: delete from source and already deployed destinations
			log.Printf("❌ Failed to deploy to destination %s, rolling back...", destAgentID)
			rollbackConfig := map[string]interface{}{
				"type":   "delete_job",
				"job_id": jobID,
			}
			s.sendJobToAgent(sourceAgentID, rollbackConfig)
			for _, deployedDest := range deployedDests {
				s.sendJobToAgent(deployedDest, rollbackConfig)
			}
			return fmt.Errorf("failed to deploy to destination agent %s: %v", destAgentID, destErr)
		}

		deployedDests = append(deployedDests, destAgentID)
		log.Printf("✅ Destination agent %s deployed successfully", destAgentID)
	}

	log.Printf("🎉 Job %s deployed successfully to source and %d destination(s)", jobID, len(destinations))
	return nil
}

// Synchronous job pause - waits for confirmation from agents
func (s *SyncToolServer) pauseJobOnAgentsSync(jobID string) error {
	// Get job details and check if multi-destination
	var sourceAgentID string
	var isMultiDest bool
	err := s.db.QueryRow(`
		SELECT source_agent_id, COALESCE(is_multi_destination, false) FROM sync_jobs WHERE id = $1
	`, jobID).Scan(&sourceAgentID, &isMultiDest)

	if err != nil {
		return fmt.Errorf("failed to get job details: %v", err)
	}

	pauseConfig := map[string]interface{}{
		"type":   "pause_job",
		"job_id": jobID,
	}

	// Pause source agent
	sourceErr := s.sendJobToAgentSync(sourceAgentID, pauseConfig)
	if sourceErr != nil {
		return fmt.Errorf("failed to pause on source agent %s: %v", sourceAgentID, sourceErr)
	}

	if isMultiDest {
		// Get all destinations from sync_job_destinations table
		rows, err := s.db.Query(`
			SELECT destination_agent_id FROM sync_job_destinations
			WHERE job_id = $1 AND status = 'active'
		`, jobID)
		if err != nil {
			return fmt.Errorf("failed to get destinations: %v", err)
		}
		defer rows.Close()

		var pausedDests []string
		for rows.Next() {
			var destAgentID string
			if err := rows.Scan(&destAgentID); err != nil {
				log.Printf("❌ Failed to scan destination: %v", err)
				continue
			}

			destErr := s.sendJobToAgentSync(destAgentID, pauseConfig)
			if destErr != nil {
				// Rollback: resume source and already paused destinations
				log.Printf("❌ Failed to pause destination %s, rolling back...", destAgentID)
				rollbackConfig := map[string]interface{}{
					"type":   "resume_job",
					"job_id": jobID,
				}
				s.sendJobToAgent(sourceAgentID, rollbackConfig)
				for _, pausedDest := range pausedDests {
					s.sendJobToAgent(pausedDest, rollbackConfig)
				}
				return fmt.Errorf("failed to pause on destination agent %s: %v", destAgentID, destErr)
			}

			pausedDests = append(pausedDests, destAgentID)
		}

		log.Printf("⏸️ Job %s paused successfully on source and %d destination(s)", jobID, len(pausedDests))
	} else {
		// Legacy single destination
		var destinationAgentID string
		err := s.db.QueryRow(`
			SELECT target_agent_id FROM sync_jobs WHERE id = $1
		`, jobID).Scan(&destinationAgentID)

		if err != nil {
			return fmt.Errorf("failed to get destination agent: %v", err)
		}

		destErr := s.sendJobToAgentSync(destinationAgentID, pauseConfig)
		if destErr != nil {
			// Rollback source pause if destination fails
			rollbackConfig := map[string]interface{}{
				"type":   "resume_job",
				"job_id": jobID,
			}
			s.sendJobToAgent(sourceAgentID, rollbackConfig)
			return fmt.Errorf("failed to pause on destination agent %s: %v", destinationAgentID, destErr)
		}

		log.Printf("⏸️ Job %s paused successfully on both agents", jobID)
	}

	return nil
}

// Synchronous job resume - waits for confirmation from agents
func (s *SyncToolServer) resumeJobOnAgentsSync(jobID string) error {
	// Get job details and check if multi-destination
	var sourceAgentID string
	var isMultiDest bool
	err := s.db.QueryRow(`
		SELECT source_agent_id, COALESCE(is_multi_destination, false) FROM sync_jobs WHERE id = $1
	`, jobID).Scan(&sourceAgentID, &isMultiDest)

	if err != nil {
		return fmt.Errorf("failed to get job details: %v", err)
	}

	resumeConfig := map[string]interface{}{
		"type":   "resume_job",
		"job_id": jobID,
	}

	// Resume source agent
	sourceErr := s.sendJobToAgentSync(sourceAgentID, resumeConfig)
	if sourceErr != nil {
		return fmt.Errorf("failed to resume on source agent %s: %v", sourceAgentID, sourceErr)
	}

	if isMultiDest {
		// Get all destinations from sync_job_destinations table
		rows, err := s.db.Query(`
			SELECT destination_agent_id FROM sync_job_destinations
			WHERE job_id = $1
		`, jobID)
		if err != nil {
			return fmt.Errorf("failed to get destinations: %v", err)
		}
		defer rows.Close()

		var resumedDests []string
		for rows.Next() {
			var destAgentID string
			if err := rows.Scan(&destAgentID); err != nil {
				log.Printf("❌ Failed to scan destination: %v", err)
				continue
			}

			destErr := s.sendJobToAgentSync(destAgentID, resumeConfig)
			if destErr != nil {
				// Rollback: pause source and already resumed destinations
				log.Printf("❌ Failed to resume destination %s, rolling back...", destAgentID)
				rollbackConfig := map[string]interface{}{
					"type":   "pause_job",
					"job_id": jobID,
				}
				s.sendJobToAgent(sourceAgentID, rollbackConfig)
				for _, resumedDest := range resumedDests {
					s.sendJobToAgent(resumedDest, rollbackConfig)
				}
				return fmt.Errorf("failed to resume on destination agent %s: %v", destAgentID, destErr)
			}

			resumedDests = append(resumedDests, destAgentID)
		}

		log.Printf("▶️ Job %s resumed successfully on source and %d destination(s)", jobID, len(resumedDests))
	} else {
		// Legacy single destination
		var destinationAgentID string
		err := s.db.QueryRow(`
			SELECT target_agent_id FROM sync_jobs WHERE id = $1
		`, jobID).Scan(&destinationAgentID)

		if err != nil {
			return fmt.Errorf("failed to get destination agent: %v", err)
		}

		destErr := s.sendJobToAgentSync(destinationAgentID, resumeConfig)
		if destErr != nil {
			// Rollback source resume if destination fails
			rollbackConfig := map[string]interface{}{
				"type":   "pause_job",
				"job_id": jobID,
			}
			s.sendJobToAgent(sourceAgentID, rollbackConfig)
			return fmt.Errorf("failed to resume on destination agent %s: %v", destinationAgentID, destErr)
		}

		log.Printf("▶️ Job %s resumed successfully on both agents", jobID)
	}

	return nil
}

// Synchronous job deletion - waits for confirmation from agents
func (s *SyncToolServer) deleteJobOnAgentsSync(jobID string) error {
	// Get job details and check if multi-destination
	var sourceAgentID string
	var isMultiDest bool
	err := s.db.QueryRow(`
		SELECT source_agent_id, COALESCE(is_multi_destination, false) FROM sync_jobs WHERE id = $1
	`, jobID).Scan(&sourceAgentID, &isMultiDest)

	if err != nil {
		return fmt.Errorf("failed to get job details: %v", err)
	}

	deleteConfig := map[string]interface{}{
		"type":   "delete_job",
		"job_id": jobID,
	}

	// Delete from source agent
	sourceErr := s.sendJobToAgentSync(sourceAgentID, deleteConfig)
	if sourceErr != nil {
		return fmt.Errorf("failed to delete on source agent %s: %v", sourceAgentID, sourceErr)
	}

	if isMultiDest {
		// Get all destinations from sync_job_destinations table
		rows, err := s.db.Query(`
			SELECT destination_agent_id FROM sync_job_destinations
			WHERE job_id = $1
		`, jobID)
		if err != nil {
			return fmt.Errorf("failed to get destinations: %v", err)
		}
		defer rows.Close()

		var deletedCount int
		for rows.Next() {
			var destAgentID string
			if err := rows.Scan(&destAgentID); err != nil {
				log.Printf("❌ Failed to scan destination: %v", err)
				continue
			}

			destErr := s.sendJobToAgentSync(destAgentID, deleteConfig)
			if destErr != nil {
				log.Printf("⚠️ Failed to delete on destination agent %s: %v (continuing anyway)", destAgentID, destErr)
				// Don't rollback deletion - best effort cleanup
			} else {
				deletedCount++
			}
		}

		log.Printf("🗑️ Job %s deleted from source and %d destination(s)", jobID, deletedCount)
	} else {
		// Legacy single destination
		var destinationAgentID string
		err := s.db.QueryRow(`
			SELECT target_agent_id FROM sync_jobs WHERE id = $1
		`, jobID).Scan(&destinationAgentID)

		if err != nil {
			return fmt.Errorf("failed to get destination agent: %v", err)
		}

		destErr := s.sendJobToAgentSync(destinationAgentID, deleteConfig)
		if destErr != nil {
			return fmt.Errorf("failed to delete on destination agent %s: %v", destinationAgentID, destErr)
		}

		log.Printf("🗑️ Job %s deleted successfully from both agents", jobID)
	}

	return nil
}

// Update job ID on agents (for when temp ID is replaced with real ID)
func (s *SyncToolServer) updateJobIDOnAgents(oldJobID, newJobID string) {
	// This would send a command to agents to update their folder IDs
	// For now, we can skip this as agents use job ID to generate folder IDs
	log.Printf("🔄 Job ID updated from %s to %s", oldJobID, newJobID)
}

// Send job command to agent synchronously and wait for response
func (s *SyncToolServer) sendJobToAgentSync(agentID string, jobConfig map[string]interface{}) error {
	// For now, we'll use the async version and add timeout later
	// In production, this should wait for a response from the agent
	return s.sendJobToAgent(agentID, jobConfig)
}

// Store folder stats response from agent
func (s *SyncToolServer) storeFolderStatsResponse(agentID string, data map[string]interface{}) {
	s.folderStatsMu.Lock()
	defer s.folderStatsMu.Unlock()

	if s.folderStats == nil {
		s.folderStats = make(map[string]map[string]interface{})
	}

	// Extract folderID from data - try multiple locations
	var folderID string

	// First try: folder_id at root level (from agent response)
	if id, ok := data["folder_id"].(string); ok && id != "" {
		folderID = id
	}

	// Second try: id inside stats object
	if folderID == "" {
		if statsData, ok := data["stats"].(map[string]interface{}); ok {
			if id, ok := statsData["id"].(string); ok {
				folderID = id
			}
		}
	}

	if folderID == "" {
		log.Printf("⚠️  Cannot store folder stats: missing folder ID in data: %+v", data)
		return
	}

	// Use composite key: "agentID:folderID"
	key := fmt.Sprintf("%s:%s", agentID, folderID)
	s.folderStats[key] = data
	log.Printf("📊 Stored folder stats for agent %s, folder %s (key: %s)", agentID, folderID, key)
}

// Get stored folder stats response for agent
func (s *SyncToolServer) getFolderStatsResponse(agentID string, folderID string) (map[string]interface{}, bool) {
	s.folderStatsMu.RLock()
	defer s.folderStatsMu.RUnlock()

	if s.folderStats == nil {
		return nil, false
	}

	// Use composite key: "agentID:folderID"
	key := fmt.Sprintf("%s:%s", agentID, folderID)
	data, exists := s.folderStats[key]
	return data, exists
}

// Clear stored folder stats response for agent (clears all folders for this agent)
func (s *SyncToolServer) clearFolderStatsResponse(agentID string) {
	s.folderStatsMu.Lock()
	defer s.folderStatsMu.Unlock()

	if s.folderStats == nil {
		return
	}

	// Find and delete all keys that start with "agentID:"
	prefix := agentID + ":"
	deletedCount := 0
	for key := range s.folderStats {
		if strings.HasPrefix(key, prefix) {
			delete(s.folderStats, key)
			deletedCount++
		}
	}

	if deletedCount > 0 {
		log.Printf("🗑️ Cleared %d cached folder stats for agent %s", deletedCount, agentID)
	}
}

// Clear stored folder stats for a specific folder only
func (s *SyncToolServer) clearFolderStatsForFolder(agentID string, folderID string) {
	s.folderStatsMu.Lock()
	defer s.folderStatsMu.Unlock()

	if s.folderStats == nil {
		return
	}

	key := fmt.Sprintf("%s:%s", agentID, folderID)
	if _, exists := s.folderStats[key]; exists {
		delete(s.folderStats, key)
		log.Printf("🗑️ Cleared cached folder stats for agent %s, folder %s (key: %s)", agentID, folderID, key)
	}
}

// Mark agent syncing status (for tracking when agents are actively syncing)
func (s *SyncToolServer) markAgentSyncingStatus(agentID string, isSyncing bool) {
	s.syncJobsMu.Lock()
	defer s.syncJobsMu.Unlock()

	if s.activeSyncJobs == nil {
		s.activeSyncJobs = make(map[string]bool)
	}

	if isSyncing {
		s.activeSyncJobs[agentID] = true
		log.Printf("📊 Marked agent %s as actively syncing", agentID)
	} else {
		s.activeSyncJobs[agentID] = false
		// DO NOT clear cache when agent becomes idle (pause/complete)
		// Cache should persist to show last known stats
		log.Printf("📊 Marked agent %s as idle, keeping cache for last known stats", agentID)
	}
}

// Check if agent is actively syncing
func (s *SyncToolServer) isAgentSyncing(agentID string) bool {
	s.syncJobsMu.RLock()
	defer s.syncJobsMu.RUnlock()
	
	if s.activeSyncJobs == nil {
		return false
	}
	
	return s.activeSyncJobs[agentID]
}

// Handle browse folders request
func (s *SyncToolServer) handleBrowseFolders(w http.ResponseWriter, r *http.Request, agentID string) {
	// Get query parameters
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/" // Default to root
	}
	
	depth := 2 // Default depth
	if depthStr := r.URL.Query().Get("depth"); depthStr != "" {
		if d, err := strconv.Atoi(depthStr); err == nil && d > 0 {
			depth = d
		}
	}
	
	log.Printf("📁 Browse folders request: agent=%s, path=%s, depth=%d", agentID, path, depth)
	
	// Check if agent is online
	s.hub.mutex.RLock()
	agent, exists := s.hub.agents[agentID]
	if !exists || !agent.isOnline || agent.send == nil {
		s.hub.mutex.RUnlock()
		log.Printf("❌ Agent %s is not online for browse request", agentID)
		http.Error(w, `{"error": "Agent is not online"}`, http.StatusServiceUnavailable)
		return
	}
	s.hub.mutex.RUnlock()
	
	// Create request ID for tracking
	requestID := fmt.Sprintf("%s-browse", agentID)
	
	// Create response channel
	responseChannel := make(chan map[string]interface{}, 1)
	
	// Register the request
	s.hub.mutex.Lock()
	s.hub.browseRequests[requestID] = responseChannel
	s.hub.mutex.Unlock()
	
	// Create browse message for agent
	browseMessage := map[string]interface{}{
		"type":  "browse_folders",
		"path":  path,
		"depth": depth,
	}
	
	// Send message to agent
	browseData, _ := json.Marshal(browseMessage)
	select {
	case agent.send <- browseData:
		log.Printf("✅ Browse request sent to agent %s for path %s", agentID, path)
	default:
		// Clean up and return error
		s.hub.mutex.Lock()
		delete(s.hub.browseRequests, requestID)
		s.hub.mutex.Unlock()
		close(responseChannel)
		http.Error(w, `{"error": "Agent channel is full"}`, http.StatusServiceUnavailable)
		return
	}
	
	// Wait for response with timeout
	select {
	case response := <-responseChannel:
		close(responseChannel)
		
		// Check if this is an error response
		if errorMsg, isError := response["error"]; isError {
			log.Printf("❌ Agent browse error: %v", errorMsg)
			http.Error(w, fmt.Sprintf(`{"error": "%v"}`, errorMsg), http.StatusInternalServerError)
			return
		}
		
		// Extract data from response
		data, hasData := response["data"]
		if !hasData {
			log.Printf("❌ Agent browse response missing data field")
			http.Error(w, `{"error": "Invalid agent response"}`, http.StatusInternalServerError)
			return
		}
		
		log.Printf("✅ Returning agent browse response for %s: %s", agentID, path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
		
	case <-time.After(30 * time.Second):
		// Clean up and return timeout error
		s.hub.mutex.Lock()
		delete(s.hub.browseRequests, requestID)
		s.hub.mutex.Unlock()
		close(responseChannel)
		log.Printf("❌ Browse request timeout for agent %s", agentID)
		http.Error(w, `{"error": "Browse request timeout"}`, http.StatusGatewayTimeout)
	}
}

// browseFileSystem browses the local filesystem (demo implementation)
func (s *SyncToolServer) browseFileSystem(path string, depth int) (map[string]interface{}, error) {
	// Import the required packages (os, ioutil are already imported)
	
	// Clean the path
	if path == "" || path == "/" {
		path = "/"
	}
	
	// Check if path exists and is accessible
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("path not accessible: %v", err)
	}
	
	result := map[string]interface{}{
		"name":         filepath.Base(path),
		"path":         path,
		"is_directory": info.IsDir(),
	}
	
	// If it's a directory and we have depth remaining, get children
	if info.IsDir() && depth > 0 {
		children, err := s.getDirectoryChildren(path, depth-1)
		if err != nil {
			log.Printf("⚠️ Failed to get children for %s: %v", path, err)
			// Don't fail completely, just return without children
		} else {
			result["children"] = children
		}
	}
	
	return result, nil
}

// getDirectoryChildren gets child directories and files for server-side browsing
func (s *SyncToolServer) getDirectoryChildren(dirPath string, remainingDepth int) ([]map[string]interface{}, error) {
	entries, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	
	var children []map[string]interface{}
	
	for _, entry := range entries {
		// Skip hidden files/directories starting with .
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		
		childPath := filepath.Join(dirPath, entry.Name())
		
		child := map[string]interface{}{
			"name":         entry.Name(),
			"path":         childPath,
			"is_directory": entry.IsDir(),
		}
		
		// If it's a directory and we have remaining depth, recurse
		if entry.IsDir() && remainingDepth > 0 {
			grandChildren, err := s.getDirectoryChildren(childPath, remainingDepth-1)
			if err != nil {
				// Skip directories we can't read
				log.Printf("⚠️ Skipping unreadable directory %s: %v", childPath, err)
				continue
			}
			child["children"] = grandChildren
		}
		
		children = append(children, child)
	}
	
	return children, nil
}

// parseFileTransferLogParams parses and validates query parameters
func (s *SyncToolServer) parseFileTransferLogParams(query url.Values) (*FileTransferLogParams, error) {
	params := &FileTransferLogParams{
		Page:  1,
		Limit: 20,
	}

	// Parse page
	if pageStr := query.Get("page"); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil && page > 0 {
			params.Page = page
		} else {
			return nil, fmt.Errorf("invalid page parameter: %s", pageStr)
		}
	}

	// Parse limit with bounds
	if limitStr := query.Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			if limit > 100 {
				params.Limit = 100 // Cap at 100 per page
			} else {
				params.Limit = limit
			}
		} else {
			return nil, fmt.Errorf("invalid limit parameter: %s", limitStr)
		}
	}

	// Parse offset
	if offsetStr := query.Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			params.Offset = offset
			// Calculate page from offset (prevent division by zero)
			if params.Limit > 0 {
				params.Page = (offset / params.Limit) + 1
			} else {
				params.Page = 1
			}
		} else {
			return nil, fmt.Errorf("invalid offset parameter: %s", offsetStr)
		}
	}

	// Parse cursor for cursor-based pagination
	params.Cursor = query.Get("cursor")

	// Parse search (support both 'search' and 'file_name' parameters)
	search := strings.TrimSpace(query.Get("search"))
	fileName := strings.TrimSpace(query.Get("file_name"))
	
	// Use file_name if provided, otherwise use search
	if fileName != "" {
		params.Search = fileName
	} else if search != "" {
		params.Search = search
	}

	// Parse status filter
	if statusStr := query.Get("status"); statusStr != "" {
		params.Status = strings.Split(statusStr, ",")
		for i, status := range params.Status {
			params.Status[i] = strings.TrimSpace(status)
		}
	}

	// Parse job name filter
	if jobStr := query.Get("job_name"); jobStr != "" {
		params.JobName = strings.Split(jobStr, ",")
		for i, job := range params.JobName {
			params.JobName[i] = strings.TrimSpace(job)
		}
	}

	// Parse action filter
	if actionStr := query.Get("action"); actionStr != "" {
		params.Action = strings.Split(actionStr, ",")
		for i, action := range params.Action {
			params.Action[i] = strings.TrimSpace(action)
		}
	}

	// Parse agent filter
	params.AgentID = strings.TrimSpace(query.Get("agent_id"))

	// Parse date filters
	params.DateFrom = query.Get("date_from")
	params.DateTo = query.Get("date_to")

	return params, nil
}

// validatePaginationLimits enforces hybrid pagination strategy
func (s *SyncToolServer) validatePaginationLimits(params *FileTransferLogParams) error {
	maxPages := s.getMaxAllowedPages(params.HasFilters())
	
	if params.Page > maxPages {
		if params.HasFilters() {
			return fmt.Errorf("page %d exceeds limit of %d pages for filtered results", params.Page, maxPages)
		} else {
			return fmt.Errorf("page %d exceeds limit of %d pages - please use search or filters to narrow results", params.Page, maxPages)
		}
	}
	
	return nil
}

// getMaxAllowedPages returns max pages based on filter context
func (s *SyncToolServer) getMaxAllowedPages(hasFilters bool) int {
	if hasFilters {
		return 100 // Allow full 100 pages when filters are applied
	}
	return 100 // Still 100, but with different error message
}

// getTotalCount gets optimized count query without complex JOINs when possible
func (qb *FileTransferLogQueryBuilder) getTotalCount() (int, error) {
	query := `SELECT COUNT(*) FROM file_transfer_logs ftl`
	conditions, args := qb.buildWhereConditions()
	
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	var count int
	err := qb.db.QueryRow(query, args...).Scan(&count)
	return count, err
}

// getData gets data with cursor or offset pagination
func (qb *FileTransferLogQueryBuilder) getData() ([]map[string]interface{}, string, error) {
	// Build optimized query with cursor pagination
	baseQuery := `SELECT ftl.id, ftl.job_id, ftl.job_name, ftl.agent_id, ftl.file_name, 
		ftl.file_path, ftl.file_size, ftl.status, ftl.action, ftl.progress, 
		ftl.transfer_rate, ftl.duration, ftl.error_message, ftl.started_at, 
		ftl.completed_at, ftl.created_at,
		sj.sync_type, sa.hostname as source_agent_name, da.hostname as destination_agent_name
		FROM file_transfer_logs ftl
		LEFT JOIN sync_jobs sj ON (
			CASE 
				WHEN ftl.job_id IS NOT NULL AND ftl.job_id ~ '^job-[0-9]+$' 
				THEN sj.id = CAST(SUBSTRING(ftl.job_id FROM 5) AS INTEGER)
				ELSE FALSE
			END
		)
		LEFT JOIN integrated_agents sa ON sj.source_agent_id = sa.agent_id  
		LEFT JOIN integrated_agents da ON sj.target_agent_id = da.agent_id`

	conditions, args := qb.buildWhereConditions()
	argIndex := len(args) + 1

	// Add cursor condition for pagination
	if qb.params.Cursor != "" {
		if cursorID, err := strconv.Atoi(qb.params.Cursor); err == nil {
			conditions = append(conditions, fmt.Sprintf("ftl.id < $%d", argIndex))
			args = append(args, cursorID)
			argIndex++
		}
	}

	if len(conditions) > 0 {
		baseQuery += " WHERE " + strings.Join(conditions, " AND ")
	}

	// Order by ID for consistent cursor pagination
	baseQuery += " ORDER BY ftl.id DESC"
	
	// Add limit
	baseQuery += fmt.Sprintf(" LIMIT $%d", argIndex)
	args = append(args, qb.params.Limit+1) // +1 to check if there's next page
	argIndex++
	
	// Add offset if provided
	if qb.params.Offset > 0 {
		baseQuery += fmt.Sprintf(" OFFSET $%d", argIndex)
		args = append(args, qb.params.Offset)
	}

	// Execute query
	rows, err := qb.db.Query(baseQuery, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	// Parse results
	var logs []map[string]interface{}
	var nextCursor string
	var hasNextPage bool

	for rows.Next() {
		logEntry, id, err := qb.scanLogRow(rows)
		if err != nil {
			log.Printf("❌ Failed to scan file transfer log row: %v", err)
			continue
		}

		logs = append(logs, logEntry)
		if qb.params.Cursor != "" {
			nextCursor = strconv.Itoa(id) // Set cursor for cursor-based pagination
		}
	}

	// Check if there's a next page
	if len(logs) > qb.params.Limit {
		hasNextPage = true
		logs = logs[:qb.params.Limit] // Remove the extra row
		
		// For cursor-based pagination, keep the cursor
		if qb.params.Cursor != "" && len(logs) > 0 {
			// Get ID of last actual record (not the extra one)
			if lastLog, ok := logs[len(logs)-1]["id"].(int); ok {
				nextCursor = strconv.Itoa(lastLog)
			}
		}
	} else {
		hasNextPage = false
		if qb.params.Cursor != "" {
			nextCursor = "" // No more pages for cursor-based
		}
	}

	// For offset-based pagination, return hasNextPage info via nextCursor
	if qb.params.Cursor == "" {
		if hasNextPage {
			nextCursor = "has_more" // Indicate there are more pages
		} else {
			nextCursor = "" // No more pages
		}
	}

	return logs, nextCursor, nil
}

// buildWhereConditions builds WHERE conditions and args
func (qb *FileTransferLogQueryBuilder) buildWhereConditions() ([]string, []interface{}) {
	var conditions []string
	var args []interface{}
	argIndex := 1

	// Always exclude empty downloading entries and metadata actions
	conditions = append(conditions, "NOT (ftl.status = 'downloading' AND (ftl.file_name IS NULL OR ftl.file_name = ''))")
	conditions = append(conditions, "ftl.action != 'metadata'")

	// Operator filtering: only show logs from jobs with assigned agents
	if qb.params.UserRole == models.RoleOperator && len(qb.params.AssignedAgents) > 0 {
		placeholders := make([]string, len(qb.params.AssignedAgents))
		for i, agentID := range qb.params.AssignedAgents {
			placeholders[i] = fmt.Sprintf("$%d", argIndex)
			args = append(args, agentID)
			argIndex++
		}
		conditions = append(conditions, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM sync_jobs sj
			WHERE CAST(sj.id AS TEXT) = ftl.job_id
			AND (sj.source_agent_id IN (%s) OR sj.dest_agent_id IN (%s))
		)`, strings.Join(placeholders, ", "), strings.Join(placeholders, ", ")))
		// Note: args already added above, will be duplicated for second IN clause
		for _, agentID := range qb.params.AssignedAgents {
			args = append(args, agentID)
			argIndex++
		}
	}

	// Search condition (optimized for performance)
	if qb.params.Search != "" {
		searchTerm := qb.params.Search
		
		// Use prefix search if possible (more efficient)
		if len(searchTerm) >= 3 && !strings.Contains(searchTerm, "%") {
			conditions = append(conditions, fmt.Sprintf("ftl.file_name ILIKE $%d", argIndex))
			args = append(args, searchTerm+"%") // Prefix search: "file_0%"
			argIndex++
		} else {
			// Fallback to full text search (slower but more flexible)
			conditions = append(conditions, fmt.Sprintf("ftl.file_name ILIKE $%d", argIndex))
			args = append(args, "%"+searchTerm+"%")
			argIndex++
		}
	}

	// Status filter
	if len(qb.params.Status) > 0 {
		placeholders := make([]string, len(qb.params.Status))
		for i, status := range qb.params.Status {
			placeholders[i] = fmt.Sprintf("$%d", argIndex)
			args = append(args, status)
			argIndex++
		}
		conditions = append(conditions, fmt.Sprintf("ftl.status IN (%s)", strings.Join(placeholders, ",")))
	}

	// Job name filter
	if len(qb.params.JobName) > 0 {
		placeholders := make([]string, len(qb.params.JobName))
		for i, jobName := range qb.params.JobName {
			placeholders[i] = fmt.Sprintf("$%d", argIndex)
			args = append(args, jobName)
			argIndex++
		}
		conditions = append(conditions, fmt.Sprintf("ftl.job_name IN (%s)", strings.Join(placeholders, ",")))
	}

	// Action filter
	if len(qb.params.Action) > 0 {
		placeholders := make([]string, len(qb.params.Action))
		for i, action := range qb.params.Action {
			placeholders[i] = fmt.Sprintf("$%d", argIndex)
			args = append(args, action)
			argIndex++
		}
		conditions = append(conditions, fmt.Sprintf("ftl.action IN (%s)", strings.Join(placeholders, ",")))
	}

	// Agent filter
	if qb.params.AgentID != "" {
		conditions = append(conditions, fmt.Sprintf("ftl.agent_id = $%d", argIndex))
		args = append(args, qb.params.AgentID)
		argIndex++
	}

	// Date range filters
	if qb.params.DateFrom != "" {
		conditions = append(conditions, fmt.Sprintf("ftl.created_at >= $%d", argIndex))
		args = append(args, qb.params.DateFrom)
		argIndex++
	}

	if qb.params.DateTo != "" {
		conditions = append(conditions, fmt.Sprintf("ftl.created_at <= $%d", argIndex))
		args = append(args, qb.params.DateTo)
		argIndex++
	}

	return conditions, args
}

// scanLogRow scans a single row from database
func (qb *FileTransferLogQueryBuilder) scanLogRow(rows *sql.Rows) (map[string]interface{}, int, error) {
	var (
		id                  int
		jobID               sql.NullString
		jobName             sql.NullString
		agentID             string
		fileName            string
		filePath            sql.NullString
		fileSize            sql.NullInt64
		status              string
		action              sql.NullString
		progress            sql.NullFloat64
		transferRate        sql.NullFloat64
		duration            sql.NullFloat64
		errorMessage        sql.NullString
		startedAt           *time.Time
		completedAt         *time.Time
		createdAt           time.Time
		syncType            sql.NullString
		sourceAgentName     sql.NullString
		destinationAgentName sql.NullString
	)

	err := rows.Scan(&id, &jobID, &jobName, &agentID, &fileName, &filePath,
		&fileSize, &status, &action, &progress, &transferRate, &duration, &errorMessage,
		&startedAt, &completedAt, &createdAt, &syncType, &sourceAgentName, &destinationAgentName)
	if err != nil {
		return nil, 0, err
	}

	logEntry := map[string]interface{}{
		"id":        id,
		"agent_id":  agentID,
		"file_name": fileName,
		"status":    status,
		"created_at": createdAt.Format(time.RFC3339),
	}

	// Add optional fields
	if jobID.Valid {
		logEntry["job_id"] = jobID.String
	}
	if jobName.Valid {
		logEntry["job_name"] = jobName.String
		logEntry["job"] = jobName.String
	}
	if action.Valid {
		logEntry["action"] = action.String
	}
	if filePath.Valid {
		logEntry["file_path"] = filePath.String
	}
	if fileSize.Valid {
		logEntry["file_size"] = fileSize.Int64
	}
	if progress.Valid {
		logEntry["progress"] = progress.Float64
	}
	if transferRate.Valid {
		logEntry["transfer_rate"] = transferRate.Float64
	}
	if duration.Valid {
		logEntry["duration"] = duration.Float64
	}
	if errorMessage.Valid {
		logEntry["error_message"] = errorMessage.String
	}
	if startedAt != nil {
		logEntry["started_at"] = startedAt.Format(time.RFC3339)
	}
	if completedAt != nil {
		logEntry["completed_at"] = completedAt.Format(time.RFC3339)
	}
	
	// Add agent names and sync mode
	if sourceAgentName.Valid {
		logEntry["source_agent_name"] = sourceAgentName.String
	} else {
		logEntry["source_agent_name"] = "N/A"
	}
	if destinationAgentName.Valid {
		logEntry["destination_agent_name"] = destinationAgentName.String
	} else {
		logEntry["destination_agent_name"] = "N/A"
	}
	
	// Map sync_type to sync_mode
	if syncType.Valid {
		syncMode := "two-way"
		if syncType.String == "sendonly" || syncType.String == "receiveonly" {
			syncMode = "one-way"
		}
		logEntry["sync_mode"] = syncMode
		
		// Build source_target field
		if sourceAgentName.Valid && destinationAgentName.Valid {
			if syncMode == "two-way" {
				logEntry["source_target"] = sourceAgentName.String + " ↔ " + destinationAgentName.String
			} else {
				logEntry["source_target"] = sourceAgentName.String + " → " + destinationAgentName.String
			}
		}
	}

	return logEntry, id, nil
}

// handleFileTransferLogs handles API requests for file transfer logs with hybrid pagination
// Features: 100-page limit + cursor pagination + enhanced search/filter
func (s *SyncToolServer) handleFileTransferLogs(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers for all requests
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	
	// Handle preflight OPTIONS request
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Check database connection
	if s.db == nil {
		http.Error(w, "Database not available", http.StatusInternalServerError)
		return
	}

	// Get user claims for operator filtering
	claims, ok := r.Context().Value("user_claims").(*models.JWTClaims)

	// Parse and validate query parameters
	query := r.URL.Query()
	params, err := s.parseFileTransferLogParams(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Add user role and assigned agents to params for filtering
	if ok {
		params.UserRole = claims.Role
		params.AssignedAgents = claims.AssignedAgents
	}

	// Check pagination limits with search/filter context
	if err := s.validatePaginationLimits(params); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
			"suggestion": "Please use search or filter to narrow down results",
			"max_pages": 100,
			"current_page": params.Page,
		})
		return
	}

	// Build optimized query with cursor pagination
	queryBuilder := &FileTransferLogQueryBuilder{
		params: params,
		db:     s.db,
	}

	// Get total count (with cache for frequent queries)
	totalCount, err := queryBuilder.getTotalCount()
	if err != nil {
		log.Printf("❌ Failed to get file transfer logs count: %v", err)
		http.Error(w, "Failed to query database", http.StatusInternalServerError)
		return
	}

	// Get data with cursor pagination
	logs, nextCursor, err := queryBuilder.getData()
	if err != nil {
		log.Printf("❌ Failed to query file transfer logs: %v", err)
		http.Error(w, "Failed to query database", http.StatusInternalServerError)
		return
	}

	// Calculate pagination metadata
	totalPages := int(math.Ceil(float64(totalCount) / float64(params.Limit)))
	maxAllowedPages := s.getMaxAllowedPages(params.HasFilters())

	// Determine has_next based on pagination mode
	var hasNext bool
	if params.Cursor != "" {
		// Cursor-based pagination: has_next based on nextCursor
		hasNext = nextCursor != ""
	} else {
		// Offset-based pagination: use nextCursor="has_more" or traditional logic
		if nextCursor == "has_more" {
			hasNext = true
		} else {
			hasNext = params.Page < totalPages && params.Page < maxAllowedPages
		}
	}

	// Clean up nextCursor for offset-based pagination
	if params.Cursor == "" && nextCursor == "has_more" {
		nextCursor = "" // Don't expose internal "has_more" flag
	}

	// Response format with hybrid pagination info
	response := map[string]interface{}{
		"logs":        logs,
		"pagination": map[string]interface{}{
			"current_page":    params.Page,
			"per_page":        params.Limit,
			"total_count":     totalCount,
			"total_pages":     totalPages,
			"max_pages":       maxAllowedPages,
			"has_next":        hasNext,
			"has_prev":        params.Page > 1,
			"next_cursor":     nextCursor,
			"has_filters":     params.HasFilters(),
		},
		"filters_applied": params.GetAppliedFilters(),
		// Legacy compatibility fields
		"total":           totalCount,
		"limit":           params.Limit,
		"offset":          func() int {
			if params.Offset > 0 {
				return params.Offset
			}
			return (params.Page - 1) * params.Limit
		}(),
	}

	// Send response
	json.NewEncoder(w).Encode(response)
	log.Printf("✅ File transfer logs API: returned %d logs (total: %d, page: %d/%d)", 
		len(logs), totalCount, params.Page, totalPages)
}

// handleTransferStats handles API requests for file transfer statistics
func (s *SyncToolServer) handleTransferStats(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers for all requests
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	
	// Handle preflight OPTIONS request
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Check database connection
	if s.db == nil {
		http.Error(w, "Database not available", http.StatusInternalServerError)
		return
	}

	// Get user claims for operator filtering
	var userRole string
	var assignedAgents []string
	claims, ok := r.Context().Value("user_claims").(*models.JWTClaims)
	if ok {
		userRole = claims.Role
		assignedAgents = claims.AssignedAgents
	}

	// Parse query parameters for filtering
	query := r.URL.Query()

	// Build WHERE conditions for filtering
	var conditions []string
	var args []interface{}
	argIndex := 1

	// Operator filtering: only show transfer stats from jobs with assigned agents
	if userRole == models.RoleOperator && len(assignedAgents) > 0 {
		placeholders := make([]string, len(assignedAgents))
		for i, agentID := range assignedAgents {
			placeholders[i] = fmt.Sprintf("$%d", argIndex)
			args = append(args, agentID)
			argIndex++
		}
		conditions = append(conditions, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM sync_jobs sj
			WHERE CAST(sj.id AS TEXT) = ftl.job_id
			AND (sj.source_agent_id IN (%s) OR sj.dest_agent_id IN (%s))
		)`, strings.Join(placeholders, ", "), strings.Join(placeholders, ", ")))
		// Duplicate args for second IN clause
		for _, agentID := range assignedAgents {
			args = append(args, agentID)
			argIndex++
		}
	}

	// Status filter
	if status := query.Get("status"); status != "" {
		statuses := strings.Split(status, ",")
		placeholders := make([]string, len(statuses))
		for i, s := range statuses {
			placeholders[i] = fmt.Sprintf("$%d", argIndex)
			args = append(args, strings.TrimSpace(s))
			argIndex++
		}
		conditions = append(conditions, fmt.Sprintf("ftl.status IN (%s)", strings.Join(placeholders, ",")))
	}

	// Job name filter
	if jobName := query.Get("job_name"); jobName != "" {
		jobNames := strings.Split(jobName, ",")
		placeholders := make([]string, len(jobNames))
		for i, name := range jobNames {
			placeholders[i] = fmt.Sprintf("$%d", argIndex)
			args = append(args, strings.TrimSpace(name))
			argIndex++
		}
		conditions = append(conditions, fmt.Sprintf("ftl.job_name IN (%s)", strings.Join(placeholders, ",")))
	}

	// Action filter
	if action := query.Get("action"); action != "" {
		actions := strings.Split(action, ",")
		placeholders := make([]string, len(actions))
		for i, a := range actions {
			placeholders[i] = fmt.Sprintf("$%d", argIndex)
			args = append(args, strings.TrimSpace(a))
			argIndex++
		}
		conditions = append(conditions, fmt.Sprintf("ftl.action IN (%s)", strings.Join(placeholders, ",")))
	}

	// Search filter (file_name)
	if search := query.Get("search"); search != "" {
		conditions = append(conditions, fmt.Sprintf("ftl.file_name ILIKE $%d", argIndex))
		args = append(args, "%"+search+"%")
		argIndex++
	}

	// Date range filters
	if dateFrom := query.Get("date_from"); dateFrom != "" {
		conditions = append(conditions, fmt.Sprintf("ftl.started_at >= $%d", argIndex))
		args = append(args, dateFrom+"T00:00:00Z")
		argIndex++
	}

	if dateTo := query.Get("date_to"); dateTo != "" {
		conditions = append(conditions, fmt.Sprintf("ftl.started_at <= $%d", argIndex))
		args = append(args, dateTo+"T23:59:59Z")
		argIndex++
	}
	
	// Build WHERE clause
	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}
	
	// Execute aggregate queries
	statsQuery := fmt.Sprintf(`
		SELECT
			COUNT(*) as total_transfers,
			COUNT(CASE WHEN ftl.status = 'completed' THEN 1 END) as completed_transfers,
			COUNT(CASE WHEN ftl.status = 'failed' THEN 1 END) as failed_transfers,
			COUNT(CASE WHEN ftl.status = 'transferring' OR ftl.status = 'in_progress' OR ftl.status = 'started' THEN 1 END) as in_progress_transfers,
			COALESCE(SUM(ftl.file_size), 0) as total_data_size,
			COALESCE(SUM(CASE WHEN ftl.duration > 0 THEN ftl.duration END), 0) as total_duration,
			COALESCE(AVG(CASE WHEN ftl.duration > 0 THEN ftl.duration END), 0) as average_duration,
			CASE WHEN COUNT(*) > 0 THEN ROUND((COUNT(CASE WHEN ftl.status = 'completed' THEN 1 END) * 100.0 / COUNT(*))::numeric, 1) ELSE 0 END as success_rate,
			MAX(ftl.created_at) as last_updated
		FROM file_transfer_logs ftl
		%s
	`, whereClause)
	
	var stats struct {
		TotalTransfers     int     `json:"total_transfers"`
		CompletedTransfers int     `json:"completed_transfers"`
		FailedTransfers    int     `json:"failed_transfers"`
		InProgressTransfers int    `json:"in_progress_transfers"`
		TotalDataSize      int64   `json:"total_data_size"`
		TotalDuration      float64 `json:"total_duration"`
		AverageDuration    float64 `json:"average_duration"`
		SuccessRate        float64 `json:"success_rate"`
		LastUpdated        *time.Time `json:"last_updated"`
	}
	
	row := s.db.QueryRow(statsQuery, args...)
	err := row.Scan(&stats.TotalTransfers, &stats.CompletedTransfers, &stats.FailedTransfers,
		&stats.InProgressTransfers, &stats.TotalDataSize, &stats.TotalDuration, &stats.AverageDuration,
		&stats.SuccessRate, &stats.LastUpdated)
	
	if err != nil {
		log.Printf("❌ Failed to query transfer stats: %v", err)
		http.Error(w, "Failed to query database", http.StatusInternalServerError)
		return
	}
	
	response := map[string]interface{}{
		"success": true,
		"data":    stats,
	}
	
	json.NewEncoder(w).Encode(response)
	log.Printf("✅ Transfer stats API: returned stats (total: %d, success_rate: %.1f%%)", 
		stats.TotalTransfers, stats.SuccessRate)
}

// handleFilterOptions handles API requests for filter dropdown options
func (s *SyncToolServer) handleFilterOptions(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers for all requests
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	
	// Handle preflight OPTIONS request
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Check database connection
	if s.db == nil {
		http.Error(w, "Database not available", http.StatusInternalServerError)
		return
	}

	// Parse query parameters for date range filtering
	query := r.URL.Query()
	var conditions []string
	var args []interface{}
	argIndex := 1
	
	// Date range filters for options
	if dateFrom := query.Get("date_from"); dateFrom != "" {
		conditions = append(conditions, fmt.Sprintf("started_at >= $%d", argIndex))
		args = append(args, dateFrom+"T00:00:00Z")
		argIndex++
	}
	
	if dateTo := query.Get("date_to"); dateTo != "" {
		conditions = append(conditions, fmt.Sprintf("started_at <= $%d", argIndex))
		args = append(args, dateTo+"T23:59:59Z")
		argIndex++
	}
	
	// Build WHERE clause
	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}
	
	// Get status options from dedicated table
	statusQuery := `
		SELECT value, label
		FROM transfer_status_options 
		WHERE is_active = true
		ORDER BY sort_order ASC, label ASC
	`
	
	statusRows, err := s.db.Query(statusQuery)
	if err != nil {
		log.Printf("❌ Failed to query status options: %v", err)
		http.Error(w, "Failed to query database", http.StatusInternalServerError)
		return
	}
	defer statusRows.Close()
	
	var statusOptions []map[string]interface{}
	for statusRows.Next() {
		var value, label string
		if err := statusRows.Scan(&value, &label); err != nil {
			continue
		}
		
		statusOptions = append(statusOptions, map[string]interface{}{
			"value": value,
			"label": label,
		})
	}
	
	// Get job options from sync_jobs table
	jobQuery := `
		SELECT id, name
		FROM sync_jobs 
		WHERE status != 'deleted'
		ORDER BY name ASC
	`
	
	jobRows, err := s.db.Query(jobQuery)
	if err != nil {
		log.Printf("❌ Failed to query job options: %v", err)
		http.Error(w, "Failed to query database", http.StatusInternalServerError)
		return
	}
	defer jobRows.Close()
	
	var jobOptions []map[string]interface{}
	for jobRows.Next() {
		var jobID int
		var jobName string
		if err := jobRows.Scan(&jobID, &jobName); err != nil {
			continue
		}
		
		jobOptions = append(jobOptions, map[string]interface{}{
			"value": jobName,
			"label": jobName,
			"job_id": jobID,
		})
	}
	
	// Get action options from dedicated table
	actionQuery := `
		SELECT value, label
		FROM transfer_action_options 
		WHERE is_active = true
		ORDER BY sort_order ASC, label ASC
	`
	
	actionRows, err := s.db.Query(actionQuery)
	if err != nil {
		log.Printf("❌ Failed to query action options: %v", err)
		http.Error(w, "Failed to query database", http.StatusInternalServerError)
		return
	}
	defer actionRows.Close()
	
	var actionOptions []map[string]interface{}
	for actionRows.Next() {
		var value, label string
		if err := actionRows.Scan(&value, &label); err != nil {
			continue
		}
		
		actionOptions = append(actionOptions, map[string]interface{}{
			"value": value,
			"label": label,
		})
	}
	
	// Get agent pairs (source -> destination)
	agentQuery := fmt.Sprintf(`
		SELECT 
			ftl.job_id,
			sa.hostname as source_agent_name, 
			da.hostname as destination_agent_name,
			COUNT(*) as count
		FROM file_transfer_logs ftl
		LEFT JOIN sync_jobs sj ON (
			CASE 
				WHEN ftl.job_id IS NOT NULL AND ftl.job_id ~ '^job-[0-9]+$' 
				THEN sj.id = CAST(SUBSTRING(ftl.job_id FROM 5) AS INTEGER)
				ELSE FALSE
			END
		)
		LEFT JOIN integrated_agents sa ON sj.source_agent_id = sa.agent_id  
		LEFT JOIN integrated_agents da ON sj.target_agent_id = da.agent_id
		%s AND sa.hostname IS NOT NULL AND da.hostname IS NOT NULL
		GROUP BY ftl.job_id, sa.hostname, da.hostname
		ORDER BY count DESC
		LIMIT 20
	`, whereClause)
	
	agentRows, err := s.db.Query(agentQuery, args...)
	if err != nil {
		log.Printf("❌ Failed to query agent options: %v", err)
		http.Error(w, "Failed to query database", http.StatusInternalServerError)
		return
	}
	defer agentRows.Close()
	
	var agentOptions []map[string]interface{}
	for agentRows.Next() {
		var jobID, sourceAgent, destAgent string
		var count int
		if err := agentRows.Scan(&jobID, &sourceAgent, &destAgent, &count); err != nil {
			continue
		}
		
		agentOptions = append(agentOptions, map[string]interface{}{
			"source":      sourceAgent,
			"destination": destAgent,
			"count":       count,
		})
	}
	
	response := map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"statuses": statusOptions,
			"jobs":     jobOptions,
			"actions":  actionOptions,
			"agents":   agentOptions,
		},
	}
	
	json.NewEncoder(w).Encode(response)
	log.Printf("✅ Filter options API: returned %d statuses, %d jobs, %d actions, %d agent pairs", 
		len(statusOptions), len(jobOptions), len(actionOptions), len(agentOptions))
}

// handleJobsList handles API requests for job names list
func (s *SyncToolServer) handleJobsList(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers for all requests
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	
	// Handle preflight OPTIONS request
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Check database connection
	if s.db == nil {
		http.Error(w, "Database not available", http.StatusInternalServerError)
		return
	}

	// Get unique job names
	jobQuery := `
		SELECT DISTINCT job_name
		FROM file_transfer_logs 
		WHERE job_name IS NOT NULL AND job_name != ''
		ORDER BY job_name
	`
	
	rows, err := s.db.Query(jobQuery)
	if err != nil {
		log.Printf("❌ Failed to query job names: %v", err)
		http.Error(w, "Failed to query database", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	
	var jobNames []string
	for rows.Next() {
		var jobName string
		if err := rows.Scan(&jobName); err != nil {
			continue
		}
		jobNames = append(jobNames, jobName)
	}
	
	response := map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"jobs": jobNames,
		},
	}
	
	json.NewEncoder(w).Encode(response)
	log.Printf("✅ Jobs list API: returned %d job names", len(jobNames))
}

// handleTriggerScan handles manual scan trigger for testing scheduled jobs
func (s *SyncToolServer) handleTriggerScan(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var requestData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	agentID, ok := requestData["agent_id"].(string)
	if !ok || agentID == "" {
		http.Error(w, "Missing agent_id", http.StatusBadRequest)
		return
	}

	folderID, ok := requestData["folder_id"].(string)
	if !ok || folderID == "" {
		http.Error(w, "Missing folder_id", http.StatusBadRequest)
		return
	}

	log.Printf("🔍 Manual scan triggered for agent %s, folder %s", agentID, folderID)

	// Create scan message
	scanMessage := map[string]interface{}{
		"type":      "scan-folder",
		"folder_id": folderID,
	}

	// Send to agent
	if err := s.sendJobToAgent(agentID, scanMessage); err != nil {
		log.Printf("❌ Failed to send scan message to agent %s: %v", agentID, err)
		http.Error(w, fmt.Sprintf("Failed to trigger scan: %v", err), http.StatusInternalServerError)
		return
	}

	// Success response
	response := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Scan triggered for folder %s on agent %s", folderID, agentID),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
	log.Printf("✅ Manual scan successfully triggered for %s/%s", agentID, folderID)
}

// handleSchedulerStatus returns the status of the job scheduler
func (s *SyncToolServer) handleSchedulerStatus(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.scheduler == nil {
		response := map[string]interface{}{
			"scheduler_running": false,
			"message": "Scheduler not initialized - no database connection",
			"schedules": map[string]interface{}{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	status, err := s.scheduler.GetScheduledJobsStatus()
	if err != nil {
		log.Printf("❌ Failed to get scheduler status: %v", err)
		http.Error(w, fmt.Sprintf("Failed to get scheduler status: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleFolderStats handles folder statistics requests
func (s *SyncToolServer) handleFolderStats(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// Get query parameters
	agentID := r.URL.Query().Get("agent_id")
	folderID := r.URL.Query().Get("folder_id")
	
	if agentID == "" {
		http.Error(w, `{"error": "agent_id parameter required"}`, http.StatusBadRequest)
		return
	}
	
	if folderID == "" {
		http.Error(w, `{"error": "folder_id parameter required"}`, http.StatusBadRequest)
		return
	}
	
	log.Printf("📊 Requesting folder stats for %s from agent %s", folderID, agentID)
	
	// Create folder stats request message
	statsMessage := map[string]interface{}{
		"type":      "get_folder_stats",
		"folder_id": folderID,
	}
	
	// Check for force refresh parameter (timestamp query param indicates refresh request)
	forceRefresh := r.URL.Query().Get("_t") != ""
	
	// Check sync status and cache logic
	isAgentSyncing := s.isAgentSyncing(agentID)
	
	// Priority logic:
	// 1. If agent is syncing: always return cached stats (if available)
	// 2. If agent is idle: use request-response pattern (unless force refresh disabled)
	if isAgentSyncing && !forceRefresh {
		// Agent is actively syncing - prioritize cache
		if cachedStats, exists := s.getFolderStatsResponse(agentID, folderID); exists {
			log.Printf("📊 Agent %s is syncing - returning cached folder stats for %s", agentID, folderID)

			response := map[string]interface{}{
				"success":     true,
				"agent_id":    agentID,
				"folder_id":   folderID,
				"stats":       cachedStats,
				"from_cache":  true,
				"sync_active": true,
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	// Handle idle agent or cache miss cases
	// Store old cache before attempting refresh
	var oldCache map[string]interface{}
	var hadOldCache bool
	if cachedStats, exists := s.getFolderStatsResponse(agentID, folderID); exists {
		oldCache = cachedStats
		hadOldCache = true
	}

	if !isAgentSyncing {
		// Agent is idle - check cache only if not forcing refresh
		if !forceRefresh {
			if hadOldCache {
				log.Printf("📊 Agent %s is idle - returning cached folder stats", agentID)

				response := map[string]interface{}{
					"success":     true,
					"agent_id":    agentID,
					"folder_id":   folderID,
					"stats":       oldCache,
					"from_cache":  true,
					"sync_active": false,
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
				return
			}
		} else {
			log.Printf("🔄 Force refresh requested for agent %s folder %s, will request fresh stats", agentID, folderID)
			// Clear cached data for this specific folder only (not all folders on the agent)
			s.clearFolderStatsForFolder(agentID, folderID)
		}
	}

	// Fall back to request-response pattern (for idle agents or cache miss)
	log.Printf("📊 Using request-response pattern for agent %s (syncing: %t)", agentID, isAgentSyncing)

	// Send request to agent
	if err := s.sendJobToAgent(agentID, statsMessage); err != nil {
		log.Printf("❌ Failed to send folder stats request to agent %s: %v", agentID, err)

		// Fallback to old cache if request failed
		if hadOldCache {
			log.Printf("⚠️ Falling back to cached stats due to request error")
			response := map[string]interface{}{
				"success":     true,
				"agent_id":    agentID,
				"folder_id":   folderID,
				"stats":       oldCache,
				"from_cache":  true,
				"fallback":    true,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}

		http.Error(w, fmt.Sprintf(`{"error": "Failed to request folder stats: %v"}`, err), http.StatusInternalServerError)
		return
	}

	log.Printf("📊 Sent folder stats request to agent %s, waiting for response...", agentID)

	// Wait for response from agent (with timeout)
	timeout := time.Second * 60
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	for {
		select {
		case <-ticker.C:
			if stats, exists := s.getFolderStatsResponse(agentID, folderID); exists {
				log.Printf("✅ Received folder stats response from agent %s for folder %s", agentID, folderID)

				// Agent now ALWAYS returns valid stats (even for paused folders)
				// No need to check globalFiles == 0 anymore after Option 1 fix
				// Just return the stats directly
				response := map[string]interface{}{
					"success":   true,
					"agent_id":  agentID,
					"folder_id": folderID,
					"stats":     stats,
					"from_cache": false,
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
				return
			}
		case <-timeoutTimer.C:
			log.Printf("⏰ Timeout waiting for folder stats response from agent %s", agentID)

			// Fallback to old cache if timeout
			if hadOldCache {
				log.Printf("⚠️ Falling back to cached stats due to timeout")
				response := map[string]interface{}{
					"success":     true,
					"agent_id":    agentID,
					"folder_id":   folderID,
					"stats":       oldCache,
					"from_cache":  true,
					"fallback":    true,
					"reason":      "timeout",
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
				return
			}

			response := map[string]interface{}{
				"success": false,
				"error":   "Timeout waiting for agent response",
				"agent_id":  agentID,
				"folder_id": folderID,
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}
	}
}

// handleFolderStatsOverall handles dashboard statistics from all jobs
func (s *SyncToolServer) handleFolderStatsOverall(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Check database
	if s.db == nil {
		http.Error(w, `{"error": "Database not available"}`, http.StatusInternalServerError)
		return
	}

	// Get user claims for operator filtering
	var userRole string
	var assignedAgents []string
	claims, ok := r.Context().Value("user_claims").(*models.JWTClaims)
	if ok {
		userRole = claims.Role
		assignedAgents = claims.AssignedAgents
	}

	// Get filter parameters
	searchKeyword := r.URL.Query().Get("search")
	syncStatusFilter := r.URL.Query().Get("sync_status") // Complete, Pending, Partial

	// Query from file_transfer_logs with aggregation, supporting search and sync_status filters
	// Use DISTINCT ON to avoid counting same file multiple times (started + completed events)
	query := `
		WITH unique_transfers AS (
			SELECT DISTINCT ON (ftl.job_id, ftl.file_name)
				ftl.job_id,
				ftl.file_name,
				ftl.file_size,
				ftl.status
			FROM file_transfer_logs ftl
			ORDER BY ftl.job_id, ftl.file_name, ftl.created_at DESC
		),
		job_stats AS (
			SELECT
				sj.id as job_id,
				sj.name as job_name,
				sj.status as job_status,
				sj.target_agent_id as destination_agent_id,
				sa.hostname as source_agent_name,
				da.hostname as destination_agent_name,
				COUNT(DISTINCT ut.file_name) as files_transferred,
				COALESCE(SUM(ut.file_size), 0) as total_bytes,
				COUNT(CASE WHEN ut.status = 'completed' THEN 1 END) as completed_files,
				COUNT(*) as total_files_in_transfer
			FROM sync_jobs sj
			LEFT JOIN integrated_agents sa ON sj.source_agent_id = sa.agent_id
			LEFT JOIN integrated_agents da ON sj.target_agent_id = da.agent_id
			LEFT JOIN unique_transfers ut ON ut.job_id = CONCAT('job-', sj.id)
			WHERE 1=1
				%s
			GROUP BY sj.id, sj.name, sj.status, sj.target_agent_id, sa.hostname, da.hostname
		)
		SELECT
			job_id, job_name, job_status, destination_agent_id,
			source_agent_name, destination_agent_name,
			files_transferred, total_bytes, completed_files, total_files_in_transfer
		FROM job_stats
		WHERE 1=1
			%s
		ORDER BY job_id
	`

	// Build WHERE conditions
	var searchCondition string
	var statusCondition string
	var args []interface{}
	argIndex := 1

	// Operator filtering
	var operatorCondition string
	if userRole == models.RoleOperator && len(assignedAgents) > 0 {
		placeholders := make([]string, len(assignedAgents))
		for i, agentID := range assignedAgents {
			placeholders[i] = fmt.Sprintf("$%d", argIndex)
			args = append(args, agentID)
			argIndex++
		}
		operatorCondition = fmt.Sprintf(`AND (sj.source_agent_id IN (%s) OR sj.target_agent_id IN (%s))`,
			strings.Join(placeholders, ", "), strings.Join(placeholders, ", "))
		// Duplicate args for second IN clause
		for _, agentID := range assignedAgents {
			args = append(args, agentID)
			argIndex++
		}
	}

	if searchKeyword != "" {
		searchCondition = fmt.Sprintf(`AND (
			sj.name ILIKE $%d OR
			sa.hostname ILIKE $%d OR
			da.hostname ILIKE $%d
		)`, argIndex, argIndex, argIndex)
		args = append(args, "%"+searchKeyword+"%")
		argIndex++
	}

	// Build sync_status filter condition (applied after aggregation)
	if syncStatusFilter != "" {
		switch syncStatusFilter {
		case "Complete":
			statusCondition = `AND total_files_in_transfer > 0 AND completed_files = total_files_in_transfer`
		case "Partial":
			statusCondition = `AND total_files_in_transfer > 0 AND completed_files > 0 AND completed_files < total_files_in_transfer`
		case "Pending":
			statusCondition = `AND (total_files_in_transfer = 0 OR completed_files = 0)`
		}
	}

	// Format query with conditions - combine operator and search condition
	combinedCondition := operatorCondition + searchCondition
	finalQuery := fmt.Sprintf(query, combinedCondition, statusCondition)

	rows, err := s.db.Query(finalQuery, args...)
	if err != nil {
		log.Printf("❌ Failed to query folder stats: %v", err)
		http.Error(w, `{"error": "Failed to fetch stats"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var activeJobs int = 0
	var totalFiles int64 = 0
	var totalBytes int64 = 0
	var completedJobsCount int = 0
	var totalJobsWithTransfers int = 0

	for rows.Next() {
		var jobID int
		var jobName, jobStatus, destAgentID string
		var sourceAgentName, destAgentName *string
		var filesTransferred, bytesTotal, completedFiles, totalFilesInTransfer int64

		if err := rows.Scan(&jobID, &jobName, &jobStatus, &destAgentID, &sourceAgentName, &destAgentName,
			&filesTransferred, &bytesTotal, &completedFiles, &totalFilesInTransfer); err != nil {
			log.Printf("❌ Failed to scan job stats row: %v", err)
			continue
		}

		// Count active jobs (status=active AND not paused)
		isPaused := (jobStatus == "paused")
		if jobStatus == "active" && !isPaused {
			activeJobs++
		}

		// Aggregate stats
		totalFiles += filesTransferred
		totalBytes += bytesTotal

		// Calculate job completion for success rate
		if totalFilesInTransfer > 0 {
			totalJobsWithTransfers++
			if completedFiles == totalFilesInTransfer {
				completedJobsCount++
			}
		}
	}

	// Calculate success rate based on completed jobs
	var successRate float64 = 0
	if totalJobsWithTransfers > 0 {
		successRate = (float64(completedJobsCount) / float64(totalJobsWithTransfers)) * 100
	}

	// Convert bytes to GB
	dataVolumeGB := float64(totalBytes) / (1024 * 1024 * 1024)

	// Build response
	response := map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"active_jobs":   activeJobs,
			"total_files":   totalFiles,
			"data_volume":   fmt.Sprintf("%.2f GB", dataVolumeGB),
			"data_volume_bytes": totalBytes,
			"success_rate":  fmt.Sprintf("%.1f%%", successRate),
			"success_rate_value": math.Round(successRate*10) / 10,
		},
	}

	json.NewEncoder(w).Encode(response)
	log.Printf("✅ Dashboard stats: active_jobs=%d, total_files=%d, data_volume=%.2f GB, success_rate=%.1f%%",
		activeJobs, totalFiles, dataVolumeGB, successRate)
}

// Stop gracefully shuts down the server and all its components
func (s *SyncToolServer) Stop() error {
	log.Println("🛑 Stopping BSync Server...")
	
	// Stop scheduler first
	if s.scheduler != nil {
		s.scheduler.Stop()
	}
	
	// Stop HTTP server
	if s.server != nil {
		if err := s.server.Close(); err != nil {
			log.Printf("❌ Error stopping HTTP server: %v", err)
		}
	}
	
	// Signal shutdown to other components
	close(s.shutdown)
	
	// Close database connection
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			log.Printf("❌ Error closing database connection: %v", err)
		}
	}
	
	log.Println("✅ BSync stopped")
	return nil
}

// ============================================
// SESSION TRACKING FUNCTIONS
// ============================================

// handleSessionEvent processes session events from agents
func (s *SyncToolServer) handleSessionEvent(agentID string, msgData map[string]interface{}) {
	if eventData, ok := msgData["event"].(map[string]interface{}); ok {
		eventType, _ := eventData["type"].(string)
		sessionData, _ := eventData["data"].(map[string]interface{})

		log.Printf("📊 [SESSION] Received %s event from agent %s", eventType, agentID)

		switch eventType {
		case "session_started":
			s.handleSessionStarted(agentID, sessionData)
		case "scan_started":
			s.handleScanStarted(agentID, sessionData)
		case "scan_completed":
			s.handleScanCompleted(agentID, sessionData)
		case "transfer_started":
			s.handleTransferStarted(agentID, sessionData)
		case "transfer_completed":
			s.handleTransferCompleted(agentID, sessionData)
		case "session_completed":
			s.handleSessionCompleted(agentID, sessionData)
		default:
			log.Printf("⚠️  Unknown session event type: %s", eventType)
		}
	}
}

// handleSessionStarted creates a new session record in database
func (s *SyncToolServer) handleSessionStarted(agentID string, data map[string]interface{}) {
	sessionID, _ := data["session_id"].(string)
	jobID, _ := data["job_id"].(string)
	currentState, _ := data["current_state"].(string)
	status, _ := data["status"].(string)
	startTime, _ := data["session_start_time"].(string)

	if sessionID == "" || jobID == "" {
		log.Printf("⚠️  Invalid session_started event: missing session_id or job_id")
		return
	}

	// Get job name from job ID
	var jobName string
	if strings.HasPrefix(jobID, "job-") {
		numericID := strings.TrimPrefix(jobID, "job-")
		s.db.QueryRow("SELECT name FROM sync_jobs WHERE id = $1", numericID).Scan(&jobName)
	}

	// Insert session record
	query := `
		INSERT INTO sync_sessions (
			session_id, job_id, job_name, agent_id,
			session_start_time, current_state, status, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (session_id) DO UPDATE SET
			current_state = EXCLUDED.current_state,
			status = EXCLUDED.status,
			updated_at = NOW()
	`

	_, err := s.db.Exec(query, sessionID, jobID, jobName, agentID, startTime, currentState, status)
	if err != nil {
		log.Printf("❌ Failed to insert session: %v", err)
		return
	}

	// Insert session event
	s.insertSessionEvent(sessionID, "session_started", currentState, data)

	log.Printf("✅ [SESSION] Session started: %s (job: %s, agent: %s)", sessionID, jobID, agentID)
}

// handleScanStarted updates session with scan start time
func (s *SyncToolServer) handleScanStarted(agentID string, data map[string]interface{}) {
	sessionID, _ := data["session_id"].(string)
	scanStartTime, _ := data["scan_start_time"].(string)

	if sessionID == "" {
		return
	}

	query := `
		UPDATE sync_sessions
		SET scan_start_time = $1, current_state = 'scanning', updated_at = NOW()
		WHERE session_id = $2
	`

	s.db.Exec(query, scanStartTime, sessionID)
	s.insertSessionEvent(sessionID, "scan_started", "scanning", data)

	log.Printf("✅ [SESSION] Scan started: %s", sessionID)
}

// handleScanCompleted updates session with scan completion
func (s *SyncToolServer) handleScanCompleted(agentID string, data map[string]interface{}) {
	sessionID, _ := data["session_id"].(string)
	scanEndTime, _ := data["scan_end_time"].(string)
	scanDuration := int64(0)
	if dur, ok := data["scan_duration_seconds"].(float64); ok {
		scanDuration = int64(dur)
	}

	if sessionID == "" {
		return
	}

	query := `
		UPDATE sync_sessions
		SET scan_end_time = $1, scan_duration_seconds = $2, updated_at = NOW()
		WHERE session_id = $3
	`

	s.db.Exec(query, scanEndTime, scanDuration, sessionID)
	s.insertSessionEvent(sessionID, "scan_completed", "syncing", data)

	log.Printf("✅ [SESSION] Scan completed: %s (duration: %ds)", sessionID, scanDuration)
}

// handleTransferStarted updates session with transfer start time
func (s *SyncToolServer) handleTransferStarted(agentID string, data map[string]interface{}) {
	sessionID, _ := data["session_id"].(string)
	transferStartTime, _ := data["transfer_start_time"].(string)

	if sessionID == "" {
		return
	}

	query := `
		UPDATE sync_sessions
		SET transfer_start_time = $1, current_state = 'syncing', updated_at = NOW()
		WHERE session_id = $2
	`

	s.db.Exec(query, transferStartTime, sessionID)
	s.insertSessionEvent(sessionID, "transfer_started", "syncing", data)

	log.Printf("✅ [SESSION] Transfer started: %s", sessionID)
}

// handleTransferCompleted updates session with transfer completion
func (s *SyncToolServer) handleTransferCompleted(agentID string, data map[string]interface{}) {
	sessionID, _ := data["session_id"].(string)
	transferEndTime, _ := data["transfer_end_time"].(string)
	transferDuration := int64(0)
	if dur, ok := data["transfer_duration_seconds"].(float64); ok {
		transferDuration = int64(dur)
	}

	if sessionID == "" {
		return
	}

	query := `
		UPDATE sync_sessions
		SET transfer_end_time = $1, transfer_duration_seconds = $2, updated_at = NOW()
		WHERE session_id = $3
	`

	s.db.Exec(query, transferEndTime, transferDuration, sessionID)
	s.insertSessionEvent(sessionID, "transfer_completed", "idle", data)

	log.Printf("✅ [SESSION] Transfer completed: %s (duration: %ds)", sessionID, transferDuration)
}

// handleSessionCompleted finalizes session with complete stats
func (s *SyncToolServer) handleSessionCompleted(agentID string, data map[string]interface{}) {
	sessionID, _ := data["session_id"].(string)
	if sessionID == "" {
		return
	}

	// Extract all stats from data
	filesTransferred := int64(0)
	totalDeltaBytes := int64(0)
	totalFullFileSize := int64(0)
	avgTransferRate := float64(0)
	peakTransferRate := float64(0)
	compressionRatio := float64(0)
	totalDuration := int64(0)

	if v, ok := data["files_transferred"].(float64); ok {
		filesTransferred = int64(v)
	}
	if v, ok := data["total_delta_bytes"].(float64); ok {
		totalDeltaBytes = int64(v)
	}
	if v, ok := data["total_full_file_size"].(float64); ok {
		totalFullFileSize = int64(v)
	}
	if v, ok := data["average_transfer_rate"].(float64); ok {
		avgTransferRate = v
	}
	if v, ok := data["peak_transfer_rate"].(float64); ok {
		peakTransferRate = v
	}
	if v, ok := data["compression_ratio"].(float64); ok {
		compressionRatio = v
	}
	if v, ok := data["total_duration_seconds"].(float64); ok {
		totalDuration = int64(v)
	}

	sessionEndTime, _ := data["session_end_time"].(string)
	status, _ := data["status"].(string)
	currentState, _ := data["current_state"].(string)

	query := `
		UPDATE sync_sessions
		SET
			session_end_time = $1,
			total_duration_seconds = $2,
			files_transferred = $3,
			total_delta_bytes = $4,
			total_full_file_size = $5,
			compression_ratio = $6,
			average_transfer_rate = $7,
			peak_transfer_rate = $8,
			status = $9,
			current_state = $10,
			updated_at = NOW()
		WHERE session_id = $11
	`

	_, err := s.db.Exec(query,
		sessionEndTime, totalDuration, filesTransferred,
		totalDeltaBytes, totalFullFileSize, compressionRatio,
		avgTransferRate, peakTransferRate, status, currentState, sessionID)

	if err != nil {
		log.Printf("❌ Failed to complete session: %v", err)
		return
	}

	// Insert final session event
	s.insertSessionEvent(sessionID, "session_completed", currentState, data)

	log.Printf("✅ [SESSION] Session completed: %s | Files: %d | Delta: %d bytes | Full: %d bytes | Ratio: %.2f%% | Duration: %ds",
		sessionID, filesTransferred, totalDeltaBytes, totalFullFileSize, compressionRatio*100, totalDuration)
}

// insertSessionEvent inserts an event into sync_session_events table
func (s *SyncToolServer) insertSessionEvent(sessionID, eventType, eventState string, data map[string]interface{}) {
	eventDataJSON, _ := json.Marshal(data)

	query := `
		INSERT INTO sync_session_events (
			session_id, event_type, event_state, event_data, timestamp, created_at
		) VALUES ($1, $2, $3, $4, NOW(), NOW())
	`

	_, err := s.db.Exec(query, sessionID, eventType, eventState, string(eventDataJSON))
	if err != nil {
		log.Printf("⚠️  Failed to insert session event: %v", err)
	}
}

// ============================================
// SESSION API HANDLERS
// ============================================

// handleSessions handles GET requests for session list with filtering
func (s *SyncToolServer) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	jobID := r.URL.Query().Get("job_id")
	agentID := r.URL.Query().Get("agent_id")
	status := r.URL.Query().Get("status")
	limitStr := r.URL.Query().Get("limit")
	limit := 50 // Default limit

	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Build query
	query := `
		SELECT
			session_id, job_id, job_name, agent_id,
			session_start_time, session_end_time, total_duration_seconds,
			scan_duration_seconds, transfer_duration_seconds,
			files_transferred, total_delta_bytes, total_full_file_size,
			compression_ratio, average_transfer_rate, peak_transfer_rate,
			current_state, status, created_at
		FROM sync_sessions
		WHERE 1=1
	`

	args := []interface{}{}
	argIndex := 1

	if jobID != "" {
		query += fmt.Sprintf(" AND job_id = $%d", argIndex)
		args = append(args, jobID)
		argIndex++
	}

	if agentID != "" {
		query += fmt.Sprintf(" AND agent_id = $%d", argIndex)
		args = append(args, agentID)
		argIndex++
	}

	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIndex)
		args = append(args, status)
		argIndex++
	}

	query += fmt.Sprintf(" ORDER BY session_start_time DESC LIMIT $%d", argIndex)
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		log.Printf("❌ Error querying sessions: %v", err)
		http.Error(w, "Failed to query sessions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	sessions := []map[string]interface{}{}
	for rows.Next() {
		var (
			sessionID, jobID, jobName, agentID, currentState, statusVal string
			sessionStartTime, createdAt                                  time.Time
			sessionEndTime                                               *time.Time
			totalDuration, scanDuration, transferDuration                *int64
			filesTransferred, totalDeltaBytes, totalFullFileSize         int64
			compressionRatio, avgRate, peakRate                          *float64
		)

		err := rows.Scan(
			&sessionID, &jobID, &jobName, &agentID,
			&sessionStartTime, &sessionEndTime, &totalDuration,
			&scanDuration, &transferDuration,
			&filesTransferred, &totalDeltaBytes, &totalFullFileSize,
			&compressionRatio, &avgRate, &peakRate,
			&currentState, &statusVal, &createdAt,
		)

		if err != nil {
			log.Printf("❌ Error scanning session row: %v", err)
			continue
		}

		session := map[string]interface{}{
			"session_id":        sessionID,
			"job_id":            jobID,
			"job_name":          jobName,
			"agent_id":          agentID,
			"session_start_time": sessionStartTime,
			"session_end_time":  sessionEndTime,
			"total_duration_seconds": totalDuration,
			"scan_duration_seconds":  scanDuration,
			"transfer_duration_seconds": transferDuration,
			"files_transferred":  filesTransferred,
			"total_delta_bytes":  totalDeltaBytes,
			"total_full_file_size": totalFullFileSize,
			"compression_ratio":  compressionRatio,
			"average_transfer_rate": avgRate,
			"peak_transfer_rate": peakRate,
			"current_state":      currentState,
			"status":             statusVal,
			"created_at":         createdAt,
		}

		// Calculate efficiency percentage
		if totalFullFileSize > 0 {
			efficiency := (1 - float64(totalDeltaBytes)/float64(totalFullFileSize)) * 100
			session["efficiency_percentage"] = efficiency
			session["bytes_saved"] = totalFullFileSize - totalDeltaBytes
		}

		sessions = append(sessions, session)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"sessions": sessions,
		"count":    len(sessions),
	})
}

// handleSessionDetails handles GET requests for session details and timeline
func (s *SyncToolServer) handleSessionDetails(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract session_id from path
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	sessionID := parts[0]

	// Get session details
	query := `
		SELECT
			session_id, job_id, job_name, agent_id,
			session_start_time, session_end_time, total_duration_seconds,
			scan_start_time, scan_end_time, scan_duration_seconds,
			transfer_start_time, transfer_end_time, transfer_duration_seconds,
			files_transferred, total_delta_bytes, total_full_file_size,
			compression_ratio, average_transfer_rate, peak_transfer_rate,
			current_state, status, error_message, created_at, updated_at
		FROM sync_sessions
		WHERE session_id = $1
	`

	var (
		sid, jobID, jobName, agentID, currentState, statusVal string
		sessionStartTime, createdAt, updatedAt                time.Time
		sessionEndTime, scanStartTime, scanEndTime            *time.Time
		transferStartTime, transferEndTime                    *time.Time
		totalDuration, scanDuration, transferDuration         *int64
		filesTransferred, totalDeltaBytes, totalFullFileSize  int64
		compressionRatio, avgRate, peakRate                   *float64
		errorMessage                                          *string
	)

	err := s.db.QueryRow(query, sessionID).Scan(
		&sid, &jobID, &jobName, &agentID,
		&sessionStartTime, &sessionEndTime, &totalDuration,
		&scanStartTime, &scanEndTime, &scanDuration,
		&transferStartTime, &transferEndTime, &transferDuration,
		&filesTransferred, &totalDeltaBytes, &totalFullFileSize,
		&compressionRatio, &avgRate, &peakRate,
		&currentState, &statusVal, &errorMessage, &createdAt, &updatedAt,
	)

	if err == sql.ErrNoRows {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	if err != nil {
		log.Printf("❌ Error querying session details: %v", err)
		http.Error(w, "Failed to get session details", http.StatusInternalServerError)
		return
	}

	// Get session events timeline
	eventsQuery := `
		SELECT event_type, event_state, timestamp, event_data
		FROM sync_session_events
		WHERE session_id = $1
		ORDER BY timestamp ASC
	`

	eventRows, err := s.db.Query(eventsQuery, sessionID)
	if err != nil {
		log.Printf("❌ Error querying session events: %v", err)
	}

	events := []map[string]interface{}{}
	if eventRows != nil {
		defer eventRows.Close()
		for eventRows.Next() {
			var eventType, eventState string
			var timestamp time.Time
			var eventDataJSON string

			eventRows.Scan(&eventType, &eventState, &timestamp, &eventDataJSON)

			event := map[string]interface{}{
				"event_type":  eventType,
				"event_state": eventState,
				"timestamp":   timestamp,
			}

			// Parse event data JSON if needed
			var eventData map[string]interface{}
			if json.Unmarshal([]byte(eventDataJSON), &eventData) == nil {
				event["data"] = eventData
			}

			events = append(events, event)
		}
	}

	// Build response
	response := map[string]interface{}{
		"session_id":        sid,
		"job_id":            jobID,
		"job_name":          jobName,
		"agent_id":          agentID,
		"session_start_time": sessionStartTime,
		"session_end_time":  sessionEndTime,
		"total_duration_seconds": totalDuration,
		"scan_start_time":   scanStartTime,
		"scan_end_time":     scanEndTime,
		"scan_duration_seconds": scanDuration,
		"transfer_start_time": transferStartTime,
		"transfer_end_time": transferEndTime,
		"transfer_duration_seconds": transferDuration,
		"files_transferred":  filesTransferred,
		"total_delta_bytes":  totalDeltaBytes,
		"total_full_file_size": totalFullFileSize,
		"compression_ratio":  compressionRatio,
		"average_transfer_rate": avgRate,
		"peak_transfer_rate": peakRate,
		"current_state":      currentState,
		"status":             statusVal,
		"error_message":      errorMessage,
		"created_at":         createdAt,
		"updated_at":         updatedAt,
		"events":             events,
	}

	// Calculate efficiency metrics
	if totalFullFileSize > 0 {
		efficiency := (1 - float64(totalDeltaBytes)/float64(totalFullFileSize)) * 100
		response["efficiency_percentage"] = efficiency
		response["bytes_saved"] = totalFullFileSize - totalDeltaBytes
	}

	// Format human-readable duration
	if totalDuration != nil {
		response["duration_display"] = formatDuration(*totalDuration)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// formatDuration formats seconds into human-readable duration
func formatDuration(seconds int64) string {
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, secs)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, secs)
	}
	return fmt.Sprintf("%ds", secs)
}

// ============================================
// MASTER DATA API HANDLERS
// ============================================

// handleGetSyncStatusMaster returns list of available sync status options for filters
func (s *SyncToolServer) handleGetSyncStatusMaster(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Check database
	if s.db == nil {
		http.Error(w, `{"error": "Database not available"}`, http.StatusInternalServerError)
		return
	}

	query := `
		SELECT code, label, description, display_order
		FROM sync_status_master
		WHERE is_active = true
		ORDER BY display_order ASC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		log.Printf("❌ Failed to query sync_status_master: %v", err)
		http.Error(w, `{"error": "Failed to fetch sync status options"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	statusOptions := []map[string]interface{}{}

	for rows.Next() {
		var code, label string
		var description *string
		var displayOrder int

		if err := rows.Scan(&code, &label, &description, &displayOrder); err != nil {
			log.Printf("❌ Failed to scan sync_status row: %v", err)
			continue
		}

		option := map[string]interface{}{
			"code":  code,
			"label": label,
			"order": displayOrder,
		}

		if description != nil {
			option["description"] = *description
		}

		statusOptions = append(statusOptions, option)
	}

	response := map[string]interface{}{
		"success": true,
		"data":    statusOptions,
		"total":   len(statusOptions),
	}

	json.NewEncoder(w).Encode(response)
	log.Printf("✅ Returned %d sync status options", len(statusOptions))
}

// handleGetJobStatusMaster returns list of available job status options
func (s *SyncToolServer) handleGetJobStatusMaster(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Check database
	if s.db == nil {
		http.Error(w, `{"error": "Database not available"}`, http.StatusInternalServerError)
		return
	}

	query := `
		SELECT code, label, description, display_order
		FROM job_status_master
		WHERE is_active = true
		ORDER BY display_order ASC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		log.Printf("❌ Failed to query job_status_master: %v", err)
		http.Error(w, `{"error": "Failed to fetch job status options"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	statusOptions := []map[string]interface{}{}

	for rows.Next() {
		var code, label string
		var description *string
		var displayOrder int

		if err := rows.Scan(&code, &label, &description, &displayOrder); err != nil {
			log.Printf("❌ Failed to scan job_status row: %v", err)
			continue
		}

		option := map[string]interface{}{
			"code":  code,
			"label": label,
			"order": displayOrder,
		}

		if description != nil {
			option["description"] = *description
		}

		statusOptions = append(statusOptions, option)
	}

	response := map[string]interface{}{
		"success": true,
		"data":    statusOptions,
		"total":   len(statusOptions),
	}

	json.NewEncoder(w).Encode(response)
	log.Printf("✅ Returned %d job status options", len(statusOptions))
}

// ============================================
// User Management Handler Wrappers
// ============================================

// handleUserLogin handles login requests
func (s *SyncToolServer) handleUserLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Decode request body
	var loginReq models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&loginReq); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate input
	if loginReq.Username == "" || loginReq.Password == "" {
		s.writeJSONError(w, http.StatusBadRequest, "Username and password are required")
		return
	}

	// Attempt login
	response, err := s.authService.Login(loginReq.Username, loginReq.Password)
	if err != nil {
		statusCode := http.StatusUnauthorized
		if err == auth.ErrUserNotActive {
			statusCode = http.StatusForbidden
		}
		s.writeJSONError(w, statusCode, err.Error())
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    response,
	})

	log.Printf("✅ User logged in: %s (role: %s)", response.User.Username, response.User.Role)
}

// handleUserLogout handles logout requests
func (s *SyncToolServer) handleUserLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	claims, _ := s.getUserClaims(r)

	// Log logout
	if claims != nil {
		log.Printf("✅ User logged out: %s", claims.Username)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Logged out successfully",
	})
}

// handleUserMe returns current user info
func (s *SyncToolServer) handleUserMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	claims, ok := s.getUserClaims(r)
	if !ok {
		s.writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Get user with agents
	user, err := s.userRepo.GetUserWithAgents(claims.UserID)
	if err != nil {
		s.writeJSONError(w, http.StatusNotFound, "User not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    user,
	})
}

// handleUsers handles user list and create
func (s *SyncToolServer) handleUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListUsers(w, r)
	case http.MethodPost:
		s.handleCreateUser(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleListUsers lists all users with filters
func (s *SyncToolServer) handleListUsers(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()
	filter := models.UserListFilter{
		Role:   query.Get("role"),
		Status: query.Get("status"),
		Search: query.Get("search"),
		Page:   1,
		Limit:  20,
	}

	if pageStr := query.Get("page"); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil && page > 0 {
			filter.Page = page
		}
	}

	if limitStr := query.Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 && limit <= 100 {
			filter.Limit = limit
		}
	}

	// Get users
	users, total, err := s.userRepo.ListUsers(filter)
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to retrieve users")
		log.Printf("❌ Failed to list users: %v", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    users,
		"total":   total,
		"page":    filter.Page,
		"limit":   filter.Limit,
	})

	log.Printf("✅ Listed %d users (total: %d)", len(users), total)
}

// handleCreateUser creates a new user
func (s *SyncToolServer) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req models.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Get current user
	claims, _ := s.getUserClaims(r)

	// Check if username exists
	if _, err := s.userRepo.GetUserByUsername(req.Username); err == nil {
		s.writeJSONError(w, http.StatusConflict, "Username already exists")
		return
	}

	// Check if email exists
	if _, err := s.userRepo.GetUserByEmail(req.Email); err == nil {
		s.writeJSONError(w, http.StatusConflict, "Email already exists")
		return
	}

	// Generate random password (12 characters with uppercase, digit, and special char)
	generatedPassword, err := utils.GenerateRandomPassword(12)
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to generate password")
		log.Printf("❌ Failed to generate password: %v", err)
		return
	}

	// Hash password
	passwordHash, err := s.authService.HashPassword(generatedPassword)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Set default status
	if req.Status == "" {
		req.Status = models.StatusActive
	}

	// Create user
	user := &models.User{
		Username:     req.Username,
		Email:        req.Email,
		Fullname:     req.Fullname,
		PasswordHash: passwordHash,
		Role:         req.Role,
		Status:       req.Status,
		CreatedBy:    sql.NullInt64{Int64: int64(claims.UserID), Valid: true},
	}

	if err := s.userRepo.CreateUser(user); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to create user")
		log.Printf("❌ Failed to create user: %v", err)
		return
	}

	// Assign agents if operator
	if req.Role == models.RoleOperator && len(req.AssignedAgents) > 0 {
		if err := s.userRepo.AssignAgentsToUser(user.ID, req.AssignedAgents, claims.UserID); err != nil {
			log.Printf("⚠️  User created but agent assignment failed: %v", err)
		}
	}

	// Send welcome email with credentials
	loginURL := config.GetWebURL() + "/login"
	err = utils.SendNewUserEmail(req.Email, req.Fullname, req.Username, generatedPassword, loginURL)
	if err != nil {
		// Log error but don't fail the request - user is already created
		log.Printf("⚠️  User created but email failed to send: %v", err)
	} else {
		log.Printf("📧 Welcome email sent to %s", req.Email)
	}

	// Get created user with agents
	createdUser, _ := s.userRepo.GetUserWithAgents(user.ID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    createdUser,
		"message": "User created successfully",
	})

	log.Printf("✅ User created: %s (role: %s) by %s", user.Username, user.Role, claims.Username)
}

// handleUserActions handles user-specific actions (get, update, delete)
func (s *SyncToolServer) handleUserActions(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from URL
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid URL")
		return
	}

	userIDStr := pathParts[3]

	// Check for sub-resources (e.g., /api/v1/users/2/agents)
	if len(pathParts) > 4 && pathParts[4] == "agents" {
		s.handleUserAgentActions(w, r, userIDStr)
		return
	}

	// Handle main user actions
	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetUser(w, r, userID)
	case http.MethodPut:
		s.handleUpdateUser(w, r, userID)
	case http.MethodDelete:
		s.handleDeleteUser(w, r, userID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetUser gets a user by ID
func (s *SyncToolServer) handleGetUser(w http.ResponseWriter, r *http.Request, userID int) {
	user, err := s.userRepo.GetUserWithAgents(userID)
	if err != nil {
		s.writeJSONError(w, http.StatusNotFound, "User not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    user,
	})
}

// handleUpdateUser updates a user
func (s *SyncToolServer) handleUpdateUser(w http.ResponseWriter, r *http.Request, userID int) {
	var req models.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	claims, _ := s.getUserClaims(r)

	// Check if user exists
	user, err := s.userRepo.GetUserByID(userID)
	if err != nil {
		s.writeJSONError(w, http.StatusNotFound, "User not found")
		return
	}

	// Build updates map
	updates := make(map[string]interface{})

	if req.Email != nil {
		// Check email not used by another user
		if existingUser, err := s.userRepo.GetUserByEmail(*req.Email); err == nil && existingUser.ID != userID {
			s.writeJSONError(w, http.StatusConflict, "Email already used by another user")
			return
		}
		updates["email"] = *req.Email
	}

	if req.Fullname != nil {
		updates["fullname"] = *req.Fullname
	}

	if req.Role != nil {
		updates["role"] = *req.Role
	}

	if req.Status != nil {
		updates["status"] = *req.Status
	}

	if req.Password != nil {
		passwordHash, err := s.authService.HashPassword(*req.Password)
		if err != nil {
			s.writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		updates["password_hash"] = passwordHash
	}

	// Update user
	if len(updates) > 0 {
		if err := s.userRepo.UpdateUser(userID, updates, claims.UserID); err != nil {
			s.writeJSONError(w, http.StatusInternalServerError, "Failed to update user")
			log.Printf("❌ Failed to update user: %v", err)
			return
		}
	}

	// Update agent assignments if provided
	if req.AssignedAgents != nil {
		newRole := user.Role
		if req.Role != nil {
			newRole = *req.Role
		}

		if newRole == models.RoleOperator {
			if err := s.userRepo.AssignAgentsToUser(userID, req.AssignedAgents, claims.UserID); err != nil {
				s.writeJSONError(w, http.StatusInternalServerError, "Failed to update agent assignments")
				return
			}
		}
	}

	// Get updated user
	updatedUser, _ := s.userRepo.GetUserWithAgents(userID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    updatedUser,
		"message": "User updated successfully",
	})

	log.Printf("✅ User updated: %d by %s", userID, claims.Username)
}

// handleDeleteUser deletes a user
func (s *SyncToolServer) handleDeleteUser(w http.ResponseWriter, r *http.Request, userID int) {
	claims, _ := s.getUserClaims(r)

	// Prevent self-deletion
	if userID == claims.UserID {
		s.writeJSONError(w, http.StatusBadRequest, "Cannot delete your own account")
		return
	}

	// Get user info before deletion
	user, err := s.userRepo.GetUserByID(userID)
	if err != nil {
		s.writeJSONError(w, http.StatusNotFound, "User not found")
		return
	}

	// Delete user
	if err := s.userRepo.SoftDeleteUser(userID, claims.UserID); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to delete user")
		log.Printf("❌ Failed to delete user: %v", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("User '%s' deleted successfully", user.Username),
	})

	log.Printf("✅ User deleted: %s by %s", user.Username, claims.Username)
}

// handleUserChangePassword handles password change
func (s *SyncToolServer) handleUserChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	claims, _ := s.getUserClaims(r)

	if err := s.authService.ChangePassword(claims.UserID, req.OldPassword, req.NewPassword); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Password changed successfully",
	})

	log.Printf("✅ Password changed for user: %s", claims.Username)
}

// handleUserAgentActions handles agent assignment actions
func (s *SyncToolServer) handleUserAgentActions(w http.ResponseWriter, r *http.Request, userIDStr string) {
	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

	switch r.Method {
	case http.MethodGet:
		// GET /api/v1/users/:id/agents
		s.handleGetUserAgents(w, r, userID)
	case http.MethodPost:
		// POST /api/v1/users/:id/agents
		s.handleAssignAgents(w, r, userID)
	case http.MethodDelete:
		// DELETE /api/v1/users/:id/agents/:agent_id
		if len(pathParts) > 5 {
			agentID := pathParts[5]
			s.handleRemoveAgentAssignment(w, r, userID, agentID)
		} else {
			s.writeJSONError(w, http.StatusBadRequest, "Agent ID required")
		}
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetUserAgents gets user's assigned agents
func (s *SyncToolServer) handleGetUserAgents(w http.ResponseWriter, r *http.Request, userID int) {
	agentIDs, err := s.userRepo.GetUserAgents(userID)
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to retrieve user agents")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    agentIDs,
	})
}

// handleAssignAgents assigns agents to a user
func (s *SyncToolServer) handleAssignAgents(w http.ResponseWriter, r *http.Request, userID int) {
	var req models.AssignAgentsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	claims, _ := s.getUserClaims(r)

	// Check if user exists and is operator
	user, err := s.userRepo.GetUserByID(userID)
	if err != nil {
		s.writeJSONError(w, http.StatusNotFound, "User not found")
		return
	}

	if user.Role != models.RoleOperator {
		s.writeJSONError(w, http.StatusBadRequest, "Can only assign agents to operators")
		return
	}

	// Assign agents
	if err := s.userRepo.AssignAgentsToUser(userID, req.AgentIDs, claims.UserID); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to assign agents")
		log.Printf("❌ Failed to assign agents: %v", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Agents assigned successfully",
	})

	log.Printf("✅ Agents assigned to user %d by %s", userID, claims.Username)
}

// handleRemoveAgentAssignment removes an agent assignment
func (s *SyncToolServer) handleRemoveAgentAssignment(w http.ResponseWriter, r *http.Request, userID int, agentID string) {
	if err := s.userRepo.RemoveAgentAssignment(userID, agentID); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to remove agent assignment")
		log.Printf("❌ Failed to remove agent assignment: %v", err)
		return
	}

	claims, _ := s.getUserClaims(r)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Agent assignment removed successfully",
	})

	log.Printf("✅ Agent %s removed from user %d by %s", agentID, userID, claims.Username)
}
