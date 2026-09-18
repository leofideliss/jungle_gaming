package ledger

import (
	"errors"
	"jungle_gaming/internal/domain/money"

	"github.com/google/uuid"
)

type Direction string

const (
	TypeCredit Direction = "CREDIT"
	TypeDebit  Direction = "DEBIT"
)

var (
	ErrInvalidType         = errors.New("tipo de movimento invalido")
	ErrInconsistentBalance = errors.New("saldo inconsistente")
)

type WalletLedger struct {
	id            uuid.UUID
	walletId      uuid.UUID
	transactionId uuid.UUID
	movementType  Direction
	beforeAmount  money.Money
	amount        money.Money
	afterAmount   money.Money
}

func NewWalletLedger(wlId uuid.UUID, trId uuid.UUID, movement Direction, bfAmount, amount money.Money) (WalletLedger, error) {
	afterAmount, err := calculateAfterBalance(bfAmount, amount, movement)
	if err != nil {
		return WalletLedger{}, err
	}

	uid, err := uuid.NewV7()
	if err != nil {
		return WalletLedger{}, err
	}

	return WalletLedger{
		id:            uid,
		walletId:      wlId,
		transactionId: trId,
		movementType:  movement,
		beforeAmount:  bfAmount,
		amount:        amount,
		afterAmount:   afterAmount,
	}, nil

}

func calculateAfterBalance(bfAmount, amount money.Money, movement Direction) (money.Money, error) {
	var expectedAmount money.Money
	var err error
	switch movement {
	case TypeCredit:
		expectedAmount, err = bfAmount.Add(amount)
	case TypeDebit:
		expectedAmount, err = bfAmount.Sub(amount)
	default:
		return money.Money{}, ErrInvalidType
	}
	if err != nil {
		return money.Money{}, err
	}

	return expectedAmount, nil
}

func RestoreWalletLedger(id uuid.UUID, wlId uuid.UUID, trId uuid.UUID, movement Direction, bfAmount, amount, afAmount money.Money) (WalletLedger, error) {
	expectedAmount, err := calculateAfterBalance(bfAmount, amount, movement)
	if err != nil {
		return WalletLedger{}, err
	}

	value, err := expectedAmount.Compare(afAmount)
	if err != nil {
		return WalletLedger{}, err
	}
	if value != 0 {
		return WalletLedger{}, ErrInconsistentBalance
	}

	return WalletLedger{
		id:            id,
		walletId:      wlId,
		transactionId: trId,
		movementType:  movement,
		beforeAmount:  bfAmount,
		amount:        amount,
		afterAmount:   afAmount,
	}, nil

}

func (w WalletLedger) AmountAfterOperation() money.Money {
	return w.afterAmount
}

func (w WalletLedger) BeforeAmount() money.Money {
	return w.beforeAmount
}

func (w WalletLedger) Amount() money.Money {
	return w.amount
}

func (w WalletLedger) ID() uuid.UUID {
	return w.id
}

func (w WalletLedger) WalletID() uuid.UUID {
	return w.walletId
}

func (w WalletLedger) TransactionID() uuid.UUID {
	return w.transactionId
}

func (w WalletLedger) MovementType() Direction {
	return w.movementType
}
