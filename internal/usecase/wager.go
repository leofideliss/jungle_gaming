package usecase

import (
	"context"
	"errors"
	"jungle_gaming/internal/domain/ledger"
	"jungle_gaming/internal/domain/money"
	"jungle_gaming/internal/domain/wager"
	"jungle_gaming/internal/domain/wallet"
	"jungle_gaming/internal/idempotency"
	"jungle_gaming/internal/repository"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidResult      = errors.New("resultado inválido")
	ErrInvalidIdempotency = errors.New("chaves iguais corpo diferente")
)

type ProcessWagerInput struct {
	ProviderID            string
	ExternalTransactionID string
	ReferenceExternalID   string
	IdempotencyKey        string
	PlayerID              uuid.UUID
	WalletID              uuid.UUID
	RoundID               string
	GameID                string
	Kind                  wager.Kind
	Amount                money.Money
}

type WagerUseCase struct {
	pool         *pgxpool.Pool
	wallets      *repository.WalletRepository
	transactions *repository.WagerTransactionRepository
	outbox       *repository.OutboxRepository
	idempotency  *idempotency.IdempotencyService
}

func NewWagerUseCase(p *pgxpool.Pool, w *repository.WalletRepository, tr *repository.WagerTransactionRepository, o *repository.OutboxRepository, i *idempotency.IdempotencyService) *WagerUseCase {
	return &WagerUseCase{
		pool:         p,
		wallets:      w,
		transactions: tr,
		outbox:       o,
		idempotency:  i,
	}
}

func (w *WagerUseCase) ProcessWagerTransaction(in ProcessWagerInput) (wager.WagerTransaction, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	payloadHash := idempotency.NewPayloadHash(in.Kind, in.PlayerID, in.WalletID, in.RoundID, in.GameID, in.ExternalTransactionID, in.Amount.String())
	payloadToStringHash, err := payloadHash.CalculatePayloadHash()
	if err != nil {
		return wager.WagerTransaction{}, err

	}

	result, err := w.idempotency.Check(ctx, in.ProviderID, in.ExternalTransactionID, payloadToStringHash)
	if err != nil {
		return wager.WagerTransaction{}, err

	}

	switch result.Result {
	case idempotency.StatusReplay:
		return result.Tr, nil
	case idempotency.StatusConflict:
		return result.Tr, ErrInvalidIdempotency
	case idempotency.StatusNew:
	default:
		return wager.WagerTransaction{}, ErrInvalidResult
	}

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return wager.WagerTransaction{}, err
	}
	defer tx.Rollback(ctx)

	newTransaction, err := w.processTransaction(ctx, tx, in, payloadToStringHash)
	if err != nil {
		return wager.WagerTransaction{}, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return wager.WagerTransaction{}, err
	}

	return newTransaction, nil
}

func (w *WagerUseCase) processTransaction(ctx context.Context, tx pgx.Tx, in ProcessWagerInput, payloadToStringHash string) (wager.WagerTransaction, error) {
	var entry ledger.WalletLedger
	var err error

	userWallet, err := w.wallets.GetWallet(ctx, tx, in.WalletID)
	if err != nil {
		return wager.WagerTransaction{}, err
	}
	trID := uuid.Must(uuid.NewV7())
	var newTransaction wager.WagerTransaction
	if in.Kind == wager.KindOpening {
		newTransaction, err = createTransactionOpening(in)
	} else {
		newTransaction, err = createTransactionDefault(in, payloadToStringHash)
	}
	if err != nil {
		return wager.WagerTransaction{}, err
	}

	switch in.Kind {
	case wager.KindBet:
		entry, err = userWallet.Debit(in.Amount, trID)
		if err != nil {
			if errors.Is(err, wallet.ErrInsufficientBalance) {
				err := newTransaction.MarkAsRejected("INSUFFICIENT_BALANCE")
				if err == nil {
					return w.saveTransaction(ctx, tx, newTransaction)
				}
			}
			return wager.WagerTransaction{}, err
		}
		err = w.wallets.UpdateWallet(ctx, tx, in.WalletID, userWallet.Balance().Cents(), trID, entry)
		if err != nil {
			return wager.WagerTransaction{}, err
		}
		err = newTransaction.MarkAsProcessed(userWallet.Balance())
		if err != nil {
			return wager.WagerTransaction{}, err
		}
		return w.saveTransaction(ctx, tx, newTransaction)
	case wager.KindLoss:
		return w.saveTransaction(ctx, tx, newTransaction)

	case wager.KindOpening:
		entry, err = userWallet.Credit(in.Amount, trID)
		if err != nil {
			return wager.WagerTransaction{}, err
		}
		err = w.wallets.UpdateWallet(ctx, tx, in.WalletID, userWallet.Balance().Cents(), trID, entry)
		if err != nil {
			return wager.WagerTransaction{}, err
		}
	case wager.KindRefund:
		refundTransaction, err := w.transactions.FindByExternalTransactionIdAndProviderId(ctx, in.ExternalTransactionID, in.ProviderID)
		if err != nil {
			if errors.Is(err, repository.ErrTransactionNotFound) {
				err = newTransaction.MarkAsPendingReference()
				if err == nil {
					return w.saveTransaction(ctx, tx, newTransaction)

				}
			}
			return wager.WagerTransaction{}, err
		}

		_, err = w.transactions.FindByTrAndStatus(ctx, refundTransaction.ID().String(), wager.KindRefund)
		if err != nil {
			if errors.Is(err, repository.ErrTransactionNotFound) {
				entry, err = userWallet.Credit(refundTransaction.Amount(), trID)
				if err != nil {
					return wager.WagerTransaction{}, err

				}
				err = w.wallets.UpdateWallet(ctx, tx, in.WalletID, userWallet.Balance().Cents(), trID, entry)
				if err != nil {
					return wager.WagerTransaction{}, err
				}
			}
			return wager.WagerTransaction{}, err
		}
		err = newTransaction.MarkAsProcessed(userWallet.Balance())
		if err != nil {
			return wager.WagerTransaction{}, err
		}
		return w.saveTransaction(ctx, tx, newTransaction)

	case wager.KindRollback:

	case wager.KindWin:
		entry, err = userWallet.Credit(in.Amount, trID)
		if err != nil {
			return wager.WagerTransaction{}, err
		}
		err = w.wallets.UpdateWallet(ctx, tx, in.WalletID, userWallet.Balance().Cents(), trID, entry)
		if err != nil {
			return wager.WagerTransaction{}, err
		}
		return w.saveTransaction(ctx, tx, newTransaction)
	}

	return w.saveTransaction(ctx, tx, newTransaction)
}

func (w *WagerUseCase) saveTransaction(ctx context.Context, tx pgx.Tx, newTransaction wager.WagerTransaction) (wager.WagerTransaction, error) {
	err := w.transactions.Insert(ctx, tx, newTransaction)
	if err != nil {
		return wager.WagerTransaction{}, err
	}
	return newTransaction, nil
}

func createTransactionDefault(in ProcessWagerInput, payloadToStringHash string) (wager.WagerTransaction, error) {
	transactionInput := wager.NewWagerTransactionInput{
		ExternalTrId:        in.ExternalTransactionID,
		ProviderID:          in.ProviderID,
		IdempotencyKey:      in.IdempotencyKey,
		WalletId:            in.WalletID,
		PlayerId:            in.PlayerID,
		GameId:              in.GameID,
		RoundID:             in.RoundID,
		ReferenceExternalId: in.ReferenceExternalID,
		PayloadHash:         payloadToStringHash,
		Kind:                in.Kind,
		Amount:              in.Amount,
	}
	return wager.NewWagerTransaction(transactionInput)
}

func createTransactionOpening(in ProcessWagerInput) (wager.WagerTransaction, error) {
	return wager.NewWagerTransactionKindOpen(in.WalletID, in.PlayerID, in.Amount)
}
