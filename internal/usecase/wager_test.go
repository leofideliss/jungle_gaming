package usecase

import (
	"context"
	"testing"

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

func newUseCase(pool *pgxpool.Pool) *WagerUseCase {
	walletRepo := repository.NewWalletRepository(pool)
	txRepo := repository.NewWagerTransactionRepository(pool)
	outboxRepo := repository.NewOutboxRepository(pool)
	idemp := idempotency.NewIdempotencyService(txRepo)
	return NewWagerUseCase(pool, walletRepo, txRepo, outboxRepo, idemp)
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

func seedBETTransaction(t *testing.T, pool *pgxpool.Pool, walletID, playerID uuid.UUID, amountCents int64) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	txID := uuid.Must(uuid.NewV7())
	providerID := "provider_test"
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

	return txID
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

	tr, err := uc.ProcessWagerTransaction(in)
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

	tr, err := uc.ProcessWagerTransaction(in)
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

	tr, err := uc.ProcessWagerTransaction(in)
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

	tr, err := uc.ProcessWagerTransaction(in)
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

	tr, err := uc.ProcessWagerTransaction(in)
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

	tr, err := uc.ProcessWagerTransaction(in)
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

	trId := seedBETTransaction(t, pool, walletID, playerID, 5000) // 50.00

	// limpa no fim
	defer pool.Exec(ctx, `DELETE FROM wallet_ledger_entries WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM transactions WHERE wallet_id = $1`, walletID)
	defer pool.Exec(ctx, `DELETE FROM wallets WHERE id = $1`, walletID)

	amount, _ := money.NewMoney("30.00", "BRL")
	in := ProcessWagerInput{
		ProviderID:            "provider-a",
		ExternalTransactionID: trId.String(),
		IdempotencyKey:        "idem-001",
		PlayerID:              playerID,
		WalletID:              walletID,
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  wager.KindRefund,
		Amount:                amount,
	}

	tr, err := uc.ProcessWagerTransaction(in)

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
