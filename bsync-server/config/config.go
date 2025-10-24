package config

import "os"

type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

func LoadSMTPConfig() *SMTPConfig {
	return &SMTPConfig{
		Host:     "smtp.gmail.com",
		Port:     "587",
		Username: "gustomanchannel@gmail.com",
		Password: "blepfefvxwahjtft", // gunakan App Password
		From:     "gustomanchannel@gmail.com",
	}
}

// GetWebURL returns the web application URL for email links
func GetWebURL() string {
	// Try to get from environment variable first
	if webURL := os.Getenv("WEB_URL"); webURL != "" {
		return webURL
	}
	// Default to localhost for development
	return "http://192.168.50.157:8000/"
}
