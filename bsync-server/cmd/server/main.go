package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"bsync-server/internal/server"
)

func main() {
	var (
		port       = flag.Int("port", 8090, "Server port")
		host       = flag.String("host", "0.0.0.0", "Server host")
		configPath = flag.String("config", "", "Configuration file path")
		logLevel   = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
		tlsEnabled = flag.Bool("tls", false, "Enable TLS/HTTPS")
		certFile   = flag.String("cert", "", "TLS certificate file path")
		keyFile    = flag.String("key", "", "TLS key file path")
		autoTLS    = flag.Bool("auto-tls", false, "Generate self-signed certificate (dev only)")
		version    = flag.Bool("version", false, "Show version information")
	)

	flag.Parse()

	if *version {
		fmt.Println("BSync Server v1.0.0")
		fmt.Println("WebSocket-based agent orchestration server")
		os.Exit(0)
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Printf("🚀 Starting BSync Server")
	log.Printf("📡 Server configuration:")
	log.Printf("  Host: %s", *host)
	log.Printf("  Port: %d", *port)
	log.Printf("  Log Level: %s", *logLevel)
	log.Printf("  TLS Enabled: %v", *tlsEnabled)
	if *configPath != "" {
		log.Printf("  Config: %s", *configPath)
	}

	config := &server.Config{
		Host:     *host,
		Port:     *port,
		LogLevel: *logLevel,
	}

	if *configPath != "" {
		if err := config.LoadFromFile(*configPath); err != nil {
			log.Printf("⚠️  Failed to load config file: %v", err)
		}
	}

	srv, err := server.NewSyncToolServer(config)
	if err != nil {
		log.Fatalf("❌ Failed to create server: %v", err)
	}

	tlsConfig := &server.TLSConfig{
		Enabled:  *tlsEnabled,
		CertFile: *certFile,
		KeyFile:  *keyFile,
		AutoTLS:  *autoTLS,
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	errChan := make(chan error, 1)
	go func() {
		if *tlsEnabled {
			if err := srv.StartWithTLS(tlsConfig); err != nil {
				errChan <- err
			}
		} else {
			if err := srv.Start(); err != nil {
				errChan <- err
			}
		}
	}()

	select {
	case sig := <-sigChan:
		log.Printf("📛 Received signal: %v", sig)
		srv.Shutdown()
	case err := <-errChan:
		log.Fatalf("❌ Server error: %v", err)
	}

	log.Println("👋 BSync Server stopped")
}