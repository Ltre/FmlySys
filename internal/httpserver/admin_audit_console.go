package httpserver

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/Ltre/FmlySys/internal/store"
)

type adminAuditView struct {
	Title, ActivePartition, AdminUsername, Kind string
	AccessLogs                                  []store.MemberAccessLog
	AuditLogs                                   []store.SuperAuditLog
	Message                                     string
}

type adminRemoteNotificationView struct {
	Title, ActivePartition, AdminUsername  string
	Configured                             bool
	Host                                   string
	Port                                   int
	Username, KeyPath, ConfigPath, Message string
}

func (s *Server) adminAuditConsole(w http.ResponseWriter, r *http.Request) {
	kind := strings.TrimSpace(r.URL.Query().Get("type"))
	if kind != "front" && kind != "admin" {
		kind = "access"
	}
	v := adminAuditView{
		Title:           "访问记录与超级审计",
		ActivePartition: s.PM.ActiveID,
		AdminUsername:   currentAdmin(r).Username,
		Kind:            kind,
		Message:         queryMessage(r),
	}
	var err error
	if kind == "access" {
		v.AccessLogs, err = s.Store.MemberAccessLogs(r.Context(), 500)
	} else {
		v.AuditLogs, err = s.Store.SuperAuditLogs(r.Context(), kind, 500)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderMedicationTemplate(w, "admin-audit.html", v)
}

func (s *Server) adminRemoteNotifications(w http.ResponseWriter, r *http.Request) {
	v := adminRemoteNotificationView{
		Title:           "服药通知远控配置",
		ActivePartition: s.PM.ActiveID,
		AdminUsername:   currentAdmin(r).Username,
		Port:            22,
		KeyPath:         remoteNotificationKeyPath(s.Config.DataDir),
		ConfigPath:      remoteNotificationConfigPath(s.Config.DataDir),
		Message:         queryMessage(r),
	}
	cfg, err := loadRemoteNotificationConfig(s.Config.DataDir)
	if err == nil {
		v.Configured = true
		v.Host, v.Port, v.Username = cfg.Host, cfg.Port, cfg.Username
	} else if !errors.Is(err, os.ErrNotExist) {
		v.Message = "读取现有远控配置失败：" + err.Error()
	}
	s.renderMedicationTemplate(w, "admin-remote-notifications.html", v)
}

func (s *Server) adminSaveRemoteNotifications(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	port, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("port")))
	password := r.FormValue("password")
	old, oldErr := loadRemoteNotificationConfig(s.Config.DataDir)
	if password == "" && oldErr == nil {
		password = old.Password
	}
	cfg := RemoteNotificationConfig{
		Host:     r.FormValue("host"),
		Port:     port,
		Username: r.FormValue("username"),
		Password: password,
	}
	if err := saveRemoteNotificationConfig(s.Config.DataDir, cfg); err != nil {
		s.fail(w, r, err)
		return
	}
	before := any(nil)
	if oldErr == nil {
		before = map[string]any{
			"host": old.Host, "port": old.Port, "username": old.Username, "password": "[REDACTED]",
		}
	}
	_ = s.Store.AuditChange(
		r.Context(),
		s.DevActorID,
		"update",
		"remote_notification_config",
		0,
		before,
		map[string]any{
			"host": cfg.Host, "port": cfg.Port, "username": cfg.Username, "password": "[REDACTED]",
		},
	)
	redirect(w, r, "/admin/remote-notifications?message="+url.QueryEscape("远控配置已使用随机 RSA 私钥 + AES-GCM 加密保存"))
}

func (s *Server) adminTestRemoteNotifications(w http.ResponseWriter, r *http.Request) {
	if err := sendTermuxMedicationNotification(s.Config.DataDir, "FmlySys 测试通知", "服药通知远控通道测试成功"); err != nil {
		redirect(w, r, "/admin/remote-notifications?message="+url.QueryEscape("测试失败："+err.Error()))
		return
	}
	_ = s.Store.AuditChange(r.Context(), s.DevActorID, "update", "remote_notification_config", 0, nil, map[string]any{"test_notification": "sent"})
	redirect(w, r, "/admin/remote-notifications?message="+url.QueryEscape("Termux 测试通知已发送"))
}

func (s *Server) WithAuditConsole(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/audit", s.adminOnly(s.adminAuditConsole))
	mux.HandleFunc("GET /admin/remote-notifications", s.adminOnly(s.adminRemoteNotifications))
	mux.HandleFunc("POST /admin/remote-notifications", s.adminOnly(s.adminSaveRemoteNotifications))
	mux.HandleFunc("POST /admin/remote-notifications/test", s.adminOnly(s.adminTestRemoteNotifications))
	mux.Handle("/", next)
	return mux
}
