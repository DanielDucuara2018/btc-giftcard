package queue

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

// FundCardMessage represents a request to fund a gift card with BTC
type FundCardMessage struct {
	CardID          string `json:"card_id"`
	FiatAmountCents int64  `json:"fiat_amount_cents"`
	FiatCurrency    string `json:"fiat_currency"`
}

// ToJSON serializes the FundCardMessage to JSON bytes.
func (m *FundCardMessage) ToJSON() ([]byte, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal fund card message: %w", err)
	}
	return data, nil
}

// FromJSONFundCard deserializes JSON bytes into a FundCardMessage and validates it.
func FromJSONFundCard(data []byte) (*FundCardMessage, error) {
	msg := &FundCardMessage{}
	if err := json.Unmarshal(data, msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal fund card message: %w", err)
	}

	if err := msg.Validate(); err != nil {
		return nil, err
	}

	return msg, nil
}

// Validate checks if the FundCardMessage has all required fields with valid values.
func (m *FundCardMessage) Validate() error {
	if m.CardID == "" {
		return errors.New("card_id is required")
	}
	if m.FiatAmountCents <= 0 {
		return errors.New("fiat_amount_cents must be greater than 0")
	}
	if m.FiatCurrency == "" {
		return errors.New("fiat_currency is required")
	}
	if len(m.FiatCurrency) != 3 {
		return fmt.Errorf("fiat_currency must be 3 characters (got %q)", m.FiatCurrency)
	}
	return nil
}

// MonitorTransactionMessage represents a request to monitor a BTC transaction
type MonitorTransactionMessage struct {
	CardID             string `json:"card_id"`
	TxHash             string `json:"tx_hash"`
	ExpectedAmountSats int64  `json:"expected_amount_sats"`
	DestinationAddr    string `json:"destination_addr"`
}

// ToJSON serializes the MonitorTransactionMessage to JSON bytes.
func (m *MonitorTransactionMessage) ToJSON() ([]byte, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal monitor transaction message: %w", err)
	}
	return data, nil
}

// FromJSONMonitorTx deserializes JSON bytes into a MonitorTransactionMessage and validates it.
func FromJSONMonitorTx(data []byte) (*MonitorTransactionMessage, error) {
	msg := &MonitorTransactionMessage{}
	if err := json.Unmarshal(data, msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal monitor transaction message: %w", err)
	}

	if err := msg.Validate(); err != nil {
		return nil, err
	}

	return msg, nil
}

// Validate checks if the MonitorTransactionMessage has all required fields with valid values.
func (m *MonitorTransactionMessage) Validate() error {
	if m.CardID == "" {
		return errors.New("card_id is required")
	}
	if m.TxHash == "" {
		return errors.New("tx_hash is required")
	}
	if len(m.TxHash) != 64 {
		return fmt.Errorf("tx_hash must be 64 characters (got %d)", len(m.TxHash))
	}
	if _, err := hex.DecodeString(m.TxHash); err != nil {
		return fmt.Errorf("tx_hash must be valid hexadecimal: %w", err)
	}
	if m.ExpectedAmountSats <= 0 {
		return errors.New("expected_amount_sats must be greater than 0")
	}
	if m.DestinationAddr == "" {
		return errors.New("destination_addr is required")
	}
	return nil
}
