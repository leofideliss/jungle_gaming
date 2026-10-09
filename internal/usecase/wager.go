package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"jungle_gaming/internal/domain/event"
	"jungle_gaming/internal/domain/ledger"
	"jungle_gaming/internal/domain/money"
	"jungle_gaming/internal/domain/wager"
	"jungle_gaming/internal/domain/wallet"
	"jungle_gaming/internal/idempotency"
	"jungle_gaming/internal/repository"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidResult      = errors.New("resultado inválido")
	ErrInvalidIdempotency = errors.New("chaves iguais corpo diferente")
	ErrInvalidKind        = errors.New("tipo invalido")
	ErrTrAlreadyRefund    = errors.New("transacao ja estornada para referencia")
	ErrTrAlreadyProcessed = errors.New("transacao ja processada anteriormente")
	ErrNotSameProviderID  = errors.New("operacao invalida")
)

type ProcessWagerInput struct {
	ProviderID            string
	ExternalTransactionID string
	ReferenceExternalID   string
	IdempotencyKey        string
	MessageID             *string
	PlayerID              uuid.UUID
	WalletID              uuid.UUID
	RoundID               string
	GameID                string
	Kind                  wager.Kind
	Amount                money.Money
}

type CreateWalletInput struct {
	PlayerID       uuid.UUID
	InitialBalance money.Money
}

type WagerUseCase struct {
	pool         *pgxpool.Pool
	wallets      *repository.WalletRepository
	transactions *repository.WagerTransactionRepository
	outbox       *repository.OutboxRepository
	inbox        *repository.InboxRepository
	idempotency  *idempotency.IdempotencyService
}

func NewWagerUseCase(p *pgxpool.Pool, w *repository.WalletRepository, tr *repository.WagerTransactionRepository, o *repository.OutboxRepository, i *idempotency.IdempotencyService, in *repository.InboxRepository) *WagerUseCase {
	return &WagerUseCase{
		pool:         p,
		wallets:      w,
		transactions: tr,
		outbox:       o,
		inbox:        in,
		idempotency:  i,
	}
}

func (w *WagerUseCase) ProcessWagerTransaction(ctx context.Context, in ProcessWagerInput) (wager.WagerTransaction, error) {
	if in.Kind == wager.KindOpening {
		return wager.WagerTransaction{}, ErrInvalidKind
	}

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
		if in.ProviderID == result.Tr.ProviderID() {
			return result.Tr, ErrTrAlreadyProcessed
		}
		return result.Tr, ErrNotSameProviderID

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
	// check inbox
	if in.MessageID != nil {
		err := w.inbox.InsertInbox(ctx, tx, *in.MessageID, "sqs-consumer")
		if errors.Is(err, repository.ErrDuplicateRow) {
			existing, err := w.transactions.FindByExternalTransactionIdAndProviderId(ctx, in.ExternalTransactionID, in.ProviderID)
			if err != nil {
				return wager.WagerTransaction{}, err
			}
			return existing, nil
		}
		if err != nil {
			return wager.WagerTransaction{}, err
		}
	}

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

	beforeBalance := userWallet.Balance()
	// Tratar tipos de operacao
	switch in.Kind {
	case wager.KindBet:
		return w.bet(ctx, tx, userWallet, in, trID, newTransaction, beforeBalance)

	case wager.KindLoss:
		if err := newTransaction.MarkAsProcessed(userWallet.Balance()); err != nil {
			return wager.WagerTransaction{}, err
		}
		return w.saveTransaction(ctx, tx, newTransaction)

	case wager.KindRefund:
		return w.refund(ctx, tx, userWallet, in, trID, newTransaction, beforeBalance)

	case wager.KindRollback:
		return w.rollback(ctx, tx, userWallet, in, trID, newTransaction, beforeBalance)

	case wager.KindWin:
		return w.win(ctx, tx, userWallet, in, trID, newTransaction, beforeBalance)

	default:
		return wager.WagerTransaction{}, ErrInvalidKind
	}
}

func (w *WagerUseCase) saveTransaction(ctx context.Context, tx pgx.Tx, newTransaction wager.WagerTransaction) (wager.WagerTransaction, error) {
	err := w.transactions.Insert(ctx, tx, newTransaction)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return wager.WagerTransaction{}, ErrTrAlreadyProcessed
		}
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

func (w *WagerUseCase) outboxTrPending(ctx context.Context, tx pgx.Tx, in ProcessWagerInput, trID uuid.UUID, newTransaction wager.WagerTransaction) (event.Event, error) {
	eventTr := event.NewWagerTransactionPendingReference(in.IdempotencyKey, event.WagerTransactionPendingReferenceData{
		TransactionID:       trID.String(),
		ProviderID:          newTransaction.ProviderID(),
		ReferenceExternalID: newTransaction.ExternalTrID(),
	})
	payloadTr, err := json.Marshal(eventTr)
	if err != nil {
		return event.Event{}, err
	}
	if err := w.outbox.Insert(ctx, tx, eventTr.EventID, trID, eventTr.EventType, eventTr.AggregateType, payloadTr); err != nil {
		return event.Event{}, err
	}
	return eventTr, nil
}

func (w *WagerUseCase) outboxTrRejected(ctx context.Context, tx pgx.Tx, in ProcessWagerInput, trID uuid.UUID, newTransaction wager.WagerTransaction, failureCode string) (event.Event, error) {
	eventTr := event.NewWagerTransactionRejected(in.IdempotencyKey, event.WagerTransactionRejectedData{
		TransactionID: trID.String(),
		Kind:          string(newTransaction.Kind()),
		ProviderID:    newTransaction.ProviderID(),
		FailureCode:   failureCode,
	})
	payloadTr, err := json.Marshal(eventTr)
	if err != nil {
		return event.Event{}, err
	}
	if err := w.outbox.Insert(ctx, tx, eventTr.EventID, trID, eventTr.EventType, eventTr.AggregateType, payloadTr); err != nil {
		return event.Event{}, err
	}
	return eventTr, nil
}

func (w *WagerUseCase) outboxTrProcessed(ctx context.Context, tx pgx.Tx, userWallet wallet.Wallet, in ProcessWagerInput, trID uuid.UUID, newTransaction wager.WagerTransaction) (event.Event, error) {
	eventTr := event.NewWagerTransactionProcessed(in.IdempotencyKey, event.WagerTransactionProcessedData{
		TransactionID: trID.String(),
		Kind:          string(newTransaction.Kind()),
		Status:        string(newTransaction.Status()),
		ProviderID:    newTransaction.ProviderID(),
		PlayerID:      newTransaction.PlayerID().String(),
		WalletID:      newTransaction.WalletID().String(),
		ResultBalance: userWallet.Balance().String(),
	})
	payloadTr, err := json.Marshal(eventTr)
	if err != nil {
		return event.Event{}, err
	}
	if err := w.outbox.Insert(ctx, tx, eventTr.EventID, trID, eventTr.EventType, eventTr.AggregateType, payloadTr); err != nil {
		return event.Event{}, err
	}
	return eventTr, nil
}

func (w *WagerUseCase) outboxUpdateWallet(ctx context.Context, tx pgx.Tx, userWallet wallet.Wallet, in ProcessWagerInput, trID uuid.UUID, beforeBalance money.Money, eventTr *event.Event, moviment ledger.Direction) (event.Event, error) {
	eventWallet := event.NewWalletBalanceChanged(in.IdempotencyKey, &eventTr.EventID, event.WalletBalanceChangedData{
		WalletID:      in.WalletID.String(),
		TransactionID: trID.String(),
		Direction:     string(moviment),
		Amount:        in.Amount.String(),
		Currency:      in.Amount.Currency(),
		BalanceBefore: beforeBalance.String(),
		BalanceAfter:  userWallet.Balance().String(),
		WalletVersion: userWallet.Version(),
	})

	payloadWallet, err := json.Marshal(eventWallet)
	if err != nil {
		return event.Event{}, err
	}
	if err := w.outbox.Insert(ctx, tx, eventWallet.EventID, in.WalletID, eventWallet.EventType, eventWallet.AggregateType, payloadWallet); err != nil {
		return event.Event{}, err
	}

	return eventWallet, nil
}

func (w *WagerUseCase) bet(ctx context.Context, tx pgx.Tx, userWallet wallet.Wallet, in ProcessWagerInput, trID uuid.UUID, newTransaction wager.WagerTransaction, beforeBalance money.Money) (wager.WagerTransaction, error) {
	entry, err := userWallet.Debit(in.Amount, trID)
	if errors.Is(err, wallet.ErrInsufficientBalance) {
		if err := newTransaction.MarkAsRejected("INSUFFICIENT_BALANCE"); err != nil {
			return wager.WagerTransaction{}, err
		}
		_, err = w.outboxTrRejected(ctx, tx, in, trID, newTransaction, "INSUFFICIENT_BALANCE")
		if err != nil {
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

	eventTr, err := w.outboxTrProcessed(ctx, tx, userWallet, in, trID, newTransaction)
	if err != nil {
		return wager.WagerTransaction{}, err
	}

	_, err = w.outboxUpdateWallet(ctx, tx, userWallet, in, trID, beforeBalance, &eventTr, ledger.TypeDebit)
	if err != nil {
		return wager.WagerTransaction{}, err
	}

	return w.saveTransaction(ctx, tx, newTransaction)
}

func (w *WagerUseCase) refund(ctx context.Context, tx pgx.Tx, userWallet wallet.Wallet, in ProcessWagerInput, trID uuid.UUID, newTransaction wager.WagerTransaction, beforeBalance money.Money) (wager.WagerTransaction, error) {
	// busca a transação de origem
	refundTransaction, err := w.transactions.FindByExternalTransactionIdAndProviderId(ctx, in.ReferenceExternalID, in.ProviderID)
	if errors.Is(err, repository.ErrTransactionNotFound) {
		if err := newTransaction.MarkAsPendingReference(); err != nil {
			return wager.WagerTransaction{}, err
		}
		if err := newTransaction.BindReferenceExternalID(in.ReferenceExternalID); err != nil {
			return wager.WagerTransaction{}, err
		}

		_, err := w.outboxTrPending(ctx, tx, in, trID, newTransaction)
		if err != nil {
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

		eventTr, err := w.outboxTrProcessed(ctx, tx, userWallet, in, trID, newTransaction)
		if err != nil {
			return wager.WagerTransaction{}, err
		}

		_, err = w.outboxUpdateWallet(ctx, tx, userWallet, in, trID, beforeBalance, &eventTr, ledger.TypeCredit)
		if err != nil {
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

	_, err = w.outboxTrRejected(ctx, tx, in, trID, newTransaction, "DUPLICATED_TRANSACTION")
	if err != nil {
		return wager.WagerTransaction{}, err
	}

	return w.saveTransaction(ctx, tx, newTransaction)
}

func (w *WagerUseCase) win(ctx context.Context, tx pgx.Tx, userWallet wallet.Wallet, in ProcessWagerInput, trID uuid.UUID, newTransaction wager.WagerTransaction, beforeBalance money.Money) (wager.WagerTransaction, error) {
	entry, err := userWallet.Credit(in.Amount, trID)
	if errors.Is(err, wallet.ErrNonPositiveAmount) {
		if err := newTransaction.MarkAsRejected("INVALID_VALUE"); err != nil {
			return wager.WagerTransaction{}, err
		}
		_, err = w.outboxTrRejected(ctx, tx, in, trID, newTransaction, "INVALID_VALUE")
		if err != nil {
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

	eventTr, err := w.outboxTrProcessed(ctx, tx, userWallet, in, trID, newTransaction)
	if err != nil {
		return wager.WagerTransaction{}, err
	}

	_, err = w.outboxUpdateWallet(ctx, tx, userWallet, in, trID, beforeBalance, &eventTr, ledger.TypeCredit)
	if err != nil {
		return wager.WagerTransaction{}, err
	}
	return w.saveTransaction(ctx, tx, newTransaction)
}

func (w *WagerUseCase) rollback(ctx context.Context, tx pgx.Tx, userWallet wallet.Wallet, in ProcessWagerInput, trID uuid.UUID, newTransaction wager.WagerTransaction, beforeBalance money.Money) (wager.WagerTransaction, error) {
	var entry ledger.WalletLedger
	var err error

	refundTransaction, err := w.transactions.FindByExternalTransactionIdAndProviderId(ctx, in.ReferenceExternalID, in.ProviderID)
	if errors.Is(err, repository.ErrTransactionNotFound) {
		if err := newTransaction.MarkAsPendingReference(); err != nil {
			return wager.WagerTransaction{}, err
		}
		_, err := w.outboxTrPending(ctx, tx, in, trID, newTransaction)
		if err != nil {
			return wager.WagerTransaction{}, err
		}

		return w.saveTransaction(ctx, tx, newTransaction)
	}
	if err != nil {
		return wager.WagerTransaction{}, err
	}

	switch refundTransaction.Kind() {
	case wager.KindBet:
		entry, err = userWallet.Credit(refundTransaction.Amount(), trID)
		if err != nil {
			return wager.WagerTransaction{}, err
		}

	case wager.KindRefund, wager.KindWin:
		entry, err = userWallet.Debit(in.Amount, trID)
		if errors.Is(err, wallet.ErrInsufficientBalance) {
			if err := newTransaction.MarkAsRejected("INSUFFICIENT_BALANCE"); err != nil {
				return wager.WagerTransaction{}, err
			}
			_, err = w.outboxTrRejected(ctx, tx, in, trID, newTransaction, "INSUFFICIENT_BALANCE")
			if err != nil {
				return wager.WagerTransaction{}, err
			}
			return w.saveTransaction(ctx, tx, newTransaction)
		}

		if err != nil {
			return wager.WagerTransaction{}, err
		}

	default:
		return wager.WagerTransaction{}, err
	}

	if err := w.wallets.UpdateWallet(ctx, tx, in.WalletID, userWallet.Balance().Cents(), trID, entry); err != nil {
		return wager.WagerTransaction{}, err
	}

	if err := newTransaction.MarkAsProcessed(userWallet.Balance()); err != nil {
		return wager.WagerTransaction{}, err
	}

	eventTr, err := w.outboxTrProcessed(ctx, tx, userWallet, in, trID, newTransaction)
	if err != nil {
		return wager.WagerTransaction{}, err
	}
	moviment := ledger.TypeDebit
	if refundTransaction.Kind() == wager.KindBet {
		moviment = ledger.TypeCredit
	}
	_, err = w.outboxUpdateWallet(ctx, tx, userWallet, in, trID, beforeBalance, &eventTr, moviment)
	if err != nil {
		return wager.WagerTransaction{}, err
	}

	return w.saveTransaction(ctx, tx, newTransaction)

}

func (w *WagerUseCase) CreateWallet(ctx context.Context, in CreateWalletInput) (wallet.Wallet, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return wallet.Wallet{}, err
	}
	defer tx.Rollback(ctx)

	// 1. cria a wallet no domínio
	newWallet, err := wallet.NewWallet(in.PlayerID.String(), in.InitialBalance)
	if err != nil {
		return wallet.Wallet{}, err
	}

	// 2. persiste a wallet
	if err := w.wallets.InsertWallet(ctx, tx, newWallet); err != nil {
		return wallet.Wallet{}, err
	}

	// 3. credita saldo inicial → gera ledger
	trID := uuid.Must(uuid.NewV7())
	entry, err := newWallet.Credit(in.InitialBalance, trID)
	if err != nil {
		return wallet.Wallet{}, err
	}

	// 4. cria OPENING transaction
	opening, err := wager.NewWagerTransactionKindOpen(newWallet.ID(), in.PlayerID, in.InitialBalance)
	if err != nil {
		return wallet.Wallet{}, err
	}
	opening.MarkAsProcessed(newWallet.Balance())

	// 5. persiste tudo
	if err := w.wallets.UpdateWallet(ctx, tx, newWallet.ID(), newWallet.Balance().Cents(), trID, entry); err != nil {
		return wallet.Wallet{}, err
	}
	if err := w.transactions.Insert(ctx, tx, opening); err != nil {
		return wallet.Wallet{}, err
	}

	// 6. commit
	if err := tx.Commit(ctx); err != nil {
		return wallet.Wallet{}, err
	}

	return newWallet, nil
}
