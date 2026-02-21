package card

import (
	messages "btc-giftcard/internal/queue"
	streams "btc-giftcard/pkg/queue"

	"btc-giftcard/internal/database"
	"btc-giftcard/pkg/logger"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Custom errors for card operations
var (
	ErrCardNotFound      = errors.New("card not found")
	ErrCardNotActive     = errors.New("card is not active")
	ErrCardAlreadyUsed   = errors.New("card has already been redeemed")
	ErrInvalidAddress    = errors.New("invalid bitcoin address")
	ErrInsufficientFunds = errors.New("insufficient funds on card")
)

// Service handles gift card business logic
// TODO: Add lndClient field of type lnd.LightningClient (the interface from internal/lnd/client.go)
//   - This enables Lightning payments (card redemption via invoice)
//   - This enables on-chain sends (card redemption to BTC address)
//   - This enables treasury balance checks (prevent overselling)
//   - Inject via NewService constructor; nil-check at startup
type Service struct {
	cardRepo *database.CardRepository
	txRepo   *database.TransactionRepository
	network  string // "testnet" or "mainnet"
	queue    *streams.StreamQueue
	// TODO: Add the following field:
	// lndClient lnd.LightningClient
}

// NewService creates a new card service instance
// TODO: Add lndClient lnd.LightningClient parameter
//   - Store in s.lndClient
//   - Update all callers (cmd/api/main.go, tests) to pass the LND client
//   - For tests, create a mock implementation of LightningClient interface
func NewService(
	cardRepo *database.CardRepository,
	txRepo *database.TransactionRepository,
	network string,
	queue *streams.StreamQueue,
) *Service {
	return &Service{
		cardRepo: cardRepo,
		txRepo:   txRepo,
		network:  network,
		queue:    queue,
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

// RedeemCardRequest contains the parameters for redeeming (spending) a card
type RedeemCardRequest struct {
	Code               string // Card redemption code
	Method             string // "lightning" or "onchain"
	AmountSats         int64  // Amount to spend (can be partial)
	DestinationAddress string // On-chain Bitcoin address (required if method=onchain)
	LightningInvoice   string // BOLT11 invoice (required if method=lightning)
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
	// ========================================================================
	// STEP 1: Validate redemption method
	// ========================================================================
	// - req.Method must be "lightning" or "onchain"
	// - If "lightning": req.LightningInvoice must be non-empty, req.DestinationAddress ignored
	// - If "onchain": req.DestinationAddress must be non-empty, req.LightningInvoice ignored
	// - Return ErrInvalidAddress or a new ErrInvalidMethod for invalid input

	// ========================================================================
	// STEP 2: Retrieve and validate card
	// ========================================================================
	// card, err := s.cardRepo.GetByCode(ctx, req.Code)
	// - If ErrCardNotFound → return ErrCardNotFound
	// - If card.Status != Active → return ErrCardNotActive
	//   (Created = not yet funded, Redeemed = already fully spent, Expired = expired)
	// - If req.AmountSats > card.BTCAmountSats → return ErrInsufficientFunds
	// - If req.AmountSats <= 0 → return error "amount must be positive"

	// ========================================================================
	// STEP 3: Execute payment via LND
	// ========================================================================
	//
	// --- Lightning path (method == "lightning") ---
	//
	// 3a. Decode and validate the BOLT11 invoice:
	//     decoded, err := s.lndClient.DecodeInvoice(ctx, req.LightningInvoice)
	//     - If err != nil → return fmt.Errorf("invalid invoice: %w", err)
	//     - If decoded.IsExpired → return error "invoice has expired"
	//     - If decoded.AmountSats != req.AmountSats → return error
	//       "invoice amount (%d sats) does not match requested amount (%d sats)"
	//       This prevents the user from submitting a 1-sat invoice for a 100k-sat card
	//     - If decoded.AmountSats == 0 → return error "zero-amount invoices not supported"
	//
	// 3b. Pay the invoice:
	//     result, err := s.lndClient.PayInvoice(ctx, req.LightningInvoice, cfg.MaxPaymentFeeSats)
	//     - If err != nil → return fmt.Errorf("lightning payment failed: %w", err)
	//     - result.PaymentHash and result.PaymentPreimage are the proof of payment
	//     - result.FeeSats is the routing fee (absorbed by us, not deducted from card)
	//
	// 3c. Set transaction fields:
	//     paymentHash = &result.PaymentHash
	//     paymentPreimage = &result.PaymentPreimage
	//     method = "lightning"
	//     txHash = nil  (no on-chain tx)
	//
	// --- On-chain path (method == "onchain") ---
	//
	// 3d. Validate destination address:
	//     - Use wallet.ValidateAddress(req.DestinationAddress, s.network)
	//       or let LND validate it (SendCoins will reject invalid addresses)
	//     - Consider minimum on-chain amount (e.g., 10,000 sats) due to
	//       mining fees making tiny on-chain sends uneconomical
	//
	// 3e. Send on-chain:
	//     result, err := s.lndClient.SendOnChain(ctx, req.DestinationAddress, req.AmountSats)
	//     - If err != nil → return fmt.Errorf("on-chain send failed: %w", err)
	//     - Note: the mining fee is paid by us FROM LND's wallet, not from amountSats
	//       The user receives exactly req.AmountSats
	//
	// 3f. Set transaction fields:
	//     txHash = &result.TxHash
	//     method = "onchain"
	//     paymentHash = nil  (no Lightning payment)
	//     toAddress = &req.DestinationAddress

	// ========================================================================
	// STEP 4: Create Transaction record in database
	// ========================================================================
	// now := time.Now().UTC()
	// tx := &database.Transaction{
	//     ID:               uuid.New().String(),
	//     CardID:           card.ID,
	//     Type:             database.Redeem,
	//     RedemptionMethod: &method,          // "lightning" or "onchain"
	//     TxHash:           txHash,           // On-chain only (nil for Lightning)
	//     PaymentHash:      paymentHash,      // Lightning only (nil for on-chain)
	//     PaymentPreimage:  paymentPreimage,  // Lightning only — PROOF OF PAYMENT
	//     LightningInvoice: lightningInvoice, // Lightning only — the BOLT11 string
	//     ToAddress:        toAddress,        // On-chain only — destination address
	//     BTCAmountSats:    req.AmountSats,
	//     Status:           database.Confirmed,  // Lightning is instant; on-chain pending
	//     Confirmations:    0,
	//     CreatedAt:        now,
	//     BroadcastAt:      &now,             // On-chain: broadcast time; Lightning: payment time
	//     ConfirmedAt:      confirmedAt,      // Lightning: &now; On-chain: nil (set later by monitor)
	// }
	// err = s.txRepo.Create(ctx, tx)
	//
	// NOTE on status:
	//   - Lightning: Status=Confirmed immediately (preimage = proof of settlement)
	//   - On-chain: Status=Pending until confirmations ≥ 1
	//     → Publish MonitorTransactionMessage to track confirmations
	//     → A monitor_tx worker updates status when confirmed

	// ========================================================================
	// STEP 5: Update card balance
	// ========================================================================
	// remainingBalance := card.BTCAmountSats - req.AmountSats
	//
	// if remainingBalance == 0:
	//     // Fully redeemed — mark card as Redeemed
	//     redeemedAt := time.Now().UTC()
	//     s.cardRepo.Update(ctx, card.ID, database.Redeemed, &remainingBalance, nil, &redeemedAt)
	// else:
	//     // Partial spend — card stays Active with reduced balance
	//     s.cardRepo.Update(ctx, card.ID, database.Active, &remainingBalance, nil, nil)
	//
	// NOTE: remainingBalance is ALWAYS ≥ 0 due to the check in Step 2
	// TODO: Consider wrapping Steps 3-5 in a database transaction for atomicity
	//   If the LND payment succeeds but DB update fails, we've paid but not recorded it.
	//   Mitigation: record the Transaction BEFORE paying, with Status=Pending,
	//   then update to Confirmed after LND returns success.

	// ========================================================================
	// STEP 6: Return response
	// ========================================================================
	// return &RedeemCardResponse{
	//     TransactionID:    tx.ID,
	//     Method:           method,
	//     TxHash:           txHash,
	//     PaymentHash:      paymentHash,
	//     BTCAmountSats:    req.AmountSats,
	//     RemainingBalance: remainingBalance,
	//     Status:           tx.Status,
	// }, nil

	return nil, errors.New("not implemented")
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
