package handler

import (
	"encoding/json"
	"jungle_gaming/internal/domain/wager"
	"jungle_gaming/internal/usecase"
	"net/http"
)

type WagerRequest struct {
	Data struct {
		ExternalTransactionID string `json:"externalTransactionId"`
		ProviderID            string `json:"providerId"`
		IdempotencyKey        string `json:"idempotencyKey"`
		PlayerID              string `json:"playerId"` // string no JSON, UUID no input
		WalletID              string `json:"walletId"`
		RoundID               string `json:"roundId"`
		GameID                string `json:"gameId"`
		Kind                  string `json:"kind"`
		Money                 struct {
			Amount   string `json:"amount"`
			Currency string `json:"currency"`
		} `json:"money"`
	} `json:"data"`
}

type WagerUseCase interface {
	ProcessWagerTransaction(in usecase.ProcessWagerInput) (wager.WagerTransaction, error)
}

type WagerHandler struct {
	wagerCase WagerUseCase
}

func NewWagerHandler(wu WagerUseCase) *WagerHandler {
	return &WagerHandler{wagerCase: wu}
}

func (wh *WagerHandler) ProcessWagerTransaction(w http.ResponseWriter, r *http.Request) {
	var in WagerRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		WriteError(w, http.StatusInternalServerError, "payload inválido")
		return
	}
	wh.wagerCase.ProcessWagerTransaction(in)
}
