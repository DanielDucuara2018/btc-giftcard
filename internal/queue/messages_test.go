package queue

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// FundCardMessage Tests
// =============================================================================

func TestFundCardMessage_ToJSON(t *testing.T) {
	msg := &FundCardMessage{
		CardID:          "550e8400-e29b-41d4-a716-446655440000",
		FiatAmountCents: 5000,
		FiatCurrency:    "USD",
	}

	data, err := msg.ToJSON()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Verify it's valid JSON
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", result["card_id"])
	assert.Equal(t, float64(5000), result["fiat_amount_cents"])
	assert.Equal(t, "USD", result["fiat_currency"])
}

func TestFromJSONFundCard_Success(t *testing.T) {
	jsonData := []byte(`{
		"card_id": "550e8400-e29b-41d4-a716-446655440000",
		"fiat_amount_cents": 10000,
		"fiat_currency": "EUR"
	}`)

	msg, err := FromJSONFundCard(jsonData)
	require.NoError(t, err)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", msg.CardID)
	assert.Equal(t, int64(10000), msg.FiatAmountCents)
	assert.Equal(t, "EUR", msg.FiatCurrency)
}

func TestFromJSONFundCard_InvalidJSON(t *testing.T) {
	jsonData := []byte(`invalid json`)

	msg, err := FromJSONFundCard(jsonData)
	assert.Error(t, err)
	assert.Nil(t, msg)
	assert.Contains(t, err.Error(), "failed to unmarshal")
}

func TestFromJSONFundCard_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		jsonData    string
		expectError string
	}{
		{
			name:        "Missing card_id",
			jsonData:    `{"fiat_amount_cents": 5000, "fiat_currency": "USD"}`,
			expectError: "card_id is required",
		},
		{
			name:        "Missing fiat_currency",
			jsonData:    `{"card_id": "123", "fiat_amount_cents": 5000}`,
			expectError: "fiat_currency is required",
		},
		{
			name:        "Zero amount",
			jsonData:    `{"card_id": "123", "fiat_amount_cents": 0, "fiat_currency": "USD"}`,
			expectError: "fiat_amount_cents must be greater than 0",
		},
		{
			name:        "Negative amount",
			jsonData:    `{"card_id": "123", "fiat_amount_cents": -100, "fiat_currency": "USD"}`,
			expectError: "fiat_amount_cents must be greater than 0",
		},
		{
			name:        "Invalid currency length",
			jsonData:    `{"card_id": "123", "fiat_amount_cents": 5000, "fiat_currency": "US"}`,
			expectError: "fiat_currency must be 3 characters",
		},
		{
			name:        "Currency too long",
			jsonData:    `{"card_id": "123", "fiat_amount_cents": 5000, "fiat_currency": "USDD"}`,
			expectError: "fiat_currency must be 3 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := FromJSONFundCard([]byte(tt.jsonData))
			assert.Error(t, err)
			assert.Nil(t, msg)
			assert.Contains(t, err.Error(), tt.expectError)
		})
	}
}

func TestFundCardMessage_RoundTrip(t *testing.T) {
	original := &FundCardMessage{
		CardID:          "550e8400-e29b-41d4-a716-446655440000",
		FiatAmountCents: 7500,
		FiatCurrency:    "GBP",
	}

	// Serialize
	data, err := original.ToJSON()
	require.NoError(t, err)

	// Deserialize
	msg, err := FromJSONFundCard(data)
	require.NoError(t, err)

	// Compare
	assert.Equal(t, original.CardID, msg.CardID)
	assert.Equal(t, original.FiatAmountCents, msg.FiatAmountCents)
	assert.Equal(t, original.FiatCurrency, msg.FiatCurrency)
}

func TestFundCardMessage_Validate(t *testing.T) {
	tests := []struct {
		name        string
		msg         *FundCardMessage
		expectError bool
		errorText   string
	}{
		{
			name: "Valid message",
			msg: &FundCardMessage{
				CardID:          "123",
				FiatAmountCents: 1000,
				FiatCurrency:    "USD",
			},
			expectError: false,
		},
		{
			name: "Empty card_id",
			msg: &FundCardMessage{
				CardID:          "",
				FiatAmountCents: 1000,
				FiatCurrency:    "USD",
			},
			expectError: true,
			errorText:   "card_id is required",
		},
		{
			name: "Zero amount",
			msg: &FundCardMessage{
				CardID:          "123",
				FiatAmountCents: 0,
				FiatCurrency:    "USD",
			},
			expectError: true,
			errorText:   "fiat_amount_cents must be greater than 0",
		},
		{
			name: "Negative amount",
			msg: &FundCardMessage{
				CardID:          "123",
				FiatAmountCents: -500,
				FiatCurrency:    "USD",
			},
			expectError: true,
			errorText:   "fiat_amount_cents must be greater than 0",
		},
		{
			name: "Empty currency",
			msg: &FundCardMessage{
				CardID:          "123",
				FiatAmountCents: 1000,
				FiatCurrency:    "",
			},
			expectError: true,
			errorText:   "fiat_currency is required",
		},
		{
			name: "Invalid currency length",
			msg: &FundCardMessage{
				CardID:          "123",
				FiatAmountCents: 1000,
				FiatCurrency:    "US",
			},
			expectError: true,
			errorText:   "fiat_currency must be 3 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorText)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// MonitorTransactionMessage Tests
// =============================================================================

func TestMonitorTransactionMessage_ToJSON(t *testing.T) {
	msg := &MonitorTransactionMessage{
		CardID:             "550e8400-e29b-41d4-a716-446655440000",
		TxHash:             "abc123def456789012345678901234567890123456789012345678901234abcd",
		ExpectedAmountSats: 74627,
		DestinationAddr:    "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh",
	}

	data, err := msg.ToJSON()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Verify it's valid JSON
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", result["card_id"])
	assert.Equal(t, "abc123def456789012345678901234567890123456789012345678901234abcd", result["tx_hash"])
	assert.Equal(t, float64(74627), result["expected_amount_sats"])
	assert.Equal(t, "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh", result["destination_addr"])
}

func TestFromJSONMonitorTx_Success(t *testing.T) {
	jsonData := []byte(`{
		"card_id": "550e8400-e29b-41d4-a716-446655440000",
		"tx_hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"expected_amount_sats": 100000,
		"destination_addr": "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh"
	}`)

	msg, err := FromJSONMonitorTx(jsonData)
	require.NoError(t, err)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", msg.CardID)
	assert.Equal(t, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", msg.TxHash)
	assert.Equal(t, int64(100000), msg.ExpectedAmountSats)
	assert.Equal(t, "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh", msg.DestinationAddr)
}

func TestFromJSONMonitorTx_InvalidJSON(t *testing.T) {
	jsonData := []byte(`invalid json`)

	msg, err := FromJSONMonitorTx(jsonData)
	assert.Error(t, err)
	assert.Nil(t, msg)
	assert.Contains(t, err.Error(), "failed to unmarshal")
}

func TestFromJSONMonitorTx_ValidationErrors(t *testing.T) {
	validTxHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	tests := []struct {
		name        string
		jsonData    string
		expectError string
	}{
		{
			name: "Missing card_id",
			jsonData: `{
				"tx_hash": "` + validTxHash + `",
				"expected_amount_sats": 100000,
				"destination_addr": "bc1q..."
			}`,
			expectError: "card_id is required",
		},
		{
			name: "Missing tx_hash",
			jsonData: `{
				"card_id": "123",
				"expected_amount_sats": 100000,
				"destination_addr": "bc1q..."
			}`,
			expectError: "tx_hash is required",
		},
		{
			name: "Invalid tx_hash length",
			jsonData: `{
				"card_id": "123",
				"tx_hash": "abc123",
				"expected_amount_sats": 100000,
				"destination_addr": "bc1q..."
			}`,
			expectError: "tx_hash must be 64 characters",
		},
		{
			name: "Invalid tx_hash format (non-hex)",
			jsonData: `{
				"card_id": "123",
				"tx_hash": "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ",
				"expected_amount_sats": 100000,
				"destination_addr": "bc1q..."
			}`,
			expectError: "tx_hash must be valid hexadecimal",
		},
		{
			name: "Zero amount",
			jsonData: `{
				"card_id": "123",
				"tx_hash": "` + validTxHash + `",
				"expected_amount_sats": 0,
				"destination_addr": "bc1q..."
			}`,
			expectError: "expected_amount_sats must be greater than 0",
		},
		{
			name: "Negative amount",
			jsonData: `{
				"card_id": "123",
				"tx_hash": "` + validTxHash + `",
				"expected_amount_sats": -100,
				"destination_addr": "bc1q..."
			}`,
			expectError: "expected_amount_sats must be greater than 0",
		},
		{
			name: "Missing destination_addr",
			jsonData: `{
				"card_id": "123",
				"tx_hash": "` + validTxHash + `",
				"expected_amount_sats": 100000
			}`,
			expectError: "destination_addr is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := FromJSONMonitorTx([]byte(tt.jsonData))
			assert.Error(t, err)
			assert.Nil(t, msg)
			assert.Contains(t, err.Error(), tt.expectError)
		})
	}
}

func TestMonitorTransactionMessage_RoundTrip(t *testing.T) {
	original := &MonitorTransactionMessage{
		CardID:             "550e8400-e29b-41d4-a716-446655440000",
		TxHash:             "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		ExpectedAmountSats: 50000,
		DestinationAddr:    "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh",
	}

	// Serialize
	data, err := original.ToJSON()
	require.NoError(t, err)

	// Deserialize
	msg, err := FromJSONMonitorTx(data)
	require.NoError(t, err)

	// Compare
	assert.Equal(t, original.CardID, msg.CardID)
	assert.Equal(t, original.TxHash, msg.TxHash)
	assert.Equal(t, original.ExpectedAmountSats, msg.ExpectedAmountSats)
	assert.Equal(t, original.DestinationAddr, msg.DestinationAddr)
}

func TestMonitorTransactionMessage_Validate(t *testing.T) {
	validTxHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	tests := []struct {
		name        string
		msg         *MonitorTransactionMessage
		expectError bool
		errorText   string
	}{
		{
			name: "Valid message",
			msg: &MonitorTransactionMessage{
				CardID:             "123",
				TxHash:             validTxHash,
				ExpectedAmountSats: 100000,
				DestinationAddr:    "bc1q...",
			},
			expectError: false,
		},
		{
			name: "Empty card_id",
			msg: &MonitorTransactionMessage{
				CardID:             "",
				TxHash:             validTxHash,
				ExpectedAmountSats: 100000,
				DestinationAddr:    "bc1q...",
			},
			expectError: true,
			errorText:   "card_id is required",
		},
		{
			name: "Empty tx_hash",
			msg: &MonitorTransactionMessage{
				CardID:             "123",
				TxHash:             "",
				ExpectedAmountSats: 100000,
				DestinationAddr:    "bc1q...",
			},
			expectError: true,
			errorText:   "tx_hash is required",
		},
		{
			name: "Invalid tx_hash length",
			msg: &MonitorTransactionMessage{
				CardID:             "123",
				TxHash:             "abc123",
				ExpectedAmountSats: 100000,
				DestinationAddr:    "bc1q...",
			},
			expectError: true,
			errorText:   "tx_hash must be 64 characters",
		},
		{
			name: "Invalid tx_hash format",
			msg: &MonitorTransactionMessage{
				CardID:             "123",
				TxHash:             "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ",
				ExpectedAmountSats: 100000,
				DestinationAddr:    "bc1q...",
			},
			expectError: true,
			errorText:   "tx_hash must be valid hexadecimal",
		},
		{
			name: "Zero amount",
			msg: &MonitorTransactionMessage{
				CardID:             "123",
				TxHash:             validTxHash,
				ExpectedAmountSats: 0,
				DestinationAddr:    "bc1q...",
			},
			expectError: true,
			errorText:   "expected_amount_sats must be greater than 0",
		},
		{
			name: "Negative amount",
			msg: &MonitorTransactionMessage{
				CardID:             "123",
				TxHash:             validTxHash,
				ExpectedAmountSats: -500,
				DestinationAddr:    "bc1q...",
			},
			expectError: true,
			errorText:   "expected_amount_sats must be greater than 0",
		},
		{
			name: "Empty destination_addr",
			msg: &MonitorTransactionMessage{
				CardID:             "123",
				TxHash:             validTxHash,
				ExpectedAmountSats: 100000,
				DestinationAddr:    "",
			},
			expectError: true,
			errorText:   "destination_addr is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorText)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
