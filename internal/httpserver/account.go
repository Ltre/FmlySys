package httpserver

import (
	"net/http"

	"github.com/Ltre/FmlySys/internal/store"
)

const totpIdentityCookie = "fmly_totp_identity"

type accountLoginMethodsView struct {
	Title       string
	Member      store.Member
	Partition   string
	Passkeys    []store.MemberPasskeyLoginMethod
	TOTPMethods []store.MemberTOTPLoginMethod
	WeChat      []store.MemberWeChatLoginMethod
}

type totpPendingView struct {
	Title    string
	Identity store.TOTPLoginIdentity
	Inactive bool
}

func (s *Server) renderAccountLoginMethods(w http.ResponseWriter, r *http.Request, member store.Member) {
	passkeys, err := s.Store.PasskeyLoginMethodsForMember(r.Context(), member.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	totpMethods, err := s.Store.TOTPLoginMethodsForMember(r.Context(), member.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	wechatMethods, err := s.Store.WeChatLoginMethodsForMember(r.Context(), member.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	v := accountLoginMethodsView{
		Title:       "账号 / 登录身份管理",
		Member:      member,
		Partition:   s.PM.ActiveID,
		Passkeys:    passkeys,
		TOTPMethods: totpMethods,
		WeChat:      wechatMethods,
	}
	if err := s.Templates.ExecuteTemplate(w, "account-login-methods.html", v); err != nil {
		http.Error(w, "账号管理页面暂时不可用", http.StatusInternalServerError)
	}
}

// ensureMemberSessionForID turns an authenticated login identity into a
// family-member session only after a live administrator association exists.
func (s *Server) ensureMemberSessionForID(w http.ResponseWriter, r *http.Request, memberID int64) (store.Member, map[string]bool, error) {
	if memberID <= 0 {
		return store.Member{}, nil, http.ErrNoCookie
	}
	if raw := cookieValue(r, "fmly_session"); raw != "" {
		if member, permissions, err := s.Store.MemberFromSession(r.Context(), raw); err == nil && member.ID == memberID {
			return member, permissions, nil
		}
		s.Store.DeleteMemberSession(r.Context(), raw)
		clearCookie(w, r, "fmly_session", "/")
	}
	member, err := s.Store.MemberByID(r.Context(), memberID)
	if err != nil {
		return store.Member{}, nil, err
	}
	raw, err := s.Store.CreateMemberSession(r.Context(), member.ID)
	if err != nil {
		return store.Member{}, nil, err
	}
	setCookie(w, r, "fmly_session", raw, "/", int(store.MemberSessionTTL.Seconds()))
	return s.Store.MemberFromSession(r.Context(), raw)
}

func (s *Server) totpLoginPendingPage(w http.ResponseWriter, r *http.Request) {
	identity, err := s.Store.TOTPLoginIdentityFromSession(r.Context(), cookieValue(r, totpIdentityCookie))
	if err != nil {
		s.Store.DeleteTOTPLoginIdentitySession(r.Context(), cookieValue(r, totpIdentityCookie))
		clearCookie(w, r, totpIdentityCookie, "/")
		redirect(w, r, "/login")
		return
	}
	if identity.MemberID > 0 && identity.MemberName != "" {
		if _, _, err := s.ensureMemberSessionForID(w, r, identity.MemberID); err != nil {
			s.fail(w, r, err)
			return
		}
		redirect(w, r, "/")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	v := totpPendingView{Title: "等待管理员关联", Identity: identity, Inactive: identity.MemberID > 0}
	if err := s.Templates.ExecuteTemplate(w, "totp-pending.html", v); err != nil {
		http.Error(w, "2FA 身份状态页面暂时不可用", http.StatusInternalServerError)
	}
}
