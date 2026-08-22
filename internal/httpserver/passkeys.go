package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/Ltre/FmlySys/internal/store"
)

const (
	passkeyLoginCookie    = "fmly_passkey_login"
	passkeyRegisterCookie = "fmly_passkey_register"
)

type passkeyPageView struct {
	Title           string
	ActivePartition string
	CurrentMember   store.Member
	Permissions     map[string]bool
	AdminUsername   string
	Passkeys        []store.PasskeyCredentialView
}

type passkeyRegisterRequest struct {
	Remark string `json:"remark"`
}

type passkeyJSONResponse struct {
	OK       bool   `json:"ok"`
	Message  string `json:"message,omitempty"`
	Redirect string `json:"redirect,omitempty"`
}

func (s *Server) WithPasskeys(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/passkey/login/options", s.passkeyLoginOptions)
	mux.HandleFunc("POST /auth/passkey/login/finish", s.passkeyLoginFinish)
	mux.HandleFunc("GET /passkeys", s.member("", s.passkeyPage))
	mux.HandleFunc("POST /passkeys/register/options", s.member("", s.passkeyRegisterOptions))
	mux.HandleFunc("POST /passkeys/register/finish", s.member("", s.passkeyRegisterFinish))
	mux.HandleFunc("POST /passkeys/{id}/delete", s.member("", s.passkeyDelete))
	mux.HandleFunc("GET /admin/passkeys", s.adminOnly(s.adminPasskeysPage))
	mux.Handle("/", next)
	return mux
}

func passkeyRequestRP(r *http.Request) (rpID, origin string, err error) {
	host := strings.TrimSpace(r.Host)
	if host == "" || strings.ContainsAny(host, "/\\@?#") {
		return "", "", errors.New("当前请求 Host 无法用于 Passkey")
	}
	for _, ch := range host {
		if unicode.IsSpace(ch) || unicode.IsControl(ch) {
			return "", "", errors.New("当前请求 Host 无法用于 Passkey")
		}
	}
	u, err := url.Parse("http://" + host)
	if err != nil || u.Hostname() == "" {
		return "", "", errors.New("当前请求 Host 无法用于 Passkey")
	}
	rpID = strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	port := u.Port()

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := firstForwardedProto(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		if forwarded != "http" && forwarded != "https" {
			return "", "", errors.New("X-Forwarded-Proto 必须为 http 或 https")
		}
		scheme = forwarded
	}
	if scheme != "https" && rpID != "localhost" && rpID != "127.0.0.1" && rpID != "::1" {
		return "", "", errors.New("Passkey 需要 HTTPS 安全上下文；请使用 HTTPS 域名访问，局域网 HTTP 地址只能改用微信登录")
	}
	originHost := rpID
	if port != "" && !((scheme == "https" && port == "443") || (scheme == "http" && port == "80")) {
		originHost = net.JoinHostPort(rpID, port)
	} else if strings.Contains(rpID, ":") {
		originHost = "[" + rpID + "]"
	}
	return rpID, scheme + "://" + originHost, nil
}

func passkeyWebAuthn(r *http.Request) (*webauthn.WebAuthn, string, error) {
	rpID, origin, err := passkeyRequestRP(r)
	if err != nil {
		return nil, "", err
	}
	wa, err := webauthn.New(&webauthn.Config{
		RPID:                  rpID,
		RPDisplayName:         "FmlySys",
		RPOrigins:             []string{origin},
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationRequired,
		},
	})
	if err != nil {
		return nil, "", err
	}
	return wa, rpID, nil
}

func passkeyJSONError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(passkeyJSONResponse{OK: false, Message: err.Error()})
}

func passkeyJSONOK(w http.ResponseWriter, redirectTo string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(passkeyJSONResponse{OK: true, Redirect: redirectTo})
}

func setPasskeyCeremonyCookie(w http.ResponseWriter, r *http.Request, name, token string) {
	setCookie(w, r, name, token, "/", int(store.PasskeyCeremonyTTL.Seconds()))
}

func clearPasskeyCeremonyCookie(w http.ResponseWriter, r *http.Request, name string) {
	clearCookie(w, r, name, "/")
}

func (s *Server) passkeyLoginOptions(w http.ResponseWriter, r *http.Request) {
	wa, rpID, err := passkeyWebAuthn(r)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	assertion, session, err := wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	token, err := s.Store.CreatePasskeyCeremony(r.Context(), "login", 0, rpID, "", session)
	if err != nil {
		passkeyJSONError(w, err, http.StatusInternalServerError)
		return
	}
	setPasskeyCeremonyCookie(w, r, passkeyLoginCookie, token)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(assertion)
}

func (s *Server) passkeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	wa, rpID, err := passkeyWebAuthn(r)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	ceremony, err := s.Store.TakePasskeyCeremony(r.Context(), cookieValue(r, passkeyLoginCookie), "login", rpID)
	clearPasskeyCeremonyCookie(w, r, passkeyLoginCookie)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}

	var memberID int64
	credential, err := wa.FinishDiscoverableLogin(func(rawID, userHandle []byte) (webauthn.User, error) {
		user, lookupErr := s.Store.PasskeyUserByHandle(r.Context(), userHandle, rawID, rpID)
		if lookupErr != nil {
			return nil, lookupErr
		}
		memberID = user.MemberID
		return user, nil
	}, ceremony.Session, r)
	if err != nil {
		passkeyJSONError(w, fmt.Errorf("Passkey 验证失败：%w", err), http.StatusUnauthorized)
		return
	}
	if memberID <= 0 {
		passkeyJSONError(w, errors.New("Passkey 未绑定有效家族成员"), http.StatusUnauthorized)
		return
	}
	if err := s.Store.UpdatePasskeyCredentialAfterLogin(r.Context(), rpID, credential); err != nil {
		passkeyJSONError(w, err, http.StatusInternalServerError)
		return
	}
	sessionToken, err := s.Store.CreateMemberSession(r.Context(), memberID)
	if err != nil {
		passkeyJSONError(w, err, http.StatusInternalServerError)
		return
	}
	setCookie(w, r, "fmly_session", sessionToken, "/", int(store.MemberSessionTTL.Seconds()))
	passkeyJSONOK(w, "/")
}

func (s *Server) passkeyRegisterOptions(w http.ResponseWriter, r *http.Request) {
	wa, rpID, err := passkeyWebAuthn(r)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	var input passkeyRegisterRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&input); err != nil {
		passkeyJSONError(w, errors.New("请输入 Passkey 备注"), http.StatusBadRequest)
		return
	}
	member := currentMember(r)
	user, err := s.Store.PasskeyUserForMember(r.Context(), member.ID, rpID, true)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	exclusions := make([]protocol.CredentialDescriptor, 0, len(user.Credentials))
	for _, credential := range user.Credentials {
		exclusions = append(exclusions, credential.Descriptor())
	}
	options := []webauthn.RegistrationOption{
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{UserVerification: protocol.VerificationRequired}),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
	}
	if len(exclusions) > 0 {
		options = append(options, webauthn.WithExclusions(exclusions))
	}
	creation, session, err := wa.BeginRegistration(user, options...)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	token, err := s.Store.CreatePasskeyCeremony(r.Context(), "register", member.ID, rpID, input.Remark, session)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	setPasskeyCeremonyCookie(w, r, passkeyRegisterCookie, token)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(creation)
}

func (s *Server) passkeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	wa, rpID, err := passkeyWebAuthn(r)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	ceremony, err := s.Store.TakePasskeyCeremony(r.Context(), cookieValue(r, passkeyRegisterCookie), "register", rpID)
	clearPasskeyCeremonyCookie(w, r, passkeyRegisterCookie)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	member := currentMember(r)
	if ceremony.MemberID != member.ID {
		passkeyJSONError(w, errors.New("Passkey 绑定会话与当前成员不匹配"), http.StatusForbidden)
		return
	}
	user, err := s.Store.PasskeyUserForMember(r.Context(), member.ID, rpID, false)
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	credential, err := wa.FinishRegistration(user, ceremony.Session, r)
	if err != nil {
		passkeyJSONError(w, fmt.Errorf("Passkey 绑定失败：%w", err), http.StatusBadRequest)
		return
	}
	if err := s.Store.SavePasskeyCredential(r.Context(), member.ID, rpID, ceremony.Remark, credential); err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	passkeyJSONOK(w, "/passkeys")
}

func (s *Server) passkeyPage(w http.ResponseWriter, r *http.Request) {
	member := currentMember(r)
	passkeys, err := s.Store.MemberPasskeys(r.Context(), member.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v := passkeyPageView{
		Title:           "通行密钥",
		ActivePartition: s.PM.ActiveID,
		CurrentMember:   member,
		Permissions:     currentPermissions(r),
		Passkeys:        passkeys,
	}
	s.renderPasskeyTemplate(w, "passkeys.html", v)
}

func (s *Server) adminPasskeysPage(w http.ResponseWriter, r *http.Request) {
	passkeys, err := s.Store.AllPasskeys(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v := passkeyPageView{
		Title:           "Passkey 管理",
		ActivePartition: s.PM.ActiveID,
		AdminUsername:   currentAdmin(r).Username,
		Passkeys:        passkeys,
	}
	s.renderPasskeyTemplate(w, "admin-passkeys.html", v)
}

func (s *Server) renderPasskeyTemplate(w http.ResponseWriter, name string, v passkeyPageView) {
	if err := s.Templates.ExecuteTemplate(w, name, v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) passkeyDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		s.fail(w, r, errors.New("Passkey ID 无效"))
		return
	}
	if err := s.Store.DeleteOwnPasskey(r.Context(), currentMember(r).ID, id); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/passkeys")
}
