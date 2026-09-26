package usecase

import (
	"context"
	"errors"
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
	ErrInvalidKind        = errors.New("tipo invalido")
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
	if in.Kind == wager.KindOpening {
		return wager.WagerTransaction{}, ErrInvalidKind
	}

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
	// Carregar a carteira
	userWallet, err := w.wallets.GetWallet(ctx, tx, in.WalletID)
	if err != nil {
		return wager.WagerTransaction{}, err
	}
	// Criar transação
	trID := uuid.Must(uuid.NewV7())
	var newTransaction wager.WagerTransaction
	newTransaction, err = createTransactionDefault(in, payloadToStringHash)
	if err != nil {
		return wager.WagerTransaction{}, err
	}
	// Tratar tipos de operacao
	switch in.Kind {

	case wager.KindBet:
		entry, err := userWallet.Debit(in.Amount, trID)
		if errors.Is(err, wallet.ErrInsufficientBalance) {
			if err := newTransaction.MarkAsRejected("INSUFFICIENT_BALANCE"); err != nil {
				return wager.WagerTransaction{}, err
			}
			return w.saveTransaction(ctx, tx, newTransaction)
		}

		if err != nil {
			return wager.WagerTransaction{}, err
		}

		if err := w.wallets.UpdateWallet(ctx, tx, in.WalletID, userWallet.Balance().Cents(), trID, entry); err != nil {
			return wager.WagerTransaction{}, err
		}

		if err := newTransaction.MarkAsProcessed(userWallet.Balance()); err != nil {
			return wager.WagerTransaction{}, err
		}
		return w.saveTransaction(ctx, tx, newTransaction)

	case wager.KindLoss:
		if err := newTransaction.MarkAsProcessed(userWallet.Balance()); err != nil {
			return wager.WagerTransaction{}, err
		}
		return w.saveTransaction(ctx, tx, newTransaction)

	case wager.KindRefund:

		// busca a transação de origem
		refundTransaction, err := w.transactions.FindByExternalTransactionIdAndProviderId(ctx, in.ReferenceExternalID, in.ProviderID)
		if errors.Is(err, repository.ErrTransactionNotFound) {
			if err := newTransaction.MarkAsPendingReference(); err != nil {
				return wager.WagerTransaction{}, err
			}
			if err := newTransaction.BindReferenceExternalID(in.ReferenceExternalID); err != nil {
				return wager.WagerTransaction{}, err
			}
			return w.saveTransaction(ctx, tx, newTransaction)
		}

		if err != nil {
			return wager.WagerTransaction{}, err
		}

		// verifica se essa transação ja não foi estornada
		_, err = w.transactions.FindByReferenceIdAndStatus(ctx, refundTransaction.ID().String(), wager.StatusProcessed)
		if errors.Is(err, repository.ErrTransactionNotFound) {
			entry, err := userWallet.Credit(refundTransaction.Amount(), trID)
			if err != nil {
				return wager.WagerTransaction{}, err
			}
			if err := newTransaction.MarkAsProcessed(userWallet.Balance()); err != nil {
				return wager.WagerTransaction{}, err
			}
			if err := w.wallets.UpdateWallet(ctx, tx, in.WalletID, userWallet.Balance().Cents(), trID, entry); err != nil {
				return wager.WagerTransaction{}, err
			}
			if err := newTransaction.BindReferenceInternalID(refundTransaction.ID()); err != nil {
				return wager.WagerTransaction{}, err
			}
			return w.saveTransaction(ctx, tx, newTransaction)
		}

		if err != nil {
			return wager.WagerTransaction{}, err
		}

		if err := newTransaction.MarkAsRejected("DUPLICATED_TRANSACTION"); err != nil {
			return wager.WagerTransaction{}, err
		}

		return w.saveTransaction(ctx, tx, newTransaction)
	// case wager.KindRollback:
	// 	refundTransaction, err := w.transactions.FindByExternalTransactionIdAndProviderId(ctx, in.ExternalTransactionID, in.ProviderID)
	// 	if err != nil {
	// 		if errors.Is(err, repository.ErrTransactionNotFound) {
	// 			err = newTransaction.MarkAsPendingReference()
	// 			if err != nil {
	// 				return wager.WagerTransaction{}, err
	// 			}
	// 		}
	// 		err := newTransaction.MarkAsFailed("SERVER_ERROR")
	// 		if err != nil {
	// 			return wager.WagerTransaction{}, err
	// 		}
	// 	}

	// 	switch refundTransaction.Kind() {
	// 	case wager.KindBet:
	// 		entry, err = userWallet.Credit(refundTransaction.Amount(), trID)
	// 		if err != nil {
	// 			return wager.WagerTransaction{}, err
	// 		}

	// 		err = newTransaction.MarkAsProcessed(userWallet.Balance())
	// 		if err != nil {
	// 			return wager.WagerTransaction{}, err
	// 		}

	// 	case wager.KindRefund, wager.KindWin:
	// 		entry, err = userWallet.Debit(in.Amount, trID)
	// 		if err != nil {
	// 			if errors.Is(err, wallet.ErrInsufficientBalance) {
	// 				err := newTransaction.MarkAsRejected("INSUFFICIENT_BALANCE")
	// 				if err == nil {
	// 					return w.saveTransaction(ctx, tx, newTransaction)
	// 				}
	// 			}
	// 			err := newTransaction.MarkAsFailed("SERVER_ERROR")
	// 			if err != nil {
	// 				return wager.WagerTransaction{}, err
	// 			}
	// 		}
	// 		err = newTransaction.MarkAsProcessed(userWallet.Balance())
	// 		if err != nil {
	// 			return wager.WagerTransaction{}, err
	// 		}
	// 	default:
	// 		return wager.WagerTransaction{}, err
	// 	}

	case wager.KindWin:
		entry, err := userWallet.Credit(in.Amount, trID)
		if errors.Is(err, wallet.ErrNonPositiveAmount) {
			if err := newTransaction.MarkAsRejected("INVALID_VALUE"); err != nil {
				return wager.WagerTransaction{}, err
			}
			return w.saveTransaction(ctx, tx, newTransaction)
		}

		if err != nil {
			return wager.WagerTransaction{}, err
		}

		if err := w.wallets.UpdateWallet(ctx, tx, in.WalletID, userWallet.Balance().Cents(), trID, entry); err != nil {
			return wager.WagerTransaction{}, err
		}

		if err := newTransaction.MarkAsProcessed(userWallet.Balance()); err != nil {
			return wager.WagerTransaction{}, err
		}
		return w.saveTransaction(ctx, tx, newTransaction)

	default:
		return wager.WagerTransaction{}, err
	}
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
