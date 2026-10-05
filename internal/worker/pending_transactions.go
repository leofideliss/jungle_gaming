package worker

import (
	"context"
	"errors"
	"jungle_gaming/internal/domain/ledger"
	"jungle_gaming/internal/domain/wager"
	"jungle_gaming/internal/domain/wallet"
	"jungle_gaming/internal/repository"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ProcessPendingTr struct {
	pool                 *pgxpool.Pool
	wagerTransactionRepo *repository.WagerTransactionRepository
	wallet               *repository.WalletRepository
}

func NewProcessPendingTr(p *pgxpool.Pool, repo *repository.WagerTransactionRepository, w *repository.WalletRepository) *ProcessPendingTr {
	return &ProcessPendingTr{
		pool:                 p,
		wagerTransactionRepo: repo,
		wallet:               w,
	}
}

func (p *ProcessPendingTr) Start(ctx context.Context) {
	log.Println("Iniciando worker transacoes pendentes")

	for {
		select {
		case <-ctx.Done():
			log.Println("Parando worker transacoes pendentes")
			return
		default:
		}

		p.process(ctx)

		time.Sleep(10 * time.Second)
	}

}

func (p *ProcessPendingTr) process(ctx context.Context) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)

	trs, err := p.wagerTransactionRepo.FindByReprocess(ctx, tx)
	if err != nil {
		return
	}

	if len(trs) == 0 {
		log.Println("Nenhuma transacao pendente para processar")
		return
	}

	const maxAttempts = 10
	for _, tr := range trs {
		ref, err := p.wagerTransactionRepo.FindByExternalTransactionIdAndProviderId(ctx, tr.ReferenceExternalID(), tr.ProviderID())
		if errors.Is(err, repository.ErrTransactionNotFound) {

			attempts := tr.Attempts() + 1
			nextAttempt := tr.NextAttemptAt().Add(backoff(attempts))

			if attempts >= maxAttempts {
				tr.MarkAsRejected("REFERENCE_NOT_FOUND")
				p.wagerTransactionRepo.UpdateStatus(ctx, tx, wager.StatusRejected, tr.ID())
				continue
			}
			if err := p.wagerTransactionRepo.UpdateRetryTrPending(ctx, tx, tr.ID(), attempts, nextAttempt); err != nil {
				continue
			}
			continue
		}

		if err != nil {
			continue
		}

		userWallet, err := p.wallet.GetWallet(ctx, tx, tr.WalletID())
		if err != nil {
			continue
		}
		var entry ledger.WalletLedger
		switch ref.Kind() {
		case wager.KindBet:
			entry, err = userWallet.Credit(ref.Amount(), tr.ID())
			if err != nil {
				continue
			}

		case wager.KindRefund, wager.KindWin:
			entry, err = userWallet.Debit(ref.Amount(), tr.ID())
			if errors.Is(err, wallet.ErrInsufficientBalance) {
				if err := tr.MarkAsRejected("INSUFFICIENT_BALANCE"); err != nil {
					continue
				}
				if err := p.wagerTransactionRepo.UpdateStatus(ctx, tx, wager.StatusRejected, tr.ID()); err != nil {
					continue
				}
			}

			if err != nil {
				continue
			}

		default:
			continue
		}

		if err := p.wallet.UpdateWallet(ctx, tx, tr.WalletID(), userWallet.Balance().Cents(), tr.ID(), entry); err != nil {
			continue
		}

		if err := tr.MarkAsProcessed(userWallet.Balance()); err != nil {
			continue
		}

		p.wagerTransactionRepo.UpdateStatus(ctx, tx, wager.StatusProcessed, tr.ID())
	}

	err = tx.Commit(ctx)
	if err != nil {
		return
	}
}

func backoff(attempt int) time.Duration {
	d := time.Duration(1<<uint(attempt)) * 5 * time.Second
	if d > 5*time.Minute {
		d = 5 * time.Minute
	}
	return d
}
