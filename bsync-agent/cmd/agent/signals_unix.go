//go:build !windows
// +build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	"bsync-agent/internal/agent"
)

// setupSignalHandling creates OS-specific signal handling for Unix systems
func setupSignalHandling() chan os.Signal {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	return sigChan
}

// handleSignal processes Unix-specific signals
func handleSignal(sig os.Signal, agent *agent.IntegratedAgent) bool {
	switch sig {
	case syscall.SIGHUP:
		// Reload configuration - Unix specific
		return handleConfigReload(agent)
	case syscall.SIGINT, syscall.SIGTERM:
		// Graceful shutdown - Cross platform
		return handleGracefulShutdown(agent)
	default:
		return false
	}
}