package handler

import (
	"encoding/json"
	"net/http"

	authentication "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
)

func Me(w http.ResponseWriter, r *http.Request) {
	account, ok := authentication.AccountFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(account)
}
