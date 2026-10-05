package handler

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

type HealthHandler struct {
	pool        *pgxpool.Pool
	keycloakURL string
}

func NewHealthHandler(p *pgxpool.Pool, keycloakURL string) *HealthHandler {
	return &HealthHandler{
		pool:        p,
		keycloakURL: keycloakURL,
	}
}

func (h *HealthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health/live", h.AppHealth)
	mux.HandleFunc("GET /health/ready", h.AppReady)

}

func (h *HealthHandler) AppHealth(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, "healthly")
}

func (h *HealthHandler) AppReady(w http.ResponseWriter, r *http.Request) {
	var count int
	if err := h.pool.QueryRow(r.Context(), "SELECT 1").Scan(&count); err != nil {
		WriteError(w, http.StatusServiceUnavailable, "postgres off")
		return
	}

	resp, err := http.Get(h.keycloakURL + "/.well-known/openid-configuration")
	if err != nil || resp.StatusCode != 200 {
		WriteError(w, http.StatusServiceUnavailable, "keycloack off")
		return
	}
	defer resp.Body.Close()

	WriteJSON(w, http.StatusOK, Response{
		Message:  "Servico disponivel",
		HttpCode: http.StatusOK,
	})
}
