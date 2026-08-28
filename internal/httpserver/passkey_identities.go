package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/Ltre/FmlySys/internal/store"
)

const (
	passkeyIdentityCookie       = "fmly_passkey_identity"
	passkeyIdentityCreateCookie = "fmly_passkey_identity_create"
	passkeyIdentityLoginCookie  = "fmly_passkey_identity_login"
	passkeyIdentityAddCookie    = "fmly_passkey_identity_add"
)

type passkeyIdentityInput struct {
	Phone  string `json:"phone"`
	Remark string `json:"remark"`
}

type passkeyIdentityAccountView struct {
	Title         string
	Identity      store.PasskeyLoginIdentityView
	Fresh         bool
	Recovered     bool
	MemberSession bool
}

type passkeyIdentityAdminView struct {
	Title           string
	ActivePartition string
	AdminUsername   string
	Identities      []store.PasskeyLoginIdentityView
	Members         []store.Member
}

func (s *Server) WithPasskeyIdentities(next http.Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /auth/passkey/create/options", s.passkeyIdentityCreateOptions)
	mux.HandleFunc("POST /auth/passkey/create/finish", s.passkeyIdentityCreateFinish)
	mux.HandleFunc("POST /auth/passkey/login/options", s.passkeyIdentityLoginOptions)
	mux.HandleFunc("POST /auth/passkey/login/finish", s.passkeyIdentityLoginFinish)

	mux.HandleFunc("GET /passkey/account", s.passkeyIdentityAccountPage)
	mux.HandleFunc("POST /passkey/account/register/options", s.passkeyIdentityAddOptions)
	mux.HandleFunc("POST /passkey/account/register/finish", s.passkeyIdentityAddFinish)
	mux.HandleFunc("POST /passkey/logout", s.passkeyIdentityLogout)

	mux.HandleFunc("GET /passkeys", s.member("", s.passkeyIdentityMemberPage))
	mux.HandleFunc("POST /passkeys/register/options", s.legacyPasskeyRegistrationDisabled)
	mux.HandleFunc("POST /passkeys/register/finish", s.legacyPasskeyRegistrationDisabled)
	mux.HandleFunc("POST /passkeys/{id}/delete", s.legacyPasskeyRegistrationDisabled)

	mux.HandleFunc("GET /admin/passkeys", s.adminOnly(s.adminPasskeyIdentitiesPage))
	mux.HandleFunc("POST /admin/passkeys/{id}/bind", s.adminOnly(s.adminBindPasskeyIdentity))
	mux.HandleFunc("POST /admin/passkeys/credentials/{id}/delete", s.adminOnly(s.adminDeletePasskeyCredential))

	// Ensure a normal family logout also invalidates the Passkey identity session.
	mux.HandleFunc("POST /logout", s.passkeyAwareLogout)

	mux.Handle("/", next)
	return mux
}

func decodePasskeyIdentityInput(w http.ResponseWriter, r *http.Request) (passkeyIdentityInput, error) {
	var input passkeyIdentityInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&input); err != nil {
		return passkeyIdentityInput{}, errors.New("请求参数无效")
	}
	return input, nil
}

func writePasskeyOptions(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(value)
}

func registrationOptions(user webauthn.User) []webauthn.RegistrationOption {
	credentials := user.WebAuthnCredentials()
	exclusions := make([]protocol.CredentialDescriptor, 0, len(credentials))
	for _, credential := range credentials {
		exclusions = append(exclusions, credential.Descriptor())
	}
	options := []webauthn.RegistrationOption{
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{UserVerification: protocol.VerificationRequired}),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
	}
	if len(exclusions) > 0 {
		options = append(options, webauthn.WithExclusions(exclusions))
	}
	return options
}

func (s *Server) passkeyIdentityCreateOptions(w http.ResponseWriter, r *http.Request) {
	wa, rpID, err := passkeyWebAuthn(r)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	input, err := decodePasskeyIdentityInput(w, r)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	user, err := s.Store.NewPasskeyLoginCandidate(r.Context(), input.Phone, input.Remark)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	creation, session, err := wa.BeginRegistration(user, registrationOptions(user)...)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	token, err := s.Store.CreatePasskeyLoginCeremony(r.Context(), "create", 0, rpID, user.Phone, user.DisplayName, session)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	setPasskeyCeremonyCookie(w, r, passkeyIdentityCreateCookie, token)
	writePasskeyOptions(w, creation)
}

func (s *Server) passkeyIdentityCreateFinish(w http.ResponseWriter, r *http.Request) {
	wa, rpID, err := passkeyWebAuthn(r)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	ceremony, err := s.Store.TakePasskeyLoginCeremony(r.Context(), cookieValue(r, passkeyIdentityCreateCookie), "create", rpID)
	clearPasskeyCeremonyCookie(w, r, passkeyIdentityCreateCookie)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	user := store.PasskeyLoginUser{
		Phone:       ceremony.Phone,
		DisplayName: ceremony.Remark,
		UserHandle:  ceremony.Session.UserID,
	}
	credential, err := wa.FinishRegistration(user, ceremony.Session, r)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	identityID, err := s.Store.CreatePasskeyLoginIdentity(r.Context(), ceremony.Phone, ceremony.Remark, rpID, ceremony.Session.UserID, credential)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	if err := s.establishPasskeyIdentitySession(w, r, identityID, 0); err != nil {
		passkeyJSONError(w, err, http.StatusInternalServerError)
		return
	}
	passkeyJSONOK(w, "/passkey/account?created=1")
}

func (s *Server) passkeyIdentityLoginOptions(w http.ResponseWriter, r *http.Request) {
	wa, rpID, err := passkeyWebAuthn(r)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	input, err := decodePasskeyIdentityInput(w, r)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	user, err := s.Store.PasskeyLoginUserByPhone(r.Context(), input.Phone, rpID)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	assertion, session, err := wa.BeginLogin(user, webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	token, err := s.Store.CreatePasskeyLoginCeremony(r.Context(), "login", user.IdentityID, rpID, user.Phone, "", session)
	if err != nil {
		passkeyJSONError(w, err, http.StatusInternalServerError)
		return
	}
	setPasskeyCeremonyCookie(w, r, passkeyIdentityLoginCookie, token)
	writePasskeyOptions(w, assertion)
}

func (s *Server) passkeyIdentityLoginFinish(w http.ResponseWriter, r *http.Request) {
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
	if err := s.establishPasskeyIdentitySession(w, r, user.IdentityID, user.MemberID); err != nil {
		passkeyJSONError(w, err, http.StatusInternalServerError)
		return
	}
	passkeyJSONOK(w, "/passkey/account?recovered=1")
}

func (s *Server) establishPasskeyIdentitySession(w http.ResponseWriter, r *http.Request, identityID, memberID int64) error {
	s.Store.DeletePasskeyLoginIdentitySession(r.Context(), cookieValue(r, passkeyIdentityCookie))
	raw, err := s.Store.CreatePasskeyLoginIdentitySession(r.Context(), identityID)
	if err != nil {
		return err
	}
	setCookie(w, r, passkeyIdentityCookie, raw, "/", int(store.PasskeyLoginIdentitySessionTTL.Seconds()))
	if memberID > 0 {
		memberSession, err := s.Store.CreateMemberSession(r.Context(), memberID)
		if err != nil {
			return err
		}
		setCookie(w, r, "fmly_session", memberSession, "/", int(store.MemberSessionTTL.Seconds()))
	}
	return nil
}

func (s *Server) passkeyIdentityAccountPage(w http.ResponseWriter, r *http.Request) {
	identity, fresh, err := s.Store.PasskeyLoginIdentityFromSession(r.Context(), cookieValue(r, passkeyIdentityCookie))
	if err != nil {
		clearCookie(w, r, passkeyIdentityCookie, "/")
		redirect(w, r, "/login")
		return
	}
	memberSession := false
	if identity.MemberID > 0 {
		if raw := cookieValue(r, "fmly_session"); raw != "" {
			if member, _, memberErr := s.Store.MemberFromSession(r.Context(), raw); memberErr == nil && member.ID == identity.MemberID {
				memberSession = true
			}
		}
		if !memberSession {
			raw, memberErr := s.Store.CreateMemberSession(r.Context(), identity.MemberID)
			if memberErr == nil {
				setCookie(w, r, "fmly_session", raw, "/", int(store.MemberSessionTTL.Seconds()))
				memberSession = true
			}
		}
	}
	v := passkeyIdentityAccountView{
		Title:         "Passkey 登录身份",
		Identity:      identity,
		Fresh:         fresh,
		Recovered:     r.URL.Query().Get("recovered") == "1",
		MemberSession: memberSession,
	}
	if err := s.Templates.ExecuteTemplate(w, "passkey-account.html", v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) freshPasskeyIdentity(r *http.Request) (store.PasskeyLoginIdentityView, error) {
	identity, fresh, err := s.Store.PasskeyLoginIdentityFromSession(r.Context(), cookieValue(r, passkeyIdentityCookie))
	if err != nil {
		return store.PasskeyLoginIdentityView{}, errors.New("Passkey 登录身份会话已失效，请重新使用已有 Passkey 登录")
	}
	if !fresh {
		return store.PasskeyLoginIdentityView{}, errors.New("为防止会话被盗后新增凭据，请重新使用已有 Passkey 登录后再添加当前设备")
	}
	return identity, nil
}

func (s *Server) passkeyIdentityAddOptions(w http.ResponseWriter, r *http.Request) {
	wa, rpID, err := passkeyWebAuthn(r)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	identity, err := s.freshPasskeyIdentity(r)
	if err != nil {
		passkeyJSONError(w, err, http.StatusUnauthorized)
		return
	}
	input, err := decodePasskeyIdentityInput(w, r)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	user, err := s.Store.PasskeyLoginUserByID(r.Context(), identity.ID, rpID)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	creation, session, err := wa.BeginRegistration(user, registrationOptions(user)...)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	token, err := s.Store.CreatePasskeyLoginCeremony(r.Context(), "add", identity.ID, rpID, identity.Phone, input.Remark, session)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	setPasskeyCeremonyCookie(w, r, passkeyIdentityAddCookie, token)
	writePasskeyOptions(w, creation)
}

func (s *Server) passkeyIdentityAddFinish(w http.ResponseWriter, r *http.Request) {
	wa, rpID, err := passkeyWebAuthn(r)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	identity, err := s.freshPasskeyIdentity(r)
	if err != nil {
		passkeyJSONError(w, err, http.StatusUnauthorized)
		return
	}
	ceremony, err := s.Store.TakePasskeyLoginCeremony(r.Context(), cookieValue(r, passkeyIdentityAddCookie), "add", rpID)
	clearPasskeyCeremonyCookie(w, r, passkeyIdentityAddCookie)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	if ceremony.IdentityID != identity.ID {
		passkeyJSONError(w, errors.New("新增 Passkey 的身份与当前登录身份不一致"), http.StatusForbidden)
		return
	}
	user, err := s.Store.PasskeyLoginUserByID(r.Context(), identity.ID, rpID)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	credential, err := wa.FinishRegistration(user, ceremony.Session, r)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	if err := s.Store.SavePasskeyLoginCredential(r.Context(), identity.ID, rpID, ceremony.Remark, credential); err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	passkeyJSONOK(w, "/passkey/account")
}

func (s *Server) passkeyIdentityMemberPage(w http.ResponseWriter, r *http.Request) {
	member := currentMember(r)
	if identity, _, err := s.Store.PasskeyLoginIdentityFromSession(r.Context(), cookieValue(r, passkeyIdentityCookie)); err == nil && identity.MemberID == member.ID {
		redirect(w, r, "/passkey/account")
		return
	}
	v := passkeyPageView{
		Title:           "通行密钥",
		ActivePartition: s.PM.ActiveID,
		CurrentMember:   member,
		Permissions:     currentPermissions(r),
	}
	s.renderPasskeyTemplate(w, "passkeys.html", v)
}

func (s *Server) legacyPasskeyRegistrationDisabled(w http.ResponseWriter, r *http.Request) {
	passkeyJSONError(w, errors.New("旧式“登录后再绑定 Passkey”流程已停用；Passkey 现在独立创建登录身份，请从登录页创建或找回"), http.StatusGone)
}

func (s *Server) adminPasskeyIdentitiesPage(w http.ResponseWriter, r *http.Request) {
	identities, err := s.Store.AllPasskeyLoginIdentities(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	members, err := s.Store.ActiveMembersForPasskey(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v := passkeyIdentityAdminView{
		Title:           "Passkey 登录身份管理",
		ActivePartition: s.PM.ActiveID,
		AdminUsername:   currentAdmin(r).Username,
		Identities:      identities,
		Members:         members,
	}
	if err := s.Templates.ExecuteTemplate(w, "admin-passkeys.html", v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) adminBindPasskeyIdentity(w http.ResponseWriter, r *http.Request) {
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
	if err := s.Store.BindPasskeyLoginIdentity(r.Context(), identityID, memberID); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/admin/passkeys")
}

func (s *Server) adminDeletePasskeyCredential(w http.ResponseWriter, r *http.Request) {
	credentialID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || credentialID <= 0 {
		s.fail(w, r, errors.New("Passkey 凭据 ID 无效"))
		return
	}
	if err := s.Store.DeletePasskeyLoginCredential(r.Context(), credentialID); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/admin/passkeys")
}

func (s *Server) passkeyIdentityLogout(w http.ResponseWriter, r *http.Request) {
	s.Store.DeletePasskeyLoginIdentitySession(r.Context(), cookieValue(r, passkeyIdentityCookie))
	s.Store.DeleteMemberSession(r.Context(), cookieValue(r, "fmly_session"))
	clearCookie(w, r, passkeyIdentityCookie, "/")
	clearCookie(w, r, "fmly_session", "/")
	redirect(w, r, "/login")
}

func (s *Server) passkeyAwareLogout(w http.ResponseWriter, r *http.Request) {
	s.passkeyIdentityLogout(w, r)
}
