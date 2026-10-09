package usecase

import (
	"errors"
	"jungle_gaming/internal/domain/money"
	"jungle_gaming/internal/domain/wager"

	"github.com/google/uuid"
)

type WagerRequestDTO struct {
	Data struct {
		ExternalTransactionID string  `json:"externalTransactionId"`
		ProviderID            string  `json:"providerId"`
		IdempotencyKey        string  `json:"idempotencyKey"`
		MessageID             *string `json:"messageId"`
		PlayerID              string  `json:"playerId"`
		WalletID              string  `json:"walletId"`
		RoundID               string  `json:"roundId"`
		GameID                string  `json:"gameId"`
		Kind                  string  `json:"kind"`
		Money                 struct {
			Amount   string `json:"amount"`
			Currency string `json:"currency"`
		} `json:"money"`
	} `json:"data"`
}

func (r WagerRequestDTO) Validate() error {
	if r.Data.PlayerID == "" {
		return errors.New("playerId é obrigatório")
	}
	if r.Data.WalletID == "" {
		return errors.New("walletId é obrigatório")
	}
	if r.Data.Kind == "" {
		return errors.New("kind é obrigatório")
	}
	if r.Data.Money.Amount == "" {
		return errors.New("amount é obrigatório")
	}
	if r.Data.Money.Currency == "" {
		return errors.New("currency é obrigatório")
	}
	return nil
}

func (r WagerRequestDTO) ToInput() (ProcessWagerInput, error) {
	playerID, err := uuid.Parse(r.Data.PlayerID)
	if err != nil {
		return ProcessWagerInput{}, err
	}
	walletID, err := uuid.Parse(r.Data.WalletID)
	if err != nil {
		return ProcessWagerInput{}, err
	}
	amount, err := money.NewMoney(r.Data.Money.Amount, r.Data.Money.Currency)
	if err != nil {
		return ProcessWagerInput{}, err
	}

	return ProcessWagerInput{
		ExternalTransactionID: r.Data.ExternalTransactionID,
		ProviderID:            r.Data.ProviderID,
		IdempotencyKey:        r.Data.IdempotencyKey,
		PlayerID:              playerID,
		WalletID:              walletID,
		RoundID:               r.Data.RoundID,
		GameID:                r.Data.GameID,
		Kind:                  wager.Kind(r.Data.Kind),
		Amount:                amount,
	}, nil
}
