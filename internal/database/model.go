package database

import (
	"time"
)

// Define a new type for the enum
type CardStatus int
type Type int
type TransactionStatus int

// Define the constants using iota
const (
	Created CardStatus = iota
	Funding
	Active
	Redeemed
	Expired
)

const (
	Fund Type = iota
	Redeem
	Payment
)

const (
	Pending TransactionStatus = iota
	Confirmed
	Failed
)

// String converts Status to database string value
// This method is called automatically by fmt.Print, JSON marshaling, etc.
func (s CardStatus) String() string {
	switch s {
	case Created:
		return "created"
	case Funding:
		return "funding"
	case Active:
		return "active"
	case Redeemed:
		return "redeemed"
	case Expired:
		return "expired"
	default:
		return "unknown"
	}
}

func (s Type) String() string {
	switch s {
	case Fund:
		return "fund"
	case Redeem:
		return "redeem"
	case Payment:
		return "payment"
	default:
		return "unknown"
	}
}

func (s TransactionStatus) String() string {
	switch s {
	case Pending:
		return "pending"
	case Confirmed:
		return "confirmed"
	case Failed:
		return "failed"
	default:
		return "unknown"
	}
}

// ParseStatus converts database string to Status enum
// Use this when reading from database or API
func ParseCardStatus(s string) CardStatus {
	switch s {
	case "created":
		return Created
	case "funding":
		return Funding
	case "active":
		return Active
	case "redeemed":
		return Redeemed
	case "expired":
		return Expired
	default:
		return Created // Default to Created if unknown
	}
}

func ParseTransactionType(s string) Type {
	switch s {
	case "fund":
		return Fund
	case "redeem":
		return Redeem
	case "payment":
		return Payment
	default:
		return Fund // Default to Fund if unknown
	}
}

func ParseTransactionStatus(s string) TransactionStatus {
	switch s {
	case "pending":
		return Pending
	case "confirmed":
		return Confirmed
	case "failed":
		return Failed
	default:
		return Pending // Default to Pending if unknown
	}
}

type Card struct {
	ID                 string     `json:"id" db:"id"`
	UserID             *string    `json:"user_id,omitempty" db:"user_id"`
	PurchaseEmail      string     `json:"purchase_email" db:"purchase_email"`
	OwnerEmail         string     `json:"owner_email" db:"owner_email"`
	Code               string     `json:"code" db:"code"`
	BTCAmountSats      int64      `json:"btc_amount_sats" db:"btc_amount_sats"`     // Satoshis (1 BTC = 100,000,000 sats)
	FiatAmountCents    int64      `json:"fiat_amount_cents" db:"fiat_amount_cents"` // Cents (e.g., $100.50 = 10050)
	FiatCurrency       string     `json:"fiat_currency" db:"fiat_currency"`
	PurchasePriceCents int64      `json:"purchase_price_cents" db:"purchase_price_cents"` // Total charged in cents
	Status             CardStatus `json:"status" db:"status"`
	CreatedAt          time.Time  `json:"created_at" db:"created_at"`
	RedeemedAt         *time.Time `json:"redeemed_at,omitempty" db:"redeemed_at"`
	FundedAt           *time.Time `json:"funded_at,omitempty" db:"funded_at"`
}

// GetBTC returns BTC amount as float64 for display (e.g., 0.00152345)
func (c *Card) GetBTC() float64 {
	return float64(c.BTCAmountSats) / 100_000_000
}

// GetFiatAmount returns fiat amount as float64 for display (e.g., 100.50)
func (c *Card) GetFiatAmount() float64 {
	return float64(c.FiatAmountCents) / 100
}

// GetPurchasePrice returns purchase price as float64 for display (e.g., 103.00)
func (c *Card) GetPurchasePrice() float64 {
	return float64(c.PurchasePriceCents) / 100
}

type Transaction struct {
	ID               string            `json:"id" db:"id"`
	CardID           string            `json:"card_id" db:"card_id"`
	Type             Type              `json:"type" db:"type"`
	RedemptionMethod *string           `json:"redemption_method,omitempty" db:"redemption_method"` // 'lightning' or 'onchain'
	TxHash           *string           `json:"tx_hash,omitempty" db:"tx_hash"`                     // On-chain tx hash (NULL for Lightning)
	PaymentHash      *string           `json:"payment_hash,omitempty" db:"payment_hash"`           // Lightning payment hash (NULL for on-chain)
	PaymentPreimage  *string           `json:"payment_preimage,omitempty" db:"payment_preimage"`   // Lightning proof of payment (set on success)
	LightningInvoice *string           `json:"lightning_invoice,omitempty" db:"lightning_invoice"` // BOLT11 invoice (NULL for on-chain)
	FromAddress      *string           `json:"from_address,omitempty" db:"from_address"`           // Source Bitcoin address (on-chain)
	ToAddress        *string           `json:"to_address,omitempty" db:"to_address"`               // Destination Bitcoin address (on-chain)
	BTCAmountSats    int64             `json:"btc_amount_sats" db:"btc_amount_sats"`               // Satoshis
	Status           TransactionStatus `json:"status" db:"status"`
	Confirmations    int               `json:"confirmations" db:"confirmations"`
	CreatedAt        time.Time         `json:"created_at" db:"created_at"`
	BroadcastAt      *time.Time        `json:"broadcast_at,omitempty" db:"broadcast_at"` // When sent to blockchain
	ConfirmedAt      *time.Time        `json:"confirmed_at,omitempty" db:"confirmed_at"` // When confirmed
}

// GetBTC returns BTC amount as float64 for display (e.g., 0.00152345)
func (t *Transaction) GetBTC() float64 {
	return float64(t.BTCAmountSats) / 100_000_000
}
