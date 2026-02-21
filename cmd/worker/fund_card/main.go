package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"btc-giftcard/config"
	"btc-giftcard/internal/database"
	"btc-giftcard/internal/exchange"
	messages "btc-giftcard/internal/queue"
	"btc-giftcard/pkg/cache"
	"btc-giftcard/pkg/logger"
	streams "btc-giftcard/pkg/queue"

	"github.com/google/uuid"
	"github.com/jinzhu/copier"
	"go.uber.org/zap"
)

var Cfg config.ApiConfig

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Initialize logger
	if err := logger.Init("development"); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer logger.Sync()

	// Load configuration
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename))))
	configPath := config.Path(root).Join("config.toml")

	if err := config.Load(configPath, &Cfg); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger.Info("Starting fund_card worker...")

	// ========================================================================
	// CUSTODIAL FUNDING MODEL
	// ========================================================================
	//
	// This worker processes FundCardMessage from Redis queue.
	// Funding is PURE ACCOUNTING — no blockchain transaction happens here.
	//
	// BTC is pre-purchased via OTC (Crypto.com OTC 2.0) and held in treasury
	// (Lightning channels + on-chain hot wallet). Cards are balance claims.
	//
	// Flow:
	//   1. API creates card (Status=Created, BTCAmountSats=0)
	//   2. API publishes FundCardMessage to "fund_card" Redis stream
	//   3. THIS WORKER:
	//      → Fetch BTC price from OTC provider (our actual cost basis)
	//      → Calculate satoshis for the card's fiat value
	//      → Check treasury has enough available balance (prevent overselling)
	//      → Reserve balance: Update card BTCAmountSats + Status=Active
	//      → Create Fund transaction record (accounting only, no tx_hash)
	//   4. Card is now active and spendable by the user
	//
	// ⚠️  NO on-chain tx, NO wallet generation, NO private keys
	// ⚠️  BTC only moves when user REDEEMS (Lightning or on-chain)
	// ========================================================================

	// Initialize Redis
	var redisCfg cache.Config
	if err := copier.Copy(&redisCfg, &Cfg.Redis); err != nil {
		return fmt.Errorf("failed to copy cache config: %w", err)
	}
	if err := cache.Init(redisCfg); err != nil {
		return fmt.Errorf("failed to initialize cache: %w", err)
	}
	defer cache.Close()

	// Initialize database
	var dbCfg database.Config
	if err := copier.Copy(&dbCfg, &Cfg.Database); err != nil {
		return fmt.Errorf("failed to copy database config: %w", err)
	}
	db, err := database.NewDB(dbCfg)
	if err != nil {
		return fmt.Errorf("failed to initialize database connection: %w", err)
	}
	defer db.Close()

	// Create repositories
	cardRepo := database.NewCardRepository(db)
	txRepo := database.NewTransactionRepository(db)

	// Create OTC price provider
	// TODO: Switch to "cryptocom_otc" provider once implemented
	// This reflects our actual BTC cost basis (not a random public exchange)
	// Fallback chain: OTC provider → Coinbase → CoinGecko
	provider, err := exchange.NewProvider("coinbase", "", nil)
	if err != nil {
		return fmt.Errorf("failed to initialize exchange provider: %w", err)
	}

	// TODO: Load treasury config
	//    - treasuryTotalSats: total BTC held (Lightning channels + hot wallet)
	//    - Available = treasuryTotalSats - SUM(unredeemed card balances)
	//    - Replace with treasury service that queries LND + hot wallet in real-time
	//
	// IMPLEMENT: Initialize LND client from config
	//   var lndCfg lnd.Config
	//   lndCfg.GRPCHost     = Cfg.LND.GRPCHost          // "gift-card-backend.lnd:10009"
	//   lndCfg.TLSCertPath  = Cfg.LND.TLSCertPath       // "./lnd-data/tls.cert"
	//   lndCfg.MacaroonPath = Cfg.LND.MacaroonPath       // "./lnd-data/admin.macaroon"
	//   lndCfg.Network      = Cfg.LND.Network            // "testnet"
	//
	//   lndClient, err := lnd.NewClient(lndCfg)
	//   if err != nil {
	//       return fmt.Errorf("failed to connect to LND: %w", err)
	//   }
	//   defer lndClient.Close()
	//
	//   // Verify LND is synced at startup
	//   info, err := lndClient.GetInfo(ctx)
	//   logger.Info("Connected to LND",
	//       zap.String("alias", info.Alias),
	//       zap.Bool("synced", info.SyncedToChain),
	//       zap.Uint32("block_height", info.BlockHeight),
	//   )
	//
	// Then pass lndClient to newMessageHandler() so processMessage can check treasury balance.

	// Setup queue consumer
	queue := streams.NewStreamQueue(cache.Client)
	streamName := "fund_card"
	groupName := "fund_workers"
	consumerName := fmt.Sprintf("fund-worker-%d", time.Now().Unix())

	// Graceful shutdown context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := queue.DeclareStream(ctx, streamName, groupName); err != nil {
		return fmt.Errorf("failed to declare the consumer group: %w", err)
	}

	// Start consumer goroutine
	handler := newMessageHandler(cardRepo, txRepo, provider)

	go func() {
		err := queue.Consume(ctx, streamName, groupName, consumerName,
			func(messageID string, data []byte) error {
				return handler.processMessage(ctx, messageID, data)
			})
		if err != nil && err != context.Canceled {
			logger.Error("Consumer error", zap.Error(err))
		}
	}()

	logger.Info("Fund card worker is running, waiting for messages...",
		zap.String("stream", streamName),
		zap.String("group", groupName),
		zap.String("consumer", consumerName),
	)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	logger.Info("Received shutdown signal", zap.String("signal", sig.String()))

	// Cancel context to stop consumer
	cancel()

	// Give the consumer time to finish processing current message
	time.Sleep(3 * time.Second)
	logger.Info("Fund card worker shut down gracefully")

	return nil
}

// messageHandler holds the dependencies needed by processMessage.
type messageHandler struct {
	cardRepo *database.CardRepository
	txRepo   *database.TransactionRepository
	provider exchange.PriceProvider
}

func newMessageHandler(
	cardRepo *database.CardRepository,
	txRepo *database.TransactionRepository,
	provider exchange.PriceProvider,
) *messageHandler {
	return &messageHandler{
		cardRepo: cardRepo,
		txRepo:   txRepo,
		provider: provider,
	}
}

// processMessage handles a single FundCardMessage from the queue.
//
// ========================================================================
// CUSTODIAL FUNDING FLOW (pure accounting — no blockchain tx)
// ========================================================================
//  1. User pays €100 (bank transfer / Stripe — handled by API)
//  2. Card created with Status=Created, BTCAmountSats=0
//  3. FundCardMessage published to "fund_card" queue
//  4. THIS WORKER processes message:
//     → Fetch BTC price from OTC provider (our cost basis)
//     → Calculate satoshis (e.g., €95 after fee / €67,000 = 141,791 sats)
//     → Check treasury available balance ≥ satoshis needed
//     → Update card: BTCAmountSats=141791, Status=Active, FundedAt=now
//     → Create Transaction record (Type=Fund, no tx_hash — just accounting)
//  5. Card is now active — user can spend (Lightning or on-chain)
//
// ⚠️  No MonitorTransactionMessage needed — no on-chain tx to monitor
// ========================================================================
func (h *messageHandler) processMessage(ctx context.Context, messageID string, data []byte) error {
	logger.Info("Processing fund_card message", zap.String("messageID", messageID))

	// Deserialize and validate message
	msg, err := messages.FromJSONFundCard(data)
	if err != nil {
		return fmt.Errorf("invalid message: %w", err)
	}
	logger.Info("Received message", zap.String("card_id", msg.CardID), zap.Int64("fiat_amount_cents", msg.FiatAmountCents), zap.String("fiat_currency", msg.FiatCurrency))

	// Fetch card from database and validate state
	card, err := h.cardRepo.GetByID(ctx, msg.CardID)
	if err != nil {
		return fmt.Errorf("error fetching card: %w", err)
	}
	if card.Status != database.Created {
		logger.Warn("Card already processed, skipping", zap.String("card_id", card.ID), zap.String("status", card.Status.String()))
		return nil // Idempotent: skip already-funded cards
	}

	// Set card status to Funding (prevents duplicate processing)
	err = h.cardRepo.Update(ctx, card.ID, database.Funding, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to set funding status: %w", err)
	}

	// Fetch BTC price from OTC provider (TODO check if it's better to fetch crypto.com price)
	price, err := h.provider.GetPrice(ctx, msg.FiatCurrency)
	if err != nil {
		return fmt.Errorf("error fetching BTC price: %w", err)
	}
	logger.Info("BTC price from OTC provider", zap.Float64("price", price), zap.String("currency", msg.FiatCurrency))

	// Calculate BTC amount in satoshis
	fiatAmount := float64(msg.FiatAmountCents) / 100.0
	btcAmount := fiatAmount / price
	satoshis := int64(btcAmount * 100_000_000)
	if satoshis <= 0 {
		logger.Error("Calculated 0 sats — price too high or amount too low")
		return nil // Permanent failure, don't retry
	}

	// Check treasury has enough available balance
	// IMPLEMENT using LND client (passed via messageHandler):
	//
	//   1. Get Lightning channel balance:
	//      channelBal, err := h.lndClient.GetChannelBalance(ctx)
	//      lightningAvailable := channelBal.LocalSats
	//
	//   2. Get on-chain wallet balance:
	//      walletBal, err := h.lndClient.GetWalletBalance(ctx)
	//      onChainAvailable := walletBal.ConfirmedSats
	//
	//   3. Calculate total treasury:
	//      totalTreasury := lightningAvailable + onChainAvailable
	//
	//   4. Query total reserved balance (sum of active + funding cards):
	//      SELECT COALESCE(SUM(btc_amount_sats), 0) FROM cards WHERE status IN ('active','funding')
	//      → totalReserved
	//      (TODO: add a GetTotalReservedBalance method to CardRepository)
	//
	//   5. available := totalTreasury - totalReserved
	//      if available < satoshis {
	//          logger.Error("Treasury insufficient",
	//              zap.Int64("needed", satoshis),
	//              zap.Int64("available", available),
	//          )
	//          // Revert card to Created so it can be retried later
	//          h.cardRepo.Update(ctx, card.ID, database.Created, nil, nil, nil)
	//          return fmt.Errorf("treasury insufficient: need %d sats, have %d available", satoshis, available)
	//      }
	//
	//   CONCURRENCY: Use Redis distributed lock to prevent race conditions
	//      lockKey := "treasury:reserve_lock"
	//      acquired, err := cache.Client.SetNX(ctx, lockKey, consumerID, 5*time.Second).Result()
	//      if !acquired { return retry }
	//      defer cache.Client.Del(ctx, lockKey)
	//      // ... check balance and reserve inside the lock ...

	// Update card — reserve the balance (this IS the funding)
	now := time.Now().UTC()
	if err := h.cardRepo.Update(ctx, card.ID, database.Active, &satoshis, &now, nil); err != nil {
		return fmt.Errorf("failed to activate card: %w", err)
	}
	logger.Info("Card funded (balance reserved)", zap.String("card_id", card.ID), zap.Int64("satoshis", satoshis))

	// Step 8: Create Fund transaction record (accounting only — no blockchain tx)
	now = time.Now().UTC()
	tx := &database.Transaction{
		ID:            uuid.New().String(),
		CardID:        card.ID,
		Type:          database.Fund,
		BTCAmountSats: satoshis,
		Status:        database.Confirmed,
		Confirmations: 0,
		CreatedAt:     now,
		ConfirmedAt:   &now,
	}
	if err := h.txRepo.Create(ctx, tx); err != nil {
		logger.Error("Failed to create fund transaction", zap.Error(err))
	}

	logger.Info("Message processed successfully", zap.String("messageID", messageID))
	return nil
}
