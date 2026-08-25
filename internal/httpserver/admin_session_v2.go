package httpserver

import (
	"net/http"

	"github.com/Ltre/FmlySys/internal/adminauth"
)

func (s *Server) adminLoginPersistent(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	u, err := s.Admin.VerifyPassword(r.Context(), r.FormValue("username"), r.FormValue("password"))
	if err != nil {
		v := s.base("后台登录")
		v.AdminConfigured, _ = s.Admin.HasAdmin(r.Context())
		v.Error = err.Error()
		s.render(w, "admin-login.html", v)
		return
	}

	stage := "totp_setup"
	if u.TOTPConfirmed {
		stage = "totp_verify"
	}
	raw, err := s.Admin.BeginPersistentSession(r.Context(), u.ID, stage)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setCookie(w, r, "fmly_admin_session", raw, "/", int(adminauth.PersistentSessionTTL.Seconds()))
	if stage == "totp_setup" {
		redirect(w, r, "/admin/totp/setup")
		return
	}
	redirect(w, r, "/admin/totp")
}

// WithAdminSessionV2 keeps administrator sessions in system.db and refreshes
// their expiry on use. The database token is authoritative; a server restart
// therefore does not sign the administrator out.
func (s *Server) WithAdminSessionV2(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /admin/login", s.adminLoginPersistent)
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := cookieValue(r, "fmly_admin_session")
		if raw != "" {
			if _, err := s.Admin.Session(r.Context(), raw); err == nil {
				if err := s.Admin.ExtendPersistentSession(r.Context(), raw); err == nil {
					setCookie(w, r, "fmly_admin_session", raw, "/", int(adminauth.PersistentSessionTTL.Seconds()))
				}
			}
		}
		next.ServeHTTP(w, r)
	}))
	return mux
}
