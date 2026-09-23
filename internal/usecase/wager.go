package usecase

import (
	"jungle_gaming/internal/domain/ledger"
	"jungle_gaming/internal/idempotency"
	"jungle_gaming/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
)

type WagerUseCase struct {
	pool         *pgxpool.Pool
	wallets      *repository.WalletRepository
	transactions *repository.WagerTransactionRepository
	ledger       *ledger.WalletLedger
	outbox       *repository.OutboxRepository
	idempotency  *idempotency.IdempotencyService
}

func NewWagerUseCase(p *pgxpool.Pool, w *repository.WalletRepository, tr *repository.WagerTransactionRepository, l *ledger.WalletLedger, o *repository.OutboxRepository, i *idempotency.IdempotencyService) *WagerUseCase {
	return &WagerUseCase{
		pool:         p,
		wallets:      w,
		transactions: tr,
		ledger:       l,
		outbox:       o,
		idempotency:  i,
	}
}
