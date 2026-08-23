package httpserver

import (
	"encoding/json"
	"net/http"
)

type passkeyIdentityResolveInput struct {
	Phone string `json:"phone"`
}

type passkeyIdentityResolveOutput struct {
	OK     bool   `json:"ok"`
	Exists bool   `json:"exists"`
	Phone  string `json:"phone"`
}

func (s *Server) WithPasskeyUnifiedLogin(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/passkey/identity/resolve", s.resolvePasskeyIdentity)
	mux.Handle("/", next)
	return mux
}

func (s *Server) resolvePasskeyIdentity(w http.ResponseWriter, r *http.Request) {
	var input passkeyIdentityResolveInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&input); err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	phone, exists, err := s.Store.PasskeyLoginIdentityExistsByPhone(r.Context(), input.Phone)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(passkeyIdentityResolveOutput{OK: true, Exists: exists, Phone: phone})
}
