//go:build windows
// +build windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	"bsync-agent/internal/agent"
)

// setupSignalHandling creates OS-specific signal handling for Windows systems
func setupSignalHandling() chan os.Signal {
	sigChan := make(chan os.Signal, 1)
	// Windows doesn't support SIGHUP, only SIGINT and SIGTERM
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	return sigChan
}

// handleSignal processes Windows-specific signals
func handleSignal(sig os.Signal, agent *agent.IntegratedAgent) bool {
	switch sig {
	case syscall.SIGINT, syscall.SIGTERM:
		// Graceful shutdown - Cross platform
		return handleGracefulShutdown(agent)
	default:
		return false
	}
}