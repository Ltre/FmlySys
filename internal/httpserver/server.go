package httpserver

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/Ltre/FmlySys/internal/adminauth"
	"github.com/Ltre/FmlySys/internal/asset"
	"github.com/Ltre/FmlySys/internal/config"
	"github.com/Ltre/FmlySys/internal/partition"
	"github.com/Ltre/FmlySys/internal/store"
	"github.com/Ltre/FmlySys/internal/wechat"
	webassets "github.com/Ltre/FmlySys/web"
)

type contextKey string

const (
	memberContextKey contextKey = "fmly-member"
	permsContextKey  contextKey = "fmly-permissions"
	adminContextKey  contextKey = "fmly-admin"
)

type Server struct {
	PM         *partition.Manager
	Store      *store.Store
	Admin      *adminauth.Service
	Config     config.Config
	DevActorID int64
	Templates  *template.Template
	mux        *http.ServeMux
}

type view struct {
	Title             string
	ActivePartition   string
	Summary           store.AssetSummary
	CurrentMember     store.Member
	CurrentBalance    int64
	Permissions       map[string]bool
	PermissionCatalog []store.PermissionDef
	MemberPermissions map[int64]map[string]bool
	Members           []store.Member
	Expenses          []store.Expense
	Expense           store.Expense
	AssetEvents       []store.AssetEvent
	AdminAssetEvents  []store.AssetEvent
	AssetInflows      []store.AssetInflowOption
	Transfers         []store.Transfer
	Reimbursements    []store.Reimbursement
	ExpenseAudits     []store.AuditLog
	ExpenseRefunds    []store.Reimbursement
	Matters           []store.Matter
	Archives          []store.Archive
	AdminQuickNotes   []store.AdminQuickMoneyNote
	PendingJoins      []store.JoinRequest
	JoinRequest       store.JoinRequest
	WeChatConfigured  bool
	DevAuthEnabled    bool
	AdminConfigured   bool
	AdminUsername     string
	TOTPSecret        string
	TOTPURI           string
	Error             string
	Message           string
}

func New(pm *partition.Manager, st *store.Store, admin *adminauth.Service, cfg config.Config, devActorID int64) (*Server, error) {
	funcs := template.FuncMap{
		"money": asset.FormatYuan,
		"humanDate": func(v string) string {
			if len(v) >= 10 {
				return v[:10]
			}
			return v
		},
		"formDT": func(v string) string {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				return t.Local().Format("2006-01-02T15:04")
			}
			return ""
		},
		"hasPerm": func(perms map[string]bool, key string) bool { return perms[key] },
		"memberHasPerm": func(all map[int64]map[string]bool, id int64, key string) bool {
			return all[id] != nil && all[id][key]
		},
		"defaultPerm": store.IsDefaultPermission,
	}
	t, err := template.New("").Funcs(funcs).ParseFS(webassets.FS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	s := &Server{PM: pm, Store: st, Admin: admin, Config: cfg, DevActorID: devActorID, Templates: t, mux: http.NewServeMux()}
	s.routes()
	return s, nil
}

func (s *Server) Handler() http.Handler { return logging(s.mux) }

func (s *Server) base(title string) view {
	return view{
		Title:             title,
		ActivePartition:   s.PM.ActiveID,
		PermissionCatalog: store.PermissionCatalog,
		WeChatConfigured:  s.Config.WeChatConfigured(),
		DevAuthEnabled:    s.Config.DevAuthEnabled,
	}
}

func (s *Server) routes() {
	staticFS, _ := fs.Sub(webassets.FS, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok\n")) })

	// Member authentication and join request flow.
	s.mux.HandleFunc("GET /login", s.loginPage)
	s.mux.HandleFunc("POST /login/dev", s.devLogin)
	s.mux.HandleFunc("GET /login/wechat", s.wechatLogin)
	s.mux.HandleFunc("GET "+WeChatCallbackPath, s.wechatCallback)
	s.mux.HandleFunc("GET /join", s.joinPage)
	s.mux.HandleFunc("POST /join", s.submitJoin)
	s.mux.HandleFunc("POST /logout", s.memberLogout)

	// Administrator password + Google Authenticator flow.
	s.mux.HandleFunc("GET /admin/login", s.adminLoginPage)
	s.mux.HandleFunc("POST /admin/login", s.adminLogin)
	s.mux.HandleFunc("GET /admin/totp/setup", s.adminTOTPSetupPage)
	s.mux.HandleFunc("GET /admin/totp/qr", s.adminTOTPQRCode)
	s.mux.HandleFunc("POST /admin/totp/setup", s.adminTOTPSetup)
	s.mux.HandleFunc("GET /admin/totp", s.adminTOTPPage)
	s.mux.HandleFunc("POST /admin/totp", s.adminTOTPVerify)
	s.mux.HandleFunc("POST /admin/logout", s.adminLogout)

	// Family member pages and writes.
	s.mux.HandleFunc("GET /", s.member("", s.dashboard))
	s.mux.HandleFunc("GET /assets", s.member("assets.view", s.assets))
	s.mux.HandleFunc("POST /assets/self-events", s.member("assets.self_change", s.createSelfAssetEvent))
	s.mux.HandleFunc("POST /assets/expenses", s.member("expenses.create", s.createExpense))
	s.mux.HandleFunc("GET /assets/expenses/{id}/edit", s.memberOrAdmin("assets.view", s.editExpense))
	s.mux.HandleFunc("POST /assets/expenses/{id}", s.memberOrAdmin("expenses.edit", s.updateExpense))
	s.mux.HandleFunc("POST /assets/transfers", s.member("transfers.create", s.createTransfer))
	s.mux.HandleFunc("POST /assets/reimbursements", s.member("reimbursements.create", s.createReimbursement))
	s.mux.HandleFunc("GET /evidence/{id}", s.memberOrAdmin("assets.view", s.evidence))

	s.mux.HandleFunc("GET /matters", s.member("matters.view", s.matters))
	s.mux.HandleFunc("POST /matters", s.member("matters.manage", s.createMatter))
	s.mux.HandleFunc("POST /matters/{id}", s.member("matters.manage", s.updateMatter))
	s.mux.HandleFunc("POST /matters/{id}/status", s.member("matters.manage", s.setMatterStatus))
	s.mux.HandleFunc("GET /share", s.member("share.view", s.share))
	s.mux.HandleFunc("POST /share", s.member("share.manage", s.createArchive))
	s.mux.HandleFunc("POST /share/{id}", s.member("share.manage", s.updateArchive))
	s.mux.HandleFunc("POST /share/{id}/attachments", s.member("share.manage", s.uploadArchive))
	s.mux.HandleFunc("POST /share/{id}/attachments/{attachmentID}/delete", s.member("share.manage", s.deleteArchiveAttachment))
	s.mux.HandleFunc("GET /files/{id}", s.member("share.view", s.file))

	// Authenticated administrator business console.
	s.mux.HandleFunc("GET /admin", s.adminOnly(s.admin))
	s.mux.HandleFunc("GET /admin/authorities", s.adminOnly(s.adminAuthorities))
	s.mux.HandleFunc("POST /admin/members", s.adminOnly(s.adminCreateMember))
	s.mux.HandleFunc("POST /admin/members/{id}/permissions", s.adminOnly(s.adminSetPermissions))
	s.mux.HandleFunc("POST /admin/join/{id}/approve", s.adminOnly(s.adminApproveJoin))
	s.mux.HandleFunc("POST /admin/join/{id}/reject", s.adminOnly(s.adminRejectJoin))
	s.mux.HandleFunc("POST /admin/assets/events", s.adminOnly(s.adminCreateAssetEvent))
	s.mux.HandleFunc("POST /admin/assets/expenses", s.adminOnly(s.adminCreateExpense))
	s.mux.HandleFunc("POST /admin/assets/transfers", s.adminOnly(s.adminCreateTransfer))
	s.mux.HandleFunc("POST /admin/assets/reimbursements", s.adminOnly(s.adminCreateReimbursement))
}

func (s *Server) render(w http.ResponseWriter, name string, v view) {
	if err := s.Templates.ExecuteTemplate(w, name, v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) fail(w http.ResponseWriter, _ *http.Request, err error) {
	http.Error(w, err.Error(), http.StatusBadRequest)
}

func redirect(w http.ResponseWriter, r *http.Request, to string) {
	http.Redirect(w, r, to, http.StatusSeeOther)
}

func secureCookie(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func setCookie(w http.ResponseWriter, r *http.Request, name, value, path string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: path, MaxAge: maxAge, HttpOnly: true, Secure: secureCookie(r), SameSite: http.SameSiteLaxMode})
}

func clearCookie(w http.ResponseWriter, r *http.Request, name, path string) {
	setCookie(w, r, name, "", path, -1)
}

func cookieValue(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *Server) member(permission string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := cookieValue(r, "fmly_session")
		m, perms, err := s.Store.MemberFromSession(r.Context(), raw)
		if err != nil {
			clearCookie(w, r, "fmly_session", "/")
			redirect(w, r, "/login")
			return
		}
		if permission != "" && !perms[permission] {
			http.Error(w, "你没有执行该操作的家族权限", http.StatusForbidden)
			return
		}
		ctx := context.WithValue(r.Context(), memberContextKey, m)
		ctx = context.WithValue(ctx, permsContextKey, perms)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := cookieValue(r, "fmly_admin_session")
		sess, err := s.Admin.Session(r.Context(), raw)
		if err != nil || sess.Stage != "authenticated" {
			clearCookie(w, r, "fmly_admin_session", "/")
			redirect(w, r, "/admin/login")
			return
		}
		ctx := context.WithValue(r.Context(), adminContextKey, sess)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) memberOrAdmin(permission string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if raw := cookieValue(r, "fmly_admin_session"); raw != "" {
			if sess, err := s.Admin.Session(r.Context(), raw); err == nil && sess.Stage == "authenticated" {
				ctx := context.WithValue(r.Context(), adminContextKey, sess)
				next(w, r.WithContext(ctx))
				return
			}
		}
		s.member(permission, next)(w, r)
	}
}

func currentMember(r *http.Request) store.Member {
	m, _ := r.Context().Value(memberContextKey).(store.Member)
	return m
}

func currentPermissions(r *http.Request) map[string]bool {
	p, _ := r.Context().Value(permsContextKey).(map[string]bool)
	if p == nil {
		return map[string]bool{}
	}
	return p
}

func currentAdmin(r *http.Request) adminauth.Session {
	a, _ := r.Context().Value(adminContextKey).(adminauth.Session)
	return a
}

func (s *Server) businessActor(r *http.Request) int64 {
	if m := currentMember(r); m.ID != 0 {
		return m.ID
	}
	return s.DevActorID
}

// ---- Member login / WeChat pending registration ----

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if raw := cookieValue(r, "fmly_session"); raw != "" {
		if _, _, err := s.Store.MemberFromSession(r.Context(), raw); err == nil {
			redirect(w, r, "/")
			return
		}
	}
	v := s.base("登录 FmlySys")
	s.render(w, "login.html", v)
}

func (s *Server) devLogin(w http.ResponseWriter, r *http.Request) {
	if !s.Config.DevAuthEnabled {
		http.NotFound(w, r)
		return
	}
	raw, err := s.Store.CreateMemberSession(r.Context(), s.DevActorID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setCookie(w, r, "fmly_session", raw, "/", int(store.MemberSessionTTL.Seconds()))
	redirect(w, r, "/")
}

func (s *Server) wechatLogin(w http.ResponseWriter, r *http.Request) {
	if !s.Config.WeChatConfigured() {
		http.Error(w, "微信登录尚未配置，请设置 FMLYSYS_WECHAT_APP_ID / FMLYSYS_WECHAT_APP_SECRET", http.StatusServiceUnavailable)
		return
	}
	callbackURL, err := WeChatCallbackURL(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	state, err := randomToken()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setCookie(w, r, "fmly_wechat_state", state, WeChatCallbackPath, 600)
	client := wechat.New(s.Config.WeChatAppID, s.Config.WeChatAppSecret)
	http.Redirect(w, r, client.LoginURL(state, callbackURL), http.StatusFound)
}

func (s *Server) wechatCallback(w http.ResponseWriter, r *http.Request) {
	want := cookieValue(r, "fmly_wechat_state")
	got := r.URL.Query().Get("state")
	clearCookie(w, r, "fmly_wechat_state", WeChatCallbackPath)
	if want == "" || got == "" || subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
		http.Error(w, "微信登录 state 校验失败，请重新扫码", http.StatusBadRequest)
		return
	}
	client := wechat.New(s.Config.WeChatAppID, s.Config.WeChatAppSecret)
	profile, err := client.Profile(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	resolved, err := s.Store.ResolveWeChat(r.Context(), profile.OpenID, profile.UnionID, profile.Nickname)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if resolved.MemberID > 0 {
		raw, err := s.Store.CreateMemberSession(r.Context(), resolved.MemberID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		setCookie(w, r, "fmly_session", raw, "/", int(store.MemberSessionTTL.Seconds()))
		clearCookie(w, r, "fmly_join", "/join")
		redirect(w, r, "/")
		return
	}
	setCookie(w, r, "fmly_join", resolved.JoinToken, "/join", int(store.JoinAccessTTL.Seconds()))
	redirect(w, r, "/join")
}

func (s *Server) joinPage(w http.ResponseWriter, r *http.Request) {
	req, err := s.Store.JoinRequestByToken(r.Context(), cookieValue(r, "fmly_join"))
	if err != nil {
		redirect(w, r, "/login")
		return
	}
	v := s.base("申请加入家族")
	v.JoinRequest = req
	s.render(w, "join.html", v)
}

func (s *Server) submitJoin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.SubmitJoinRequest(r.Context(), cookieValue(r, "fmly_join"), r.FormValue("real_name"), r.FormValue("relation")); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/join")
}

func (s *Server) memberLogout(w http.ResponseWriter, r *http.Request) {
	s.Store.DeleteMemberSession(r.Context(), cookieValue(r, "fmly_session"))
	clearCookie(w, r, "fmly_session", "/")
	redirect(w, r, "/login")
}

// ---- Administrator login / Google Authenticator ----

func (s *Server) adminLoginPage(w http.ResponseWriter, r *http.Request) {
	if raw := cookieValue(r, "fmly_admin_session"); raw != "" {
		if sess, err := s.Admin.Session(r.Context(), raw); err == nil && sess.Stage == "authenticated" {
			redirect(w, r, "/admin")
			return
		}
	}
	v := s.base("后台登录")
	v.AdminConfigured, _ = s.Admin.HasAdmin(r.Context())
	s.render(w, "admin-login.html", v)
}

func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
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
	raw, err := s.Admin.BeginSession(r.Context(), u.ID, stage)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setCookie(w, r, "fmly_admin_session", raw, "/", int((12 * time.Hour).Seconds()))
	if stage == "totp_setup" {
		redirect(w, r, "/admin/totp/setup")
	} else {
		redirect(w, r, "/admin/totp")
	}
}

func (s *Server) adminStage(r *http.Request, stage string) (adminauth.Session, string, error) {
	raw := cookieValue(r, "fmly_admin_session")
	sess, err := s.Admin.Session(r.Context(), raw)
	if err != nil || sess.Stage != stage {
		return adminauth.Session{}, raw, errorsForStage(stage)
	}
	return sess, raw, nil
}

func errorsForStage(stage string) error {
	return fmt.Errorf("后台认证状态无效（%s），请重新登录", stage)
}

func (s *Server) adminTOTPSetupPage(w http.ResponseWriter, r *http.Request) {
	sess, _, err := s.adminStage(r, "totp_setup")
	if err != nil {
		redirect(w, r, "/admin/login")
		return
	}
	secret, err := s.Admin.EnsureTOTPSecret(r.Context(), sess.UserID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v := s.base("绑定 Google Authenticator")
	v.AdminUsername = sess.Username
	v.TOTPSecret = secret
	v.TOTPURI = adminauth.OTPAuthURI(sess.Username, secret)
	s.render(w, "admin-totp-setup.html", v)
}

func (s *Server) adminTOTPQRCode(w http.ResponseWriter, r *http.Request) {
	sess, _, err := s.adminStage(r, "totp_setup")
	if err != nil {
		http.Error(w, "后台认证状态无效", http.StatusUnauthorized)
		return
	}
	secret, err := s.Admin.EnsureTOTPSecret(r.Context(), sess.UserID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	png, err := qrcode.Encode(adminauth.OTPAuthURI(sess.Username, secret), qrcode.Medium, 256)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png)
}

func (s *Server) adminTOTPSetup(w http.ResponseWriter, r *http.Request) {
	sess, raw, err := s.adminStage(r, "totp_setup")
	if err != nil {
		redirect(w, r, "/admin/login")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Admin.ConfirmTOTP(r.Context(), sess.UserID, r.FormValue("code")); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Admin.SetSessionStage(r.Context(), raw, "authenticated"); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/admin")
}

func (s *Server) adminTOTPPage(w http.ResponseWriter, r *http.Request) {
	sess, _, err := s.adminStage(r, "totp_verify")
	if err != nil {
		redirect(w, r, "/admin/login")
		return
	}
	v := s.base("Google Authenticator 验证")
	v.AdminUsername = sess.Username
	s.render(w, "admin-totp.html", v)
}

func (s *Server) adminTOTPVerify(w http.ResponseWriter, r *http.Request) {
	sess, raw, err := s.adminStage(r, "totp_verify")
	if err != nil {
		redirect(w, r, "/admin/login")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Admin.VerifyTOTP(r.Context(), sess.UserID, r.FormValue("code")); err != nil {
		v := s.base("Google Authenticator 验证")
		v.AdminUsername = sess.Username
		v.Error = err.Error()
		s.render(w, "admin-totp.html", v)
		return
	}
	if err := s.Admin.SetSessionStage(r.Context(), raw, "authenticated"); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/admin")
}

func (s *Server) adminLogout(w http.ResponseWriter, r *http.Request) {
	s.Admin.DeleteSession(r.Context(), cookieValue(r, "fmly_admin_session"))
	clearCookie(w, r, "fmly_admin_session", "/")
	redirect(w, r, "/admin/login")
}

// ---- Family business pages ----

func (s *Server) familyMembers(ctx context.Context) ([]store.Member, error) {
	members, err := s.Store.Members(ctx)
	if err != nil {
		return nil, err
	}
	if s.Config.DevAuthEnabled {
		return members, nil
	}
	out := members[:0]
	for _, m := range members {
		if m.ID != s.DevActorID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (s *Server) populateCommon(r *http.Request, v *view, currentID int64) error {
	var err error
	if currentID > 0 {
		v.CurrentMember, err = s.Store.MemberByID(r.Context(), currentID)
		if err != nil {
			return err
		}
		v.CurrentBalance, err = s.Store.HolderBalanceV2(r.Context(), currentID)
		if err != nil {
			return err
		}
	}
	v.Summary, err = s.Store.AssetSummaryV2(r.Context())
	if err != nil {
		return err
	}
	v.Members, err = s.familyMembers(r.Context())
	if err != nil {
		return err
	}
	v.Expenses, err = s.Store.ExpensesV2(r.Context())
	if err != nil {
		return err
	}
	for i := range v.Expenses {
		v.Expenses[i].Evidence, _ = s.Store.EvidenceFor(r.Context(), "expense", v.Expenses[i].ID)
	}
	v.Transfers, err = s.Store.Transfers(r.Context())
	if err != nil {
		return err
	}
	for i := range v.Transfers {
		v.Transfers[i].Evidence, _ = s.Store.EvidenceFor(r.Context(), "transfer", v.Transfers[i].ID)
	}
	v.Reimbursements, err = s.Store.Reimbursements(r.Context())
	if err != nil {
		return err
	}
	for i := range v.Reimbursements {
		v.Reimbursements[i].Evidence, _ = s.Store.EvidenceFor(r.Context(), "reimbursement", v.Reimbursements[i].ID)
	}
	v.Matters, err = s.Store.Matters(r.Context())
	return err
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	v := s.base("FmlySys")
	v.CurrentMember = currentMember(r)
	v.Permissions = currentPermissions(r)
	var err error
	if v.Permissions["assets.view"] {
		v.Summary, err = s.Store.AssetSummaryV2(r.Context())
		if err != nil {
			s.fail(w, r, err)
			return
		}
	}
	if v.Permissions["matters.view"] {
		v.Matters, err = s.Store.Matters(r.Context())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if len(v.Matters) > 6 {
			v.Matters = v.Matters[:6]
		}
	}
	if v.Permissions["share.view"] {
		v.Archives, err = s.Store.FamilyArchives(r.Context())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if len(v.Archives) > 6 {
			v.Archives = v.Archives[:6]
		}
	}
	s.render(w, "dashboard.html", v)
}

func (s *Server) assets(w http.ResponseWriter, r *http.Request) {
	m := currentMember(r)
	v := s.base("公共资产")
	v.Permissions = currentPermissions(r)
	if err := s.populateCommon(r, &v, m.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	v.AssetEvents, _ = s.Store.AssetEvents(r.Context())
	s.render(w, "assets.html", v)
}

func (s *Server) admin(w http.ResponseWriter, r *http.Request) {
	v := s.base("管理后台")
	v.AdminUsername = currentAdmin(r).Username
	if err := s.populateCommon(r, &v, 0); err != nil {
		s.fail(w, r, err)
		return
	}
	var err error
	v.AdminAssetEvents, err = s.Store.AssetEventsDetailed(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.AssetInflows, err = s.Store.AssetInflowOptions(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.PendingJoins, err = s.Store.PendingJoinRequests(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.MemberPermissions, err = s.Store.AllMemberPermissions(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.AdminQuickNotes, err = s.Store.AdminQuickMoneyNotes(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, "admin.html", v)
}

func (s *Server) adminAuthorities(w http.ResponseWriter, r *http.Request) {
	v := s.base("成员与权限")
	v.AdminUsername = currentAdmin(r).Username
	var err error
	v.Members, err = s.familyMembers(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.MemberPermissions, err = s.Store.AllMemberPermissions(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, "admin-authorities.html", v)
}

func (s *Server) matters(w http.ResponseWriter, r *http.Request) {
	v := s.base("家族事务")
	v.CurrentMember = currentMember(r)
	v.Permissions = currentPermissions(r)
	var err error
	v.Members, err = s.familyMembers(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.Matters, err = s.Store.Matters(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, "matters.html", v)
}

func (s *Server) share(w http.ResponseWriter, r *http.Request) {
	v := s.base("信息共享")
	v.CurrentMember = currentMember(r)
	v.Permissions = currentPermissions(r)
	var err error
	v.Archives, err = s.Store.FamilyArchives(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, "share.html", v)
}

func parseMultipart(r *http.Request) ([]*multipart.FileHeader, error) {
	if err := r.ParseMultipartForm(220 << 20); err != nil {
		return nil, err
	}
	files := r.MultipartForm.File["evidence"]
	if err := store.ValidateEvidenceFiles(files); err != nil {
		return nil, err
	}
	return files, nil
}

func evidenceDir(s *Server) string { return filepath.Join(s.PM.ActiveDir, "uploads", "evidence") }

func (s *Server) createSelfAssetEvent(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	actor := currentMember(r).ID
	if _, err = s.Store.AddSelfAssetChange(r.Context(), actor, r.FormValue("event_type"), amount, r.FormValue("description"), formDateTime(r.FormValue("occurred_at"))); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/assets")
}

func (s *Server) createExpense(w http.ResponseWriter, r *http.Request) {
	files, err := parseMultipart(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	actor := currentMember(r).ID
	in := store.ExpenseInputV2{Title: r.FormValue("title"), Category: r.FormValue("category"), AmountCent: amount, OccurredAt: formDateTime(r.FormValue("occurred_at")), HandlerMemberID: actor, PaymentChannel: r.FormValue("payment_channel"), Merchant: r.FormValue("merchant"), Description: r.FormValue("description"), MatterID: parseID(r.FormValue("matter_id"))}
	id, err := s.Store.CreateExpenseAuto(r.Context(), actor, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err = s.Store.SaveEvidenceFiles(r.Context(), actor, "expense", id, evidenceDir(s), files); err != nil {
		s.fail(w, r, fmt.Errorf("消费已保存，但支付凭证保存失败：%w", err))
		return
	}
	redirect(w, r, "/assets")
}

func (s *Server) adminCreateExpense(w http.ResponseWriter, r *http.Request) {
	s.createExpenseRedirect(w, r, "/admin")
}

func (s *Server) createExpenseRedirect(w http.ResponseWriter, r *http.Request, to string) {
	files, err := parseMultipart(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	in := store.ExpenseInputV2{Title: r.FormValue("title"), Category: r.FormValue("category"), AmountCent: amount, OccurredAt: formDateTime(r.FormValue("occurred_at")), HandlerMemberID: parseID(r.FormValue("handler_id")), PaymentChannel: r.FormValue("payment_channel"), Merchant: r.FormValue("merchant"), Description: r.FormValue("description"), MatterID: parseID(r.FormValue("matter_id"))}
	id, err := s.Store.CreateExpenseAuto(r.Context(), s.DevActorID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err = s.Store.SaveEvidenceFiles(r.Context(), s.DevActorID, "expense", id, evidenceDir(s), files); err != nil {
		s.fail(w, r, fmt.Errorf("消费已保存，但支付凭证保存失败：%w", err))
		return
	}
	redirect(w, r, to)
}

func (s *Server) createTransfer(w http.ResponseWriter, r *http.Request) {
	files, err := parseMultipart(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	actor := currentMember(r).ID
	other := parseID(r.FormValue("counterparty_id"))
	from, to := actor, other
	if r.FormValue("direction") == "FROM" {
		from, to = other, actor
	} else if r.FormValue("direction") != "TO" {
		s.fail(w, r, fmt.Errorf("请选择转账方向"))
		return
	}
	id, err := s.Store.CreateTransferV2(r.Context(), actor, from, to, amount, r.FormValue("purpose"), r.FormValue("payment_channel"), formDateTime(r.FormValue("occurred_at")), parseID(r.FormValue("matter_id")))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err = s.Store.SaveEvidenceFiles(r.Context(), actor, "transfer", id, evidenceDir(s), files); err != nil {
		s.fail(w, r, fmt.Errorf("转账已保存，但转账凭证保存失败：%w", err))
		return
	}
	redirect(w, r, "/assets")
}

func (s *Server) adminCreateTransfer(w http.ResponseWriter, r *http.Request) {
	files, err := parseMultipart(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	id, err := s.Store.CreateTransferV2(r.Context(), s.DevActorID, parseID(r.FormValue("from_id")), parseID(r.FormValue("to_id")), amount, r.FormValue("purpose"), r.FormValue("payment_channel"), formDateTime(r.FormValue("occurred_at")), parseID(r.FormValue("matter_id")))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err = s.Store.SaveEvidenceFiles(r.Context(), s.DevActorID, "transfer", id, evidenceDir(s), files); err != nil {
		s.fail(w, r, fmt.Errorf("转账已保存，但转账凭证保存失败：%w", err))
		return
	}
	redirect(w, r, "/admin")
}

func (s *Server) createReimbursement(w http.ResponseWriter, r *http.Request) {
	s.createReimbursementForHolder(w, r, currentMember(r).ID, currentMember(r).ID, "/assets")
}

func (s *Server) adminCreateReimbursement(w http.ResponseWriter, r *http.Request) {
	s.createReimbursementForHolder(w, r, s.DevActorID, parseID(r.FormValue("holder_id")), "/admin")
}

func (s *Server) createReimbursementForHolder(w http.ResponseWriter, r *http.Request, actor, holder int64, to string) {
	files, err := parseMultipart(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	id, err := s.Store.CreateReimbursementV2(r.Context(), actor, parseID(r.FormValue("expense_id")), holder, amount, r.FormValue("payment_channel"), formDateTime(r.FormValue("occurred_at")), r.FormValue("note"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err = s.Store.SaveEvidenceFiles(r.Context(), actor, "reimbursement", id, evidenceDir(s), files); err != nil {
		s.fail(w, r, fmt.Errorf("报销已保存，但转账凭证保存失败：%w", err))
		return
	}
	redirect(w, r, to)
}

func (s *Server) editExpense(w http.ResponseWriter, r *http.Request) {
	v := s.base("编辑公共消费")
	if m := currentMember(r); m.ID != 0 {
		v.CurrentMember = m
		v.Permissions = currentPermissions(r)
	} else {
		v.Permissions = map[string]bool{"expenses.edit": true}
		v.AdminUsername = currentAdmin(r).Username
	}
	id := parseID(r.PathValue("id"))
	var err error
	v.Expense, err = s.Store.ExpenseByIDV2(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.Matters, err = s.Store.Matters(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.ExpenseAudits, err = s.Store.ExpenseAuditLogs(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.ExpenseRefunds, err = s.Store.ReimbursementsForExpense(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for i := range v.ExpenseRefunds {
		v.ExpenseRefunds[i].Evidence, _ = s.Store.EvidenceFor(r.Context(), "reimbursement", v.ExpenseRefunds[i].ID)
	}
	s.render(w, "expense-edit.html", v)
}

func (s *Server) updateExpense(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	id := parseID(r.PathValue("id"))
	in := store.ExpenseUpdate{Title: r.FormValue("title"), Category: r.FormValue("category"), AmountCent: amount, OccurredAt: formDateTime(r.FormValue("occurred_at")), PaymentChannel: r.FormValue("payment_channel"), Merchant: r.FormValue("merchant"), Description: r.FormValue("description"), MatterID: parseID(r.FormValue("matter_id"))}
	if err = s.Store.UpdateExpenseV2(r.Context(), s.businessActor(r), id, in); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/assets/expenses/"+strconv.FormatInt(id, 10)+"/edit")
}

func (s *Server) evidence(w http.ResponseWriter, r *http.Request) {
	path, name, err := s.Store.EvidencePath(r.Context(), parseID(r.PathValue("id")), evidenceDir(s))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", strings.ReplaceAll(name, "\"", "")))
	http.ServeFile(w, r, path)
}

func (s *Server) createMatter(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	actor := currentMember(r).ID
	in := store.MatterInput{ParentID: parseID(r.FormValue("parent_id")), Title: r.FormValue("title"), Type: r.FormValue("type"), Description: r.FormValue("description"), Status: r.FormValue("status"), StartDate: r.FormValue("start_date"), DueDate: r.FormValue("due_date"), OwnerMemberID: parseID(r.FormValue("owner_id"))}
	if err := s.Store.CreateMatter(r.Context(), actor, in); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/matters")
}

func matterInputFromRequest(r *http.Request) store.MatterInput {
	return store.MatterInput{
		ParentID:      parseID(r.FormValue("parent_id")),
		Title:         r.FormValue("title"),
		Type:          r.FormValue("type"),
		Description:   r.FormValue("description"),
		Status:        r.FormValue("status"),
		StartDate:     r.FormValue("start_date"),
		DueDate:       r.FormValue("due_date"),
		OwnerMemberID: parseID(r.FormValue("owner_id")),
	}
}

func (s *Server) updateMatter(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.UpdateMatter(r.Context(), currentMember(r).ID, parseID(r.PathValue("id")), matterInputFromRequest(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/matters#matter-"+r.PathValue("id"))
}

func (s *Server) setMatterStatus(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.SetMatterStatus(r.Context(), currentMember(r).ID, parseID(r.PathValue("id")), r.FormValue("status")); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/matters")
}

func (s *Server) createArchive(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	// Family members may create family-visible records. admin-only records are created from the admin side in a later dedicated archive console.
	if _, err := s.Store.CreateArchive(r.Context(), currentMember(r).ID, r.FormValue("title"), r.FormValue("category"), r.FormValue("content"), "family"); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/share")
}

func (s *Server) updateArchive(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.UpdateFamilyArchive(r.Context(), currentMember(r).ID, parseID(r.PathValue("id")), r.FormValue("title"), r.FormValue("category"), r.FormValue("content")); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/share#archive-"+r.PathValue("id"))
}

func (s *Server) uploadArchive(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(220 << 20); err != nil {
		s.fail(w, r, err)
		return
	}
	file := r.MultipartForm.File["file"]
	if len(file) == 0 {
		s.fail(w, r, fmt.Errorf("请选择附件"))
		return
	}
	for _, header := range file {
		if err := s.Store.SaveArchiveAttachment(r.Context(), currentMember(r).ID, parseID(r.PathValue("id")), filepath.Join(s.PM.ActiveDir, "uploads"), header); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	redirect(w, r, "/share")
}

func (s *Server) deleteArchiveAttachment(w http.ResponseWriter, r *http.Request) {
	archiveID := parseID(r.PathValue("id"))
	attachmentID := parseID(r.PathValue("attachmentID"))
	if err := s.Store.DeleteFamilyArchiveAttachment(r.Context(), currentMember(r).ID, archiveID, attachmentID, filepath.Join(s.PM.ActiveDir, "uploads")); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/share#archive-"+r.PathValue("id"))
}

func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	path, name, err := s.Store.FamilyAttachmentPath(r.Context(), parseID(r.PathValue("id")), filepath.Join(s.PM.ActiveDir, "uploads"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", strings.ReplaceAll(name, "\"", "")))
	http.ServeFile(w, r, path)
}

// ---- Admin member and permission management ----

func formPermissions(r *http.Request) []string { return r.Form["permissions"] }

func (s *Server) adminCreateMember(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	if _, err := s.Store.CreateMemberWithPermissions(r.Context(), s.DevActorID, r.FormValue("name"), r.FormValue("relation"), formPermissions(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/admin/authorities")
}

func (s *Server) adminSetPermissions(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.SetMemberPermissionsAudited(r.Context(), s.DevActorID, parseID(r.PathValue("id")), formPermissions(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/admin/authorities#member-"+r.PathValue("id"))
}

func (s *Server) adminApproveJoin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	_, err := s.Store.ApproveJoinRequest(r.Context(), s.DevActorID, parseID(r.PathValue("id")), parseID(r.FormValue("member_id")), r.FormValue("new_name"), r.FormValue("new_relation"), formPermissions(r), currentAdmin(r).Username)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/admin")
}

func (s *Server) adminRejectJoin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.RejectJoinRequest(r.Context(), s.DevActorID, parseID(r.PathValue("id")), currentAdmin(r).Username, r.FormValue("reason")); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/admin")
}

func (s *Server) adminCreateAssetEvent(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if _, err = s.Store.AddAssetEventDetailed(r.Context(), s.DevActorID, parseID(r.FormValue("holder_id")), r.FormValue("event_type"), amount, r.FormValue("description"), formDateTime(r.FormValue("occurred_at")), parseID(r.FormValue("related_event_id"))); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/admin")
}

func parseID(v string) int64 { n, _ := strconv.ParseInt(v, 10, 64); return n }

func formDateTime(v string) string {
	if v == "" {
		return time.Now().UTC().Format(time.RFC3339)
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04", v, time.Local); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return v
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
