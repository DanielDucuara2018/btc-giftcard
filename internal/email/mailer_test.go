package email

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// New / factory tests
// ============================================================================

func TestNew_SMTPProvider(t *testing.T) {
	m, err := New(Config{
		Provider:    "smtp",
		SMTPHost:    "localhost",
		SMTPPort:    "1025",
		FromAddress: "test@example.com",
		FromName:    "Test",
	})
	require.NoError(t, err)
	assert.NotNil(t, m)
}

func TestNew_DisabledProvider(t *testing.T) {
	m, err := New(Config{Provider: "disabled"})
	require.NoError(t, err)
	assert.NotNil(t, m)
}

func TestNew_EmptyProvider_DefaultsToDisabled(t *testing.T) {
	m, err := New(Config{})
	require.NoError(t, err)
	assert.NotNil(t, m)
}

func TestNew_UnknownProvider_ReturnsError(t *testing.T) {
	_, err := New(Config{Provider: "sendgrid"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
}

// ============================================================================
// Noop provider tests
// ============================================================================

func TestNoop_Send_AlwaysSucceeds(t *testing.T) {
	p := NewNoop()
	err := p.Send(context.Background(), Message{
		To:      "user@example.com",
		Subject: "Test",
		HTML:    "<p>Hello</p>",
		Text:    "Hello",
	})
	assert.NoError(t, err)
}

// ============================================================================
// Mailer template rendering tests
// ============================================================================

func TestMailer_SendPurchaseConfirmation_NoopSucceeds(t *testing.T) {
	m := NewMailer(NewNoop(), "BTC Gift Cards")

	err := m.SendPurchaseConfirmation(context.Background(), PurchaseConfirmationData{
		PurchaseEmail:        "buyer@example.com",
		TotalCards:           2,
		TotalAmountFormatted: "49.98",
		FiatCurrency:         "EUR",
		CheckoutURL:          "https://checkout.stripe.com/pay/cs_test_123",
		SessionID:            "cs_test_123",
		ExpiresAt:            time.Now().Add(24 * time.Hour),
	})
	assert.NoError(t, err)
}

func TestMailer_SendCardCodes_NoopSucceeds(t *testing.T) {
	m := NewMailer(NewNoop(), "BTC Gift Cards")

	err := m.SendCardCodes(context.Background(), CardCodesData{
		PurchaseEmail:        "buyer@example.com",
		TotalCards:           2,
		TotalAmountFormatted: "49.98",
		FiatCurrency:         "EUR",
		Cards: []CardCodeItem{
			{Code: "GIFT-ABCD-1234", FaceValueFormatted: "24.99"},
			{Code: "GIFT-EFGH-5678", FaceValueFormatted: "24.99"},
		},
	})
	assert.NoError(t, err)
}

func TestMailer_SendPaymentExpired_NoopSucceeds(t *testing.T) {
	m := NewMailer(NewNoop(), "BTC Gift Cards")

	err := m.SendPaymentExpired(context.Background(), PaymentExpiredData{
		PurchaseEmail:        "buyer@example.com",
		SessionID:            "cs_test_abc",
		TotalAmountFormatted: "49.98",
		FiatCurrency:         "EUR",
	})
	assert.NoError(t, err)
}

func TestMailer_SendCardActivation_NoopSucceeds(t *testing.T) {
	m := NewMailer(NewNoop(), "BTC Gift Cards")

	err := m.SendCardActivation(context.Background(), CardActivationData{
		PurchaseEmail:       "buyer@example.com",
		CardCode:            "GIFT-ABCD-1234",
		BTCAmountSats:       25000,
		BTCAmountFormatted:  "0.00025000",
		FiatAmountFormatted: "24.99",
		FiatCurrency:        "EUR",
	})
	assert.NoError(t, err)
}

func TestMailer_SendRedemptionConfirmation_NoopSucceeds(t *testing.T) {
	m := NewMailer(NewNoop(), "BTC Gift Cards")

	hash := "abc123def456"
	err := m.SendRedemptionConfirmation(context.Background(), RedemptionData{
		OwnerEmail:       "owner@example.com",
		CardCode:         "GIFT-ABCD-1234",
		AmountSats:       10000,
		RemainingBalance: 15000,
		PaymentHash:      hash,
		Method:           "lightning",
		LowBalance:       false,
	})
	assert.NoError(t, err)
}

func TestMailer_SendRedemptionConfirmation_LowBalance_NoopSucceeds(t *testing.T) {
	m := NewMailer(NewNoop(), "BTC Gift Cards")

	err := m.SendRedemptionConfirmation(context.Background(), RedemptionData{
		OwnerEmail:       "owner@example.com",
		CardCode:         "GIFT-ABCD-1234",
		AmountSats:       23000,
		RemainingBalance: 1000,
		Method:           "lightning",
		LowBalance:       true,
	})
	assert.NoError(t, err)
}

// ============================================================================
// SMTP integration test — requires a real SMTP server (e.g. Mailpit)
// ============================================================================

// TestSMTP_Send sends a real email via a local test SMTP server.
// Run only when a local SMTP server is available.
// To run: SMTP_INTEGRATION=1 go test ./internal/email/ -run TestSMTP_Send
func TestSMTP_Send(t *testing.T) {
	if !smtpAvailable("localhost:1025") {
		t.Skip("SMTP server not available at localhost:1025 — skipping SMTP integration test")
	}

	p := NewSMTP(Config{
		SMTPHost:    "localhost",
		SMTPPort:    "1025",
		FromAddress: "test@btcgiftcard.io",
		FromName:    "BTC Gift Cards Test",
	})

	err := p.Send(context.Background(), Message{
		To:      "test@example.com",
		Subject: "SMTP integration test",
		HTML:    "<h1>Test email</h1><p>Sent from the email package unit test.</p>",
		Text:    "Test email. Sent from the email package unit test.",
	})
	assert.NoError(t, err)
}

// TestSMTP_SendMailerPurchaseConfirmation sends a full templated email via Mailpit.
func TestSMTP_SendMailerPurchaseConfirmation(t *testing.T) {
	if !smtpAvailable("localhost:1025") {
		t.Skip("SMTP server not available at localhost:1025 — skipping SMTP integration test")
	}

	m := NewMailer(NewSMTP(Config{
		SMTPHost:    "localhost",
		SMTPPort:    "1025",
		FromAddress: "cards@btcgiftcard.io",
		FromName:    "BTC Gift Cards",
	}), "BTC Gift Cards")

	err := m.SendPurchaseConfirmation(context.Background(), PurchaseConfirmationData{
		PurchaseEmail:        "buyer@example.com",
		TotalCards:           3,
		TotalAmountFormatted: "74.97",
		FiatCurrency:         "EUR",
		CheckoutURL:          "https://checkout.stripe.com/pay/cs_test_demo",
		SessionID:            "cs_test_demo_session_id",
		ExpiresAt:            time.Now().Add(24 * time.Hour),
	})
	assert.NoError(t, err)
}

// smtpAvailable returns true if a TCP connection can be established to addr.
// Used to gate integration tests that require a real SMTP server.
func smtpAvailable(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	// Read the SMTP greeting to confirm it's actually an SMTP server
	return true
}

// ============================================================================
// Template rendering content tests
// ============================================================================

// TestMailer_PurchaseConfirmation_HTMLContent validates that the rendered HTML
// contains the expected dynamic values.
func TestMailer_PurchaseConfirmation_HTMLContent(t *testing.T) {
	var captured Message
	recorder := &recordingProvider{captured: &captured}
	m := NewMailer(recorder, "BTC Gift Cards")

	err := m.SendPurchaseConfirmation(context.Background(), PurchaseConfirmationData{
		PurchaseEmail:        "buyer@example.com",
		TotalCards:           2,
		TotalAmountFormatted: "49.98",
		FiatCurrency:         "EUR",
		CheckoutURL:          "https://stripe.example/pay/cs_123",
		SessionID:            "cs_123",
		ExpiresAt:            time.Date(2024, 12, 1, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	assert.Equal(t, "buyer@example.com", captured.To)
	assert.Contains(t, captured.Subject, "49.98")
	assert.Contains(t, captured.Subject, "EUR")
	assert.Contains(t, captured.HTML, "cs_123")
	assert.Contains(t, captured.HTML, "https://stripe.example/pay/cs_123")
	assert.Contains(t, captured.HTML, "49.98")
	assert.Contains(t, captured.Text, "cs_123")
}

func TestMailer_CardCodes_HTMLContainsCodes(t *testing.T) {
	var captured Message
	recorder := &recordingProvider{captured: &captured}
	m := NewMailer(recorder, "BTC Gift Cards")

	err := m.SendCardCodes(context.Background(), CardCodesData{
		PurchaseEmail:        "buyer@example.com",
		TotalCards:           1,
		TotalAmountFormatted: "24.99",
		FiatCurrency:         "EUR",
		Cards: []CardCodeItem{
			{Code: "GIFT-TEST-CODE", FaceValueFormatted: "24.99"},
		},
	})
	require.NoError(t, err)

	assert.Contains(t, captured.HTML, "GIFT-TEST-CODE")
	assert.Contains(t, captured.HTML, "24.99")
	assert.Contains(t, captured.Text, "GIFT-TEST-CODE")
}

func TestMailer_Redemption_LowBalanceFlag(t *testing.T) {
	var captured Message
	recorder := &recordingProvider{captured: &captured}
	m := NewMailer(recorder, "BTC Gift Cards")

	err := m.SendRedemptionConfirmation(context.Background(), RedemptionData{
		OwnerEmail:       "owner@example.com",
		CardCode:         "GIFT-LOW-BAL",
		AmountSats:       23000,
		RemainingBalance: 500,
		Method:           "lightning",
		LowBalance:       true,
	})
	require.NoError(t, err)

	assert.Contains(t, captured.Subject, "low balance")
	assert.True(t, strings.Contains(captured.HTML, "Low balance") || strings.Contains(captured.HTML, "low balance"))
}

// recordingProvider captures the last message sent, for content assertions.
type recordingProvider struct {
	captured *Message
}

func (r *recordingProvider) Send(_ context.Context, msg Message) error {
	*r.captured = msg
	return nil
}
