package main

import (
	"fmt"
	"log"

	"bsync-agent/internal/agent"
)

// reloadAgentConfig reloads the agent configuration from file
func reloadAgentConfig(agent *agent.IntegratedAgent) error {
	// Check if config file is specified
	if *configFile == "" {
		return fmt.Errorf("no config file specified for reload")
	}
	
	// Load new config
	newConfig, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load new config: %w", err)
	}
	
	// Apply new event debug setting to Syncthing
	if syncthingInstance := agent.GetSyncthing(); syncthingInstance != nil {
		syncthingInstance.SetEventDebug(newConfig.EventDebug)
		log.Printf("Event debug setting updated to: %v", newConfig.EventDebug)
	}
	
	log.Println("Configuration reload completed")
	return nil
}