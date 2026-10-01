package handler

import (
	"context"
	"encoding/json"
	"errors"
	"jungle_gaming/internal/domain/money"
	"jungle_gaming/internal/domain/wager"
	"jungle_gaming/internal/middleware"
	"jungle_gaming/internal/usecase"
	"net/http"

	"github.com/google/uuid"
)

type WagerRequest struct {
	Data struct {
		ExternalTransactionID string `json:"externalTransactionId"`
		ProviderID            string `json:"providerId"`
		IdempotencyKey        string `json:"idempotencyKey"`
		PlayerID              string `json:"playerId"`
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

func (wh *WagerHandler) RegisterRoutes(mx *http.ServeMux) {
	mx.HandleFunc("POST /wagering/transactions", wh.ProcessWagerTransaction)
}

func (wh *WagerHandler) ProcessWagerTransaction(w http.ResponseWriter, r *http.Request) {
	var in WagerRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	in.Data.IdempotencyKey = r.Header.Get("x-idempotency-key")
	in.Data.ProviderID = getProviderID(r.Context())

	wagerInput, err := in.ToUseCaseInput()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	res, err := wh.wagerCase.ProcessWagerTransaction(wagerInput)

	if errors.Is(err, usecase.ErrInvalidIdempotency) {
		WriteError(w, http.StatusConflict, err.Error())
		return
	}
	if errors.Is(err, usecase.ErrInvalidResult) {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if errors.Is(err, usecase.ErrTrAlreadyProcessed) {
		WriteJSON(w, http.StatusOK, res)
		return
	}

	if errors.Is(err, usecase.ErrNotSameProviderID) {
		WriteError(w, http.StatusForbidden, err.Error())
		return
	}

	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, res)
}

func (r *WagerRequest) ToUseCaseInput() (usecase.ProcessWagerInput, error) {

	parsedPlayerID, err := uuid.Parse(r.Data.PlayerID)
	if err != nil {
		return usecase.ProcessWagerInput{}, err
	}
	parsedWalletID, err := uuid.Parse(r.Data.WalletID)
	if err != nil {
		return usecase.ProcessWagerInput{}, err
	}

	amount, err := money.NewMoney(r.Data.Money.Amount, r.Data.Money.Currency)
	if err != nil {
		return usecase.ProcessWagerInput{}, err
	}

	return usecase.ProcessWagerInput{
		ExternalTransactionID: r.Data.ExternalTransactionID,
		ProviderID:            r.Data.ProviderID,
		IdempotencyKey:        r.Data.IdempotencyKey,
		PlayerID:              parsedPlayerID,
		WalletID:              parsedWalletID,
		RoundID:               r.Data.RoundID,
		GameID:                r.Data.GameID,
		Kind:                  wager.Kind(r.Data.Kind),
		Amount:                amount,
	}, nil
}

func getProviderID(ctx context.Context) string {
	v, ok := ctx.Value(middleware.ProviderID).(string)
	if !ok {
		return ""
	}
	return v
}
