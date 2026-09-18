package repository

import (
	"context"
	"errors"
	"time"

	"jungle_gaming/internal/domain/ledger"
	"jungle_gaming/internal/domain/money"
	"jungle_gaming/internal/domain/wallet"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrWalletNotFound      = errors.New("carteira não encontrada")
	ErrInsufficientBalance = errors.New("saldo insuficiente")
)

type walletRow struct {
	id        uuid.UUID
	playerID  uuid.UUID
	currency  string
	balance   int64
	version   int64
	createdAt time.Time
	updatedAt time.Time
}

type WalletRepository struct {
	pool *pgxpool.Pool
}

func NewWalletRepository(pool *pgxpool.Pool) *WalletRepository {
	return &WalletRepository{pool: pool}
}

func (r *WalletRepository) GetWallet(ctx context.Context, id uuid.UUID) (wallet.Wallet, error) {
	var row walletRow

	err := r.pool.QueryRow(ctx,
		`SELECT id, player_id, balance, currency, version, created_at, updated_at FROM wallets WHERE id = $1`,
		id,
	).Scan(
		&row.id,
		&row.playerID,
		&row.balance,
		&row.currency,
		&row.version,
		&row.createdAt,
		&row.updatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return wallet.Wallet{}, ErrWalletNotFound
		}
		return wallet.Wallet{}, err
	}

	balance, err := money.RestoreMoney(row.balance, row.currency)
	if err != nil {
		return wallet.Wallet{}, err
	}
	return wallet.RestoreWallet(
		row.id,
		row.playerID,
		balance,
		row.version,
		row.createdAt,
		row.updatedAt,
	)
}

func (r *WalletRepository) DebitWallet(ctx context.Context, walletID uuid.UUID, amount money.Money, txID uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var row walletRow
	err = tx.QueryRow(ctx,
		`SELECT id, player_id, balance, currency, version, created_at, updated_at
		   FROM wallets WHERE id = $1 FOR UPDATE`,
		walletID,
	).Scan(&row.id, &row.playerID, &row.balance, &row.currency, &row.version, &row.createdAt, &row.updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrWalletNotFound
		}
		return err
	}

	balance, err := money.RestoreMoney(row.balance, row.currency)
	if err != nil {
		return err
	}
	w, err := wallet.RestoreWallet(row.id, row.playerID, balance, row.version, row.createdAt, row.updatedAt)
	if err != nil {
		return err
	}

	entry, err := w.Debit(amount, txID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE wallets SET balance = $1, version = $2, updated_at = NOW() WHERE id = $3`,
		w.Balance().Cents(), w.Version(), walletID,
	)
	if err != nil {
		return err
	}

	if err = r.insertLedgerTx(ctx, tx, entry); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *WalletRepository) insertLedgerTx(ctx context.Context, tx pgx.Tx, e ledger.WalletLedger) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO wallet_ledger_entries
		   (id, wallet_id, transaction_id, movement_type, before_amount, amount, after_amount, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`,
		e.ID(),
		e.WalletID(),
		e.TransactionID(),
		string(e.MovementType()),
		e.BeforeAmount().Cents(),
		e.Amount().Cents(),
		e.AmountAfterOperation().Cents(),
	)
	return err
}
