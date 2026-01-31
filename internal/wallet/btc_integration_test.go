//go:build integration
// +build integration

package wallet

import (
	"testing"

	"btc-giftcard/pkg/logger"
)

func init() {
	// Initialize logger for integration tests
	_ = logger.Init("development")
}

// TestGetUTXOsIntegration tests fetching UTXOs from blockchain API
// TODO: Implement this test
// Requirements:
// 1. Fund a testnet address using: https://testnet-faucet.mempool.co/
// 2. Generate or import a wallet with known testnet address
// 3. Call GetUTXOs() on the funded wallet
// 4. Verify UTXOs array is not empty
// 5. Verify each UTXO has: TxHash, Vout, Value, Status.Confirmed
// 6. Verify values match blockchain explorer
func TestGetUTXOsIntegration(t *testing.T) {
	t.Skip("TODO: Implement GetUTXOs integration test - requires funded testnet address")

	// Example structure:
	// wallet, err := GenerateWallet("testnet")
	// // Manually fund wallet.Address at faucet, wait for confirmation
	// utxos, err := wallet.GetUTXOs()
	// assert UTXOs exist and have expected values
}

// TestGetBalanceIntegration tests balance calculation from real blockchain
// TODO: Implement this test
// Requirements:
// 1. Use funded testnet wallet (same as TestGetUTXOsIntegration)
// 2. Call GetBalance() on the wallet
// 3. Verify balance matches expected amount from faucet
// 4. Verify balance is sum of all confirmed UTXOs
// 5. Test edge case: unconfirmed transactions should not count
func TestGetBalanceIntegration(t *testing.T) {
	t.Skip("TODO: Implement GetBalance integration test - requires funded testnet address")

	// Example structure:
	// wallet := importFundedTestnetWallet()
	// balance, err := wallet.GetBalance()
	// verify balance > 0
	// verify balance matches blockchain explorer
}

// TestCreateTransactionIntegration tests transaction creation with real UTXOs
// TODO: Implement this test
// Requirements:
// 1. Use funded testnet wallet with known UTXOs
// 2. Create valid recipient address (another testnet address you control)
// 3. Call CreateTransaction() with amount < balance
// 4. Verify transaction has correct inputs (from UTXOs)
// 5. Verify transaction has correct outputs (recipient + change)
// 6. Verify fee calculation is reasonable (amount = inputs - outputs)
// 7. Test edge cases: exact amount (no change), multiple UTXOs needed
func TestCreateTransactionIntegration(t *testing.T) {
	t.Skip("TODO: Implement CreateTransaction integration test - requires funded testnet address")

	// Example structure:
	// wallet := importFundedTestnetWallet()
	// recipientAddr := "tb1q..." // Another testnet address
	// amount := btcutil.Amount(5000) // 5000 sats
	// feeRate := int64(1) // 1 sat/vbyte
	// tx, err := wallet.CreateTransaction(recipientAddr, amount, feeRate)
	// verify tx structure
	// verify inputs match expected UTXOs
	// verify outputs: recipient + optional change
}

// TestSignTransactionIntegration tests signing a real transaction
// TODO: Implement this test
// Requirements:
// 1. Create unsigned transaction (from TestCreateTransactionIntegration)
// 2. Get UTXOs used in the transaction
// 3. Call SignTransaction() with transaction and UTXOs
// 4. Verify transaction has witness data (SegWit signatures)
// 5. Verify signature count matches input count
// 6. Optionally verify signature with btcec.Verify (advanced)
func TestSignTransactionIntegration(t *testing.T) {
	t.Skip("TODO: Implement SignTransaction integration test - requires funded testnet address")

	// Example structure:
	// wallet := importFundedTestnetWallet()
	// tx, err := wallet.CreateTransaction(...)
	// utxos, err := wallet.GetUTXOs()
	// signedTx, err := wallet.SignTransaction(tx, utxos)
	// verify signedTx.TxIn[i].Witness is not empty for each input
}

// TestBroadcastTransactionIntegration tests broadcasting to testnet
// TODO: Implement this test
// Requirements:
// 1. Create and sign a valid transaction (combine previous tests)
// 2. Call BroadcastTransaction() with signed transaction
// 3. Verify no error is returned
// 4. Verify response contains transaction ID
// 5. Check transaction appears on testnet explorer: https://blockstream.info/testnet/
// 6. IMPORTANT: Use small amounts to avoid wasting testnet coins
func TestBroadcastTransactionIntegration(t *testing.T) {
	t.Skip("TODO: Implement BroadcastTransaction integration test - requires funded testnet address")

	// Example structure:
	// wallet := importFundedTestnetWallet()
	// recipientAddr := generateNewTestnetAddress()
	// tx, err := wallet.CreateTransaction(recipientAddr, 1000, 1)
	// signedTx, err := wallet.SignTransaction(tx, utxos)
	// txID, err := wallet.BroadcastTransaction(signedTx)
	// verify txID is not empty
	// log txID for manual verification on blockstream.info/testnet/tx/{txID}
}

// TestCompleteRedemptionFlow tests entire card redemption process
// TODO: Implement this test
// Requirements:
// 1. Simulate complete gift card redemption flow
// 2. Generate card wallet (seller's perspective)
// 3. Fund card wallet (simulates exchange purchase)
// 4. Import wallet from WIF (simulates backend decrypting card)
// 5. Create transaction to user's address (redemption)
// 6. Sign and broadcast transaction
// 7. Verify transaction succeeds on testnet
// 8. This is the most important integration test - validates entire system
func TestCompleteRedemptionFlow(t *testing.T) {
	t.Skip("TODO: Implement complete redemption flow integration test")

	// Example flow:
	// Step 1: Card creation (seller creates card)
	// cardWallet, err := GenerateWallet("testnet")
	// encryptedWIF := encrypt(cardWallet.PrivateKey) // Use crypto package
	// // Store: cardWallet.Address, encryptedWIF in database
	//
	// Step 2: Card funding (exchange sends BTC to card)
	// // Manually fund cardWallet.Address at faucet
	// // In production: exchange API would do this
	//
	// Step 3: User redeems card (backend processes redemption)
	// decryptedWIF := decrypt(encryptedWIF)
	// redeemWallet, err := ImportWalletFromWIF(decryptedWIF, "testnet")
	// balance, err := redeemWallet.GetBalance()
	// require balance > 0
	//
	// Step 4: Send to user's address
	// userAddress := "tb1q..." // User provides their address
	// tx, err := redeemWallet.CreateTransaction(userAddress, balance-fee, feeRate)
	// signedTx, err := redeemWallet.SignTransaction(tx, utxos)
	// txID, err := redeemWallet.BroadcastTransaction(signedTx)
	//
	// Step 5: Verify on blockchain
	// log.Info("Redemption TX:", txID)
	// // Manual verification on blockstream.info/testnet/tx/{txID}
}

// Helper function template for importing a funded testnet wallet
// TODO: Implement this helper
// This should return a wallet with known WIF that has testnet funds
// Store the WIF securely (not in code - use env var or test fixture file)
func importFundedTestnetWallet() (*Wallet, error) {
	// TODO: Implement
	// fundedWIF := os.Getenv("TEST_FUNDED_WIF") // Or load from fixture
	// return ImportWalletFromWIF(fundedWIF, "testnet")
	return nil, nil
}

// Instructions for running integration tests:
//
// 1. Get testnet Bitcoin:
//    - Visit: https://testnet-faucet.mempool.co/
//    - Generate a testnet wallet: go run examples/generate_testnet_wallet.go
//    - Request testnet BTC to the generated address
//    - Wait for 1 confirmation (~10 minutes)
//
// 2. Set environment variable with funded WIF:
//    export TEST_FUNDED_WIF="cT1Yn..."
//
// 3. Run integration tests:
//    go test -tags=integration ./internal/wallet -v
//
// 4. Monitor transactions:
//    https://blockstream.info/testnet/
//
// Note: Integration tests are excluded from normal test runs (go test ./...)
// because they require external dependencies (funded testnet addresses).
// They must be run explicitly with the -tags=integration flag.
