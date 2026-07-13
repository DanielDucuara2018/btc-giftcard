// Package email provides a swappable transactional email abstraction.
//
// The Provider interface is the single send primitive; all business-level email
// logic lives in Mailer, which wraps a Provider and renders HTML templates.
//
// Implementations:
//   - SMTP provider (smtp.go) — works with Mailpit locally and AWS SES in prod
//   - No-op provider (noop.go) — used in unit tests and when provider="disabled"
package email

import "context"

// Provider sends a single, pre-rendered email message.
// Implementations may use SMTP, an HTTP API (Resend, SendGrid), or a no-op
// stub for testing.
type Provider interface {
	Send(ctx context.Context, msg Message) error
}

// Message is the fully-rendered payload passed to a Provider.
type Message struct {
	To      string // recipient address
	Subject string
	HTML    string // rich HTML body
	Text    string // plain-text fallback (always set; some clients prefer it)
}

// Config holds all email provider settings, mirroring the [email] config
// section and GIFTER_EMAIL_* environment variables.
type Config struct {
	// Provider selects the sending backend: "smtp" | "disabled"
	// "disabled" silently drops all messages (useful for local dev without
	// Mailpit and for unit tests).
	Provider string

	// SMTP settings — used when Provider = "smtp".
	// For Mailpit (local dev): SMTPHost = "gift-card-backend.mail", Port = "1025", no auth.
	// For AWS SES (production): SMTPHost = "email-smtp.<region>.amazonaws.com", Port = "587",
	//   SMTPTLS = true, SMTPUser/SMTPPass from SES SMTP credentials.
	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	SMTPTLS  bool

	// FromAddress is the envelope and header From address.
	FromAddress string
	// FromName is the display name shown in the From header.
	FromName string
}
