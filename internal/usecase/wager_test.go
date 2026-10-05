package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"jungle_gaming/internal/domain/money"
	"jungle_gaming/internal/domain/wager"
	"jungle_gaming/internal/idempotency"
	"jungle_gaming/internal/repository"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(),
		"postgres://jungle:jungle_local@localhost:5432/jungle_gaming?sslmode=disable")
	if err != nil {
		t.Fatalf("conectar: %v", err)
	}
	return pool
}

func cleanup(t *testing.T, pool *pgxpool.Pool, walletID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	pool.Exec(ctx, `DELETE FROM outbox WHERE aggregate_id = $1`, walletID)
	pool.Exec(ctx, `DELETE FROM wallet_ledger_entries WHERE wallet_id = $1`, walletID)
	pool.Exec(ctx, `DELETE FROM transactions WHERE wallet_id = $1`, walletID)
	pool.Exec(ctx, `DELETE FROM inbox`)
	pool.Exec(ctx, `DELETE FROM wallets WHERE id = $1`, walletID)
}

func newUseCase(pool *pgxpool.Pool) *WagerUseCase {
	walletRepo := repository.NewWalletRepository(pool)
	txRepo := repository.NewWagerTransactionRepository(pool)
	outboxRepo := repository.NewOutboxRepository(pool)
	inboxRepo := repository.NewInboxRepository(pool)
	idemp := idempotency.NewIdempotencyService(txRepo)
	return NewWagerUseCase(pool, walletRepo, txRepo, outboxRepo, idemp, inboxRepo)
}

func seedWallet(t *testing.T, pool *pgxpool.Pool, balanceCents int64) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	walletID := uuid.Must(uuid.NewV7())
	playerID := uuid.Must(uuid.NewV7())
	_, err := pool.Exec(ctx,
		`INSERT INTO wallets (id, player_id, balance, currency, version, created_at, updated_at)
		 VALUES ($1, $2, $3, 'BRL', 1, NOW(), NOW())`,
		walletID, playerID, balanceCents)
	if err != nil {
		t.Fatalf("seed wallet: %v", err)
	}
	return walletID, playerID
}

func seedBETTransaction(t *testing.T, pool *pgxpool.Pool, walletID, playerID uuid.UUID, amountCents int64) string {
	t.Helper()
	ctx := context.Background()

	txID := uuid.Must(uuid.NewV7())
	providerID := "provider-a"
	extTxID := "ext_tx_" + uuid.Must(uuid.NewV7()).String()
	idempotencyKey := "idem_key_" + uuid.Must(uuid.NewV7()).String()
	payloadHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	roundID := "round_" + uuid.Must(uuid.NewV7()).String()
	gameID := "game_test"

	query := `
		INSERT INTO transactions (
			id, kind, status, player_id, wallet_id, amount, currency,
			provider_id, external_transaction_id, idempotency_key, payload_hash,
			round_id, game_id, created_at, updated_at
		) VALUES (
			$1, 'BET', 'PROCESSED', $2, $3, $4, 'BRL',
			$5, $6, $7, $8,
			$9, $10, NOW(), NOW()
		)`

	_, err := pool.Exec(ctx, query,
		txID, playerID, walletID, amountCents,
		providerID, extTxID, idempotencyKey, payloadHash,
		roundID, gameID,
	)

	if err != nil {
		t.Fatalf("seed BET transaction failed: %v", err)
	}

	return extTxID
}

func TestProcessBet_Success(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	uc := newUseCase(pool)
	walletID, playerID := seedWallet(t, pool, 10000) // 100.00

	// limpa no fim
	defer pool.Exec(ctx, `DELETE FROM wallet_ledger_entries WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM transactions WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM wallets WHERE id = $1`, walletID)

	amount, _ := money.NewMoney("30.00", "BRL")
	in := ProcessWagerInput{
		ProviderID:            "provider-a",
		ExternalTransactionID: "ext-bet-001",
		IdempotencyKey:        "idem-001",
		PlayerID:              playerID,
		WalletID:              walletID,
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  wager.KindBet,
		Amount:                amount,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tr, err := uc.ProcessWagerTransaction(ctx, in)
	if err != nil {
		t.Fatalf("processar bet: %v", err)
	}

	if tr.Status() != wager.StatusProcessed {
		t.Errorf("status = %s, esperado PROCESSED", tr.Status())
	}

	var balance int64
	err = pool.QueryRow(ctx, `SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&balance)
	if err != nil {
		t.Fatalf("ler saldo: %v", err)
	}
	if balance != 7000 {
		t.Errorf("saldo = %d, esperado 7000", balance)
	}

	var count int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM wallet_ledger_entries WHERE wallet_id = $1 AND movement_type = 'DEBIT'`,
		walletID).Scan(&count)
	if err != nil {
		t.Fatalf("contar ledger: %v", err)
	}
	if count != 1 {
		t.Errorf("lançamentos de débito = %d, esperado 1", count)
	}
}

func TestProcessBet_InsufficientBalance(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	uc := newUseCase(pool)
	walletID, playerID := seedWallet(t, pool, 5000) // 50.00 (insuficiente pra 80)

	defer pool.Exec(ctx, `DELETE FROM wallet_ledger_entries WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM transactions WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM wallets WHERE id = $1`, walletID)

	amount, _ := money.NewMoney("80.00", "BRL")
	in := ProcessWagerInput{
		ProviderID:            "provider-a",
		ExternalTransactionID: "ext-bet-002",
		IdempotencyKey:        "idem-002",
		PlayerID:              playerID,
		WalletID:              walletID,
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  wager.KindBet,
		Amount:                amount,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tr, err := uc.ProcessWagerTransaction(ctx, in)
	if err != nil {
		t.Fatalf("processar bet insuficiente nao deveria dar erro tecnico: %v", err)
	}

	if tr.Status() != wager.StatusRejected {
		t.Errorf("status = %s, esperado REJECTED", tr.Status())
	}

	var balance int64
	pool.QueryRow(ctx, `SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&balance)
	if balance != 5000 {
		t.Errorf("saldo = %d, esperado 5000 (intacto)", balance)
	}

	var count int
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM wallet_ledger_entries WHERE wallet_id = $1`, walletID).Scan(&count)
	if count != 0 {
		t.Errorf("ledger deveria estar vazio, tem %d", count)
	}
}

func TestProcessLoss_Success(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	uc := newUseCase(pool)
	walletID, playerID := seedWallet(t, pool, 10000) // 100.00

	// limpa no fim
	defer pool.Exec(ctx, `DELETE FROM wallet_ledger_entries WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM transactions WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM wallets WHERE id = $1`, walletID)

	amount, _ := money.NewMoney("30.00", "BRL")
	in := ProcessWagerInput{
		ProviderID:            "provider-a",
		ExternalTransactionID: "ext-loss-001",
		IdempotencyKey:        "idem-001",
		PlayerID:              playerID,
		WalletID:              walletID,
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  wager.KindLoss,
		Amount:                amount,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tr, err := uc.ProcessWagerTransaction(ctx, in)
	if err != nil {
		t.Fatalf("processar loss: %v", err)
	}

	if tr.Status() != wager.StatusProcessed {
		t.Errorf("status = %s, esperado PROCESSED", tr.Status())
	}

	var balance int64
	err = pool.QueryRow(ctx, `SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&balance)
	if err != nil {
		t.Fatalf("ler saldo: %v", err)
	}
	if balance != 10000 {
		t.Errorf("saldo = %d, esperado 10000", balance)
	}

	var count int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM wallet_ledger_entries WHERE wallet_id = $1 AND movement_type = 'DEBIT'`,
		walletID).Scan(&count)
	if err != nil {
		t.Fatalf("contar ledger: %v", err)
	}
	if count != 0 {
		t.Errorf("lançamentos esperado = %d, esperado 0", count)
	}
}

func TestProcessWin_Success(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	uc := newUseCase(pool)
	walletID, playerID := seedWallet(t, pool, 10000) // 100.00

	// limpa no fim
	defer pool.Exec(ctx, `DELETE FROM wallet_ledger_entries WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM transactions WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM wallets WHERE id = $1`, walletID)

	amount, _ := money.NewMoney("30.00", "BRL")
	in := ProcessWagerInput{
		ProviderID:            "provider-a",
		ExternalTransactionID: "ext-win-001",
		IdempotencyKey:        "idem-001",
		PlayerID:              playerID,
		WalletID:              walletID,
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  wager.KindWin,
		Amount:                amount,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tr, err := uc.ProcessWagerTransaction(ctx, in)
	if err != nil {
		t.Fatalf("processar win: %v", err)
	}

	if tr.Status() != wager.StatusProcessed {
		t.Errorf("status = %s, esperado PROCESSED", tr.Status())
	}

	var balance int64
	err = pool.QueryRow(ctx, `SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&balance)
	if err != nil {
		t.Fatalf("ler saldo: %v", err)
	}
	if balance != 13000 {
		t.Errorf("saldo = %d, esperado 13000", balance)
	}

	var count int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM wallet_ledger_entries WHERE wallet_id = $1 AND movement_type = 'CREDIT'`,
		walletID).Scan(&count)
	if err != nil {
		t.Fatalf("contar ledger: %v", err)
	}
	if count != 1 {
		t.Errorf("lançamentos de crédito = %d, esperado 1", count)
	}
}

func TestProcessWin_Failed(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	uc := newUseCase(pool)
	walletID, playerID := seedWallet(t, pool, 10000) // 100.00

	// limpa no fim
	defer pool.Exec(ctx, `DELETE FROM wallet_ledger_entries WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM transactions WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM wallets WHERE id = $1`, walletID)

	amount, _ := money.NewMoney("30.00", "BRL")
	in := ProcessWagerInput{
		ProviderID:            "provider-a",
		ExternalTransactionID: "ext-win-001",
		IdempotencyKey:        "idem-001",
		PlayerID:              playerID,
		WalletID:              walletID,
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  wager.KindWin,
		Amount:                amount,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tr, err := uc.ProcessWagerTransaction(ctx, in)
	if err != nil {
		t.Fatalf("processar win: %v", err)
	}

	if tr.Status() != wager.StatusProcessed {
		t.Errorf("status = %s, esperado PROCESSED", tr.Status())
	}

	var balance int64
	err = pool.QueryRow(ctx, `SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&balance)
	if err != nil {
		t.Fatalf("ler saldo: %v", err)
	}
	if balance != 13000 {
		t.Errorf("saldo = %d, esperado 13000", balance)
	}

	var count int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM wallet_ledger_entries WHERE wallet_id = $1 AND movement_type = 'CREDIT'`,
		walletID).Scan(&count)
	if err != nil {
		t.Fatalf("contar ledger: %v", err)
	}
	if count != 1 {
		t.Errorf("lançamentos de crédito = %d, esperado 1", count)
	}
}

func TestProcessWin_InvalidValue(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	uc := newUseCase(pool)
	walletID, playerID := seedWallet(t, pool, 5000) // 50.00 (insuficiente pra 80)

	defer pool.Exec(ctx, `DELETE FROM wallet_ledger_entries WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM transactions WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM wallets WHERE id = $1`, walletID)

	amount, _ := money.NewMoney("-20.00", "BRL")
	in := ProcessWagerInput{
		ProviderID:            "provider-a",
		ExternalTransactionID: "ext-win-002",
		IdempotencyKey:        "idem-002",
		PlayerID:              playerID,
		WalletID:              walletID,
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  wager.KindWin,
		Amount:                amount,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tr, err := uc.ProcessWagerTransaction(ctx, in)
	if err != nil {
		t.Fatalf("processar win valor negativo nao deveria dar erro tecnico: %v", err)
	}

	if tr.Status() != wager.StatusRejected {
		t.Errorf("status = %s, esperado REJECTED", tr.Status())
	}

	var balance int64
	pool.QueryRow(ctx, `SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&balance)
	if balance != 5000 {
		t.Errorf("saldo = %d, esperado 5000 (intacto)", balance)
	}

	var count int
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM wallet_ledger_entries WHERE wallet_id = $1`, walletID).Scan(&count)
	if count != 0 {
		t.Errorf("ledger deveria estar vazio, tem %d", count)
	}
}

func TestProcessRefund_Success(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	uc := newUseCase(pool)
	walletID, playerID := seedWallet(t, pool, 10000) // 100.00

	extTrId := seedBETTransaction(t, pool, walletID, playerID, 5000) // 50.00

	// limpa no fim
	defer pool.Exec(ctx, `DELETE FROM wallet_ledger_entries WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM transactions WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM wallets WHERE id = $1`, walletID)

	amount, _ := money.NewMoney("30.00", "BRL")
	in := ProcessWagerInput{
		ProviderID:            "provider-a",
		ExternalTransactionID: extTrId + "2",
		ReferenceExternalID:   extTrId,
		IdempotencyKey:        "idem-001",
		PlayerID:              playerID,
		WalletID:              walletID,
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  wager.KindRefund,
		Amount:                amount,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tr, err := uc.ProcessWagerTransaction(ctx, in)
	if err != nil {
		t.Fatalf("processar refund: %v", err)
	}

	if tr.Status() != wager.StatusProcessed {
		t.Errorf("status = %s, esperado PROCESSED", tr.Status())
	}

	var balance int64
	err = pool.QueryRow(ctx, `SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&balance)
	if err != nil {
		t.Fatalf("ler saldo: %v", err)
	}
	if balance != 15000 {
		t.Errorf("saldo = %d, esperado 15000", balance)
	}

	var count int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM wallet_ledger_entries WHERE wallet_id = $1 AND movement_type = 'CREDIT'`,
		walletID).Scan(&count)
	if err != nil {
		t.Fatalf("contar ledger: %v", err)
	}
	if count != 1 {
		t.Errorf("lançamentos de crédito = %d, esperado 1", count)
	}
}

func TestProcessRefund_NotFound(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	uc := newUseCase(pool)
	walletID, playerID := seedWallet(t, pool, 10000) // 100.00

	extTrId := seedBETTransaction(t, pool, walletID, playerID, 5000) // 50.00

	// limpa no fim
	defer pool.Exec(ctx, `DELETE FROM wallet_ledger_entries WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM transactions WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM wallets WHERE id = $1`, walletID)

	amount, _ := money.NewMoney("30.00", "BRL")
	in := ProcessWagerInput{
		ProviderID:            "provider-a",
		ExternalTransactionID: extTrId + "2",
		ReferenceExternalID:   extTrId + "_test",
		IdempotencyKey:        "idem-001",
		PlayerID:              playerID,
		WalletID:              walletID,
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  wager.KindRefund,
		Amount:                amount,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tr, err := uc.ProcessWagerTransaction(ctx, in)
	if err != nil {
		t.Fatalf("processar refund: %v", err)
	}

	if tr.Status() != wager.StatusPendingReference {
		t.Errorf("status = %s, esperado PENDING_REFERENCE", tr.Status())
	}

	var balance int64
	err = pool.QueryRow(ctx, `SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&balance)
	if err != nil {
		t.Fatalf("ler saldo: %v", err)
	}
	if balance != 10000 {
		t.Errorf("saldo = %d, esperado 10000", balance)
	}

	var count int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM wallet_ledger_entries WHERE wallet_id = $1 AND movement_type = 'CREDIT'`,
		walletID).Scan(&count)
	if err != nil {
		t.Fatalf("contar ledger: %v", err)
	}
	if count != 0 {
		t.Errorf("lançamentos esperado = %d, esperado 0", count)
	}
}

func TestProcessTwiceRefund_Success(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	uc := newUseCase(pool)
	walletID, playerID := seedWallet(t, pool, 10000) // 100.00

	extTrId := seedBETTransaction(t, pool, walletID, playerID, 5000) // 50.00

	// limpa no fim
	defer pool.Exec(ctx, `DELETE FROM wallet_ledger_entries WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM transactions WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM wallets WHERE id = $1`, walletID)

	amount, _ := money.NewMoney("30.00", "BRL")
	in := ProcessWagerInput{
		ProviderID:            "provider-a",
		ExternalTransactionID: extTrId + "2",
		ReferenceExternalID:   extTrId,
		IdempotencyKey:        "idem-001",
		PlayerID:              playerID,
		WalletID:              walletID,
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  wager.KindRefund,
		Amount:                amount,
	}

	in2 := ProcessWagerInput{
		ProviderID:            "provider-a",
		ExternalTransactionID: extTrId + "3",
		ReferenceExternalID:   extTrId,
		IdempotencyKey:        "idem-003",
		PlayerID:              playerID,
		WalletID:              walletID,
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  wager.KindRefund,
		Amount:                amount,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tr, err := uc.ProcessWagerTransaction(ctx, in)
	if err != nil {
		t.Fatalf("processar refund: %v", err)
	}

	tr, err = uc.ProcessWagerTransaction(ctx, in2)
	if err != nil {
		t.Fatalf("processar refund: %v", err)
	}

	if tr.Status() != wager.StatusRejected {
		t.Errorf("status = %s, esperado REJECTED", tr.Status())
	}

	var balance int64
	err = pool.QueryRow(ctx, `SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&balance)
	if err != nil {
		t.Fatalf("ler saldo: %v", err)
	}
	if balance != 15000 {
		t.Errorf("saldo = %d, esperado 15000", balance)
	}

	var count int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM wallet_ledger_entries WHERE wallet_id = $1 AND movement_type = 'CREDIT'`,
		walletID).Scan(&count)
	if err != nil {
		t.Fatalf("contar ledger: %v", err)
	}
	if count != 1 {
		t.Errorf("lançamentos de crédito = %d, esperado 1", count)
	}
}

func TestProcessRollBack_Success(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	uc := newUseCase(pool)
	walletID, playerID := seedWallet(t, pool, 10000) // 100.00

	extTrId := seedBETTransaction(t, pool, walletID, playerID, 5000) // 50.00

	// limpa no fim
	defer pool.Exec(ctx, `DELETE FROM wallet_ledger_entries WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM transactions WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM wallets WHERE id = $1`, walletID)

	amount, _ := money.NewMoney("30.00", "BRL")
	in := ProcessWagerInput{
		ProviderID:            "provider-a",
		ExternalTransactionID: extTrId + "2",
		ReferenceExternalID:   extTrId,
		IdempotencyKey:        "idem-001",
		PlayerID:              playerID,
		WalletID:              walletID,
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  wager.KindRollback,
		Amount:                amount,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tr, err := uc.ProcessWagerTransaction(ctx, in)
	if err != nil {
		t.Fatalf("processar rollback: %v", err)
	}

	if tr.Status() != wager.StatusProcessed {
		t.Errorf("status = %s, esperado PROCESSED", tr.Status())
	}

	var balance int64
	err = pool.QueryRow(ctx, `SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&balance)
	if err != nil {
		t.Fatalf("ler saldo: %v", err)
	}
	if balance != 15000 {
		t.Errorf("saldo = %d, esperado 15000", balance)
	}

	var count int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM wallet_ledger_entries WHERE wallet_id = $1 AND movement_type = 'CREDIT'`,
		walletID).Scan(&count)
	if err != nil {
		t.Fatalf("contar ledger: %v", err)
	}
	if count != 1 {
		t.Errorf("lançamentos de crédito = %d, esperado 1", count)
	}
}

func TestIdempotency_50ParallelSameBet(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()
	uc := newUseCase(pool)

	walletID, playerID := seedWallet(t, pool, 100000) // 1000.00
	defer cleanup(t, pool, walletID)

	amount, _ := money.NewMoney("10.00", "BRL")
	in := ProcessWagerInput{
		ProviderID:            "provider-a",
		ExternalTransactionID: "ext-50-test",
		IdempotencyKey:        "idem-50-test",
		PlayerID:              playerID,
		WalletID:              walletID,
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  wager.KindBet,
		Amount:                amount,
	}

	var wg sync.WaitGroup
	results := make([]error, 50)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	wg.Add(50)
	for i := 0; i < 50; i++ {
		go func(idx int) {
			defer wg.Done()
			_, results[idx] = uc.ProcessWagerTransaction(ctx, in)
		}(i)
	}
	wg.Wait()

	// contar erros técnicos (não replay/idempotência)
	var errosTecnicos int
	for _, err := range results {
		if err != nil && !errors.Is(err, ErrTrAlreadyProcessed) && !errors.Is(err, ErrNotSameProviderID) {
			errosTecnicos++
			t.Logf("erro: %v", err)
		}
	}
	if errosTecnicos > 0 {
		t.Errorf("erros técnicos: %d", errosTecnicos)
	}

	// saldo: 1000.00 - 10.00 = 990.00 (99000 centavos)
	var balance int64
	pool.QueryRow(context.Background(),
		`SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&balance)
	if balance != 99000 {
		t.Errorf("saldo = %d, esperado 99000", balance)
	}

	// um único débito no ledger
	var count int
	pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM wallet_ledger_entries WHERE wallet_id = $1`,
		walletID).Scan(&count)
	if count != 1 {
		t.Errorf("lançamentos = %d, esperado 1", count)
	}
}

func TestParallel_DistinctWallets(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()
	uc := newUseCase(pool)

	walletA, playerA := seedWallet(t, pool, 50000)
	walletB, playerB := seedWallet(t, pool, 50000)
	defer cleanup(t, pool, walletA)
	defer cleanup(t, pool, walletB)

	amount, _ := money.NewMoney("20.00", "BRL")

	var wg sync.WaitGroup
	var errA, errB error
	wg.Add(2)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go func() {
		defer wg.Done()
		_, errA = uc.ProcessWagerTransaction(ctx, ProcessWagerInput{
			ProviderID: "provider-a", ExternalTransactionID: "ext-a",
			IdempotencyKey: "idem-a", PlayerID: playerA, WalletID: walletA,
			RoundID: "round-1", GameID: "game-1", Kind: wager.KindBet, Amount: amount,
		})
	}()

	go func() {
		defer wg.Done()
		_, errB = uc.ProcessWagerTransaction(ctx, ProcessWagerInput{
			ProviderID: "provider-a", ExternalTransactionID: "ext-b",
			IdempotencyKey: "idem-b", PlayerID: playerB, WalletID: walletB,
			RoundID: "round-1", GameID: "game-1", Kind: wager.KindBet, Amount: amount,
		})
	}()

	wg.Wait()

	if errA != nil {
		t.Errorf("carteira A falhou: %v", errA)
	}
	if errB != nil {
		t.Errorf("carteira B falhou: %v", errB)
	}

	// ambas debitadas independentemente
	var balA, balB int64
	pool.QueryRow(context.Background(), `SELECT balance FROM wallets WHERE id = $1`, walletA).Scan(&balA)
	pool.QueryRow(context.Background(), `SELECT balance FROM wallets WHERE id = $1`, walletB).Scan(&balB)

	if balA != 48000 {
		t.Errorf("saldo A = %d, esperado 48000", balA)
	}
	if balB != 48000 {
		t.Errorf("saldo B = %d, esperado 48000", balB)
	}
}

func TestReconciliation_BalanceMatchesLedger(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()
	uc := newUseCase(pool)

	walletID, playerID := seedWallet(t, pool, 100000) // 1000.00
	defer cleanup(t, pool, walletID)

	// processa várias operações
	ops := []struct {
		extID  string
		kind   wager.Kind
		amount string
	}{
		{"bet-1", wager.KindBet, "50.00"},
		{"bet-2", wager.KindBet, "30.00"},
		{"win-1", wager.KindWin, "80.00"},
		{"bet-3", wager.KindBet, "20.00"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, op := range ops {
		amt, _ := money.NewMoney(op.amount, "BRL")
		uc.ProcessWagerTransaction(ctx, ProcessWagerInput{
			ProviderID: "provider-a", ExternalTransactionID: op.extID,
			IdempotencyKey: "idem-" + op.extID, PlayerID: playerID, WalletID: walletID,
			RoundID: "round-1", GameID: "game-1", Kind: op.kind, Amount: amt,
		})
	}

	// saldo esperado: 1000 - 50 - 30 + 80 - 20 = 980.00

	// saldo na carteira
	var walletBalance int64
	pool.QueryRow(context.Background(),
		`SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&walletBalance)

	// saldo calculado pelo ledger
	var ledgerBalance int64
	pool.QueryRow(context.Background(),
		`SELECT COALESCE(
            SUM(CASE WHEN movement_type = 'CREDIT' THEN amount ELSE -amount END),
            0
         ) FROM wallet_ledger_entries WHERE wallet_id = $1`, walletID).Scan(&ledgerBalance)

	// o saldo inicial (1000.00 = 100000 centavos) não está no ledger, então soma
	ledgerBalance += 100000

	if walletBalance != ledgerBalance {
		t.Errorf("reconciliação falhou: saldo carteira = %d, saldo ledger = %d", walletBalance, ledgerBalance)
	}

	// valor esperado
	if walletBalance != 98000 {
		t.Errorf("saldo = %d, esperado 98000 (980.00)", walletBalance)
	}
}

func TestIdempotency_Replay(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()
	uc := newUseCase(pool)

	walletID, playerID := seedWallet(t, pool, 100000)
	defer cleanup(t, pool, walletID)

	amount, _ := money.NewMoney("25.00", "BRL")
	in := ProcessWagerInput{
		ProviderID: "provider-a", ExternalTransactionID: "ext-replay",
		IdempotencyKey: "idem-replay", PlayerID: playerID, WalletID: walletID,
		RoundID: "round-1", GameID: "game-1", Kind: wager.KindBet, Amount: amount,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// primeira chamada — processa
	tr1, err := uc.ProcessWagerTransaction(ctx, in)
	if err != nil {
		t.Fatalf("primeira chamada: %v", err)
	}

	// segunda chamada — replay
	tr2, err := uc.ProcessWagerTransaction(ctx, in)
	if !errors.Is(err, ErrTrAlreadyProcessed) {
		t.Fatalf("esperava ErrTrAlreadyProcessed, recebi: %v", err)
	}

	// resultado deve ser o mesmo
	if tr1.ID() != tr2.ID() {
		t.Errorf("IDs diferentes: %s vs %s", tr1.ID(), tr2.ID())
	}

	// saldo debitado uma vez só
	var balance int64
	pool.QueryRow(context.Background(),
		`SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&balance)
	if balance != 97500 {
		t.Errorf("saldo = %d, esperado 97500", balance)
	}
}
