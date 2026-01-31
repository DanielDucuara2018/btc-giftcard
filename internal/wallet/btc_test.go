package wallet

import (
	"strings"
	"testing"

	"btc-giftcard/pkg/logger"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// Initialize logger for tests
	_ = logger.Init("development")
}

// TestGenerateWalletMainnet tests wallet generation on mainnet
func TestGenerateWalletMainnet(t *testing.T) {
	wallet, err := GenerateWallet("mainnet")
	require.NoError(t, err, "GenerateWallet should succeed")
	require.NotNil(t, wallet, "Wallet should not be nil")

	// Verify network
	assert.Equal(t, "mainnet", wallet.Network)

	// Verify address format (mainnet SegWit starts with bc1)
	assert.True(t, strings.HasPrefix(wallet.Address, "bc1"), "Mainnet address should start with bc1")

	// Verify private key format (WIF should start with L or K for compressed mainnet keys)
	assert.True(t, strings.HasPrefix(wallet.PrivateKey, "L") || strings.HasPrefix(wallet.PrivateKey, "K"),
		"Mainnet WIF should start with L or K")

	// Verify public key exists and is hex
	assert.NotEmpty(t, wallet.PublicKey, "Public key should not be empty")

	// Verify address is valid
	_, err = btcutil.DecodeAddress(wallet.Address, &chaincfg.MainNetParams)
	if err != nil {
		t.Errorf("Generated invalid address: %v", err)
	}
}

// TestGenerateWalletTestnet tests wallet generation on testnet
func TestGenerateWalletTestnet(t *testing.T) {
	wallet, err := GenerateWallet("testnet")
	if err != nil {
		t.Fatalf("GenerateWallet failed: %v", err)
	}

	// Verify network
	if wallet.Network != "testnet" {
		t.Errorf("Expected network 'testnet', got '%s'", wallet.Network)
	}

	// Verify address format (testnet SegWit starts with tb1)
	if !strings.HasPrefix(wallet.Address, "tb1") {
		t.Errorf("Expected testnet address to start with 'tb1', got '%s'", wallet.Address)
	}

	// Verify private key format (testnet WIF starts with c)
	if !strings.HasPrefix(wallet.PrivateKey, "c") {
		t.Errorf("Expected testnet WIF to start with 'c', got '%s'", wallet.PrivateKey[:1])
	}

	// Verify address is valid
	_, err = btcutil.DecodeAddress(wallet.Address, &chaincfg.TestNet3Params)
	if err != nil {
		t.Errorf("Generated invalid address: %v", err)
	}
}

// TestGenerateWalletUniqueness tests that each wallet is unique
func TestGenerateWalletUniqueness(t *testing.T) {
	wallet1, _ := GenerateWallet("testnet")
	wallet2, _ := GenerateWallet("testnet")
	wallet3, _ := GenerateWallet("testnet")

	// All addresses should be different
	if wallet1.Address == wallet2.Address || wallet1.Address == wallet3.Address || wallet2.Address == wallet3.Address {
		t.Error("Generated wallets have duplicate addresses")
	}

	// All private keys should be different
	if wallet1.PrivateKey == wallet2.PrivateKey || wallet1.PrivateKey == wallet3.PrivateKey || wallet2.PrivateKey == wallet3.PrivateKey {
		t.Error("Generated wallets have duplicate private keys")
	}

	// All public keys should be different
	if string(wallet1.PublicKey) == string(wallet2.PublicKey) {
		t.Error("Generated wallets have duplicate public keys")
	}
}

// TestGenerateWalletInvalidNetwork tests error handling for invalid network
func TestGenerateWalletInvalidNetwork(t *testing.T) {
	testCases := []string{
		"invalid",
		"",
		"bitcoin",
		"MAINNET",
		"test",
	}

	for _, network := range testCases {
		wallet, err := GenerateWallet(network)
		if err == nil {
			t.Errorf("Expected error for network '%s', but got wallet: %v", network, wallet)
		}
		if !strings.Contains(err.Error(), "invalid network") {
			t.Errorf("Expected 'invalid network' error, got: %v", err)
		}
	}
}

// TestValidateAddressMainnet tests address validation on mainnet
func TestValidateAddressMainnet(t *testing.T) {
	testCases := []struct {
		name    string
		address string
		valid   bool
	}{
		// Valid mainnet addresses
		{"Valid SegWit", "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4", true},
		{"Valid Legacy", "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa", true},
		{"Valid P2SH", "3Cbq7aT1tY8kMxWLbitaG7yT6bPbKChq64", true}, // P2SH multisig address

		// Invalid addresses
		{"Empty", "", false},
		{"Invalid checksum", "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t5", false},
		{"Testnet on mainnet", "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", false},
		{"Random string", "not-a-bitcoin-address", false},
		{"Too short", "bc1q", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			valid, err := ValidateAddress(tc.address, "mainnet")
			if err != nil {
				t.Fatalf("ValidateAddress returned error: %v", err)
			}

			if valid != tc.valid {
				t.Errorf("Address '%s': expected valid=%v, got valid=%v", tc.address, tc.valid, valid)
			}
		})
	}
}

// TestValidateAddressTestnet tests address validation on testnet
func TestValidateAddressTestnet(t *testing.T) {
	testCases := []struct {
		name    string
		address string
		valid   bool
	}{
		// Valid testnet addresses
		{"Valid testnet SegWit", "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", true},
		{"Valid testnet legacy", "n1ZCYg9YXtB5XCZazLxSmPDa8iwJRZHhGx", true},

		// Invalid addresses
		{"Mainnet on testnet", "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4", false},
		{"Empty", "", false},
		{"Invalid", "invalid-address", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			valid, err := ValidateAddress(tc.address, "testnet")
			if err != nil {
				t.Fatalf("ValidateAddress returned error: %v", err)
			}

			if valid != tc.valid {
				t.Errorf("Address '%s': expected valid=%v, got valid=%v", tc.address, tc.valid, valid)
			}
		})
	}
}

// TestValidateAddressInvalidNetwork tests error handling for invalid network
func TestValidateAddressInvalidNetwork(t *testing.T) {
	_, err := ValidateAddress("bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4", "invalid")
	if err == nil {
		t.Error("Expected error for invalid network")
	}
	if !strings.Contains(err.Error(), "invalid network") {
		t.Errorf("Expected 'invalid network' error, got: %v", err)
	}
}

// TestGeneratedAddressValidation tests that generated addresses are valid
func TestGeneratedAddressValidation(t *testing.T) {
	// Generate mainnet wallet
	mainnetWallet, err := GenerateWallet("mainnet")
	if err != nil {
		t.Fatalf("GenerateWallet failed: %v", err)
	}

	// Validate generated mainnet address
	valid, err := ValidateAddress(mainnetWallet.Address, "mainnet")
	if err != nil {
		t.Fatalf("ValidateAddress failed: %v", err)
	}
	if !valid {
		t.Errorf("Generated mainnet address is invalid: %s", mainnetWallet.Address)
	}

	// Generate testnet wallet
	testnetWallet, err := GenerateWallet("testnet")
	if err != nil {
		t.Fatalf("GenerateWallet failed: %v", err)
	}

	// Validate generated testnet address
	valid, err = ValidateAddress(testnetWallet.Address, "testnet")
	if err != nil {
		t.Fatalf("ValidateAddress failed: %v", err)
	}
	if !valid {
		t.Errorf("Generated testnet address is invalid: %s", testnetWallet.Address)
	}
}

// TestWalletAddressLength tests address length constraints
func TestWalletAddressLength(t *testing.T) {
	wallet, err := GenerateWallet("mainnet")
	if err != nil {
		t.Fatalf("GenerateWallet failed: %v", err)
	}

	// SegWit addresses are typically 42-62 characters
	addrLen := len(wallet.Address)
	if addrLen < 42 || addrLen > 62 {
		t.Errorf("Address length %d is outside expected range (42-62)", addrLen)
	}
}

// TestWalletPrivateKeyLength tests private key length
func TestWalletPrivateKeyLength(t *testing.T) {
	wallet, err := GenerateWallet("mainnet")
	if err != nil {
		t.Fatalf("GenerateWallet failed: %v", err)
	}

	// WIF compressed private keys are 52 characters
	if len(wallet.PrivateKey) != 52 {
		t.Errorf("Expected WIF length 52, got %d", len(wallet.PrivateKey))
	}
}

// TestWalletPublicKeyLength tests public key length
func TestWalletPublicKeyLength(t *testing.T) {
	wallet, err := GenerateWallet("mainnet")
	if err != nil {
		t.Fatalf("GenerateWallet failed: %v", err)
	}

	// Compressed public key is 33 bytes
	if len(wallet.PublicKey) != 33 {
		t.Errorf("Expected public key length 33 bytes, got %d", len(wallet.PublicKey))
	}
}

// BenchmarkGenerateWallet benchmarks wallet generation
func BenchmarkGenerateWallet(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = GenerateWallet("testnet")
	}
}

// BenchmarkValidateAddress benchmarks address validation
func BenchmarkValidateAddress(b *testing.B) {
	address := "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ValidateAddress(address, "mainnet")
	}
}

// TestImportWalletFromWIF tests importing wallet from WIF
func TestImportWalletFromWIF(t *testing.T) {
	// Generate a wallet first
	original, err := GenerateWallet("testnet")
	if err != nil {
		t.Fatalf("Failed to generate wallet: %v", err)
	}

	// Import it back using the WIF
	imported, err := ImportWalletFromWIF(original.PrivateKey, "testnet")
	if err != nil {
		t.Fatalf("Failed to import wallet: %v", err)
	}

	// Verify imported wallet matches original
	if imported.Address != original.Address {
		t.Errorf("Address mismatch: expected %s, got %s", original.Address, imported.Address)
	}

	if imported.PrivateKey != original.PrivateKey {
		t.Errorf("PrivateKey mismatch")
	}

	if string(imported.PublicKey) != string(original.PublicKey) {
		t.Errorf("PublicKey mismatch")
	}

	if imported.Network != original.Network {
		t.Errorf("Network mismatch")
	}
}

// TestImportWalletFromWIFNetworkMismatch tests network validation
func TestImportWalletFromWIFNetworkMismatch(t *testing.T) {
	// Generate mainnet wallet
	wallet, err := GenerateWallet("mainnet")
	if err != nil {
		t.Fatalf("Failed to generate wallet: %v", err)
	}

	// Try to import as testnet (should fail)
	_, err = ImportWalletFromWIF(wallet.PrivateKey, "testnet")
	if err == nil {
		t.Error("Expected error when importing mainnet WIF as testnet")
	}
}

// TestImportWalletFromWIFInvalidWIF tests invalid WIF handling
func TestImportWalletFromWIFInvalidWIF(t *testing.T) {
	invalidWIFs := []string{
		"",
		"invalid-wif-format",
		"L1234567890",
		"NotAValidWIFAtAll123456789012345678901234567",
	}

	for _, wif := range invalidWIFs {
		_, err := ImportWalletFromWIF(wif, "mainnet")
		if err == nil {
			t.Errorf("Expected error for invalid WIF: %s", wif)
		}
	}
}

// TestSelectCoins tests coin selection algorithm
func TestSelectCoins(t *testing.T) {
	utxos := []UTXO{
		{TxHash: "hash1", Vout: 0, Value: 10000, Status: struct {
			Confirmed   bool `json:"confirmed"`
			BlockHeight int  `json:"block_height"`
		}{Confirmed: true, BlockHeight: 100}},
		{TxHash: "hash2", Vout: 0, Value: 20000, Status: struct {
			Confirmed   bool `json:"confirmed"`
			BlockHeight int  `json:"block_height"`
		}{Confirmed: true, BlockHeight: 101}},
		{TxHash: "hash3", Vout: 0, Value: 50000, Status: struct {
			Confirmed   bool `json:"confirmed"`
			BlockHeight int  `json:"block_height"`
		}{Confirmed: true, BlockHeight: 102}},
	}

	tests := []struct {
		name      string
		amount    btcutil.Amount
		feeRate   int64
		expectErr bool
	}{
		{"Small amount", 5000, 1, false},
		{"Medium amount", 15000, 1, false},
		{"Large amount", 70000, 1, false},
		{"Insufficient funds", 100000, 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selected, totalInput, change, err := selectCoins(utxos, tt.amount, tt.feeRate)

			if tt.expectErr {
				if err == nil {
					t.Error("Expected error for insufficient funds")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if len(selected) == 0 {
				t.Error("No UTXOs selected")
			}

			if totalInput < tt.amount {
				t.Errorf("Total input (%d) less than amount (%d)", totalInput, tt.amount)
			}

			// Verify change is reasonable
			if change >= totalInput {
				t.Errorf("Change (%d) should be less than total input (%d)", change, totalInput)
			}
		})
	}
}

// TestSelectCoinsUnconfirmed tests that unconfirmed UTXOs are skipped
func TestSelectCoinsUnconfirmed(t *testing.T) {
	utxos := []UTXO{
		{TxHash: "hash1", Vout: 0, Value: 100000, Status: struct {
			Confirmed   bool `json:"confirmed"`
			BlockHeight int  `json:"block_height"`
		}{Confirmed: false, BlockHeight: 0}}, // Unconfirmed
	}

	_, _, _, err := selectCoins(utxos, 5000, 1)
	if err == nil {
		t.Error("Expected error when only unconfirmed UTXOs available")
	}
}

// TestSelectCoinsDust tests dust threshold handling
func TestSelectCoinsDust(t *testing.T) {
	// Create UTXOs that will result in dust change
	utxos := []UTXO{
		{TxHash: "hash1", Vout: 0, Value: 10000, Status: struct {
			Confirmed   bool `json:"confirmed"`
			BlockHeight int  `json:"block_height"`
		}{Confirmed: true, BlockHeight: 100}},
	}

	// Amount that leaves < 546 sats as change
	amount := btcutil.Amount(9500)
	feeRate := int64(1)

	_, _, change, err := selectCoins(utxos, amount, feeRate)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Change should be 0 when dust threshold would be violated
	if change > 0 && change < 546 {
		t.Errorf("Change (%d) is dust (< 546 sats)", change)
	}
}

// TestGetNetworkConfig tests network parameter helper
func TestGetNetworkConfig(t *testing.T) {
	mainnetParams := getNetworkConfig("mainnet")
	if mainnetParams.Name != "mainnet" {
		t.Errorf("Expected mainnet params, got %s", mainnetParams.Name)
	}

	testnetParams := getNetworkConfig("testnet")
	if testnetParams.Name != "testnet3" {
		t.Errorf("Expected testnet3 params, got %s", testnetParams.Name)
	}

	// Any invalid network defaults to testnet
	invalidParams := getNetworkConfig("invalid")
	if invalidParams.Name != "testnet3" {
		t.Errorf("Expected testnet3 for invalid network, got %s", invalidParams.Name)
	}
}

// TestCreateTransactionValidation tests input validation
func TestCreateTransactionValidation(t *testing.T) {
	wallet, err := GenerateWallet("testnet")
	if err != nil {
		t.Fatalf("Failed to generate wallet: %v", err)
	}

	tests := []struct {
		name      string
		toAddress string
		amount    btcutil.Amount
		feeRate   int64
		expectErr bool
	}{
		{"Invalid address", "invalid-address", 10000, 1, true},
		{"Zero amount", "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", 0, 1, true},
		{"Negative amount", "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", -100, 1, true},
		{"Zero fee rate", "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", 10000, 0, true},
		{"Negative fee rate", "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", 10000, -1, true},
		{"Network mismatch", "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4", 10000, 1, true}, // mainnet addr on testnet wallet
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := wallet.CreateTransaction(tt.toAddress, tt.amount, tt.feeRate)
			if tt.expectErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// Note: GetUTXOs, GetBalance, CreateTransaction (with real UTXOs),
// SignTransaction, and BroadcastTransaction require either:
// 1. Mocked HTTP responses (for unit tests)
// 2. Real testnet Bitcoin with funded addresses (for integration tests)
//
// These should be in separate integration test file with build tag:
// // +build integration
//
// For now, we test the validation and coin selection logic which doesn't
// require external dependencies.

// BenchmarkImportWalletFromWIF benchmarks wallet import
func BenchmarkImportWalletFromWIF(b *testing.B) {
	wallet, _ := GenerateWallet("testnet")
	wif := wallet.PrivateKey

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ImportWalletFromWIF(wif, "testnet")
	}
}

// BenchmarkSelectCoins benchmarks coin selection
func BenchmarkSelectCoins(b *testing.B) {
	utxos := []UTXO{
		{TxHash: "hash1", Vout: 0, Value: 10000, Status: struct {
			Confirmed   bool `json:"confirmed"`
			BlockHeight int  `json:"block_height"`
		}{Confirmed: true, BlockHeight: 100}},
		{TxHash: "hash2", Vout: 0, Value: 20000, Status: struct {
			Confirmed   bool `json:"confirmed"`
			BlockHeight int  `json:"block_height"`
		}{Confirmed: true, BlockHeight: 101}},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, _ = selectCoins(utxos, 15000, 1)
	}
}
