package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"bsync-agent/pkg/types"
	"github.com/spf13/viper"
)

// LoadServerConfig loads server configuration from file and environment
func LoadServerConfig(configPath string) (*types.ServerConfig, error) {
	v := viper.New()
	
	// Set config file path
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("server")
		v.SetConfigType("yaml")
		// Add cross-platform config paths
		for _, path := range getConfigPaths("server") {
			v.AddConfigPath(path)
		}
	}

	// Set environment variables
	v.SetEnvPrefix("SYNCTOOL")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Set defaults
	setServerDefaults(v)

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	var config types.ServerConfig
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := validateServerConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// LoadAgentConfig loads agent configuration from file and environment
func LoadAgentConfig(configPath string) (*types.AgentConfig, error) {
	v := viper.New()
	
	// Set config file path
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		// Add cross-platform config paths
		for _, path := range getConfigPaths("agent") {
			v.AddConfigPath(path)
		}
	}

	// Set environment variables
	v.SetEnvPrefix("SYNC_AGENT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Set defaults
	setAgentDefaults(v)

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	var config types.AgentConfig
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Set hostname if not provided
	if config.Hostname == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("failed to get hostname: %w", err)
		}
		config.Hostname = hostname
	}

	// Validate configuration
	if err := validateAgentConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// setServerDefaults sets default values for server configuration
func setServerDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.tls.enabled", false)
	// Set cross-platform TLS certificate defaults
	certFile, keyFile, caFile := getTLSDefaults()
	v.SetDefault("server.tls.cert_file", certFile)
	v.SetDefault("server.tls.key_file", keyFile)
	v.SetDefault("server.tls.ca_file", caFile)

	// Database defaults
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.name", "synctool")
	v.SetDefault("database.user", "synctool")
	v.SetDefault("database.password", "")
	v.SetDefault("database.ssl_mode", "disable")
	v.SetDefault("database.max_connections", 100)
	v.SetDefault("database.idle_connections", 10)
	v.SetDefault("database.connection_lifetime", "1h")

	// Redis defaults
	v.SetDefault("redis.host", "localhost")
	v.SetDefault("redis.port", 6379)
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)

	// Security defaults
	v.SetDefault("security.jwt_secret", "")
	v.SetDefault("security.admin_approval_required", true)
	v.SetDefault("security.session_timeout", "24h")
	v.SetDefault("security.max_failed_logins", 5)
	v.SetDefault("security.lockout_duration", "30m")

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.file", getLogFilePath())
}

// setAgentDefaults sets default values for agent configuration
func setAgentDefaults(v *viper.Viper) {
	v.SetDefault("management_server", "http://localhost:8080")
	v.SetDefault("hostname", "")
	v.SetDefault("heartbeat_interval", "30s")
	v.SetDefault("data_dir", "./agent-data")
	v.SetDefault("log_level", "info")
	v.SetDefault("event_debug", false)

	// Syncthing defaults
	v.SetDefault("syncthing.home", "./agent-data/syncthing")
	v.SetDefault("syncthing.gui_enabled", false)
	v.SetDefault("syncthing.relays_enabled", false)
	v.SetDefault("syncthing.global_announce_enabled", false)
	v.SetDefault("syncthing.local_announce_enabled", false)
	v.SetDefault("syncthing.upnp_enabled", false)

	// Monitoring defaults
	v.SetDefault("monitoring.enabled", true)
	v.SetDefault("monitoring.report_interval", "30s")
	v.SetDefault("monitoring.auto_resync_enabled", true)
	v.SetDefault("monitoring.auto_resync_interval", "30s")
}

// validateServerConfig validates server configuration
func validateServerConfig(config *types.ServerConfig) error {
	// Validate server settings
	if config.Server.Port <= 0 || config.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", config.Server.Port)
	}

	// Validate TLS settings
	if config.Server.TLS.Enabled {
		if config.Server.TLS.CertFile == "" {
			return fmt.Errorf("TLS cert file is required when TLS is enabled")
		}
		if config.Server.TLS.KeyFile == "" {
			return fmt.Errorf("TLS key file is required when TLS is enabled")
		}
	}

	// Validate database settings
	if config.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}
	if config.Database.Port <= 0 || config.Database.Port > 65535 {
		return fmt.Errorf("invalid database port: %d", config.Database.Port)
	}
	if config.Database.Name == "" {
		return fmt.Errorf("database name is required")
	}
	if config.Database.User == "" {
		return fmt.Errorf("database user is required")
	}

	// Validate Redis settings
	if config.Redis.Host == "" {
		return fmt.Errorf("Redis host is required")
	}
	if config.Redis.Port <= 0 || config.Redis.Port > 65535 {
		return fmt.Errorf("invalid Redis port: %d", config.Redis.Port)
	}

	// Validate security settings
	if config.Security.JWTSecret == "" {
		return fmt.Errorf("JWT secret is required")
	}
	if len(config.Security.JWTSecret) < 32 {
		return fmt.Errorf("JWT secret must be at least 32 characters long")
	}

	// Validate session timeout
	if _, err := time.ParseDuration(config.Security.SessionTimeout); err != nil {
		return fmt.Errorf("invalid session timeout: %w", err)
	}

	// Validate lockout duration
	if _, err := time.ParseDuration(config.Security.LockoutDuration); err != nil {
		return fmt.Errorf("invalid lockout duration: %w", err)
	}

	return nil
}

// validateAgentConfig validates agent configuration
func validateAgentConfig(config *types.AgentConfig) error {
	// Validate management server URL
	if config.ManagementServer == "" {
		return fmt.Errorf("management server URL is required")
	}

	// Validate hostname
	if config.Hostname == "" {
		return fmt.Errorf("hostname is required")
	}

	// Validate heartbeat interval
	if _, err := time.ParseDuration(config.HeartbeatInterval); err != nil {
		return fmt.Errorf("invalid heartbeat interval: %w", err)
	}

	// Validate data directory
	if config.DataDir == "" {
		return fmt.Errorf("data directory is required")
	}

	// Validate Syncthing home directory
	if config.Syncthing.Home == "" {
		return fmt.Errorf("Syncthing home directory is required")
	}

	return nil
}

// GetDatabaseDSN returns the database connection string
func GetDatabaseDSN(config types.DatabaseSettings) string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.User, config.Password, config.Name, config.SSLMode)
}

// GetRedisAddr returns the Redis connection address
func GetRedisAddr(config types.RedisSettings) string {
	return fmt.Sprintf("%s:%d", config.Host, config.Port)
}

// IsProduction returns true if running in production mode
func IsProduction() bool {
	env := os.Getenv("SYNCTOOL_ENV")
	return env == "production" || env == "prod"
}

// IsDevelopment returns true if running in development mode
func IsDevelopment() bool {
	env := os.Getenv("SYNCTOOL_ENV")
	return env == "development" || env == "dev" || env == ""
}

// GetLogLevel returns the log level based on environment
func GetLogLevel(configLevel string) string {
	if IsDevelopment() && configLevel == "info" {
		return "debug"
	}
	return configLevel
}

// EnsureDirectories creates necessary directories if they don't exist
func EnsureDirectories(config *types.ServerConfig) error {
	dirs := getSystemDirectories()

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// EnsureAgentDirectories creates necessary directories for agent
func EnsureAgentDirectories(config *types.AgentConfig) error {
	dirs := []string{
		config.DataDir,
		config.Syncthing.Home,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// ConfigWatcher watches configuration file for changes
type ConfigWatcher struct {
	configPath string
	onChange   func()
}

// NewConfigWatcher creates a new configuration file watcher
func NewConfigWatcher(configPath string, onChange func()) *ConfigWatcher {
	return &ConfigWatcher{
		configPath: configPath,
		onChange:   onChange,
	}
}

// Start starts watching the configuration file
func (w *ConfigWatcher) Start() error {
	// TODO: Implement file watcher using fsnotify
	// This would allow hot-reloading of configuration
	return nil
}

// Stop stops watching the configuration file
func (w *ConfigWatcher) Stop() {
	// TODO: Implement cleanup
}

// Environment variable helpers
func GetEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func GetRequiredEnv(key string) (string, error) {
	value := os.Getenv(key)
	if value == "" {
		return "", fmt.Errorf("required environment variable %s is not set", key)
	}
	return value, nil
}