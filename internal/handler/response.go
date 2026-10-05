package handler

import (
	"encoding/json"
	"net/http"
)

type Response struct {
	Message  string `json:"message"`
	HttpCode int    `json:"http_code"`
}

func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}
