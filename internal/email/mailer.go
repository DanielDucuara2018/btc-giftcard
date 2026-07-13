package email

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"time"

	"btc-giftcard/pkg/logger"

	"go.uber.org/zap"
)

//go:embed templates/*.html
var templatesFS embed.FS

// Mailer renders email templates and delivers them via a Provider.
//
// Create one at startup with New or NewMailer and inject it into services
// that need to send transactional emails.
//
// All Send* methods return an error — callers (e.g. card.Service) are expected
// to wrap each call in a goroutine and log errors without propagating them:
//
//	go func() {
//	    if err := mailer.SendPurchaseConfirmation(context.Background(), data); err != nil {
//	        logger.Error("email failed", zap.Error(err))
//	    }
//	}()
type Mailer struct {
	provider  Provider
	templates map[string]*template.Template
	from      string // sender display name
}

// New creates a Mailer from cfg.
// Provider is selected based on cfg.Provider:
//   - "smtp"     → SMTP client (Mailpit locally, AWS SES SMTP in production)
//   - "disabled" → no-op (messages silently dropped)
//   - any other  → error
func New(cfg Config) (*Mailer, error) {
	var p Provider
	switch cfg.Provider {
	case "smtp":
		p = NewSMTP(cfg)
	case "disabled", "":
		p = NewNoop()
	default:
		return nil, fmt.Errorf("email: unknown provider %q (valid: smtp, disabled)", cfg.Provider)
	}
	return NewMailer(p, cfg.FromName), nil
}

// NewMailer creates a Mailer backed by the given Provider.
// fromName is the display name used in the "From" header.
// Use this for tests (pass NewNoop()) or when constructing with a custom provider.
func NewMailer(provider Provider, fromName string) *Mailer {
	return &Mailer{
		provider:  provider,
		templates: parseTemplates(),
		from:      fromName,
	}
}

// parseTemplates pre-parses all email templates from the embedded FS.
// Each template file is a standalone HTML document.
// Panics on parse error — template files are embedded at compile time,
// so a parse error is a programming mistake, not a runtime condition.
func parseTemplates() map[string]*template.Template {
	names := []string{
		"purchase_confirmation",
		"card_codes",
		"payment_expired",
		"card_activation",
		"redemption_confirmation",
	}
	tmpl := make(map[string]*template.Template, len(names))
	for _, name := range names {
		path := "templates/" + name + ".html"
		t, err := template.New(name).ParseFS(templatesFS, path)
		if err != nil {
			panic(fmt.Sprintf("email: failed to parse template %s: %v", path, err))
		}
		tmpl[name] = t
	}
	return tmpl
}

// render executes the named template with data and returns the HTML string.
func (m *Mailer) render(name string, data any) (string, error) {
	t, ok := m.templates[name]
	if !ok {
		return "", fmt.Errorf("email: unknown template %q", name)
	}
	var buf bytes.Buffer
	// Templates are parsed from files; the template name within the set is the
	// base filename (e.g. "purchase_confirmation.html"), not the short name.
	if err := t.ExecuteTemplate(&buf, name+".html", data); err != nil {
		err = fmt.Errorf("email: failed to render template %q: %w", name, err)
		logger.Error("email: template render failed",
			zap.String("template", name),
			zap.Error(err),
		)
		return "", err
	}
	return buf.String(), nil
}

// ============================================================================
// Typed data structs
// ============================================================================

// PurchaseConfirmationData holds data for the purchase confirmation email,
// sent immediately after POST /api/cards (payment still pending at this point).
type PurchaseConfirmationData struct {
	PurchaseEmail        string
	TotalCards           int
	TotalAmountFormatted string // e.g. "49.99"
	FiatCurrency         string // e.g. "EUR"
	CheckoutURL          string
	SessionID            string
	ExpiresAt            time.Time
	ExpiresAtFormatted   string // computed by SendPurchaseConfirmation
}

// CardCodeItem is one card within a CardCodesData batch.
type CardCodeItem struct {
	Code               string
	FaceValueFormatted string // e.g. "25.00"
}

// CardCodesData holds data for the card codes delivery email,
// sent when checkout.session.completed fires and payment_status is set to paid.
type CardCodesData struct {
	PurchaseEmail        string
	TotalCards           int
	TotalAmountFormatted string
	FiatCurrency         string
	Cards                []CardCodeItem
}

// PaymentExpiredData holds data for the payment expired email,
// sent when checkout.session.expired fires.
type PaymentExpiredData struct {
	PurchaseEmail        string
	SessionID            string
	TotalAmountFormatted string
	FiatCurrency         string
}

// CardActivationData holds data for the card activation email,
// sent after FundCard sets status = active.
type CardActivationData struct {
	PurchaseEmail       string
	CardCode            string
	BTCAmountSats       int64
	BTCAmountFormatted  string // e.g. "0.00025000"
	FiatAmountFormatted string
	FiatCurrency        string
}

// RedemptionData holds data for the redemption confirmation email,
// sent after a successful RedeemCard.
type RedemptionData struct {
	OwnerEmail       string
	CardCode         string
	AmountSats       int64
	RemainingBalance int64
	PaymentHash      string // empty string if not applicable
	TxHash           string // empty string if not applicable
	Method           string // "lightning" | "onchain"
	LowBalance       bool   // true when remaining < 10% of original
}

// ============================================================================
// Typed send methods
// ============================================================================

// SendPurchaseConfirmation sends an order confirmation to the buyer immediately
// after a card order is placed (payment is still pending at this point).
func (m *Mailer) SendPurchaseConfirmation(ctx context.Context, data PurchaseConfirmationData) error {
	data.ExpiresAtFormatted = data.ExpiresAt.UTC().Format("Jan 2, 2006 15:04 UTC")

	html, err := m.render("purchase_confirmation", data)
	if err != nil {
		return err
	}
	plain := fmt.Sprintf(
		"Your BTC Gift Card order has been received.\n\n"+
			"Cards: %d\nTotal: %s %s\nSession: %s\nExpires: %s\n\n"+
			"Complete your payment: %s",
		data.TotalCards, data.TotalAmountFormatted, data.FiatCurrency,
		data.SessionID, data.ExpiresAtFormatted, data.CheckoutURL,
	)
	return m.provider.Send(ctx, Message{
		To:      data.PurchaseEmail,
		Subject: fmt.Sprintf("Your BTC Gift Card order — %s %s", data.TotalAmountFormatted, data.FiatCurrency),
		HTML:    html,
		Text:    plain,
	})
}

// SendCardCodes sends the card redemption codes once payment is confirmed.
func (m *Mailer) SendCardCodes(ctx context.Context, data CardCodesData) error {
	html, err := m.render("card_codes", data)
	if err != nil {
		return err
	}

	codeLines := ""
	for _, c := range data.Cards {
		codeLines += fmt.Sprintf("  - %s (%s %s)\n", c.Code, c.FaceValueFormatted, data.FiatCurrency)
	}
	plain := fmt.Sprintf(
		"Your payment was confirmed. Here are your Bitcoin Gift Card codes:\n\n%s\n"+
			"Keep these codes safe — they are equivalent to cash.\n",
		codeLines,
	)
	return m.provider.Send(ctx, Message{
		To:      data.PurchaseEmail,
		Subject: fmt.Sprintf("Your Bitcoin Gift Card%s — payment confirmed", pluralS(data.TotalCards)),
		HTML:    html,
		Text:    plain,
	})
}

// SendPaymentExpired notifies the buyer that their checkout session expired
// and no charge was made.
func (m *Mailer) SendPaymentExpired(ctx context.Context, data PaymentExpiredData) error {
	html, err := m.render("payment_expired", data)
	if err != nil {
		return err
	}
	plain := fmt.Sprintf(
		"Your payment session for %s %s (session: %s) has expired.\n\n"+
			"No charge was made. You can place a new order at any time.",
		data.TotalAmountFormatted, data.FiatCurrency, data.SessionID,
	)
	return m.provider.Send(ctx, Message{
		To:      data.PurchaseEmail,
		Subject: "Your BTC Gift Card payment session has expired",
		HTML:    html,
		Text:    plain,
	})
}

// SendCardActivation notifies the buyer that a card has been funded with BTC
// and is now ready to use.
func (m *Mailer) SendCardActivation(ctx context.Context, data CardActivationData) error {
	html, err := m.render("card_activation", data)
	if err != nil {
		return err
	}
	plain := fmt.Sprintf(
		"Your Bitcoin Gift Card is now active!\n\n"+
			"Code:    %s\nBalance: %d sats (%s BTC)\nFace value: %s %s\n\n"+
			"Keep this code safe — it is equivalent to cash.",
		data.CardCode, data.BTCAmountSats, data.BTCAmountFormatted,
		data.FiatAmountFormatted, data.FiatCurrency,
	)
	return m.provider.Send(ctx, Message{
		To:      data.PurchaseEmail,
		Subject: fmt.Sprintf("Your Bitcoin Gift Card is active — %d sats", data.BTCAmountSats),
		HTML:    html,
		Text:    plain,
	})
}

// SendRedemptionConfirmation notifies the card owner after a successful spend.
func (m *Mailer) SendRedemptionConfirmation(ctx context.Context, data RedemptionData) error {
	html, err := m.render("redemption_confirmation", data)
	if err != nil {
		return err
	}
	subject := fmt.Sprintf("Bitcoin sent — %d sats from your gift card", data.AmountSats)
	if data.LowBalance {
		subject = fmt.Sprintf("Bitcoin sent — low balance warning (%d sats remaining)", data.RemainingBalance)
	}
	plain := fmt.Sprintf(
		"Your Bitcoin Gift Card redemption was successful.\n\n"+
			"Card:      %s\nSent:      %d sats\nRemaining: %d sats\nMethod:    %s\n",
		data.CardCode, data.AmountSats, data.RemainingBalance, data.Method,
	)
	if data.PaymentHash != "" {
		plain += fmt.Sprintf("Payment hash: %s\n", data.PaymentHash)
	}
	if data.TxHash != "" {
		plain += fmt.Sprintf("Transaction: %s\n", data.TxHash)
	}
	return m.provider.Send(ctx, Message{
		To:      data.OwnerEmail,
		Subject: subject,
		HTML:    html,
		Text:    plain,
	})
}

// pluralS returns "s" when n != 1 (for English pluralisation).
func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
