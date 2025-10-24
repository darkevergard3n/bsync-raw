package handlers

import (
	"database/sql"
	"net/http"

	"bsync-server/internal/repository"

	"github.com/gin-gonic/gin"
)

// DashboardHandlers handles dashboard-related HTTP requests
type DashboardHandlers struct {
	dashboardRepo *repository.DashboardRepository
}

// NewDashboardHandlers creates a new dashboard handlers instance
func NewDashboardHandlers(db *sql.DB) *DashboardHandlers {
	return &DashboardHandlers{
		dashboardRepo: repository.NewDashboardRepository(db),
	}
}

// GetRoleList handles GET /api/roles
// @Summary Get list of user roles
// @Description Get all available user roles with descriptions
// @Tags Dashboard
// @Produce json
// @Success 200 {object} map[string]interface{} "success: true, data: []RoleInfo"
// @Failure 500 {object} map[string]interface{} "error message"
// @Router /api/roles [get]
func (h *DashboardHandlers) GetRoleList(c *gin.Context) {
	roles, err := h.dashboardRepo.GetRoleList()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to retrieve role list",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    roles,
	})
}

// GetUserStats handles GET /api/dashboard/user-stats
// @Summary Get user statistics
// @Description Get statistics about users (total, admin, operator, active)
// @Tags Dashboard
// @Produce json
// @Success 200 {object} map[string]interface{} "success: true, data: UserStats"
// @Failure 500 {object} map[string]interface{} "error message"
// @Router /api/dashboard/user-stats [get]
func (h *DashboardHandlers) GetUserStats(c *gin.Context) {
	stats, err := h.dashboardRepo.GetUserStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to retrieve user statistics",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// GetLicensedAgents handles GET /api/agents/licensed
// @Summary Get list of licensed agents
// @Description Get all agents that have been licensed
// @Tags Dashboard
// @Produce json
// @Success 200 {object} map[string]interface{} "success: true, data: []LicensedAgent"
// @Failure 500 {object} map[string]interface{} "error message"
// @Router /api/agents/licensed [get]
func (h *DashboardHandlers) GetLicensedAgents(c *gin.Context) {
	agents, err := h.dashboardRepo.GetLicensedAgents()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to retrieve licensed agents",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    agents,
		"count":   len(agents),
	})
}

// GetDashboardStats handles GET /api/dashboard/stats
// @Summary Get main dashboard statistics
// @Description Get main dashboard statistics (agents, jobs, users, files, data transferred)
// @Tags Dashboard
// @Produce json
// @Success 200 {object} map[string]interface{} "success: true, data: DashboardStats"
// @Failure 500 {object} map[string]interface{} "error message"
// @Router /api/dashboard/stats [get]
func (h *DashboardHandlers) GetDashboardStats(c *gin.Context) {
	stats, err := h.dashboardRepo.GetDashboardStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to retrieve dashboard statistics",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// GetDailyFileTransferStats handles GET /api/dashboard/daily-transfer-stats
// @Summary Get daily file transfer statistics
// @Description Get file transfer statistics for the last 7 days
// @Tags Dashboard
// @Produce json
// @Success 200 {object} map[string]interface{} "success: true, data: []DailyFileTransferStats"
// @Failure 500 {object} map[string]interface{} "error message"
// @Router /api/dashboard/daily-transfer-stats [get]
func (h *DashboardHandlers) GetDailyFileTransferStats(c *gin.Context) {
	stats, err := h.dashboardRepo.GetDailyFileTransferStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to retrieve daily transfer statistics",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// GetTopJobsPerformance handles GET /api/dashboard/top-jobs-performance
// @Summary Get top jobs performance
// @Description Get top 5 jobs by file count and top 5 jobs by data size
// @Tags Dashboard
// @Produce json
// @Success 200 {object} map[string]interface{} "success: true, data: TopJobsPerformance"
// @Failure 500 {object} map[string]interface{} "error message"
// @Router /api/dashboard/top-jobs-performance [get]
func (h *DashboardHandlers) GetTopJobsPerformance(c *gin.Context) {
	// Get top jobs by file count
	topJobsByFileCount, err := h.dashboardRepo.GetTopJobsByFileCount()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to retrieve top jobs by file count",
			"details": err.Error(),
		})
		return
	}

	// Get top jobs by data size
	topJobsByDataSize, err := h.dashboardRepo.GetTopJobsByDataSize()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to retrieve top jobs by data size",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"top_jobs_by_file_count": topJobsByFileCount,
			"top_jobs_by_data_size":  topJobsByDataSize,
		},
	})
}

// GetRecentFileTransferEvents handles GET /api/dashboard/recent-events
// @Summary Get recent file transfer events
// @Description Get last 5 completed file transfer events
// @Tags Dashboard
// @Produce json
// @Success 200 {object} map[string]interface{} "success: true, data: []RecentFileTransferEvent"
// @Failure 500 {object} map[string]interface{} "error message"
// @Router /api/dashboard/recent-events [get]
func (h *DashboardHandlers) GetRecentFileTransferEvents(c *gin.Context) {
	events, err := h.dashboardRepo.GetRecentFileTransferEvents()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to retrieve recent file transfer events",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    events,
	})
}

// GetCompleteDashboard handles GET /api/dashboard/complete
// @Summary Get complete dashboard data
// @Description Get all dashboard data in one API call (stats, user stats, daily stats, top jobs, recent events)
// @Tags Dashboard
// @Produce json
// @Success 200 {object} map[string]interface{} "success: true, data: DashboardResponse"
// @Failure 500 {object} map[string]interface{} "error message"
// @Router /api/dashboard/complete [get]
func (h *DashboardHandlers) GetCompleteDashboard(c *gin.Context) {
	dashboardData, err := h.dashboardRepo.GetCompleteDashboard()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to retrieve complete dashboard data",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    dashboardData,
	})
}
