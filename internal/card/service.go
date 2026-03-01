package card

import (
	"btc-giftcard/internal/lnd"
	messages "btc-giftcard/internal/queue"
	"btc-giftcard/internal/wallet"
	"btc-giftcard/pkg/cache"
	streams "btc-giftcard/pkg/queue"

	"btc-giftcard/internal/database"
	"btc-giftcard/pkg/logger"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Custom errors for card operations
var (
	ErrCardNotFound        = errors.New("card not found")
	ErrCardNotActive       = errors.New("card is not active")
	ErrCardAlreadyUsed     = errors.New("card has already been redeemed")
	ErrInsufficientFunds   = errors.New("insufficient funds on card")
	ErrInsufficientBalance = errors.New("insufficient treasury balance")
	ErrTreasuryLockBusy    = errors.New("treasury lock is held by another process")
	ErrInvalidMethod       = errors.New("invalid redeem method")
	ErrInvalidAddress      = errors.New("invalid bitcoin address")
	ErrLightningInvoice    = errors.New("lightning invoice is required")
)

// Treasury cache and lock constants
const (
	treasuryAvailableCacheKey = "treasury:available_sats"
	treasuryAvailableCacheTTL = 10 * time.Second
	treasuryLockKey           = "treasury:lock"
	treasuryLockTTL           = 5 * time.Second
)

// On-chain redemption defaults
const (
	defaultTargetConf    int32 = 6     // ~1 hour confirmation target
	minOnChainAmountSats int64 = 10000 // 10k sats minimum (dust protection)
)

// Card-level lock for concurrent redemption protection
const (
	cardLockPrefix = "card:lock:"
	cardLockTTL    = 10 * time.Second
)

// Service handles gift card business logic.
type Service struct {
	cardRepo  *database.CardRepository
	txRepo    *database.TransactionRepository
	network   string // "testnet" or "mainnet"
	queue     *streams.StreamQueue
	lndClient *lnd.Client
}

// NewService creates a new card service instance.
func NewService(
	cardRepo *database.CardRepository,
	txRepo *database.TransactionRepository,
	network string,
	queue *streams.StreamQueue,
	lndClient *lnd.Client,
) *Service {
	return &Service{
		cardRepo:  cardRepo,
		txRepo:    txRepo,
		network:   network,
		queue:     queue,
		lndClient: lndClient,
	}
}

// GetTreasuryAvailableBalance returns the available treasury balance (total LND
// holdings minus reserved card balances). Results are cached in Redis for 10s
// to avoid hitting LND (~50-100ms latency) on every call.
func (s *Service) GetTreasuryAvailableBalance(ctx context.Context) (int64, error) {
	// Try cache first
	if cached, err := cache.Get(ctx, treasuryAvailableCacheKey); err == nil && cached != "" {
		if val, parseErr := strconv.ParseInt(cached, 10, 64); parseErr == nil {
			return val, nil
		}
		// Invalid cache value — fall through to recompute
	}

	// Compute from LND + DB
	available, err := s.computeTreasuryBalance(ctx)
	if err != nil {
		return 0, err
	}

	// Cache the result (best-effort, don't fail on cache error)
	if cacheErr := cache.Set(ctx, treasuryAvailableCacheKey, strconv.FormatInt(available, 10), treasuryAvailableCacheTTL); cacheErr != nil {
		logger.Warn("failed to cache treasury balance", zap.Error(cacheErr))
	}

	return available, nil
}

// computeTreasuryBalance fetches LND balances and DB reserved amounts
// to calculate the available treasury balance without caching.
func (s *Service) computeTreasuryBalance(ctx context.Context) (int64, error) {
	channelBal, err := s.lndClient.GetChannelBalance(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get channel balance: %w", err)
	}

	walletBal, err := s.lndClient.GetWalletBalance(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get wallet balance: %w", err)
	}

	totalTreasury := channelBal.LocalSats + walletBal.ConfirmedSats

	totalReserved, err := s.cardRepo.GetTotalReservedBalance(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch total reserved balance: %w", err)
	}

	available := totalTreasury - totalReserved
	if available < 0 {
		logger.Error("treasury oversold: available balance is negative",
			zap.Int64("total_treasury", totalTreasury),
			zap.Int64("total_reserved", totalReserved),
		)
		return 0, ErrInsufficientBalance
	}

	return available, nil
}

// AcquireTreasuryLock acquires a distributed lock for treasury reserve operations.
// Used by fund_card workers to prevent race conditions when multiple workers
// try to reserve balance simultaneously:
//
//	acquired, err := s.AcquireTreasuryLock(ctx)
//	if !acquired { /* another worker is reserving */ }
//	defer s.ReleaseTreasuryLock(ctx)
//	balance, _ := s.GetTreasuryAvailableBalance(ctx)
//	// ... reserve card ...
//
// Returns true if the lock was acquired, false if another process holds it.
func (s *Service) AcquireTreasuryLock(ctx context.Context) (bool, error) {
	acquired, err := cache.SetNX(ctx, treasuryLockKey, "locked", treasuryLockTTL)
	if err != nil {
		return false, fmt.Errorf("failed to acquire treasury lock: %w", err)
	}
	if !acquired {
		return false, ErrTreasuryLockBusy
	}
	return true, nil
}

// ReleaseTreasuryLock releases the distributed treasury lock.
func (s *Service) ReleaseTreasuryLock(ctx context.Context) {
	if _, err := cache.Delete(ctx, treasuryLockKey); err != nil {
		logger.Warn("failed to release treasury lock", zap.Error(err))
	}
}

// InvalidateTreasuryCache removes the cached treasury balance.
// Call after card funding or redemption to force a fresh computation.
func (s *Service) InvalidateTreasuryCache(ctx context.Context) {
	if _, err := cache.Delete(ctx, treasuryAvailableCacheKey); err != nil {
		logger.Warn("failed to invalidate treasury cache", zap.Error(err))
	}
}

// CreateCardRequest contains the parameters for creating a new gift card
// Note: BTCAmountSats is NOT provided at creation - it will be calculated and set
// by the funding worker based on the current BTC/fiat exchange rate.
type CreateCardRequest struct {
	FiatAmountCents    int64  // Face value in cents ($100 = 10000)
	FiatCurrency       string // "USD", "EUR", etc.
	PurchasePriceCents int64  // Total charged including fees
	UserID             *string
	PurchaseEmail      string
}

// CreateCardResponse contains the created card details
type CreateCardResponse struct {
	CardID        string
	Code          string
	BTCAmountSats int64
	Status        database.CardStatus
	CreatedAt     time.Time
}

// CreateCard creates a new gift card as a balance claim on the treasury.
// No wallet or private key is generated — cards are custodial.
func (s *Service) CreateCard(ctx context.Context, req CreateCardRequest) (*CreateCardResponse, error) {
	// 1. Generate a unique card code
	code, err := s.generateCardCode(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate card code: %w", err)
	}

	// 2. Create Card struct (custodial model — no wallet, no keys)
	// BTCAmountSats is 0 and will be set by the funding worker
	// based on the current exchange rate when the card is funded.
	card := &database.Card{
		ID:                 uuid.New().String(),
		UserID:             req.UserID,
		PurchaseEmail:      req.PurchaseEmail,
		OwnerEmail:         req.PurchaseEmail,
		Code:               code,
		BTCAmountSats:      0, // Will be set by funding worker based on current BTC price
		FiatAmountCents:    req.FiatAmountCents,
		FiatCurrency:       req.FiatCurrency,
		PurchasePriceCents: req.PurchasePriceCents,
		Status:             database.Created,
		CreatedAt:          time.Now().UTC(),
	}

	// 3. Save card to database
	err = s.cardRepo.Create(ctx, card)
	if err != nil {
		if errors.Is(err, database.ErrCardCodeExists) {
			return nil, fmt.Errorf("card code collision (unexpected): %w", err)
		}
		return nil, fmt.Errorf("failed to save card: %w", err)
	}

	// 4. Publish FundCardMessage to queue (don't fail card creation if this fails)
	msg := messages.FundCardMessage{
		CardID:          card.ID,
		FiatAmountCents: card.FiatAmountCents,
		FiatCurrency:    card.FiatCurrency,
	}

	msgJSON, err := msg.ToJSON()
	if err != nil {
		logger.Error("Failed to serialize FundCardMessage",
			zap.String("card_id", card.ID),
			zap.Error(err),
		)
	} else {
		_, err = s.queue.Publish(ctx, "fund_card", msgJSON)
		if err != nil {
			logger.Error("Failed to publish FundCardMessage",
				zap.String("card_id", card.ID),
				zap.Error(err),
			)
		} else {
			logger.Info("Published FundCardMessage",
				zap.String("card_id", card.ID),
			)
		}
	}

	// 5. Return response
	return &CreateCardResponse{
		CardID:        card.ID,
		Code:          card.Code,
		BTCAmountSats: card.BTCAmountSats,
		Status:        card.Status,
		CreatedAt:     card.CreatedAt,
	}, nil
}

type RedeemCardMethod string

const (
	OnChain   RedeemCardMethod = "onchain"
	Lightning RedeemCardMethod = "lightning"
)

// RedeemCardRequest contains the parameters for redeeming (spending) a card
type RedeemCardRequest struct {
	Code               string           // Card redemption code
	Method             RedeemCardMethod // "lightning" or "onchain"
	AmountSats         int64            // Amount to spend (can be partial)
	DestinationAddress string           // On-chain Bitcoin address (required if method=onchain)
	LightningInvoice   string           // BOLT11 invoice (required if method=lightning)
}

// RedeemCardResponse contains the redemption transaction details
type RedeemCardResponse struct {
	TransactionID    string
	Method           string  // "lightning" or "onchain"
	TxHash           *string // On-chain tx hash (nil for Lightning)
	PaymentHash      *string // Lightning payment hash (nil for on-chain)
	BTCAmountSats    int64
	RemainingBalance int64 // Card's remaining balance after this spend
	Status           database.TransactionStatus
}

// RedeemCard processes a card spend (full or partial) via Lightning or on-chain.
// Cards support partial spends — multiple transactions until balance = 0.
func (s *Service) RedeemCard(ctx context.Context, req RedeemCardRequest) (*RedeemCardResponse, error) {
	// Step 1: Validate input
	if err := s.validateRedeemRequest(req); err != nil {
		return nil, err
	}

	// Step 2: Acquire per-card lock (prevent concurrent double-spend)
	lockKey := cardLockPrefix + req.Code
	acquired, err := cache.SetNX(ctx, lockKey, "locked", cardLockTTL)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire card lock: %w", err)
	}
	if !acquired {
		return nil, errors.New("card is being processed by another request")
	}
	defer cache.Delete(ctx, lockKey)

	// Step 3: Retrieve and validate card
	card, err := s.validateCardForRedemption(ctx, req.Code, req.AmountSats)
	if err != nil {
		return nil, err
	}

	// Step 4: Execute payment via LND
	payResult, err := s.executePayment(ctx, req)
	if err != nil {
		return nil, err
	}

	// Step 5: Create transaction record
	now := time.Now().UTC()
	tx, err := s.recordRedemptionTransaction(ctx, card.ID, req, payResult, now)
	if err != nil {
		return nil, err
	}

	// Step 6: Update card balance
	remainingBalance, err := s.updateCardBalance(ctx, card.ID, card.BTCAmountSats, req.AmountSats)
	if err != nil {
		return nil, err
	}

	// Step 7: Invalidate treasury cache (balance changed)
	s.InvalidateTreasuryCache(ctx)

	// Step 8: Publish monitor message for on-chain transactions
	if req.Method == OnChain && payResult.TxHash != nil {
		s.publishMonitorTransaction(ctx, card.ID, tx.ID, *payResult.TxHash, req.AmountSats, req.DestinationAddress)
	}

	logger.Info("Card redeemed successfully",
		zap.String("card_id", card.ID),
		zap.String("tx_id", tx.ID),
		zap.String("method", string(req.Method)),
		zap.Int64("amount_sats", req.AmountSats),
		zap.Int64("remaining_sats", remainingBalance),
	)

	return &RedeemCardResponse{
		TransactionID:    tx.ID,
		Method:           string(req.Method),
		TxHash:           payResult.TxHash,
		PaymentHash:      payResult.PaymentHash,
		BTCAmountSats:    req.AmountSats,
		RemainingBalance: remainingBalance,
		Status:           tx.Status,
	}, nil
}

// ============================================================================
// RedeemCard helpers — each method has a single concern
// ============================================================================

// validateRedeemRequest validates the redemption request fields.
func (s *Service) validateRedeemRequest(req RedeemCardRequest) error {
	switch req.Method {
	case Lightning:
		if req.LightningInvoice == "" {
			return ErrLightningInvoice
		}
	case OnChain:
		if req.DestinationAddress == "" {
			return ErrInvalidAddress
		}
	default:
		return ErrInvalidMethod
	}

	if req.AmountSats <= 0 {
		return errors.New("amount must be positive")
	}

	return nil
}

// validateCardForRedemption retrieves a card and checks it can be redeemed.
func (s *Service) validateCardForRedemption(ctx context.Context, code string, amountSats int64) (*database.Card, error) {
	card, err := s.GetCardByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	if card.Status != database.Active {
		return nil, ErrCardNotActive
	}

	if amountSats > card.BTCAmountSats {
		return nil, ErrInsufficientFunds
	}

	return card, nil
}

// paymentOutput holds the results of executePayment (unified for both paths).
type paymentOutput struct {
	PaymentHash     *string
	PaymentPreimage *string
	TxHash          *string
	ToAddress       *string
	Invoice         *string
	Status          database.TransactionStatus
	ConfirmedAt     *time.Time
}

// executePayment dispatches to the correct payment path (Lightning or on-chain).
func (s *Service) executePayment(ctx context.Context, req RedeemCardRequest) (*paymentOutput, error) {
	switch req.Method {
	case Lightning:
		return s.executeLightningPayment(ctx, req.LightningInvoice, req.AmountSats)
	case OnChain:
		return s.executeOnChainPayment(ctx, req.DestinationAddress, req.AmountSats)
	default:
		return nil, ErrInvalidMethod
	}
}

// executeLightningPayment decodes, validates, and pays a BOLT11 invoice.
func (s *Service) executeLightningPayment(ctx context.Context, invoice string, amountSats int64) (*paymentOutput, error) {
	// Decode and validate
	decoded, err := s.lndClient.DecodeInvoice(ctx, invoice)
	if err != nil {
		return nil, fmt.Errorf("invalid invoice: %w", err)
	}

	if decoded.AmountSats == 0 {
		return nil, errors.New("zero-amount invoices not supported")
	}

	if decoded.IsExpired {
		return nil, errors.New("invoice has expired")
	}

	if decoded.AmountSats != amountSats {
		return nil, fmt.Errorf("invoice amount (%d sats) does not match requested amount (%d sats)", decoded.AmountSats, amountSats)
	}

	// Pay the invoice
	logger.Info("Paying Lightning invoice",
		zap.Int64("amount_sats", amountSats),
		zap.String("destination", decoded.Destination),
	)

	result, err := s.lndClient.PayInvoice(ctx, invoice, s.lndClient.Cfg.MaxPaymentFeeSats)
	if err != nil {
		return nil, fmt.Errorf("lightning payment failed: %w", err)
	}

	// Verify payment actually succeeded (PayInvoice could return non-error with failed status)
	if result.Status != lnd.Succeeded {
		return nil, fmt.Errorf("lightning payment did not succeed: status=%s", result.Status)
	}

	now := time.Now().UTC()
	return &paymentOutput{
		PaymentHash:     &result.PaymentHash,
		PaymentPreimage: &result.PaymentPreimage,
		Invoice:         &invoice,
		Status:          database.Confirmed, // Lightning settles instantly
		ConfirmedAt:     &now,
	}, nil
}

// executeOnChainPayment validates the address and sends an on-chain transaction.
func (s *Service) executeOnChainPayment(ctx context.Context, address string, amountSats int64) (*paymentOutput, error) {
	// Validate destination address
	isValid, err := wallet.ValidateAddress(address, s.network)
	if err != nil {
		return nil, fmt.Errorf("failed to validate address: %w", err)
	}
	if !isValid {
		return nil, ErrInvalidAddress
	}

	// Enforce minimum on-chain amount (mining fees make tiny sends uneconomical)
	if amountSats < minOnChainAmountSats {
		return nil, fmt.Errorf("on-chain minimum is %d sats", minOnChainAmountSats)
	}

	// Send on-chain
	logger.Info("Sending on-chain transaction",
		zap.Int64("amount_sats", amountSats),
		zap.String("destination", address),
		zap.Int32("target_conf", defaultTargetConf),
	)

	result, err := s.lndClient.SendOnChain(ctx, address, amountSats, defaultTargetConf)
	if err != nil {
		return nil, fmt.Errorf("on-chain send failed: %w", err)
	}

	return &paymentOutput{
		TxHash:    &result.TxHash,
		ToAddress: &address,
		Status:    database.Pending, // Confirmed later by monitor worker
	}, nil
}

// recordRedemptionTransaction creates a Transaction record for the redemption.
func (s *Service) recordRedemptionTransaction(
	ctx context.Context,
	cardID string,
	req RedeemCardRequest,
	pay *paymentOutput,
	now time.Time,
) (*database.Transaction, error) {
	method := string(req.Method)
	tx := &database.Transaction{
		ID:               uuid.New().String(),
		CardID:           cardID,
		Type:             database.Redeem,
		RedemptionMethod: &method,
		TxHash:           pay.TxHash,
		PaymentHash:      pay.PaymentHash,
		PaymentPreimage:  pay.PaymentPreimage,
		LightningInvoice: pay.Invoice,
		ToAddress:        pay.ToAddress,
		BTCAmountSats:    req.AmountSats,
		Status:           pay.Status,
		Confirmations:    0,
		CreatedAt:        now,
		BroadcastAt:      &now,
		ConfirmedAt:      pay.ConfirmedAt,
	}

	if err := s.txRepo.Create(ctx, tx); err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	return tx, nil
}

// updateCardBalance deducts the spend amount and marks the card redeemed if balance is zero.
func (s *Service) updateCardBalance(ctx context.Context, cardID string, currentBalance, spendAmount int64) (int64, error) {
	remaining := currentBalance - spendAmount
	status := database.Active
	var redeemedAt *time.Time

	if remaining == 0 {
		status = database.Redeemed
		t := time.Now().UTC()
		redeemedAt = &t
	}

	if err := s.cardRepo.Update(ctx, cardID, status, &remaining, nil, redeemedAt); err != nil {
		return 0, fmt.Errorf("failed to update card: %w", err)
	}

	return remaining, nil
}

// publishMonitorTransaction publishes a MonitorTransactionMessage so a worker
// can track on-chain confirmations and update the transaction status.
func (s *Service) publishMonitorTransaction(ctx context.Context, cardID, txID, txHash string, amountSats int64, destAddr string) {
	msg := messages.MonitorTransactionMessage{
		CardID:             cardID,
		TxHash:             txHash,
		ExpectedAmountSats: amountSats,
		DestinationAddr:    destAddr,
	}

	msgJSON, err := msg.ToJSON()
	if err != nil {
		logger.Error("Failed to serialize MonitorTransactionMessage",
			zap.String("card_id", cardID),
			zap.String("tx_id", txID),
			zap.Error(err),
		)
		return
	}

	if _, err := s.queue.Publish(ctx, "monitor_tx", msgJSON); err != nil {
		logger.Error("Failed to publish MonitorTransactionMessage",
			zap.String("card_id", cardID),
			zap.String("tx_hash", txHash),
			zap.Error(err),
		)
	} else {
		logger.Info("Published MonitorTransactionMessage",
			zap.String("card_id", cardID),
			zap.String("tx_hash", txHash),
		)
	}
}

// GetCardByCode retrieves card details by redemption code.
func (s *Service) GetCardByCode(ctx context.Context, code string) (*database.Card, error) {
	card, err := s.cardRepo.GetByCode(ctx, code)
	if err != nil {
		if errors.Is(err, database.ErrCardNotFound) {
			return nil, ErrCardNotFound
		}
		return nil, fmt.Errorf("failed to get card: %w", err)
	}
	return card, nil
}

// GetCardBalance returns the remaining balance (in satoshis) for a card.
// In the custodial model, this is simply the btc_amount_sats field in the database.
func (s *Service) GetCardBalance(ctx context.Context, cardID string) (int64, error) {
	card, err := s.cardRepo.GetByID(ctx, cardID)
	if err != nil {
		if errors.Is(err, database.ErrCardNotFound) {
			return 0, ErrCardNotFound
		}
		return 0, fmt.Errorf("failed to get card: %w", err)
	}
	return card.BTCAmountSats, nil
}

// ValidateCardCode checks if a card code is valid and usable.
// Returns the card status without sensitive information.
func (s *Service) ValidateCardCode(ctx context.Context, code string) (database.CardStatus, error) {
	card, err := s.cardRepo.GetByCode(ctx, code)
	if err != nil {
		if errors.Is(err, database.ErrCardNotFound) {
			return database.Expired, ErrCardNotFound
		}
		return database.Expired, fmt.Errorf("failed to validate card: %w", err)
	}
	return card.Status, nil
}

// Helper function to generate a unique card code
// Format: GIFT-XXXX-YYYY-ZZZZ (16 alphanumeric characters in groups)
func (s *Service) generateCardCode(ctx context.Context) (string, error) {
	// Character set excluding visually similar characters (O, 0, I, 1, L)
	const charset = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	const codeLength = 16

	for attempt := 0; attempt < 5; attempt++ {
		// Generate 16 random characters
		code := make([]byte, codeLength)
		if _, err := rand.Read(code); err != nil {
			return "", fmt.Errorf("failed to generate random bytes: %w", err)
		}
		for i := range code {
			code[i] = charset[int(code[i])%len(charset)]
		}

		// Format as GIFT-XXXX-YYYY-ZZZZ
		codeStr := string(code)
		formattedCode := fmt.Sprintf("GIFT-%s-%s-%s",
			codeStr[0:4],
			codeStr[4:8],
			codeStr[8:12],
		)

		// Check uniqueness in database
		_, err := s.cardRepo.GetByCode(ctx, formattedCode)
		if err != nil {
			if errors.Is(err, database.ErrCardNotFound) {
				// Code is unique, return it
				return formattedCode, nil
			}
			// Other database error
			return "", fmt.Errorf("failed to check code uniqueness: %w", err)
		}
		// Code exists, retry
	}

	return "", errors.New("failed to generate unique card code after 5 attempts")
}
