package card

import (
	messages "btc-giftcard/internal/queue"
	streams "btc-giftcard/pkg/queue"

	"btc-giftcard/internal/crypto"
	"btc-giftcard/internal/database"
	"btc-giftcard/internal/wallet"
	"btc-giftcard/pkg/logger"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcutil"
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
type Service struct {
	cardRepo      *database.CardRepository
	txRepo        *database.TransactionRepository
	encryptionKey []byte // Master key for encrypting private keys (from env/KMS)
	network       string // "testnet" or "mainnet"
	queue         *streams.StreamQueue
}

// NewService creates a new card service instance
func NewService(
	cardRepo *database.CardRepository,
	txRepo *database.TransactionRepository,
	encryptionKey []byte,
	network string,
	queue *streams.StreamQueue,
) *Service {
	return &Service{
		cardRepo:      cardRepo,
		txRepo:        txRepo,
		encryptionKey: encryptionKey,
		network:       network,
		queue:         queue,
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
	WalletAddress string
	BTCAmountSats int64
	Status        database.CardStatus
	CreatedAt     time.Time
}

// CreateCard generates a new gift card with a unique wallet and encrypted private key.
// This implements the first part of the purchase flow from the README.
func (s *Service) CreateCard(ctx context.Context, req CreateCardRequest) (*CreateCardResponse, error) {
	// 1. Generate a unique card code
	code, err := s.generateCardCode(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate card code: %w", err)
	}

	// 2. Generate new Bitcoin wallet
	btcWallet, err := wallet.GenerateWallet(s.network)
	if err != nil {
		return nil, fmt.Errorf("failed to generate wallet: %w", err)
	}

	// 3. Encrypt the private key
	encryptedPrivKey, err := crypto.Encrypt(btcWallet.PrivateKey, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt private key: %w", err)
	}

	// 4. Create Card struct
	// Note: BTCAmountSats is set to 0 here and will be calculated by the funding worker
	// based on the current exchange rate when the card is actually funded.
	card := &database.Card{
		ID:                 uuid.New().String(),
		UserID:             req.UserID,
		PurchaseEmail:      req.PurchaseEmail,
		OwnerEmail:         req.PurchaseEmail,
		Code:               code,
		WalletAddress:      btcWallet.Address,
		EncryptedPrivKey:   encryptedPrivKey,
		BTCAmountSats:      0, // Will be set by funding worker based on current BTC price
		FiatAmountCents:    req.FiatAmountCents,
		FiatCurrency:       req.FiatCurrency,
		PurchasePriceCents: req.PurchasePriceCents,
		Status:             database.Created,
		CreatedAt:          time.Now().UTC(),
	}

	// 5. Save card to database
	err = s.cardRepo.Create(ctx, card)
	if err != nil {
		if errors.Is(err, database.ErrCardCodeExists) {
			// This should be extremely rare due to code uniqueness check
			return nil, fmt.Errorf("card code collision (unexpected): %w", err)
		}
		if errors.Is(err, database.ErrCardAddressExists) {
			// This should be virtually impossible with cryptographic key generation
			return nil, fmt.Errorf("wallet address collision (critical): %w", err)
		}
		return nil, fmt.Errorf("failed to save card: %w", err)
	}

	// 6. Publish FundCardMessage to queue (don't fail card creation if this fails)
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

	// 7. Return response (DO NOT include encrypted private key)
	return &CreateCardResponse{
		CardID:        card.ID,
		Code:          card.Code,
		WalletAddress: card.WalletAddress,
		BTCAmountSats: card.BTCAmountSats,
		Status:        card.Status,
		CreatedAt:     card.CreatedAt,
	}, nil
}

// RedeemCardRequest contains the parameters for redeeming a card
type RedeemCardRequest struct {
	Code               string // Card redemption code
	DestinationAddress string // User's Bitcoin address to send funds to
}

// RedeemCardResponse contains the redemption transaction details
type RedeemCardResponse struct {
	TransactionID   string
	TxHash          string // Bitcoin transaction hash
	BTCAmountSats   int64
	Status          database.TransactionStatus
	DestinationAddr string
	BroadcastAt     time.Time
}

// RedeemCard processes a card redemption by sending Bitcoin to the user's address.
// This implements the redemption flow from the README.
func (s *Service) RedeemCard(ctx context.Context, req RedeemCardRequest) (*RedeemCardResponse, error) {
	// TODO: Implement card redemption logic
	// Steps:
	// 1. Validate destination address using wallet.ValidateAddress()
	//    - Return ErrInvalidAddress if validation fails
	isValid, err := wallet.ValidateAddress(req.DestinationAddress, s.network)
	if err != nil {
		return nil, fmt.Errorf("failed destination validation address: %w", err)
	}
	if !isValid {
		return nil, fmt.Errorf("destination address is invalid: %s", req.DestinationAddress)
	}

	// 2. Retrieve card from database using cardRepo.GetByCode()
	//    - Return ErrCardNotFound if card doesn't exist
	card, err := s.cardRepo.GetByCode(ctx, req.Code)
	if err != nil {
		return nil, err
	}

	// 3. Validate card status
	//    - Must be Active status (not Created, Funding, Redeemed, or Expired)
	//    - Return ErrCardNotActive if not active
	//    - Return ErrCardAlreadyUsed if status is Redeemed
	if card.Status == database.Redeemed {
		return nil, ErrCardAlreadyUsed
	}

	if card.Status != database.Active {
		return nil, ErrCardNotActive
	}

	// 4. Decrypt private key using crypto.Decrypt()
	//    - Convert encrypted key to []byte
	//    - Store decrypted WIF in memory (SECURITY: clear after use!)
	DecryptedPrivKey, err := crypto.Decrypt(card.EncryptedPrivKey, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt private key %w", err)
	}

	// 5. Import wallet using wallet.ImportWalletFromWIF()
	//    - Use decrypted WIF
	//    - Handle errors (corrupted key, invalid format)
	btcWallet, err := wallet.ImportWalletFromWIF(DecryptedPrivKey, s.network)
	if err != nil {
		return nil, fmt.Errorf("failed to import wallet from WIF private key %w", err)
	}

	// 6. Get UTXOs for card's wallet address
	//    - Use wallet.GetUTXOs(card.WalletAddress)
	//    - Calculate total available balance
	//    - Return ErrInsufficientFunds if balance < card.BTCAmountSats
	balance, err := btcWallet.GetBalance()
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet balance %w", err)
	}

	if balance < btcutil.Amount(card.BTCAmountSats) {
		return nil, ErrInsufficientFunds
	}

	// 7. Create redemption transaction
	//    - Create transaction sending to req.DestinationAddress
	//    - Amount: card.BTCAmountSats minus network fee
	//    - Calculate appropriate fee (query fee rate from blockchain API)
	//    - Sign transaction with imported wallet
	//
	// 8. Broadcast transaction to Bitcoin network
	//    - Get transaction hash
	//    - Handle broadcast errors (network issues, invalid tx)
	//
	// 9. Create Transaction record in database
	//    - ID: uuid.New().String()
	//    - CardID: card.ID
	//    - Type: database.Redeem
	//    - TxHash: transaction hash from broadcast
	//    - FromAddress: card.WalletAddress
	//    - ToAddress: req.DestinationAddress
	//    - BTCAmountSats: amount sent (minus fees)
	//    - Status: database.Pending
	//    - Confirmations: 0
	//    - CreatedAt: time.Now().UTC()
	//    - BroadcastAt: time.Now().UTC()
	//    - ConfirmedAt: nil
	//
	// 10. Update card status to Redeemed
	//     - Set status to database.Redeemed
	//     - Set RedeemedAt to time.Now().UTC()
	//     - Use cardRepo.Update()
	//
	// 11. SECURITY: Clear private key from memory
	//     - Zero out the WIF string
	//     - Clear any byte arrays containing key material
	//
	// 12. Return RedeemCardResponse with transaction details
	//
	// Error handling:
	// - Use database transactions for atomicity
	// - Rollback on any failure
	// - Always clear private key from memory (defer statement)

	return nil, errors.New("not implemented")
}

// GetCardByCode retrieves card details by redemption code.
// Does NOT return the private key.
func (s *Service) GetCardByCode(ctx context.Context, code string) (*database.Card, error) {
	// TODO: Implement card retrieval
	// Steps:
	// 1. Use cardRepo.GetByCode() to retrieve card
	// 2. Return ErrCardNotFound if not found
	// 3. SECURITY: Ensure encrypted private key is NOT exposed in API responses
	//    (handled by API layer, but good to document here)

	return nil, errors.New("not implemented")
}

// GetCardBalance queries the blockchain for the current balance of a card's wallet.
// This may differ from btc_amount_sats if there have been partial redemptions
// or additional deposits.
func (s *Service) GetCardBalance(ctx context.Context, cardID string) (int64, error) {
	// TODO: Implement balance check
	// Steps:
	// 1. Retrieve card from database using cardRepo.GetByID()
	// 2. Query blockchain UTXOs using wallet.GetUTXOs(card.WalletAddress)
	// 3. Sum up all UTXO values to get current balance in satoshis
	// 4. Return total balance
	//
	// Note: This is the ACTUAL on-chain balance, which may differ from
	// card.BTCAmountSats if:
	// - Funding transaction has more/less than expected
	// - Someone sent additional BTC to the address
	// - Partial redemption occurred

	return 0, errors.New("not implemented")
}

// ValidateCardCode checks if a card code is valid and usable.
// Returns the card status without sensitive information.
func (s *Service) ValidateCardCode(ctx context.Context, code string) (database.CardStatus, error) {
	// TODO: Implement card validation
	// Steps:
	// 1. Retrieve card using cardRepo.GetByCode()
	// 2. Return ErrCardNotFound if card doesn't exist
	// 3. Return card status (Created, Funding, Active, Redeemed, Expired)
	//
	// Usage:
	// - API can call this to check card status before redemption
	// - Mobile app can validate code as user types
	// - Merchant POS can verify card is valid before accepting

	return database.Expired, errors.New("not implemented")
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
