package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
	
	"bsync-server/internal/types"
)

// handleAgentLicenses handles CRUD operations for agent-license mappings
// GET /api/v1/agent-licenses - List all agent-license mappings
// POST /api/v1/agent-licenses - Create new agent-license mapping
func (s *SyncToolServer) handleAgentLicenses(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	
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
		s.listAgentLicenses(w, r)
	case "POST":
		s.createAgentLicense(w, r)
	default:
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleAgentLicenseActions handles actions on specific agent-license mappings
// GET /api/v1/agent-licenses/{agent_id} - Get mapping by agent ID
// DELETE /api/v1/agent-licenses/{agent_id} - Delete mapping (stops jobs)
func (s *SyncToolServer) handleAgentLicenseActions(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if s.db == nil {
		http.Error(w, `{"error": "Database not available"}`, http.StatusServiceUnavailable)
		return
	}

	// Parse URL path: /api/v1/agent-licenses/{agent_id}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/agent-licenses/"), "/")
	if len(pathParts) < 1 || pathParts[0] == "" {
		http.Error(w, `{"error": "Invalid URL format. Expected: /api/v1/agent-licenses/{agent_id}"}`, http.StatusBadRequest)
		return
	}

	agentID := pathParts[0]

	switch r.Method {
	case "GET":
		s.getAgentLicense(w, r, agentID)
	case "DELETE":
		s.deleteAgentLicense(w, r, agentID)
	default:
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// listAgentLicenses retrieves all agent-license mappings
func (s *SyncToolServer) listAgentLicenses(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT al.id, al.agent_id, al.license_id, al.created_at, al.updated_at,
		       l.license_key,
		       ia.hostname, ia.ip_address, ia.status
		FROM agent_licenses al
		JOIN licenses l ON al.license_id = l.id
		LEFT JOIN integrated_agents ia ON al.agent_id = ia.agent_id
		ORDER BY al.created_at DESC
	`)
	if err != nil {
		log.Printf("❌ Failed to query agent-licenses: %v", err)
		http.Error(w, `{"error": "Failed to fetch agent-license mappings"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	mappings := []map[string]interface{}{}
	
	for rows.Next() {
		var id, licenseID int
		var agentID, licenseKey string
		var createdAt, updatedAt time.Time
		var hostname, ipAddress, status sql.NullString
		
		if err := rows.Scan(&id, &agentID, &licenseID, &createdAt, &updatedAt, 
			&licenseKey, &hostname, &ipAddress, &status); err != nil {
			log.Printf("❌ Failed to scan agent-license row: %v", err)
			continue
		}
		
		mapping := map[string]interface{}{
			"id":          id,
			"agent_id":    agentID,
			"license_id":  licenseID,
			"license_key": licenseKey,
			"created_at":  createdAt.Format(time.RFC3339),
			"updated_at":  updatedAt.Format(time.RFC3339),
			"agent_info": map[string]interface{}{
				"hostname":   hostname.String,
				"ip_address": ipAddress.String,
				"status":     status.String,
				"exists":     hostname.Valid, // Agent exists in integrated_agents table
			},
		}
		
		mappings = append(mappings, mapping)
	}

	response := map[string]interface{}{
		"data":  mappings,
		"total": len(mappings),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// createAgentLicense creates a new agent-license mapping
func (s *SyncToolServer) createAgentLicense(w http.ResponseWriter, r *http.Request) {
	var req types.CreateAgentLicenseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.AgentID == "" || req.LicenseID == 0 {
		http.Error(w, `{"error": "agent_id and license_id are required"}`, http.StatusBadRequest)
		return
	}

	// Start transaction
	tx, err := s.db.Begin()
	if err != nil {
		log.Printf("❌ Failed to begin transaction: %v", err)
		http.Error(w, `{"error": "Failed to create agent-license mapping"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// 1. Check if license exists
	var licenseExists bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM licenses WHERE id = $1)", req.LicenseID).Scan(&licenseExists)
	if err != nil {
		log.Printf("❌ Failed to check license existence: %v", err)
		http.Error(w, `{"error": "Failed to create agent-license mapping"}`, http.StatusInternalServerError)
		return
	}
	
	if !licenseExists {
		http.Error(w, `{"error": "License not found"}`, http.StatusBadRequest)
		return
	}

	// 2. Check if agent already has a license (would conflict with UNIQUE constraint)
	var existingLicenseID sql.NullInt64
	err = tx.QueryRow("SELECT license_id FROM agent_licenses WHERE agent_id = $1", req.AgentID).Scan(&existingLicenseID)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("❌ Failed to check existing agent license: %v", err)
		http.Error(w, `{"error": "Failed to create agent-license mapping"}`, http.StatusInternalServerError)
		return
	}
	
	// 3. If agent has existing license, stop jobs first
	var deletedJobs []types.DeletedJobInfo
	if existingLicenseID.Valid {
		log.Printf("🔄 Agent %s already has license %d, stopping jobs and updating...", req.AgentID, existingLicenseID.Int64)
		
		// Get jobs for this agent
		jobs, err := s.getJobsByAgent(tx, req.AgentID)
		if err != nil {
			log.Printf("❌ Failed to get jobs for agent: %v", err)
			http.Error(w, `{"error": "Failed to create agent-license mapping"}`, http.StatusInternalServerError)
			return
		}
		
		// Stop each job
		for _, job := range jobs {
			// First stop the job on agents
			if err := s.deleteJobOnAgentsSync(job.JobID); err != nil {
				log.Printf("❌ Failed to stop job %s on agents: %v", job.JobID, err)
				// Continue to delete from database even if stopping on agents failed
			} else {
				log.Printf("✅ Stopped job on agents: %s (%s)", job.JobID, job.JobName)
			}
			
			// Then delete from database
			_, err := tx.Exec("DELETE FROM sync_jobs WHERE id = $1", job.JobID)
			if err != nil {
				log.Printf("❌ Failed to delete job %s from database: %v", job.JobID, err)
			} else {
				deletedJobs = append(deletedJobs, types.DeletedJobInfo{
					JobID:   job.JobID,
					JobName: job.JobName,
				})
				log.Printf("✅ Deleted job from database: %s (%s)", job.JobID, job.JobName)
			}
		}
		
		// Delete existing mapping
		_, err = tx.Exec("DELETE FROM agent_licenses WHERE agent_id = $1", req.AgentID)
		if err != nil {
			log.Printf("❌ Failed to delete existing agent-license mapping: %v", err)
			http.Error(w, `{"error": "Failed to create agent-license mapping"}`, http.StatusInternalServerError)
			return
		}
	}

	// 4. Create new mapping
	var id int
	err = tx.QueryRow(`
		INSERT INTO agent_licenses (agent_id, license_id, created_at) 
		VALUES ($1, $2, NOW()) 
		RETURNING id
	`, req.AgentID, req.LicenseID).Scan(&id)
	
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") && strings.Contains(err.Error(), "license_id") {
			http.Error(w, `{"error": "License is already assigned to another agent"}`, http.StatusConflict)
		} else {
			log.Printf("❌ Failed to create agent-license mapping: %v", err)
			http.Error(w, `{"error": "Failed to create agent-license mapping"}`, http.StatusInternalServerError)
		}
		return
	}

	// 5. Commit transaction
	if err := tx.Commit(); err != nil {
		log.Printf("❌ Failed to commit transaction: %v", err)
		http.Error(w, `{"error": "Failed to create agent-license mapping"}`, http.StatusInternalServerError)
		return
	}

	// 6. Build response
	action := "created"
	if existingLicenseID.Valid {
		action = "updated"
	}
	
	response := types.AgentLicenseMappingResponse{
		Success:     true,
		Message:     fmt.Sprintf("Agent-license mapping %s successfully", action),
		AgentID:     req.AgentID,
		LicenseID:   &req.LicenseID,
		DeletedJobs: deletedJobs,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
	
	log.Printf("✅ Agent-license mapping %s: Agent=%s, License=%d, ID=%d, Jobs deleted=%d", 
		action, req.AgentID, req.LicenseID, id, len(deletedJobs))
}

// getAgentLicense retrieves a specific agent-license mapping
func (s *SyncToolServer) getAgentLicense(w http.ResponseWriter, r *http.Request, agentID string) {
	var mapping types.AgentLicense
	var licenseKey string
	
	err := s.db.QueryRow(`
		SELECT al.id, al.agent_id, al.license_id, al.created_at, al.updated_at, l.license_key
		FROM agent_licenses al
		JOIN licenses l ON al.license_id = l.id
		WHERE al.agent_id = $1
	`, agentID).Scan(&mapping.ID, &mapping.AgentID, &mapping.LicenseID, 
		&mapping.CreatedAt, &mapping.UpdatedAt, &licenseKey)
	
	if err == sql.ErrNoRows {
		http.Error(w, `{"error": "Agent-license mapping not found"}`, http.StatusNotFound)
		return
	}
	
	if err != nil {
		log.Printf("❌ Failed to get agent-license mapping: %v", err)
		http.Error(w, `{"error": "Failed to get agent-license mapping"}`, http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"id":          mapping.ID,
		"agent_id":    mapping.AgentID,
		"license_id":  mapping.LicenseID,
		"license_key": licenseKey,
		"created_at":  mapping.CreatedAt.Format(time.RFC3339),
		"updated_at":  mapping.UpdatedAt.Format(time.RFC3339),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// deleteAgentLicense removes an agent-license mapping and stops associated jobs
func (s *SyncToolServer) deleteAgentLicense(w http.ResponseWriter, r *http.Request, agentID string) {
	// Start transaction
	tx, err := s.db.Begin()
	if err != nil {
		log.Printf("❌ Failed to begin transaction: %v", err)
		http.Error(w, `{"error": "Failed to delete agent-license mapping"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// 1. Check if mapping exists
	var mappingExists bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM agent_licenses WHERE agent_id = $1)", agentID).Scan(&mappingExists)
	if err != nil {
		log.Printf("❌ Failed to check mapping existence: %v", err)
		http.Error(w, `{"error": "Failed to delete agent-license mapping"}`, http.StatusInternalServerError)
		return
	}
	
	if !mappingExists {
		http.Error(w, `{"error": "Agent-license mapping not found"}`, http.StatusNotFound)
		return
	}

	// 2. Get all jobs involving this agent and stop them
	log.Printf("🗑️ Stopping all jobs for agent %s...", agentID)
	
	jobs, err := s.getJobsByAgent(tx, agentID)
	if err != nil {
		log.Printf("❌ Failed to get jobs for agent: %v", err)
		http.Error(w, `{"error": "Failed to delete agent-license mapping"}`, http.StatusInternalServerError)
		return
	}
	
	var deletedJobs []types.DeletedJobInfo
	for _, job := range jobs {
		// First stop the job on agents
		if err := s.deleteJobOnAgentsSync(job.JobID); err != nil {
			log.Printf("❌ Failed to stop job %s on agents: %v", job.JobID, err)
			// Continue to delete from database even if stopping on agents failed
		} else {
			log.Printf("✅ Stopped job on agents: %s (%s)", job.JobID, job.JobName)
		}
		
		// Then delete from database
		_, err := tx.Exec("DELETE FROM sync_jobs WHERE id = $1", job.JobID)
		if err != nil {
			log.Printf("❌ Failed to delete job %s from database: %v", job.JobID, err)
		} else {
			deletedJobs = append(deletedJobs, types.DeletedJobInfo{
				JobID:   job.JobID,
				JobName: job.JobName,
			})
			log.Printf("✅ Deleted job from database: %s (%s)", job.JobID, job.JobName)
		}
	}

	// 3. Delete the mapping
	result, err := tx.Exec("DELETE FROM agent_licenses WHERE agent_id = $1", agentID)
	if err != nil {
		log.Printf("❌ Failed to delete agent-license mapping: %v", err)
		http.Error(w, `{"error": "Failed to delete agent-license mapping"}`, http.StatusInternalServerError)
		return
	}
	
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, `{"error": "Agent-license mapping not found"}`, http.StatusNotFound)
		return
	}

	// 4. Commit transaction
	if err := tx.Commit(); err != nil {
		log.Printf("❌ Failed to commit transaction: %v", err)
		http.Error(w, `{"error": "Failed to delete agent-license mapping"}`, http.StatusInternalServerError)
		return
	}

	// 5. Build response
	response := types.AgentLicenseMappingResponse{
		Success:     true,
		Message:     "Agent-license mapping deleted successfully",
		AgentID:     agentID,
		DeletedJobs: deletedJobs,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
	
	log.Printf("✅ Agent-license mapping deleted: Agent=%s, Jobs deleted=%d", agentID, len(deletedJobs))
}