package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"jungle_gaming/internal/domain/money"
	"jungle_gaming/internal/domain/wallet"
	"jungle_gaming/internal/usecase"

	"github.com/google/uuid"
)

type WalletUseCaseInterface interface {
	CreateWallet(ctx context.Context, in usecase.CreateWalletInput) (wallet.Wallet, error)
}

type CreateWalletRequest struct {
	PlayerID       string `json:"playerId"`
	InitialBalance struct {
		Amount   string `json:"amount"`
		Currency string `json:"currency"`
	} `json:"initialBalance"`
}

type WalletHandler struct {
	uc WalletUseCaseInterface
}

func NewWalletHandler(uc WalletUseCaseInterface) *WalletHandler {
	return &WalletHandler{uc: uc}
}

func (h *WalletHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /wallets", h.Create)
}

func (h *WalletHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateWalletRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	playerID, err := uuid.Parse(req.PlayerID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "playerId inválido")
		return
	}

	balance, err := money.NewMoney(req.InitialBalance.Amount, req.InitialBalance.Currency)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.uc.CreateWallet(r.Context(), usecase.CreateWalletInput{
		PlayerID:       playerID,
		InitialBalance: balance,
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":       result.ID().String(),
		"playerId": result.PlayerID().String(),
		"balance": map[string]string{
			"amount":   result.Balance().String(),
			"currency": result.Balance().Currency(),
		},
		"version": result.Version(),
	})
}
