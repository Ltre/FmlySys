package httpserver

import (
	"net/http"
	"strconv"
	"strings"
)

// WithAdminMemberDelete extends the existing authenticated member-permission
// endpoint with a delete action without adding a second administrative URL.
// Normal permission saves pass through unchanged.
func (s *Server) WithAdminMemberDelete(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/admin/members/") || !strings.HasSuffix(r.URL.Path, "/permissions") {
			next.ServeHTTP(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			s.fail(w, r, err)
			return
		}
		if r.FormValue("member_action") != "delete" {
			next.ServeHTTP(w, r)
			return
		}

		raw := cookieValue(r, "fmly_admin_session")
		sess, err := s.Admin.Session(r.Context(), raw)
		if err != nil || sess.Stage != "authenticated" {
			clearCookie(w, r, "fmly_admin_session", "/")
			redirect(w, r, "/admin/login")
			return
		}

		idPart := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/admin/members/"), "/permissions")
		if idPart == "" || strings.Contains(idPart, "/") {
			http.NotFound(w, r)
			return
		}
		memberID, err := strconv.ParseInt(idPart, 10, 64)
		if err != nil || memberID <= 0 {
			http.NotFound(w, r)
			return
		}
		if _, err := s.Store.DeleteMemberSmart(r.Context(), s.DevActorID, memberID); err != nil {
			s.fail(w, r, err)
			return
		}
		redirect(w, r, "/admin")
	})
}
