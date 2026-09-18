package wallet

import (
	"errors"
	"jungle_gaming/internal/domain/ledger"
	"jungle_gaming/internal/domain/money"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidID                 = errors.New("Identificador invalido")
	ErrInvalidVersion            = errors.New("Version invalido")
	ErrInitialValueNegative      = errors.New("Valor inicial negativo")
	ErrNonPositiveAmount         = errors.New("Valor deve ser maior que zero")
	ErrInitialValueZero          = errors.New("Valor inicial zero")
	ErrCurrencyInWalletIsNotSame = errors.New("Moeda da carteira diverge da transacao")
	ErrInsufficientBalance       = errors.New("Saldo insuficiente")
)

type Wallet struct {
	id        uuid.UUID
	playerId  uuid.UUID
	balance   money.Money
	version   int64
	createdAt time.Time
	updatedAt time.Time
}

func NewWallet(pid string, b money.Money) (Wallet, error) {
	playerID, err := uuid.Parse(pid)
	if err != nil {
		return Wallet{}, ErrInvalidID
	}

	if playerID == uuid.Nil {
		return Wallet{}, ErrInvalidID
	}

	uid, err := uuid.NewV7()
	if err != nil {
		return Wallet{}, err
	}

	if b.IsNegative() {
		return Wallet{}, ErrInitialValueNegative
	}

	timeNow := time.Now().UTC()
	return Wallet{
		id:        uid,
		playerId:  playerID,
		balance:   b,
		version:   1,
		createdAt: timeNow,
		updatedAt: timeNow,
	}, nil

}

func RestoreWallet(id uuid.UUID, pid uuid.UUID, b money.Money, v int64, cr time.Time, up time.Time) (Wallet, error) {
	if id == uuid.Nil {
		return Wallet{}, ErrInvalidID
	}

	if pid == uuid.Nil {
		return Wallet{}, ErrInvalidID
	}

	if v < 1 {
		return Wallet{}, ErrInvalidVersion
	}

	return Wallet{
		id:        id,
		playerId:  pid,
		balance:   b,
		version:   v,
		createdAt: cr,
		updatedAt: up,
	}, nil
}

func (w *Wallet) Credit(m money.Money, trId uuid.UUID) (ledger.WalletLedger, error) {
	var walletLedger ledger.WalletLedger

	if !m.IsPositive() {
		return walletLedger, ErrNonPositiveAmount
	}

	walletLedger, err := ledger.NewWalletLedger(w.id, trId, ledger.TypeCredit, w.balance, m)
	if err != nil {
		return walletLedger, err
	}

	newBalance := walletLedger.AmountAfterOperation()
	w.balance = newBalance
	w.version++
	w.updatedAt = time.Now().UTC()

	return walletLedger, nil
}

func (w *Wallet) Debit(m money.Money, trId uuid.UUID) (ledger.WalletLedger, error) {
	var walletLedger ledger.WalletLedger

	if !m.IsPositive() {
		return walletLedger, ErrNonPositiveAmount
	}

	resp, err := w.balance.Compare(m)
	if err != nil {
		return walletLedger, err

	}

	if resp < 0 {
		return walletLedger, ErrInsufficientBalance
	}

	walletLedger, err = ledger.NewWalletLedger(w.id, trId, ledger.TypeDebit, w.balance, m)
	if err != nil {
		return walletLedger, err
	}

	newBalance := walletLedger.AmountAfterOperation()
	w.balance = newBalance
	w.version++
	w.updatedAt = time.Now().UTC()

	return walletLedger, nil
}

func (w Wallet) Balance() money.Money {
	return w.balance
}

func (w Wallet) Version() int64 {
	return w.version
}
