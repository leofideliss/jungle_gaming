package worker

import (
	"context"
	"testing"

	"jungle_gaming/internal/repository"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(),
		"postgres://jungle:jungle_local@localhost:5432/jungle_gaming?sslmode=disable")
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	return pool
}

func seedWallet(t *testing.T, pool *pgxpool.Pool, balanceCents int64) (uuid.UUID, uuid.UUID) {
	t.Helper()
	walletID := uuid.Must(uuid.NewV7())
	playerID := uuid.Must(uuid.NewV7())
	_, err := pool.Exec(context.Background(),
		`INSERT INTO wallets (id, player_id, balance, currency, version, created_at, updated_at)
		 VALUES ($1, $2, $3, 'BRL', 1, NOW(), NOW())`,
		walletID, playerID, balanceCents)
	if err != nil {
		t.Fatalf("seed wallet: %v", err)
	}
	return walletID, playerID
}

func seedTransaction(t *testing.T, pool *pgxpool.Pool, walletID, playerID uuid.UUID, kind, status string, amountCents int64, extTxID, providerID string) uuid.UUID {
	t.Helper()
	txID := uuid.Must(uuid.NewV7())
	_, err := pool.Exec(context.Background(),
		`INSERT INTO transactions (id, kind, status, player_id, wallet_id, amount, currency,
			provider_id, external_transaction_id, idempotency_key, payload_hash,
			round_id, game_id, attempts, next_attempt_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, 'BRL', $7, $8, $9, $10, $11, $12, 0, NOW(), NOW(), NOW())`,
		txID, kind, status, playerID, walletID, amountCents,
		providerID, extTxID,
		"idem-"+extTxID, "hash-"+extTxID,
		"round-1", "game-1",
	)
	if err != nil {
		t.Fatalf("seed transaction: %v", err)
	}
	return txID
}

func seedPendingReference(t *testing.T, pool *pgxpool.Pool, walletID, playerID uuid.UUID, kind string, amountCents int64, extTxID, providerID, refExtTxID string, attempts int) uuid.UUID {
	t.Helper()
	txID := uuid.Must(uuid.NewV7())
	_, err := pool.Exec(context.Background(),
		`INSERT INTO transactions (id, kind, status, player_id, wallet_id, amount, currency,
			provider_id, external_transaction_id, idempotency_key, payload_hash,
			round_id, game_id, reference_external_transaction_id, attempts, next_attempt_at, created_at, updated_at)
		 VALUES ($1, $2, 'PENDING_REFERENCE', $3, $4, $5, 'BRL', $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW(), NOW())`,
		txID, kind, playerID, walletID, amountCents,
		providerID, extTxID,
		"idem-"+extTxID, "hash-"+extTxID,
		"round-1", "game-1",
		refExtTxID, attempts,
	)
	if err != nil {
		t.Fatalf("seed pending reference: %v", err)
	}
	return txID
}

func cleanup(t *testing.T, pool *pgxpool.Pool, walletID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	pool.Exec(ctx, `DELETE FROM outbox WHERE aggregate_id = $1`, walletID)
	pool.Exec(ctx, `DELETE FROM wallet_ledger_entries WHERE wallet_id = $1`, walletID)
	pool.Exec(ctx, `DELETE FROM transactions WHERE wallet_id = $1`, walletID)
	pool.Exec(ctx, `DELETE FROM wallets WHERE id = $1`, walletID)
}

func newWorker(pool *pgxpool.Pool) *ProcessPendingTr {
	return NewProcessPendingTr(
		pool,
		repository.NewWagerTransactionRepository(pool),
		repository.NewWalletRepository(pool),
	)
}

func TestPendingReference_ResolvesWhenReferenceFound(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()
	w := newWorker(pool)

	walletID, playerID := seedWallet(t, pool, 10000) // 100.00
	defer cleanup(t, pool, walletID)

	// 1. cria o BET original (a referência que o REFUND procura)
	betExtID := "ext-bet-ref-001"
	seedTransaction(t, pool, walletID, playerID, "BET", "PROCESSED", 5000, betExtID, "provider-a")

	// 2. cria o REFUND como PENDING_REFERENCE (referenciando o BET)
	refundExtID := "ext-refund-001"
	refundID := seedPendingReference(t, pool, walletID, playerID, "REFUND", 5000, refundExtID, "provider-a", betExtID, 0)

	// 3. roda o worker UMA vez
	w.process(context.Background())

	// 4. verifica: REFUND virou PROCESSED
	var status string
	pool.QueryRow(context.Background(),
		`SELECT status FROM transactions WHERE id = $1`, refundID).Scan(&status)
	if status != "PROCESSED" {
		t.Errorf("status = %s, esperado PROCESSED", status)
	}

	// 5. verifica: saldo creditado (100.00 + 50.00 = 150.00)
	var balance int64
	pool.QueryRow(context.Background(),
		`SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&balance)
	if balance != 15000 {
		t.Errorf("saldo = %d, esperado 15000", balance)
	}

	// 6. verifica: um lançamento CREDIT no ledger
	var count int
	pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM wallet_ledger_entries WHERE wallet_id = $1 AND movement_type = 'CREDIT'`,
		walletID).Scan(&count)
	if count != 1 {
		t.Errorf("lançamentos de crédito = %d, esperado 1", count)
	}
}

func TestPendingReference_IncreasesAttemptsWhenNotFound(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()
	w := newWorker(pool)

	walletID, playerID := seedWallet(t, pool, 10000)
	defer cleanup(t, pool, walletID)

	// cria REFUND pendente com referência que NÃO existe
	refundExtID := "ext-refund-retry"
	refundID := seedPendingReference(t, pool, walletID, playerID, "REFUND", 5000, refundExtID, "provider-a", "ext-bet-inexistente", 0)

	// roda o worker
	w.process(context.Background())

	// verifica: ainda PENDING_REFERENCE, attempts incrementou
	var status string
	var attempts int
	pool.QueryRow(context.Background(),
		`SELECT status, attempts FROM transactions WHERE id = $1`, refundID).Scan(&status, &attempts)

	if status != "PENDING_REFERENCE" {
		t.Errorf("status = %s, esperado PENDING_REFERENCE", status)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, esperado 1", attempts)
	}

	// saldo intacto
	var balance int64
	pool.QueryRow(context.Background(),
		`SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&balance)
	if balance != 10000 {
		t.Errorf("saldo = %d, esperado 10000 (intacto)", balance)
	}
}

func TestPendingReference_RejectsWhenMaxAttemptsExceeded(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()
	w := newWorker(pool)

	walletID, playerID := seedWallet(t, pool, 10000)
	defer cleanup(t, pool, walletID)

	// cria REFUND pendente já com 9 tentativas (próxima é a 10ª = maxAttempts)
	refundExtID := "ext-refund-max"
	refundID := seedPendingReference(t, pool, walletID, playerID, "REFUND", 5000, refundExtID, "provider-a", "ext-bet-inexistente", 9)
	// roda o worker
	w.process(context.Background())

	// verifica: virou REJECTED
	var status, failureCode string
	pool.QueryRow(context.Background(),
		`SELECT status, failure_code FROM transactions WHERE id = $1`, refundID).Scan(&status, &failureCode)

	if status != "REJECTED" {
		t.Errorf("status = %s, esperado REJECTED", status)
	}

	// saldo intacto
	var balance int64
	pool.QueryRow(context.Background(),
		`SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&balance)
	if balance != 10000 {
		t.Errorf("saldo = %d, esperado 10000 (intacto)", balance)
	}
}
