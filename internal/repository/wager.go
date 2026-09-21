package repository

import (
	"context"
	"errors"
	"jungle_gaming/internal/domain/money"
	"jungle_gaming/internal/domain/wager"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrTransactionNotFound = errors.New("transação não encontrada")
)

type wagerTransactionRow struct {
	ID                             uuid.UUID
	Kind                           wager.Kind
	Status                         wager.Status
	PlayerID                       uuid.UUID
	WalletID                       uuid.UUID
	Amount                         int64
	Currency                       string
	ProviderID                     *string // podem ser null
	ExternalTransactionID          *string
	IdempotencyKey                 *string
	PayloadHash                    *string
	RoundID                        *string
	GameID                         *string
	ReferenceExternalTransactionID *string
	ReferenceTransactionID         *uuid.UUID
	FailureCode                    *string
	ResultBalance                  *int64
	CreatedAt                      time.Time
	UpdatedAt                      time.Time
}

type WagerTransactionRepository struct {
	pool *pgxpool.Pool
}

func NewWagerTransactionRepository(p *pgxpool.Pool) *WagerTransactionRepository {
	return &WagerTransactionRepository{pool: p}
}

func (r *WagerTransactionRepository) Insert(ctx context.Context, tx pgx.Tx, wt wager.WagerTransaction) error {
	row := toWagerRow(wt)
	_, err := tx.Exec(ctx,
		`INSERT INTO transactions (id, kind, status, player_id, wallet_id, amount, currency,
            provider_id, external_transaction_id, idempotency_key, payload_hash, round_id, game_id,
            reference_external_transaction_id, reference_transaction_id, failure_code, result_balance)
         VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		row.ID, row.Kind, row.Status, row.PlayerID, row.WalletID, row.Amount, row.Currency,
		row.ProviderID, row.ExternalTransactionID, row.IdempotencyKey, row.PayloadHash, row.RoundID, row.GameID,
		row.ReferenceExternalTransactionID, row.ReferenceTransactionID, row.FailureCode, row.ResultBalance,
	)
	return err
}

func (r *WagerTransactionRepository) FindByExternalTransactionId(ctx context.Context, externalTxID string) (wager.WagerTransaction, error) {
	var row wagerTransactionRow

	err := r.pool.QueryRow(ctx,
		`SELECT id, kind, status, player_id, wallet_id, amount, currency,
		        provider_id, external_transaction_id, idempotency_key, payload_hash,
		        round_id, game_id, reference_external_transaction_id,
		        reference_transaction_id, failure_code, result_balance
		   FROM transactions
		  WHERE external_transaction_id = $1`,
		externalTxID,
	).Scan(
		&row.ID, &row.Kind, &row.Status, &row.PlayerID, &row.WalletID, &row.Amount, &row.Currency,
		&row.ProviderID, &row.ExternalTransactionID, &row.IdempotencyKey, &row.PayloadHash,
		&row.RoundID, &row.GameID, &row.ReferenceExternalTransactionID,
		&row.ReferenceTransactionID, &row.FailureCode, &row.ResultBalance,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return wager.WagerTransaction{}, ErrTransactionNotFound
		}
		return wager.WagerTransaction{}, err
	}

	input, err := row.toRestoreInput()
	if err != nil {
		return wager.WagerTransaction{}, err
	}
	return wager.RestoreWagerTransaction(input)
}

func (r *WagerTransactionRepository) FindByProviderID(ctx context.Context, providerID string) (wager.WagerTransaction, error) {
	var row wagerTransactionRow

	err := r.pool.QueryRow(ctx,
		`SELECT id, kind, status, player_id, wallet_id, amount, currency,
		        provider_id, external_transaction_id, idempotency_key, payload_hash,
		        round_id, game_id, reference_external_transaction_id,
		        reference_transaction_id, failure_code, result_balance
		   FROM transactions
		  WHERE provider_id = $1 `,
		providerID,
	).Scan(
		&row.ID, &row.Kind, &row.Status, &row.PlayerID, &row.WalletID, &row.Amount, &row.Currency,
		&row.ProviderID, &row.ExternalTransactionID, &row.IdempotencyKey, &row.PayloadHash,
		&row.RoundID, &row.GameID, &row.ReferenceExternalTransactionID,
		&row.ReferenceTransactionID, &row.FailureCode, &row.ResultBalance,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return wager.WagerTransaction{}, ErrTransactionNotFound
		}
		return wager.WagerTransaction{}, err
	}

	input, err := row.toRestoreInput()
	if err != nil {
		return wager.WagerTransaction{}, err
	}
	return wager.RestoreWagerTransaction(input)
}

func (r *WagerTransactionRepository) UpdateStatus(ctx context.Context, tx pgx.Tx, status wager.Status, id uuid.UUID) error {
	res, err := tx.Exec(ctx, `UPDATE transactions SET status = $1 , updated_at = NOW() WHERE id = $2`, status, id)

	if err != nil {
		return err
	}

	if res.RowsAffected() == 0 {
		return ErrTransactionNotFound
	}

	return nil
}

func toWagerRow(wt wager.WagerTransaction) wagerTransactionRow {
	return wagerTransactionRow{
		ID:                             wt.ID(),
		Kind:                           wt.Kind(),
		Status:                         wt.Status(),
		PlayerID:                       wt.PlayerID(),
		WalletID:                       wt.WalletID(),
		Amount:                         wt.Amount().Cents(),
		Currency:                       wt.Amount().Currency(),
		ProviderID:                     nilIfEmpty(wt.ProviderID()),
		ExternalTransactionID:          nilIfEmpty(wt.ExternalTrID()),
		IdempotencyKey:                 nilIfEmpty(wt.IdempotencyKey()),
		PayloadHash:                    nilIfEmpty(wt.PayloadHash()),
		RoundID:                        nilIfEmpty(wt.RoundID()),
		GameID:                         nilIfEmpty(wt.GameID()),
		ReferenceExternalTransactionID: nilIfEmpty(wt.ReferenceExternalID()),
		ReferenceTransactionID:         nilIfNilUUID(wt.ReferenceInternalID()),
		FailureCode:                    nilIfEmpty(wt.FailureCode()),
		ResultBalance:                  resultBalancePtr(wt.ResultBalance()),
		CreatedAt:                      wt.CreatedAt(),
		UpdatedAt:                      wt.UpdatedAt(),
	}
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nilIfNilUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func resultBalancePtr(m money.Money) *int64 {
	if m.Currency() == "" {
		return nil
	}
	c := m.Cents()
	return &c
}

func (row wagerTransactionRow) toRestoreInput() (wager.RestoreWagerTransactionInput, error) {
	amount, err := money.RestoreMoney(row.Amount, row.Currency)
	if err != nil {
		return wager.RestoreWagerTransactionInput{}, err
	}

	var resultBalance money.Money
	if row.ResultBalance != nil {
		resultBalance, err = money.RestoreMoney(*row.ResultBalance, row.Currency)
		if err != nil {
			return wager.RestoreWagerTransactionInput{}, err
		}
	}

	return wager.RestoreWagerTransactionInput{
		Id:                  row.ID,
		ExternalTrId:        derefString(row.ExternalTransactionID),
		ProviderID:          derefString(row.ProviderID),
		IdempotencyKey:      derefString(row.IdempotencyKey),
		WalletId:            row.WalletID,
		PlayerId:            row.PlayerID,
		GameId:              derefString(row.GameID),
		RoundID:             derefString(row.RoundID),
		ReferenceExternalId: derefString(row.ReferenceExternalTransactionID),
		ReferenceInternalId: derefUUID(row.ReferenceTransactionID),
		PayloadHash:         derefString(row.PayloadHash),
		Kind:                row.Kind,
		Status:              row.Status,
		FailureCode:         derefString(row.FailureCode),
		Amount:              amount,
		ResultBalance:       resultBalance,
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
	}, nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefUUID(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return *id
}
