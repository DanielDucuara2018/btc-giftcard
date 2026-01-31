package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrTransactionNotFound is returned when a transaction is not found in the database
	ErrTransactionNotFound = errors.New("transaction not found")
)

// TransactionRepository handles all database operations for transactions
type TransactionRepository struct {
	db *pgxpool.Pool
}

// NewTransactionRepository creates a new transaction repository instance
func NewTransactionRepository(db *DB) *TransactionRepository {
	return &TransactionRepository{
		db: db.pool,
	}
}

// Create inserts a new transaction into the database.
// The tx_hash field can be NULL before the transaction is broadcast.
func (r *TransactionRepository) Create(ctx context.Context, tx *Transaction) error {
	query := `INSERT INTO transactions (
		id,
		card_id, 
		type, 
		tx_hash, 
		from_address, 
		to_address,
		btc_amount_sats,
		status,
		confirmations,
		created_at,
		broadcast_at,
		confirmed_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	_, err := r.db.Exec(
		ctx,
		query,
		tx.ID,
		tx.CardID,
		tx.Type.String(),
		tx.TxHash,
		tx.FromAddress,
		tx.ToAddress,
		tx.BTCAmountSats,
		tx.Status.String(),
		tx.Confirmations,
		tx.CreatedAt,
		tx.BroadcastAt,
		tx.ConfirmedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	return nil
}

// GetByID retrieves a transaction by its UUID.
// Returns ErrTransactionNotFound if the ID does not exist.
func (r *TransactionRepository) GetByID(ctx context.Context, id string) (*Transaction, error) {
	query := `SELECT 
		id, card_id, type, tx_hash, from_address, to_address,
		btc_amount_sats, status, confirmations, created_at,
		broadcast_at, confirmed_at
    FROM transactions WHERE id = $1`

	var transaction Transaction
	var typeStr string
	var statusStr string

	err := r.db.QueryRow(ctx, query, id).Scan(
		&transaction.ID,
		&transaction.CardID,
		&typeStr,
		&transaction.TxHash,
		&transaction.FromAddress,
		&transaction.ToAddress,
		&transaction.BTCAmountSats,
		&statusStr,
		&transaction.Confirmations,
		&transaction.CreatedAt,
		&transaction.BroadcastAt,
		&transaction.ConfirmedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTransactionNotFound
		}
		return nil, fmt.Errorf("failed to get transaction with id %s: %w", id, err)
	}

	transaction.Type = ParseTransactionType(typeStr)
	transaction.Status = ParseTransactionStatus(statusStr)
	return &transaction, nil
}

// GetByTxHash retrieves a transaction by its blockchain transaction hash.
// Returns ErrTransactionNotFound if no transaction with that hash exists.
func (r *TransactionRepository) GetByTxHash(ctx context.Context, txHash string) (*Transaction, error) {
	query := `SELECT 
		id, card_id, type, tx_hash, from_address, to_address,
		btc_amount_sats, status, confirmations, created_at,
		broadcast_at, confirmed_at
    FROM transactions WHERE tx_hash = $1`

	var transaction Transaction
	var typeStr string
	var statusStr string

	err := r.db.QueryRow(ctx, query, txHash).Scan(
		&transaction.ID,
		&transaction.CardID,
		&typeStr,
		&transaction.TxHash,
		&transaction.FromAddress,
		&transaction.ToAddress,
		&transaction.BTCAmountSats,
		&statusStr,
		&transaction.Confirmations,
		&transaction.CreatedAt,
		&transaction.BroadcastAt,
		&transaction.ConfirmedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTransactionNotFound
		}
		return nil, fmt.Errorf("failed to get transaction with tx hash %s: %w", txHash, err)
	}

	transaction.Type = ParseTransactionType(typeStr)
	transaction.Status = ParseTransactionStatus(statusStr)
	return &transaction, nil
}

// ListByCardID retrieves all transactions for a specific card, ordered by creation date (newest first).
// Returns an empty slice if the card has no transactions.
func (r *TransactionRepository) ListByCardID(ctx context.Context, cardID string) ([]*Transaction, error) {
	query := `SELECT 
		id, card_id, type, tx_hash, from_address, to_address,
		btc_amount_sats, status, confirmations, created_at,
		broadcast_at, confirmed_at
    FROM transactions WHERE card_id = $1 ORDER BY created_at DESC`

	rows, err := r.db.Query(ctx, query, cardID)
	if err != nil {
		return nil, fmt.Errorf("failed to get transations of card %s: %w", cardID, err)
	}
	defer rows.Close()

	var transactions []*Transaction
	for rows.Next() {
		var transaction Transaction
		var typeStr string
		var statusStr string

		err := rows.Scan(
			&transaction.ID,
			&transaction.CardID,
			&typeStr,
			&transaction.TxHash,
			&transaction.FromAddress,
			&transaction.ToAddress,
			&transaction.BTCAmountSats,
			&statusStr,
			&transaction.Confirmations,
			&transaction.CreatedAt,
			&transaction.BroadcastAt,
			&transaction.ConfirmedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction row: %w", err)
		}

		transaction.Type = ParseTransactionType(typeStr)
		transaction.Status = ParseTransactionStatus(statusStr)
		transactions = append(transactions, &transaction)
	}

	// Check for any errors that occurred during iteration
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error during row iteration: %w", err)
	}

	return transactions, nil
}

// Update updates a transaction's status, confirmation count, and timestamps.
// Uses COALESCE to preserve existing timestamp values when nil is passed.
// Returns ErrTransactionNotFound if the transaction ID does not exist.
func (r *TransactionRepository) Update(ctx context.Context, id string, status TransactionStatus, confirmations int, broadcastAt, confirmedAt *time.Time) error {
	query := `UPDATE transactions 
		SET status = $2,
			confirmations = $3,
			broadcast_at = COALESCE($4, broadcast_at),
			confirmed_at = COALESCE($5, confirmed_at)
		WHERE id = $1`

	commandTag, err := r.db.Exec(ctx, query, id, status.String(), confirmations, broadcastAt, confirmedAt)
	if err != nil {
		return fmt.Errorf("failed to update transaction with id %s: %w", id, err)
	}

	if commandTag.RowsAffected() == 0 {
		return ErrTransactionNotFound
	}

	return nil
}
