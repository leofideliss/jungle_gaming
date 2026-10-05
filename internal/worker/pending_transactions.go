package worker

import (
	"context"
	"errors"
	"jungle_gaming/internal/domain/wager"
	"jungle_gaming/internal/repository"
	"jungle_gaming/internal/usecase"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ProcessPendingTr struct {
	pool                    *pgxpool.Pool
	wagerTransactionRepo    *repository.WagerTransactionRepository
	wagerTransactionUseCase usecase.WagerUseCase
}

func NewProcessPendingTr(p *pgxpool.Pool, repo *repository.WagerTransactionRepository, u usecase.WagerUseCase) *ProcessPendingTr {
	return &ProcessPendingTr{
		pool:                    p,
		wagerTransactionRepo:    repo,
		wagerTransactionUseCase: u,
	}
}

func (p *ProcessPendingTr) Start(ctx context.Context) {
	log.Println("Iniciando worker transacoes pendentes")

	for {
		select {
		case <-ctx.Done():
			log.Println("Parando worker transacoes pendentes")
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
	maxAttempts := 10
	trs, err := p.wagerTransactionRepo.FindByReprocess(ctx, tx)
	if err != nil {
		return
	}

	for _, tr := range trs {
		_, err := p.wagerTransactionRepo.FindByExternalTransactionIdAndProviderId(ctx, tr.ReferenceExternalID(), tr.ProviderID())
		if errors.Is(err, repository.ErrTransactionNotFound) {
			attempts := tr.Attempts()
			switch {
			case attempts >= maxAttempts:
				if err := p.wagerTransactionRepo.UpdateStatus(ctx, tx, wager.StatusFailed, tr.ID()); err != nil {
					return
				}
			default:
				attempts++
				_, err := p.wagerTransactionUseCase.ProcessWagerTransaction(ctx, tr, usecase.ProcessWagerInput{
					ExternalTransactionID: tr.ExternalTrID(),
					ProviderID:            tr.ProviderID(),
					IdempotencyKey:        tr.IdempotencyKey(),
					PlayerID:              tr.PlayerID(),
					WalletID:              tr.WalletID(),
					RoundID:               tr.RoundID(),
					GameID:                tr.GameID(),
					Kind:                  tr.Kind(),
					Amount:                tr.Amount()})

				if err != nil {
					tr.NextAttemptAt().Add(time.Second * time.Duration(attempts))
					if err := p.wagerTransactionRepo.UpdateRetryTrPending(ctx, tx, tr.ID(), attempts, tr.NextAttemptAt()); err != nil {
						return
					}
				}
			}
		}

	}

	err = tx.Commit(ctx)
	if err != nil {
		return
	}
}
