package server

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	
	"bsync-server/internal/types"
)

// handleLicenses handles CRUD operations for licenses
// GET /api/v1/licenses - List all licenses
// POST /api/v1/licenses - Create new license
func (s *SyncToolServer) handleLicenses(w http.ResponseWriter, r *http.Request) {
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
		s.listLicenses(w, r)
	case "POST":
		s.createLicense(w, r)
	default:
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleLicenseActions handles actions on specific licenses
// GET /api/v1/licenses/{id} - Get license by ID
// PUT /api/v1/licenses/{id} - Update license
// DELETE /api/v1/licenses/{id} - Delete license
func (s *SyncToolServer) handleLicenseActions(w http.ResponseWriter, r *http.Request) {
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

	// Parse URL path: /api/v1/licenses/{id}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/licenses/"), "/")
	if len(pathParts) < 1 || pathParts[0] == "" {
		http.Error(w, `{"error": "Invalid URL format. Expected: /api/v1/licenses/{id}"}`, http.StatusBadRequest)
		return
	}

	licenseID, err := strconv.Atoi(pathParts[0])
	if err != nil {
		http.Error(w, `{"error": "Invalid license ID"}`, http.StatusBadRequest)
		return
	}

	switch r.Method {
	case "GET":
		s.getLicense(w, r, licenseID)
	case "PUT":
		s.updateLicense(w, r, licenseID)
	case "DELETE":
		s.deleteLicense(w, r, licenseID)
	default:
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// listLicenses retrieves all licenses
func (s *SyncToolServer) listLicenses(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT l.id, l.license_key, l.created_at, l.updated_at,
		       al.agent_id
		FROM licenses l
		LEFT JOIN agent_licenses al ON l.id = al.license_id
		ORDER BY l.created_at DESC
	`)
	if err != nil {
		log.Printf("❌ Failed to query licenses: %v", err)
		http.Error(w, `{"error": "Failed to fetch licenses"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	licenses := []map[string]interface{}{}
	
	for rows.Next() {
		var id int
		var licenseKey string
		var createdAt, updatedAt time.Time
		var agentID sql.NullString
		
		if err := rows.Scan(&id, &licenseKey, &createdAt, &updatedAt, &agentID); err != nil {
			log.Printf("❌ Failed to scan license row: %v", err)
			continue
		}
		
		license := map[string]interface{}{
			"id":          id,
			"license_key": licenseKey,
			"created_at":  createdAt.Format(time.RFC3339),
			"updated_at":  updatedAt.Format(time.RFC3339),
			"in_use":      agentID.Valid,
		}
		
		if agentID.Valid {
			license["assigned_to"] = agentID.String
		}
		
		licenses = append(licenses, license)
	}

	response := map[string]interface{}{
		"data":  licenses,
		"total": len(licenses),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// createLicense creates a new license
func (s *SyncToolServer) createLicense(w http.ResponseWriter, r *http.Request) {
	var req types.CreateLicenseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.LicenseKey == "" {
		http.Error(w, `{"error": "license_key is required"}`, http.StatusBadRequest)
		return
	}

	var id int
	err := s.db.QueryRow(`
		INSERT INTO licenses (license_key, created_at, updated_at) 
		VALUES ($1, NOW(), NOW()) 
		RETURNING id
	`, req.LicenseKey).Scan(&id)
	
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			http.Error(w, `{"error": "License key already exists"}`, http.StatusConflict)
		} else {
			log.Printf("❌ Failed to create license: %v", err)
			http.Error(w, `{"error": "Failed to create license"}`, http.StatusInternalServerError)
		}
		return
	}

	response := map[string]interface{}{
		"success":     true,
		"message":     "License created successfully",
		"id":          id,
		"license_key": req.LicenseKey,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
	
	log.Printf("✅ License created: ID=%d, Key=%s", id, req.LicenseKey)
}

// getLicense retrieves a specific license
func (s *SyncToolServer) getLicense(w http.ResponseWriter, r *http.Request, licenseID int) {
	var license types.License
	var agentID sql.NullString
	
	err := s.db.QueryRow(`
		SELECT l.id, l.license_key, l.created_at, l.updated_at,
		       al.agent_id
		FROM licenses l
		LEFT JOIN agent_licenses al ON l.id = al.license_id
		WHERE l.id = $1
	`, licenseID).Scan(&license.ID, &license.LicenseKey, &license.CreatedAt, &license.UpdatedAt, &agentID)
	
	if err == sql.ErrNoRows {
		http.Error(w, `{"error": "License not found"}`, http.StatusNotFound)
		return
	}
	
	if err != nil {
		log.Printf("❌ Failed to get license: %v", err)
		http.Error(w, `{"error": "Failed to get license"}`, http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"id":          license.ID,
		"license_key": license.LicenseKey,
		"created_at":  license.CreatedAt.Format(time.RFC3339),
		"updated_at":  license.UpdatedAt.Format(time.RFC3339),
		"in_use":      agentID.Valid,
	}
	
	if agentID.Valid {
		response["assigned_to"] = agentID.String
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// updateLicense updates a license
func (s *SyncToolServer) updateLicense(w http.ResponseWriter, r *http.Request, licenseID int) {
	var req types.CreateLicenseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.LicenseKey == "" {
		http.Error(w, `{"error": "license_key is required"}`, http.StatusBadRequest)
		return
	}

	result, err := s.db.Exec(`
		UPDATE licenses 
		SET license_key = $1, updated_at = NOW() 
		WHERE id = $2
	`, req.LicenseKey, licenseID)
	
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			http.Error(w, `{"error": "License key already exists"}`, http.StatusConflict)
		} else {
			log.Printf("❌ Failed to update license: %v", err)
			http.Error(w, `{"error": "Failed to update license"}`, http.StatusInternalServerError)
		}
		return
	}
	
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, `{"error": "License not found"}`, http.StatusNotFound)
		return
	}

	response := map[string]interface{}{
		"success":     true,
		"message":     "License updated successfully",
		"id":          licenseID,
		"license_key": req.LicenseKey,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
	
	log.Printf("✅ License updated: ID=%d, Key=%s", licenseID, req.LicenseKey)
}

// deleteLicense deletes a license and stops associated jobs
func (s *SyncToolServer) deleteLicense(w http.ResponseWriter, r *http.Request, licenseID int) {
	// Start transaction
	tx, err := s.db.Begin()
	if err != nil {
		log.Printf("❌ Failed to begin transaction: %v", err)
		http.Error(w, `{"error": "Failed to delete license"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// 1. Find which agent is using this license
	var agentID sql.NullString
	err = tx.QueryRow(`
		SELECT agent_id FROM agent_licenses WHERE license_id = $1
	`, licenseID).Scan(&agentID)
	
	if err != nil && err != sql.ErrNoRows {
		log.Printf("❌ Failed to find agent for license: %v", err)
		http.Error(w, `{"error": "Failed to delete license"}`, http.StatusInternalServerError)
		return
	}

	var deletedJobs []types.DeletedJobInfo
	
	// 2. If agent exists, stop all jobs involving this agent
	if agentID.Valid {
		log.Printf("🗑️ License %d is used by agent %s, stopping all jobs...", licenseID, agentID.String)
		
		// Get all jobs involving this agent (as source or destination)
		jobs, err := s.getJobsByAgent(tx, agentID.String)
		if err != nil {
			log.Printf("❌ Failed to get jobs for agent: %v", err)
			http.Error(w, `{"error": "Failed to delete license"}`, http.StatusInternalServerError)
			return
		}
		
		// Stop each job
		for _, job := range jobs {
			// First stop the job on agents
			if err := s.deleteJobOnAgentsSync(job.JobID); err != nil {
				log.Printf("❌ Failed to stop job %s on agents: %v", job.JobID, err)
				// Continue with other jobs - don't fail the entire operation
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
	}

	// 3. Delete the license (CASCADE will delete agent_licenses mapping)
	result, err := tx.Exec(`DELETE FROM licenses WHERE id = $1`, licenseID)
	if err != nil {
		log.Printf("❌ Failed to delete license: %v", err)
		http.Error(w, `{"error": "Failed to delete license"}`, http.StatusInternalServerError)
		return
	}
	
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, `{"error": "License not found"}`, http.StatusNotFound)
		return
	}

	// 4. Commit transaction
	if err := tx.Commit(); err != nil {
		log.Printf("❌ Failed to commit transaction: %v", err)
		http.Error(w, `{"error": "Failed to delete license"}`, http.StatusInternalServerError)
		return
	}

	// 5. Build response
	response := types.LicenseDeleteResponse{
		Success: true,
		Message: "License deleted successfully",
		DeletedJobs: deletedJobs,
	}
	
	if agentID.Valid {
		response.AffectedAgent = &agentID.String
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
	
	log.Printf("✅ License %d deleted successfully. Affected agent: %v, Jobs deleted: %d", 
		licenseID, agentID.String, len(deletedJobs))
}

// getJobsByAgent retrieves all jobs involving a specific agent
func (s *SyncToolServer) getJobsByAgent(tx *sql.Tx, agentID string) ([]struct{ JobID, JobName string }, error) {
	rows, err := tx.Query(`
		SELECT id, name 
		FROM sync_jobs 
		WHERE source_agent_id = $1 OR target_agent_id = $1
	`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []struct{ JobID, JobName string }
	for rows.Next() {
		var job struct{ JobID, JobName string }
		if err := rows.Scan(&job.JobID, &job.JobName); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	
	return jobs, nil
}