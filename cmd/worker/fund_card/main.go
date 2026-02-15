package main

import (
	"context"
	"fmt"
	"os"

	"btc-giftcard/pkg/logger"

	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	if err := logger.Init("development"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	logger.Info("Starting fund_card worker...")

	// ========================================================================
	// BTC ACQUISITION STRATEGY (Choose one - implement before this worker)
	// ========================================================================
	//
	// OPTION 1: Stripe + Hot Wallet (Recommended for MVP)
	// ────────────────────────────────────────────────────
	// • GUI: User pays $100 via Stripe (credit card)
	// • BTC Source: Hot wallet (YOUR treasury address on blockchain)
	//   - One-time setup: Generate hot wallet address (bc1q...)
	//   - Monthly refill: Buy BTC on exchange → Withdraw to hot wallet
	//   - This worker: Transfer from hot wallet → card wallets
	// • Env needed: HOT_WALLET_WIF (encrypted private key)
	//
	// OPTION 3: Payment Processor + Treasury
	// ───────────────────────────────────────
	// • GUI: User pays $100 via Coinbase Commerce / BTCPay / Strike
	// • BTC Source: Payment processor auto-converts USD → BTC
	//   - BTC arrives at YOUR treasury address
	//   - This worker: Transfer from treasury → card wallets
	// • Env needed: TREASURY_WALLET_WIF (encrypted private key)
	//
	// ⚠️  NOTE: This worker does NOT buy BTC from exchanges
	// ⚠️  BTC must already be in hot wallet / treasury before running
	// ========================================================================

	// TODO: Initialize all dependencies needed for worker
	//
	// 1. Setup Redis client for queue
	//    - redisClient := redis.NewClient(&redis.Options{...})
	//    - Use getEnv() for REDIS_HOST, REDIS_PASSWORD, REDIS_DB
	//    - Test connection with Ping()
	//    - defer redisClient.Close()
	//
	// 2. Setup database connection
	//    - db := database.NewDB(host, port, user, password, dbname)
	//    - Use getEnv() for all DB connection params
	//    - defer db.Close()
	//
	// 3. Create repositories
	//    - cardRepo := database.NewCardRepository(db)
	//    - txRepo := database.NewTransactionRepository(db)
	//
	// 4. Create exchange provider for BTC price (fetch only)
	//    - httpClient := &http.Client{Timeout: 10 * time.Second}
	//    - priceProvider := exchange.NewProvider("coinbase", "", httpClient)
	//    - Used to calculate satoshis from fiat amount
	//
	// 5. Load hot wallet / treasury private key (32 bytes for AES-256)
	//    - hotWalletWIF := getEnv("HOT_WALLET_WIF", "")
	//    - Import wallet: wallet.ImportWalletFromWIF(hotWalletWIF, network)
	//    - This is the SOURCE of BTC for funding cards
	//
	// 6. Load encryption key for card private keys
	//    - encryptionKey := []byte(getEnv("ENCRYPTION_KEY", ""))
	//    - Validate length is 32 bytes

	// TODO: Setup queue
	// - queue := streams.NewStreamQueue(redisClient)
	// - streamName := "fund_card"
	// - groupName := "workers"
	// - consumerName := fmt.Sprintf("worker-%d", time.Now().Unix())
	// - Declare stream: queue.DeclareStream(ctx, streamName, groupName)

	// TODO: Setup graceful shutdown
	// - sigChan := make(chan os.Signal, 1)
	// - signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	// - ctx, cancel := context.WithCancel(context.Background())
	// - defer cancel()
	//
	// - Start consumer in goroutine:
	//   go func() {
	//       err := queue.Consume(ctx, streamName, groupName, consumerName,
	//           func(messageID string, data []byte) error {
	//               return processMessage(ctx, messageID, data)
	//           })
	//       if err != nil && err != context.Canceled {
	//           logger.Error("Consumer error", zap.Error(err))
	//       }
	//   }()
	//
	// - Wait for shutdown: sig := <-sigChan
	// - Cancel context: cancel()
	// - Wait for cleanup: time.Sleep(5 * time.Second)

	logger.Info("Worker not yet implemented - add your code here")
}

// processMessage handles a single fund_card message
// This is the core business logic that processes each card funding request
//
// ========================================================================
// FUNDING FLOW (for both Option 1 and Option 3)
// ========================================================================
//  1. User pays $100 (Stripe or Payment Processor - handled by GUI)
//  2. Card created with Status=Created, BTCAmountSats=0
//  3. FundCardMessage published to queue
//  4. THIS WORKER processes message:
//     → Fetch BTC price
//     → Calculate satoshis
//     → Transfer from HOT WALLET/TREASURY → Card wallet (on-chain tx)
//     → Update card Status=Active, BTCAmountSats=X
//  5. MonitorTransactionMessage published for confirmation tracking
//
// ========================================================================
func processMessage(ctx context.Context, messageID string, data []byte) error {
	logger.Info("Processing message", zap.String("messageID", messageID))

	// TODO: Implement message processing logic
	//
	// Step 1: Deserialize and validate message
	//   - msg, err := messages.FromJSONFundCard(data)
	//   - if err != nil { return fmt.Errorf("invalid message: %w", err) }
	//   - if err := msg.Validate(); err != nil { return err }
	//   - Log card_id, fiat_amount_cents, fiat_currency
	//
	// Step 2: Fetch card from database
	//   - card, err := cardRepo.GetByID(ctx, msg.CardID)
	//   - if err != nil { return err }
	//   - Check if card.Status == database.Created
	//   - If not Created, log warning and return nil (skip, already processed)
	//
	// Step 3: Update card status to Funding (prevents duplicate processing)
	//   - card.Status = database.Funding
	//   - if err := cardRepo.Update(ctx, card); err != nil { return err }
	//   - Log status change
	//
	// Step 4: Fetch current BTC price from exchange provider
	//   - price, err := priceProvider.GetPrice(ctx, msg.FiatCurrency)
	//   - if err != nil { return err } // Retry on exchange API failure
	//   - logger.Info("BTC price fetched", zap.Float64("price", price), zap.String("currency", msg.FiatCurrency))
	//
	// Step 5: Calculate BTC amount in satoshis
	//   - fiatAmount := float64(msg.FiatAmountCents) / 100.0  // $50.00
	//   - btcAmount := fiatAmount / price                     // 0.00074627 BTC
	//   - satoshis := int64(btcAmount * 100_000_000)         // 74627 sats
	//   - logger.Info("Calculated BTC amount", zap.Int64("satoshis", satoshis), zap.Float64("btc", btcAmount))
	//
	// Step 6: Send BTC from hot wallet/treasury to card's unique wallet
	//   ┌────────────────────────────────────────────────────────────┐
	//   │ OPTION 1 (MVP - Mock): Generate fake txHash for testing   │
	//   ├────────────────────────────────────────────────────────────┤
	//   │ txHash := fmt.Sprintf("mock_tx_%s_%d",                     │
	//   │     card.ID[:8], time.Now().Unix())                        │
	//   │ logger.Warn("MOCK: Would send BTC",                        │
	//   │     zap.String("from", "hot_wallet"),                      │
	//   │     zap.String("to", card.WalletAddress),                  │
	//   │     zap.Int64("satoshis", satoshis))                       │
	//   └────────────────────────────────────────────────────────────┘
	//
	//   ┌────────────────────────────────────────────────────────────┐
	//   │ OPTION 2 (Production): Real on-chain BTC transfer         │
	//   ├────────────────────────────────────────────────────────────┤
	//   │ // Import hot wallet from environment                      │
	//   │ hotWalletWIF := os.Getenv("HOT_WALLET_WIF")               │
	//   │ hotWallet, err := wallet.ImportWalletFromWIF(              │
	//   │     hotWalletWIF, network)                                 │
	//   │ if err != nil { return err }                               │
	//   │                                                            │
	//   │ // Create and broadcast transaction                        │
	//   │ txHash, err := hotWallet.SendBTC(                          │
	//   │     card.WalletAddress, satoshis)                          │
	//   │ if err != nil {                                            │
	//   │     logger.Error("Failed to send BTC", zap.Error(err))     │
	//   │     // TODO: Update card status to Failed?                 │
	//   │     return err                                             │
	//   │ }                                                          │
	//   │                                                            │
	//   │ logger.Info("BTC sent on-chain",                           │
	//   │     zap.String("txhash", txHash),                          │
	//   │     zap.String("to", card.WalletAddress))                  │
	//   └────────────────────────────────────────────────────────────┘
	//
	// Step 7: Update card with BTC amount and funding timestamp
	//   - now := time.Now().UTC()
	//   - card.BTCAmountSats = satoshis
	//   - card.FundedAt = &now
	//   - if err := cardRepo.Update(ctx, card); err != nil { return err }
	//   - logger.Info("Card updated with BTC amount", zap.String("card_id", card.ID))
	//
	// Step 8: Create transaction record in database
	//   - now := time.Now().UTC()
	//   - tx := &database.Transaction{
	//       ID:            uuid.New().String(),
	//       CardID:        card.ID,
	//       Type:          database.Fund,
	//       TxHash:        txHash,
	//       FromAddress:   "", // Hot wallet address (optional)
	//       ToAddress:     card.WalletAddress,
	//       BTCAmountSats: satoshis,
	//       Status:        database.Pending,
	//       Confirmations: 0,
	//       CreatedAt:     now,
	//       BroadcastAt:   &now,
	//   }
	//   - if err := txRepo.Create(ctx, tx); err != nil { return err }
	//
	// Step 9: Publish MonitorTransactionMessage to monitor_tx stream
	//   - monitorMsg := messages.MonitorTransactionMessage{
	//       CardID:             card.ID,
	//       TxHash:             txHash,
	//       ExpectedAmountSats: satoshis,
	//       DestinationAddr:    card.WalletAddress,
	//   }
	//   - monitorJSON, err := monitorMsg.ToJSON()
	//   - if err != nil { return err }
	//   - if _, err := queue.Publish(ctx, "monitor_tx", monitorJSON); err != nil {
	//       logger.Error("Failed to publish monitor message", zap.Error(err))
	//       return err
	//   }
	//   - logger.Info("Published MonitorTransactionMessage", zap.String("txhash", txHash))
	//
	// Error handling strategy:
	//   - Return error → Retry (transient failures like DB down, exchange API timeout)
	//   - Return nil → Skip (permanent failures like invalid card status)
	//   - Log all errors with appropriate context (card_id, messageID, etc.)

	logger.Info("Message processed successfully", zap.String("messageID", messageID))
	return nil
}

// getEnv gets environment variable with default value
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// getEnvInt gets environment variable as int with default value
func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	var intValue int
	if _, err := fmt.Sscanf(value, "%d", &intValue); err != nil {
		return defaultValue
	}
	return intValue
}
