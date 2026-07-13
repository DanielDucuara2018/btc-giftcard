package email

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"time"

	"btc-giftcard/pkg/logger"

	"go.uber.org/zap"
)

// smtpProvider sends email via any SMTP server.
//
// Local dev (Mailpit): no auth, plain connection on port 1025.
// Production (AWS SES): PLAIN auth + STARTTLS on port 587.
type smtpProvider struct {
	cfg Config
}

// NewSMTP creates an SMTP Provider from cfg.
// The connection is established lazily on each Send call — no persistent
// connection is maintained, which keeps the implementation simple and avoids
// idle-timeout issues with cloud SMTP relays.
func NewSMTP(cfg Config) Provider {
	return &smtpProvider{cfg: cfg}
}

// Send dials the configured SMTP server, authenticates if credentials are
// provided, and delivers msg.
//
// STARTTLS is negotiated automatically if the server advertises it.
// For Mailpit (no TLS, no auth) this degrades gracefully.
func (s *smtpProvider) Send(ctx context.Context, msg Message) error {
	addr := net.JoinHostPort(s.cfg.SMTPHost, s.cfg.SMTPPort)

	logger.Info("email: attempting send",
		zap.String("to", msg.To),
		zap.String("subject", msg.Subject),
		zap.String("smtp_host", s.cfg.SMTPHost),
		zap.String("smtp_port", s.cfg.SMTPPort),
	)

	// Build auth only when credentials are provided.
	// Mailpit accepts anonymous connections (MP_SMTP_AUTH_ACCEPT_ANY=1),
	// so passing nil auth is correct for local dev.
	var auth smtp.Auth
	if s.cfg.SMTPUser != "" {
		auth = smtp.PlainAuth("", s.cfg.SMTPUser, s.cfg.SMTPPass, s.cfg.SMTPHost)
	}

	raw := buildMIMEMessage(s.cfg.FromAddress, s.cfg.FromName, msg)

	// smtp.SendMail handles the full handshake: EHLO, optional STARTTLS,
	// optional AUTH, DATA, QUIT.
	if err := smtp.SendMail(addr, auth, s.cfg.FromAddress, []string{msg.To}, raw); err != nil {
		logger.Error("email: send failed",
			zap.String("to", msg.To),
			zap.String("subject", msg.Subject),
			zap.Error(err),
		)
		return fmt.Errorf("smtp: failed to send email to %s: %w", msg.To, err)
	}

	logger.Info("email: sent successfully",
		zap.String("to", msg.To),
		zap.String("subject", msg.Subject),
	)
	return nil
}

// buildMIMEMessage constructs a MIME multipart/alternative message with a
// plain-text part and an HTML part so that all email clients can render it.
func buildMIMEMessage(fromAddr, fromName string, msg Message) []byte {
	boundary := fmt.Sprintf("==btcgc_%d==", time.Now().UnixNano())

	var buf bytes.Buffer

	// Headers
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")
	if fromName != "" {
		fmt.Fprintf(&buf, "From: %s <%s>\r\n",
			mime.QEncoding.Encode("utf-8", fromName), fromAddr)
	} else {
		fmt.Fprintf(&buf, "From: %s\r\n", fromAddr)
	}
	fmt.Fprintf(&buf, "To: %s\r\n", msg.To)
	fmt.Fprintf(&buf, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", msg.Subject))
	fmt.Fprintf(&buf, "Content-Type: multipart/alternative; boundary=%q\r\n", boundary)
	fmt.Fprintf(&buf, "\r\n")

	// Plain-text part (rendered first; email clients show the last matching part)
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	fmt.Fprintf(&buf, "Content-Type: text/plain; charset=utf-8\r\n")
	fmt.Fprintf(&buf, "\r\n")
	fmt.Fprintf(&buf, "%s\r\n", msg.Text)

	// HTML part
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	fmt.Fprintf(&buf, "Content-Type: text/html; charset=utf-8\r\n")
	fmt.Fprintf(&buf, "\r\n")
	fmt.Fprintf(&buf, "%s\r\n", msg.HTML)

	// Closing boundary
	fmt.Fprintf(&buf, "--%s--\r\n", boundary)

	return buf.Bytes()
}
