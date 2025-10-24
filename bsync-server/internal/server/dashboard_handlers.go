package server

import (
	"encoding/json"
	"net/http"

	"bsync-server/internal/models"
	"bsync-server/internal/repository"
)

// Dashboard API handlers

// handleGetRoleList handles GET /api/v1/roles
func (s *SyncToolServer) handleGetRoleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	dashboardRepo := repository.NewDashboardRepository(s.db)
	roles, err := dashboardRepo.GetRoleList()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to retrieve role list",
			"details": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    roles,
	})
}

// handleGetUserStats handles GET /api/v1/dashboard/user-stats
func (s *SyncToolServer) handleGetUserStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	dashboardRepo := repository.NewDashboardRepository(s.db)
	stats, err := dashboardRepo.GetUserStats()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to retrieve user statistics",
			"details": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    stats,
	})
}

// handleGetLicensedAgents handles GET /api/v1/agents/licensed
func (s *SyncToolServer) handleGetLicensedAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Get user claims from context
	claims, ok := r.Context().Value("user_claims").(*models.JWTClaims)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Unauthorized",
		})
		return
	}

	dashboardRepo := repository.NewDashboardRepository(s.db)
	agents, err := dashboardRepo.GetLicensedAgents()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to retrieve licensed agents",
			"details": err.Error(),
		})
		return
	}

	// Filter agents based on user role
	// If operator, only show assigned agents
	if claims.Role == models.RoleOperator {
		assignedAgentMap := make(map[string]bool)
		for _, agentID := range claims.AssignedAgents {
			assignedAgentMap[agentID] = true
		}

		filteredAgents := make([]models.LicensedAgent, 0)
		for _, agent := range agents {
			if assignedAgentMap[agent.AgentID] {
				filteredAgents = append(filteredAgents, agent)
			}
		}
		agents = filteredAgents
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    agents,
		"count":   len(agents),
	})
}

// handleGetDashboardStats handles GET /api/v1/dashboard/stats
func (s *SyncToolServer) handleGetDashboardStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Get user claims for filtering
	var assignedAgents []string
	claims, ok := r.Context().Value("user_claims").(*models.JWTClaims)
	if ok && claims.Role == models.RoleOperator {
		assignedAgents = claims.AssignedAgents
	}

	dashboardRepo := repository.NewDashboardRepository(s.db)
	stats, err := dashboardRepo.GetDashboardStats(assignedAgents)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to retrieve dashboard statistics",
			"details": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    stats,
	})
}

// handleGetDailyFileTransferStats handles GET /api/v1/dashboard/daily-transfer-stats
func (s *SyncToolServer) handleGetDailyFileTransferStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Get user claims for filtering
	var assignedAgents []string
	claims, ok := r.Context().Value("user_claims").(*models.JWTClaims)
	if ok && claims.Role == models.RoleOperator {
		assignedAgents = claims.AssignedAgents
	}

	dashboardRepo := repository.NewDashboardRepository(s.db)
	stats, err := dashboardRepo.GetDailyFileTransferStats(assignedAgents)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to retrieve daily transfer statistics",
			"details": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    stats,
	})
}

// handleGetTopJobsPerformance handles GET /api/v1/dashboard/top-jobs-performance
func (s *SyncToolServer) handleGetTopJobsPerformance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Get user claims for filtering
	var assignedAgents []string
	claims, ok := r.Context().Value("user_claims").(*models.JWTClaims)
	if ok && claims.Role == models.RoleOperator {
		assignedAgents = claims.AssignedAgents
	}

	dashboardRepo := repository.NewDashboardRepository(s.db)

	// Get top jobs by file count
	topJobsByFileCount, err := dashboardRepo.GetTopJobsByFileCount(assignedAgents)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to retrieve top jobs by file count",
			"details": err.Error(),
		})
		return
	}

	// Get top jobs by data size
	topJobsByDataSize, err := dashboardRepo.GetTopJobsByDataSize(assignedAgents)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to retrieve top jobs by data size",
			"details": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"top_jobs_by_file_count": topJobsByFileCount,
			"top_jobs_by_data_size":  topJobsByDataSize,
		},
	})
}

// handleGetRecentFileTransferEvents handles GET /api/v1/dashboard/recent-events
func (s *SyncToolServer) handleGetRecentFileTransferEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Get user claims for filtering
	var assignedAgents []string
	claims, ok := r.Context().Value("user_claims").(*models.JWTClaims)
	if ok && claims.Role == models.RoleOperator {
		assignedAgents = claims.AssignedAgents
	}

	dashboardRepo := repository.NewDashboardRepository(s.db)
	events, err := dashboardRepo.GetRecentFileTransferEvents(assignedAgents)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to retrieve recent file transfer events",
			"details": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    events,
	})
}

// handleGetCompleteDashboard handles GET /api/v1/dashboard/complete
func (s *SyncToolServer) handleGetCompleteDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Get user claims for filtering
	var assignedAgents []string
	claims, ok := r.Context().Value("user_claims").(*models.JWTClaims)
	if ok && claims.Role == models.RoleOperator {
		assignedAgents = claims.AssignedAgents
	}

	dashboardRepo := repository.NewDashboardRepository(s.db)
	dashboardData, err := dashboardRepo.GetCompleteDashboard(assignedAgents)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to retrieve complete dashboard data",
			"details": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    dashboardData,
	})
}
