package httpserver

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Ltre/FmlySys/internal/store"
)

type passkeyHomeView struct {
	Title           string
	ActivePartition string
	Identity        store.PasskeyLoginIdentityView
}

// WithPasskeyFrontDoorFixes makes a valid Passkey identity session a real
// front-end login state. Family-data authorization still comes from member
// association and member permissions; an unbound Passkey identity gets the
// authenticated home shell without family business permissions.
func (s *Server) WithPasskeyFrontDoorFixes(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.passkeyAwareDashboard)
	mux.HandleFunc("GET /account", s.frontAccountEntry)
	mux.HandleFunc("POST /auth/passkey/create/finish", rewritePasskeySuccessToHome(next))
	mux.HandleFunc("POST /auth/passkey/login/finish", rewritePasskeySuccessToHome(next))
	mux.Handle("/", next)
	return mux
}

func rewritePasskeySuccessToHome(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		capture := newCaptureResponseWriter()
		next.ServeHTTP(capture, r)

		for key, values := range capture.header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}

		var payload map[string]any
		if capture.status >= 200 && capture.status < 300 && json.Unmarshal(capture.body.Bytes(), &payload) == nil {
			if ok, _ := payload["ok"].(bool); ok {
				payload["redirect"] = "/"
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.Header().Del("Content-Length")
				w.WriteHeader(capture.status)
				_ = json.NewEncoder(w).Encode(payload)
				return
			}
		}

		w.WriteHeader(capture.status)
		_, _ = w.Write(capture.body.Bytes())
	}
}

func (s *Server) passkeyAwareDashboard(w http.ResponseWriter, r *http.Request) {
	// Existing member sessions continue to behave exactly as before.
	if raw := cookieValue(r, "fmly_session"); raw != "" {
		if member, permissions, err := s.Store.MemberFromSession(r.Context(), raw); err == nil {
			s.renderDashboardForMember(w, r, member, permissions)
			return
		}
		clearCookie(w, r, "fmly_session", "/")
	}

	identity, _, err := s.Store.PasskeyLoginIdentityFromSession(r.Context(), cookieValue(r, passkeyIdentityCookie))
	if err != nil {
		clearCookie(w, r, passkeyIdentityCookie, "/")
		redirect(w, r, "/login")
		return
	}

	if identity.MemberID > 0 {
		member, err := s.Store.MemberByID(r.Context(), identity.MemberID)
		if err != nil {
			s.renderUnboundPasskeyHome(w, identity)
			return
		}
		permissions, err := s.Store.MemberPermissions(r.Context(), member.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		raw, err := s.Store.CreateMemberSession(r.Context(), member.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		setCookie(w, r, "fmly_session", raw, "/", int(store.MemberSessionTTL.Seconds()))
		s.renderDashboardForMember(w, r, member, permissions)
		return
	}

	s.renderUnboundPasskeyHome(w, identity)
}

func (s *Server) renderDashboardForMember(w http.ResponseWriter, r *http.Request, member store.Member, permissions map[string]bool) {
	ctx := context.WithValue(r.Context(), memberContextKey, member)
	ctx = context.WithValue(ctx, permsContextKey, permissions)
	s.dashboard(w, r.WithContext(ctx))
}

func (s *Server) renderUnboundPasskeyHome(w http.ResponseWriter, identity store.PasskeyLoginIdentityView) {
	v := passkeyHomeView{
		Title:           "FmlySys",
		ActivePartition: s.PM.ActiveID,
		Identity:        identity,
	}
	if err := s.Templates.ExecuteTemplate(w, "passkey-home.html", v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) frontAccountEntry(w http.ResponseWriter, r *http.Request) {
	if _, _, err := s.Store.PasskeyLoginIdentityFromSession(r.Context(), cookieValue(r, passkeyIdentityCookie)); err == nil {
		redirect(w, r, "/passkey/account")
		return
	}
	if raw := cookieValue(r, "fmly_session"); raw != "" {
		if _, _, err := s.Store.MemberFromSession(r.Context(), raw); err == nil {
			redirect(w, r, "/passkeys")
			return
		}
	}
	redirect(w, r, "/login")
}
