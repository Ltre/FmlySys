package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Ltre/FmlySys/internal/store"
)

type passkeyIdentityBindingAdminView struct {
	Title           string
	ActivePartition string
	AdminUsername   string
	Identities      []store.PasskeyIdentityBindingView
	Members         []store.Member
}

type passkeyIdentityAccountV2View struct {
	Title               string
	Identity            store.PasskeyLoginIdentityView
	Fresh               bool
	Recovered           bool
	MemberSession       bool
	EffectiveMemberID   int64
	EffectiveMemberName string
}

func (s *Server) passkeyIdentityLoginFinishV2(w http.ResponseWriter, r *http.Request) {
	wa, rpID, err := passkeyWebAuthn(r)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	ceremony, err := s.Store.TakePasskeyLoginCeremony(r.Context(), cookieValue(r, passkeyIdentityLoginCookie), "login", rpID)
	clearPasskeyCeremonyCookie(w, r, passkeyIdentityLoginCookie)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	user, err := s.Store.PasskeyLoginUserByID(r.Context(), ceremony.IdentityID, rpID)
	if err != nil {
		passkeyJSONError(w, err, http.StatusUnauthorized)
		return
	}
	credential, err := wa.FinishLogin(user, ceremony.Session, r)
	if err != nil {
		passkeyJSONError(w, errors.New("Passkey 验证失败；手机号只能定位身份，不能代替 Passkey 验证"), http.StatusUnauthorized)
		return
	}
	if err := s.Store.UpdatePasskeyLoginCredentialAfterLogin(r.Context(), user.IdentityID, rpID, credential); err != nil {
		passkeyJSONError(w, err, http.StatusInternalServerError)
		return
	}

	memberID, err := s.Store.ResolvePasskeyCredentialMember(r.Context(), user.IdentityID, rpID, credential.ID)
	if err != nil {
		passkeyJSONError(w, err, http.StatusInternalServerError)
		return
	}

	// Replace both identity and member sessions atomically from the browser's
	// point of view. This prevents a stale member cookie from surviving an
	// unbound credential login and ensures credential B can enter a different
	// member from credential A even though both belong to one login identity.
	s.Store.DeletePasskeyLoginIdentitySession(r.Context(), cookieValue(r, passkeyIdentityCookie))
	s.Store.DeleteMemberSession(r.Context(), cookieValue(r, "fmly_session"))
	clearCookie(w, r, passkeyIdentityCookie, "/")
	clearCookie(w, r, "fmly_session", "/")

	rawIdentity, err := s.Store.CreatePasskeyLoginIdentitySessionForMember(r.Context(), user.IdentityID, memberID)
	if err != nil {
		passkeyJSONError(w, err, http.StatusInternalServerError)
		return
	}
	setCookie(w, r, passkeyIdentityCookie, rawIdentity, "/", int(store.PasskeyLoginIdentitySessionTTL.Seconds()))
	if memberID > 0 {
		rawMember, err := s.Store.CreateMemberSession(r.Context(), memberID)
		if err != nil {
			passkeyJSONError(w, err, http.StatusInternalServerError)
			return
		}
		setCookie(w, r, "fmly_session", rawMember, "/", int(store.MemberSessionTTL.Seconds()))
	}
	passkeyJSONOK(w, "/passkey/account?recovered=1")
}

func (s *Server) passkeyIdentityAccountPageV2(w http.ResponseWriter, r *http.Request) {
	rawIdentity := cookieValue(r, passkeyIdentityCookie)
	identity, fresh, err := s.Store.PasskeyLoginIdentityFromSession(r.Context(), rawIdentity)
	if err != nil {
		clearCookie(w, r, passkeyIdentityCookie, "/")
		redirect(w, r, "/login")
		return
	}
	effectiveMemberID, err := s.Store.PasskeyLoginSessionEffectiveMember(r.Context(), rawIdentity)
	if err != nil {
		effectiveMemberID = identity.MemberID
	}

	memberSession := false
	if effectiveMemberID > 0 {
		if raw := cookieValue(r, "fmly_session"); raw != "" {
			if member, _, memberErr := s.Store.MemberFromSession(r.Context(), raw); memberErr == nil && member.ID == effectiveMemberID {
				memberSession = true
			}
		}
		if !memberSession {
			raw, memberErr := s.Store.CreateMemberSession(r.Context(), effectiveMemberID)
			if memberErr == nil {
				setCookie(w, r, "fmly_session", raw, "/", int(store.MemberSessionTTL.Seconds()))
				memberSession = true
			}
		}
	} else {
		s.Store.DeleteMemberSession(r.Context(), cookieValue(r, "fmly_session"))
		clearCookie(w, r, "fmly_session", "/")
	}

	effectiveMemberName := ""
	if effectiveMemberID > 0 {
		if member, memberErr := s.Store.MemberByID(r.Context(), effectiveMemberID); memberErr == nil {
			effectiveMemberName = member.Name
		}
	}
	v := passkeyIdentityAccountV2View{
		Title:               "Passkey 登录身份",
		Identity:            identity,
		Fresh:               fresh,
		Recovered:           r.URL.Query().Get("recovered") == "1",
		MemberSession:       memberSession,
		EffectiveMemberID:   effectiveMemberID,
		EffectiveMemberName: effectiveMemberName,
	}
	if err := s.Templates.ExecuteTemplate(w, "passkey-account-v2.html", v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) adminPasskeyIdentitiesPageV2(w http.ResponseWriter, r *http.Request) {
	identities, err := s.Store.AllPasskeyIdentityBindings(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	members, err := s.Store.ActiveMembersForPasskey(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v := passkeyIdentityBindingAdminView{
		Title:           "Passkey 登录身份管理",
		ActivePartition: s.PM.ActiveID,
		AdminUsername:   currentAdmin(r).Username,
		Identities:      identities,
		Members:         members,
	}
	if err := s.Templates.ExecuteTemplate(w, "admin-passkeys-v2.html", v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) adminBindPasskeyCredentialV2(w http.ResponseWriter, r *http.Request) {
	credentialID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || credentialID <= 0 {
		s.fail(w, r, errors.New("Passkey 凭据 ID 无效"))
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	memberID, err := strconv.ParseInt(r.FormValue("member_id"), 10, 64)
	if err != nil || memberID < 0 {
		s.fail(w, r, errors.New("成员 ID 无效"))
		return
	}
	if err := s.Store.BindPasskeyCredentialMember(r.Context(), s.DevActorID, credentialID, memberID); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/admin/passkeys")
}

func (s *Server) adminBindPasskeyIdentityV2(w http.ResponseWriter, r *http.Request) {
	identityID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || identityID <= 0 {
		s.fail(w, r, errors.New("Passkey 登录身份 ID 无效"))
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	memberID, err := strconv.ParseInt(r.FormValue("member_id"), 10, 64)
	if err != nil || memberID < 0 {
		s.fail(w, r, errors.New("成员 ID 无效"))
		return
	}
	if err := s.Store.BindPasskeyIdentityDefaultAudited(r.Context(), s.DevActorID, identityID, memberID); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/admin/passkeys")
}

func (s *Server) WithPasskeyCredentialBindings(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/passkey/login/finish", s.passkeyIdentityLoginFinishV2)
	mux.HandleFunc("GET /passkey/account", s.passkeyIdentityAccountPageV2)
	mux.HandleFunc("GET /admin/passkeys", s.adminOnly(s.adminPasskeyIdentitiesPageV2))
	mux.HandleFunc("POST /admin/passkeys/{id}/bind", s.adminOnly(s.adminBindPasskeyIdentityV2))
	mux.HandleFunc("POST /admin/passkeys/credentials/{id}/bind", s.adminOnly(s.adminBindPasskeyCredentialV2))
	mux.Handle("/", next)
	return mux
}
