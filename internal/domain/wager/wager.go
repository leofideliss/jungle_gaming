package wager

import (
	"errors"
	"jungle_gaming/internal/domain/money"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidKind           = errors.New("tipo invalido")
	ErrMissingExternalFields = errors.New("dados externos invalidos para o tipo")
	ErrMissingInternalData   = errors.New("dados internos invalidos para o tipo")
	ErrInvalidID             = errors.New("id invalido")
	ErrInvalidStatus         = errors.New("status invalido")

	ErrInvalidChangeStatus = errors.New("operacao nao permitida nesse status")
)

type Kind string

const (
	KindOpening  Kind = "OPENING"
	KindBet      Kind = "BET"
	KindWin      Kind = "WIN"
	KindLoss     Kind = "LOSS"
	KindRefund   Kind = "REFUND"
	KindRollback Kind = "ROLLBACK"
)

type Status string

const (
	StatusPending          Status = "PENDING"
	StatusPendingReference Status = "PENDING_REFERENCE"
	StatusProcessed        Status = "PROCESSED"
	StatusRejected         Status = "REJECTED"
	StatusFailed           Status = "FAILED"
)

type WagerTransaction struct {
	id           uuid.UUID
	externalTrId string

	providerID          string
	idempotencyKey      string
	walletId            uuid.UUID
	playerId            uuid.UUID
	gameId              string
	roundID             string
	referenceExternalId string
	referenceInternalId uuid.UUID

	payloadHash string

	kind        Kind
	status      Status
	failureCode string

	amount        money.Money
	resultBalance money.Money

	createdAt time.Time
	updatedAt time.Time
}

type NewWagerTransactionInput struct {
	ExternalTrId        string
	ProviderID          string
	IdempotencyKey      string
	WalletId            uuid.UUID
	PlayerId            uuid.UUID
	GameId              string
	RoundID             string
	ReferenceExternalId string
	PayloadHash         string
	Kind                Kind
	Amount              money.Money
}

type NewWagerTransactionOpenInput struct {
	WalletId            uuid.UUID
	PlayerId            uuid.UUID
	ReferenceExternalId string
	Amount              money.Money
}

type RestoreWagerTransactionInput struct {
	Id                  uuid.UUID
	ExternalTrId        string
	ProviderID          string
	IdempotencyKey      string
	WalletId            uuid.UUID
	PlayerId            uuid.UUID
	GameId              string
	RoundID             string
	ReferenceExternalId string
	ReferenceInternalId uuid.UUID
	PayloadHash         string
	Kind                Kind
	Status              Status
	FailureCode         string
	Amount              money.Money
	ResultBalance       money.Money
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func NewWagerTransaction(wgInput NewWagerTransactionInput) (WagerTransaction, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return WagerTransaction{}, err
	}

	switch wgInput.Kind {
	case KindBet, KindLoss, KindRefund, KindRollback, KindWin:
		break
	default:
		return WagerTransaction{}, ErrInvalidKind
	}

	if wgInput.ProviderID == "" || wgInput.ExternalTrId == "" || wgInput.IdempotencyKey == "" || wgInput.PayloadHash == "" || wgInput.RoundID == "" || wgInput.GameId == "" {
		return WagerTransaction{}, ErrMissingExternalFields
	}

	timeNow := time.Now().UTC()
	return WagerTransaction{
		id:                  id,
		externalTrId:        wgInput.ExternalTrId,
		providerID:          wgInput.ProviderID,
		idempotencyKey:      wgInput.IdempotencyKey,
		walletId:            wgInput.WalletId,
		playerId:            wgInput.PlayerId,
		gameId:              wgInput.GameId,
		roundID:             wgInput.RoundID,
		referenceExternalId: wgInput.ReferenceExternalId,

		payloadHash: wgInput.PayloadHash,
		kind:        wgInput.Kind,
		status:      StatusPending,
		amount:      wgInput.Amount,
		createdAt:   timeNow,
		updatedAt:   timeNow,
	}, nil
}

func NewWagerTransactionKindOpen(walletId uuid.UUID, playerId uuid.UUID, amount money.Money) (WagerTransaction, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return WagerTransaction{}, err
	}

	if walletId == uuid.Nil || playerId == uuid.Nil || !amount.IsPositive() {
		return WagerTransaction{}, ErrMissingExternalFields
	}

	timeNow := time.Now().UTC()
	return WagerTransaction{
		id:        id,
		walletId:  walletId,
		playerId:  playerId,
		kind:      KindOpening,
		status:    StatusPending,
		amount:    amount,
		createdAt: timeNow,
		updatedAt: timeNow,
	}, nil
}

func RestoreWagerTransaction(in RestoreWagerTransactionInput) (WagerTransaction, error) {
	if in.Id == uuid.Nil {
		return WagerTransaction{}, ErrInvalidID
	}

	switch in.Kind {
	case KindBet, KindLoss, KindOpening, KindRefund, KindRollback, KindWin:
	default:
		return WagerTransaction{}, ErrInvalidKind
	}

	switch in.Status {
	case StatusPending, StatusPendingReference, StatusProcessed, StatusRejected, StatusFailed:
	default:
		return WagerTransaction{}, ErrInvalidStatus
	}

	return WagerTransaction{
		id:                  in.Id,
		externalTrId:        in.ExternalTrId,
		providerID:          in.ProviderID,
		idempotencyKey:      in.IdempotencyKey,
		walletId:            in.WalletId,
		playerId:            in.PlayerId,
		gameId:              in.GameId,
		roundID:             in.RoundID,
		referenceExternalId: in.ReferenceExternalId,
		referenceInternalId: in.ReferenceInternalId,
		payloadHash:         in.PayloadHash,
		kind:                in.Kind,
		status:              in.Status,
		failureCode:         in.FailureCode,
		amount:              in.Amount,
		resultBalance:       in.ResultBalance,
		createdAt:           in.CreatedAt,
		updatedAt:           in.UpdatedAt,
	}, nil
}

func (w *WagerTransaction) MarkAsProcessed(resultBalance money.Money) error {

	switch w.status {
	case StatusPending, StatusPendingReference:
	default:
		return ErrInvalidChangeStatus
	}

	w.resultBalance = resultBalance
	w.updatedAt = time.Now().UTC()
	w.status = StatusProcessed
	return nil
}

func (w *WagerTransaction) MarkAsRejected(failureCode string) error {
	switch w.status {
	case StatusPending, StatusPendingReference:
	default:
		return ErrInvalidChangeStatus
	}

	w.failureCode = failureCode
	w.updatedAt = time.Now().UTC()
	w.status = StatusRejected
	return nil
}

func (w *WagerTransaction) MarkAsPendingReference() error {
	switch w.status {
	case StatusPending:
	default:
		return ErrInvalidChangeStatus
	}

	w.status = StatusPendingReference
	w.updatedAt = time.Now().UTC()
	return nil
}

func (w *WagerTransaction) MarkAsFailed(failureCode string) error {
	switch w.status {
	case StatusPending, StatusPendingReference:
	default:
		return ErrInvalidChangeStatus
	}

	w.status = StatusFailed
	w.updatedAt = time.Now().UTC()
	w.failureCode = failureCode
	return nil
}

func (w WagerTransaction) ID() uuid.UUID                  { return w.id }
func (w WagerTransaction) ExternalTrID() string           { return w.externalTrId }
func (w WagerTransaction) ProviderID() string             { return w.providerID }
func (w WagerTransaction) IdempotencyKey() string         { return w.idempotencyKey }
func (w WagerTransaction) WalletID() uuid.UUID            { return w.walletId }
func (w WagerTransaction) PlayerID() uuid.UUID            { return w.playerId }
func (w WagerTransaction) GameID() string                 { return w.gameId }
func (w WagerTransaction) RoundID() string                { return w.roundID }
func (w WagerTransaction) ReferenceExternalID() string    { return w.referenceExternalId }
func (w WagerTransaction) ReferenceInternalID() uuid.UUID { return w.referenceInternalId }
func (w WagerTransaction) PayloadHash() string            { return w.payloadHash }
func (w WagerTransaction) Kind() Kind                     { return w.kind }
func (w WagerTransaction) Status() Status                 { return w.status }
func (w WagerTransaction) FailureCode() string            { return w.failureCode }
func (w WagerTransaction) Amount() money.Money            { return w.amount }
func (w WagerTransaction) ResultBalance() money.Money     { return w.resultBalance }
func (w WagerTransaction) CreatedAt() time.Time           { return w.createdAt }
func (w WagerTransaction) UpdatedAt() time.Time           { return w.updatedAt }
