package utils

import (
	"fmt"
	"net/smtp"
	"strings"

	"bsync-server/config"
)

// SendEmail mengirim email sederhana (plain text)
func SendEmail(to []string, subject, body string) error {
	cfg := config.LoadSMTPConfig()

	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)

	// Header email
	header := make(map[string]string)
	header["From"] = cfg.From
	header["To"] = strings.Join(to, ", ")
	header["Subject"] = subject
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = `text/plain; charset="UTF-8"`

	// Susun pesan
	var msg string
	for k, v := range header {
		msg += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	msg += "\r\n" + body

	// Kirim
	addr := cfg.Host + ":" + cfg.Port
	err := smtp.SendMail(addr, auth, cfg.From, to, []byte(msg))
	if err != nil {
		return fmt.Errorf("gagal mengirim email: %v", err)
	}

	return nil
}

// SendHTMLEmail mengirim email HTML
func SendHTMLEmail(to []string, subject, htmlBody string) error {
	cfg := config.LoadSMTPConfig()

	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)

	header := make(map[string]string)
	header["From"] = cfg.From
	header["To"] = strings.Join(to, ", ")
	header["Subject"] = subject
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = `text/html; charset="UTF-8"`

	var msg string
	for k, v := range header {
		msg += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	msg += "\r\n" + htmlBody

	addr := cfg.Host + ":" + cfg.Port
	err := smtp.SendMail(addr, auth, cfg.From, to, []byte(msg))
	if err != nil {
		return fmt.Errorf("gagal mengirim email HTML: %v", err)
	}

	return nil
}
