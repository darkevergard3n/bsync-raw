package types

import (
	"time"
)

// AgentEvent represents an event from the agent
type AgentEvent struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// AgentStatus represents the status of an agent
type AgentStatus string

const (
	AgentStatusOnline   AgentStatus = "online"
	AgentStatusOffline  AgentStatus = "offline"
	AgentStatusError    AgentStatus = "error"
	AgentStatusUpdating AgentStatus = "updating"
)

// SyncthingConfig represents Syncthing configuration for the agent
type SyncthingConfig struct {
	Home               string `yaml:"home" json:"home"`
	UseSystemSyncthing bool   `yaml:"use_system_syncthing" json:"use_system_syncthing"`
	APIKey             string `yaml:"api_key" json:"api_key"`
	APIPort            int    `yaml:"api_port" json:"api_port"`
}