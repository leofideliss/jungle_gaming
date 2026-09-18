package wallet

import (
	"errors"
	"testing"

	"jungle_gaming/internal/domain/money"

	"github.com/google/uuid"
)

func mustMoney(t *testing.T, amount string) money.Money {
	t.Helper()
	m, err := money.NewMoney(amount, "BRL")
	if err != nil {
		t.Fatalf("setup money %q: %v", amount, err)
	}
	return m
}

func newTestWallet(t *testing.T, initial string) Wallet {
	t.Helper()
	w, err := NewWallet(uuid.Must(uuid.NewV7()).String(), mustMoney(t, initial))
	if err != nil {
		t.Fatalf("setup wallet: %v", err)
	}
	return w
}

func assertBalance(t *testing.T, w Wallet, expected string) {
	t.Helper()
	want := mustMoney(t, expected)
	cmp, err := w.Balance().Compare(want)
	if err != nil {
		t.Fatalf("compare balance: %v", err)
	}
	if cmp != 0 {
		t.Errorf("saldo = %s, esperado %s", w.Balance().String(), want.String())
	}
}

func TestCredit(t *testing.T) {
	w := newTestWallet(t, "100.00")
	_, err := w.Credit(mustMoney(t, "10.00"), uuid.Must(uuid.NewV7()))
	if err != nil {
		t.Fatalf("credit: %v", err)
	}
	assertBalance(t, w, "110.00")
	if w.Version() != 2 {
		t.Errorf("version = %d, esperado 2", w.Version())
	}
}

func TestDebit(t *testing.T) {
	w := newTestWallet(t, "100.00")
	_, err := w.Debit(mustMoney(t, "30.00"), uuid.Must(uuid.NewV7()))
	if err != nil {
		t.Fatalf("debit: %v", err)
	}
	assertBalance(t, w, "70.00")
	if w.Version() != 2 {
		t.Errorf("version = %d, esperado 2", w.Version())
	}
}

func TestDebitInsufficientBalance(t *testing.T) {
	w := newTestWallet(t, "100.00")
	_, err := w.Debit(mustMoney(t, "150.00"), uuid.Must(uuid.NewV7()))
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("esperava ErrInsufficientBalance, recebi %v", err)
	}
	assertBalance(t, w, "100.00")
	if w.Version() != 1 {
		t.Errorf("version = %d, esperado 1 (nao pode mutar apos rejeicao)", w.Version())
	}
}

func TestDebitToZero(t *testing.T) {
	w := newTestWallet(t, "100.00")
	_, err := w.Debit(mustMoney(t, "100.00"), uuid.Must(uuid.NewV7()))
	if err != nil {
		t.Fatalf("debit ate zero deve ser permitido: %v", err)
	}
	assertBalance(t, w, "0.00")
}

func TestCreditRejectsZero(t *testing.T) {
	w := newTestWallet(t, "100.00")
	_, err := w.Credit(mustMoney(t, "0.00"), uuid.Must(uuid.NewV7()))
	if !errors.Is(err, ErrNonPositiveAmount) {
		t.Errorf("esperava ErrNonPositiveAmount, recebi %v", err)
	}
	assertBalance(t, w, "100.00")
}
