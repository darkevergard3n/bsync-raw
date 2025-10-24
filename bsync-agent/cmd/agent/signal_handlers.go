package main

import (
	"context"
	"log"
	"time"

	"bsync-agent/internal/agent"
)

// handleConfigReload handles configuration reload requests
func handleConfigReload(agent *agent.IntegratedAgent) bool {
	log.Println("Reloading configuration...")
	if err := reloadAgentConfig(agent); err != nil {
		log.Printf("Failed to reload config: %v", err)
	} else {
		log.Println("Configuration reloaded successfully")
	}
	return false // Continue running
}

// handleGracefulShutdown handles graceful shutdown requests
func handleGracefulShutdown(agent *agent.IntegratedAgent) bool {
	log.Println("Initiating graceful shutdown...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := agent.Stop(); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
	}()
	
	select {
	case <-done:
		log.Println("Graceful shutdown completed")
	case <-shutdownCtx.Done():
		log.Println("Forced shutdown due to timeout")
	}
	return true // Exit application
}