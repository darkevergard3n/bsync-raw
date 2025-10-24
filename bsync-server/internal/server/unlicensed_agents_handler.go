package server

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// handleUnlicensedAgents returns agents that don't have licenses assigned
// GET /api/v1/agents/unlicensed
func (s *SyncToolServer) handleUnlicensedAgents(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "GET" {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	if s.db == nil {
		http.Error(w, `{"error": "Database not available"}`, http.StatusServiceUnavailable)
		return
	}

	// Get agents that don't have licenses
	rows, err := s.db.Query(`
		SELECT ia.id, ia.agent_id, ia.device_id, ia.hostname, ia.ip_address, 
		       ia.os, ia.architecture, ia.version, ia.status, ia.approval_status, 
		       ia.last_heartbeat, ia.created_at, ia.updated_at, ia.data_dir
		FROM integrated_agents ia
		LEFT JOIN agent_licenses al ON ia.agent_id = al.agent_id
		WHERE al.agent_id IS NULL
		ORDER BY ia.created_at DESC
	`)
	if err != nil {
		log.Printf("❌ Failed to query unlicensed agents: %v", err)
		http.Error(w, `{"error": "Failed to fetch unlicensed agents"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	agents := []map[string]interface{}{}
	
	for rows.Next() {
		var id, agentID, deviceID, hostname, ipAddress, os, architecture, version, status, approvalStatus string
		var dataDir interface{} // Can be NULL
		var lastHeartbeat, createdAt, updatedAt time.Time
		
		if err := rows.Scan(&id, &agentID, &deviceID, &hostname, &ipAddress, &os, &architecture, &version, &status, &approvalStatus, &lastHeartbeat, &createdAt, &updatedAt, &dataDir); err != nil {
			log.Printf("❌ Failed to scan unlicensed agent row: %v", err)
			continue
		}
		
		dataDirStr := ""
		if dataDir != nil {
			dataDirStr = dataDir.(string)
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
			"data_dir":         dataDirStr,
			"has_license":      false, // Always false for this endpoint
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