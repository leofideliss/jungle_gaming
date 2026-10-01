package handler

import (
	"context"
	"encoding/json"
	"errors"
	"jungle_gaming/internal/domain/wager"
	"jungle_gaming/internal/middleware"
	"jungle_gaming/internal/usecase"
	"net/http"
)

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
	var in usecase.WagerRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	in.Data.IdempotencyKey = r.Header.Get("x-idempotency-key")
	in.Data.ProviderID = getProviderID(r.Context())

	wagerInput, err := in.ToInput()
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

func getProviderID(ctx context.Context) string {
	v, ok := ctx.Value(middleware.ProviderID).(string)
	if !ok {
		return ""
	}
	return v
}
