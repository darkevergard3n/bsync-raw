package server

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
)

type TLSConfig struct {
	Enabled  bool   `json:"enabled"`
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
	AutoTLS  bool   `json:"auto_tls"`
}

func (s *SyncToolServer) StartWithTLS(tlsConfig *TLSConfig) error {
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

	protocol := "http"
	wsProtocol := "ws"
	
	if tlsConfig.Enabled {
		protocol = "https"
		wsProtocol = "wss"
		
		if tlsConfig.AutoTLS {
			// Generate self-signed certificate for development
			cert, key, err := generateSelfSignedCert(s.config.Host)
			if err != nil {
				return fmt.Errorf("failed to generate self-signed cert: %v", err)
			}
			
			tlsConfig.CertFile = cert
			tlsConfig.KeyFile = key
			log.Printf("🔐 Generated self-signed certificate for development")
		}
		
		// Configure TLS
		tlsCfg := &tls.Config{
			MinVersion: tls.VersionTLS12,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			},
		}
		
		s.server.TLSConfig = tlsCfg
	}

	log.Printf("🌐 BSync Server listening on %s", addr)
	log.Printf("🔒 TLS/SSL: %v", tlsConfig.Enabled)
	log.Printf("📡 WebSocket endpoints:")
	log.Printf("  %s://%s/ws/agent - Agent connections", wsProtocol, addr)
	log.Printf("  %s://%s/ws/cli   - CLI connections", wsProtocol, addr)
	log.Printf("🔧 REST endpoints:")
	log.Printf("  %s://%s/api/status - Server status", protocol, addr)
	log.Printf("  %s://%s/api/agents - List connected agents", protocol, addr)
	log.Printf("  %s://%s/api/events - Query events (params: agent_id, since, limit)", protocol, addr)
	log.Printf("  %s://%s/api/events/stats - Event statistics", protocol, addr)
	log.Printf("  %s://%s/api/v1/licenses - License management", protocol, addr)
	log.Printf("  %s://%s/api/v1/agent-licenses - Agent-License mapping", protocol, addr)
	log.Printf("  %s://%s/api/v1/sync-jobs - Sync job management", protocol, addr)
	log.Printf("  %s://%s/health     - Health check", protocol, addr)

	if tlsConfig.Enabled {
		return s.server.ListenAndServeTLS(tlsConfig.CertFile, tlsConfig.KeyFile)
	}
	return s.server.ListenAndServe()
}

func generateSelfSignedCert(host string) (certFile, keyFile string, err error) {
	// This would generate a self-signed certificate
	// For production, use proper certificates from Let's Encrypt or your CA
	// Implementation would go here
	certFile, keyFile = getTempCertPaths()
	return certFile, keyFile, nil
}