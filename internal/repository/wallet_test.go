package repository

import (
	"context"
	"errors"
	"sync"
	"testing"

	"jungle_gaming/internal/domain/ledger"
	"jungle_gaming/internal/domain/money"
	"jungle_gaming/internal/domain/wallet"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := "postgres://jungle:jungle_local@localhost:5432/jungle_gaming?sslmode=disable"
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("conectar no banco: %v", err)
	}
	return pool
}

func TestConcurrentDebits_80_80_over_100(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	repo := NewWalletRepository(pool)

	walletID := uuid.Must(uuid.NewV7())
	playerID := uuid.Must(uuid.NewV7())

	_, err := pool.Exec(ctx,
		`INSERT INTO wallets (id, player_id, balance, currency, version, created_at, updated_at)
		 VALUES ($1, $2, $3, 'BRL', 1, NOW(), NOW())`,
		walletID, playerID, 10000,
	)
	if err != nil {
		t.Fatalf("setup carteira: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM wallet_ledger_entries WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM wallets WHERE id = $1`, walletID)

	amount80, err := money.NewMoney("80.00", "BRL")
	if err != nil {
		t.Fatalf("setup money: %v", err)
	}
	t.Logf("TESTE walletID=%s", walletID)

	debit := func() error {
		return repo.DebitWallet(ctx, walletID, amount80, uuid.Must(uuid.NewV7()))
	}

	var wg sync.WaitGroup
	results := make([]error, 2)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx] = debit()
		}(i)
	}
	wg.Wait()

	var sucessos, rejeicoes int
	for _, err := range results {
		switch {
		case err == nil:
			sucessos++
		case errors.Is(err, ErrInsufficientBalance), errors.Is(err, wallet.ErrInsufficientBalance):
			rejeicoes++
		default:
			t.Fatalf("erro inesperado: %v", err)
		}
	}
	if sucessos != 1 || rejeicoes != 1 {
		t.Fatalf("esperava 1 sucesso e 1 rejeicao, obtive %d sucessos e %d rejeicoes", sucessos, rejeicoes)
	}

	var finalBalance int64
	err = pool.QueryRow(ctx, `SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&finalBalance)
	if err != nil {
		t.Fatalf("ler saldo final: %v", err)
	}
	if finalBalance != 2000 {
		t.Errorf("saldo final = %d centavos, esperado 2000 (20.00)", finalBalance)
	}

	var ledgerCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM wallet_ledger_entries WHERE wallet_id = $1 AND movement_type = $2`,
		walletID, string(ledger.TypeDebit),
	).Scan(&ledgerCount)
	if err != nil {
		t.Fatalf("contar ledger: %v", err)
	}
	if ledgerCount != 1 {
		t.Errorf("lancamentos de debito = %d, esperado 1", ledgerCount)
	}
}
