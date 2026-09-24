package idempotency

import (
	"context"
	"errors"
	"jungle_gaming/internal/domain/wager"
	"jungle_gaming/internal/repository"
)

type Status string

const (
	StatusNew      Status = "NEW"
	StatusReplay   Status = "REPLAY"
	StatusConflict Status = "CONFLICT"
	StatusError    Status = "ERROR"
)

type IdempotencyResult struct {
	Result Status
	Tr     wager.WagerTransaction
}

type transactionFinder interface {
	FindByExternalTransactionIdAndProviderId(ctx context.Context, externalId, providerId string) (wager.WagerTransaction, error)
}

type IdempotencyService struct {
	finder transactionFinder
}

func NewIdempotencyService(i transactionFinder) *IdempotencyService {
	return &IdempotencyService{finder: i}
}

func (i *IdempotencyService) Check(ctx context.Context, providerId, externalID, payload string) (IdempotencyResult, error) {
	tr, err := i.finder.FindByExternalTransactionIdAndProviderId(ctx, externalID, providerId)

	if err != nil {
		if errors.Is(err, repository.ErrTransactionNotFound) {
			return IdempotencyResult{Result: StatusNew, Tr: wager.WagerTransaction{}}, nil

		}
		return IdempotencyResult{Result: StatusError, Tr: wager.WagerTransaction{}}, err

	}

	if tr.PayloadHash() == payload {
		return IdempotencyResult{Result: StatusReplay, Tr: tr}, nil

	}

	return IdempotencyResult{Result: StatusConflict, Tr: tr}, nil

}
