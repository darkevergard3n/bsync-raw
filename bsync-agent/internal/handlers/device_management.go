package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"bsync-agent/internal/agent"
	"bsync-agent/internal/embedded"
)

// DeviceManagementHandler handles device-related API requests
type DeviceManagementHandler struct {
	agent *agent.IntegratedAgent
}

// NewDeviceManagementHandler creates a new device management handler
func NewDeviceManagementHandler(agent *agent.IntegratedAgent) *DeviceManagementHandler {
	return &DeviceManagementHandler{
		agent: agent,
	}
}

// AddDeviceRequest represents a request to add a new device
type AddDeviceRequest struct {
	DeviceID string `json:"device_id"`
	Name     string `json:"name"`
	Address  string `json:"address"`
}

// AddDeviceResponse represents the response after adding a device
type AddDeviceResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// GetDeviceIDResponse represents the response containing device ID
type GetDeviceIDResponse struct {
	DeviceID string `json:"device_id"`
}

// ListDevicesResponse represents the response containing all devices
type ListDevicesResponse struct {
	Devices map[string]*embedded.ConnectionInfo `json:"devices"`
}

// HandleAddDevice handles POST /api/v1/devices
func (h *DeviceManagementHandler) HandleAddDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AddDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.DeviceID == "" || req.Name == "" || req.Address == "" {
		http.Error(w, "Missing required fields: device_id, name, address", http.StatusBadRequest)
		return
	}

	// Add device using internal method
	err := h.agent.AddDevice(req.DeviceID, req.Name, req.Address)
	
	response := AddDeviceResponse{
		Success: err == nil,
	}
	
	if err != nil {
		response.Message = fmt.Sprintf("Failed to add device: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		response.Message = fmt.Sprintf("Device %s added successfully", req.Name)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleGetDeviceID handles GET /api/v1/device/id
func (h *DeviceManagementHandler) HandleGetDeviceID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	deviceID := h.agent.GetDeviceID()
	
	response := GetDeviceIDResponse{
		DeviceID: deviceID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleListDevices handles GET /api/v1/devices
func (h *DeviceManagementHandler) HandleListDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	connections, err := h.agent.GetConnections()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get devices: %v", err), http.StatusInternalServerError)
		return
	}

	response := ListDevicesResponse{
		Devices: connections,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}