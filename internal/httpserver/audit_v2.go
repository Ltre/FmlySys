package httpserver

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Ltre/FmlySys/internal/store"
)

type auditPageLink struct {
	Number  int
	Current bool
}

type adminAuditV2View struct {
	Title           string
	ActivePartition string
	AdminUsername   string
	Kind            string
	AccessLogs      []store.MemberAccessLog
	AuditLogs       []store.SuperAuditLog
	Page            int
	TotalPages      int
	TotalRows       int
	Pages           []auditPageLink
	Timezone        string
}

func auditPageNumbers(page, total int) []auditPageLink {
	if total <= 0 {
		return nil
	}
	if page < 1 {
		page = 1
	}
	if page > total {
		page = total
	}
	start := page - 4
	if start < 1 {
		start = 1
	}
	end := start + 9
	if end > total {
		end = total
		start = end - 9
		if start < 1 {
			start = 1
		}
	}
	out := make([]auditPageLink, 0, end-start+1)
	for n := start; n <= end; n++ {
		out = append(out, auditPageLink{Number: n, Current: n == page})
	}
	return out
}

func parseAuditPage(raw string) int {
	page, _ := strconv.Atoi(strings.TrimSpace(raw))
	if page < 1 {
		page = 1
	}
	return page
}

func totalAuditPages(rows int) int {
	if rows <= 0 {
		return 1
	}
	return (rows + store.AuditPageSize - 1) / store.AuditPageSize
}

func (s *Server) adminAuditConsoleV2(w http.ResponseWriter, r *http.Request) {
	kind := strings.TrimSpace(r.URL.Query().Get("type"))
	if kind != "front" && kind != "admin" {
		kind = "access"
	}
	page := parseAuditPage(r.URL.Query().Get("page"))
	v := adminAuditV2View{
		Title:           "访问记录与超级审计",
		ActivePartition: s.PM.ActiveID,
		AdminUsername:   currentAdmin(r).Username,
		Kind:            kind,
		Page:            page,
		Timezone:        requestTimezone(r),
	}

	load := func() error {
		var err error
		if kind == "access" {
			v.AccessLogs, v.TotalRows, err = s.Store.MemberAccessLogsPage(r.Context(), v.Page)
		} else {
			v.AuditLogs, v.TotalRows, err = s.Store.SuperAuditLogsPage(r.Context(), kind, v.Page)
		}
		return err
	}
	if err := load(); err != nil {
		s.fail(w, r, err)
		return
	}
	v.TotalPages = totalAuditPages(v.TotalRows)
	if v.Page > v.TotalPages {
		v.Page = v.TotalPages
		if err := load(); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	v.Pages = auditPageNumbers(v.Page, v.TotalPages)
	if err := s.Templates.ExecuteTemplate(w, "admin-audit-v2.html", v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) adminTestRemoteNotificationsV2(w http.ResponseWriter, r *http.Request) {
	if err := sendTermuxMedicationNotification(s.Config.DataDir, "FmlySys 测试通知", "服药通知远控通道测试成功"); err != nil {
		redirect(w, r, "/admin/remote-notifications?message="+url.QueryEscape("测试失败："+err.Error()))
		return
	}
	// 这里只进行一次外部通道测试，并没有修改远控配置数据，所以不写
	// audit_logs。超级审计只记录真正的数据变更。
	redirect(w, r, "/admin/remote-notifications?message="+url.QueryEscape("Termux 测试通知已发送"))
}

func shouldRecordBusinessAccessV2(r *http.Request) bool {
	return !store.IsLikelyFrontendResourcePath(r.URL.Path)
}

// WithSuperAuditV2 deliberately has no request-derived fallback audit. A row
// enters super_audit_logs only when the business layer actually wrote an
// audit_logs fact while the request was executing.
func (s *Server) WithSuperAuditV2(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		meta := s.auditRequestMeta(r)
		if meta.MemberID > 0 && shouldRecordBusinessAccessV2(r) {
			member := store.Member{ID: meta.MemberID, Name: meta.MemberName}
			if err := s.Store.RecordMemberAccess(r.Context(), meta.IPAddress, member, r.Method, r.URL.RequestURI()); err != nil {
				log.Printf("record member access: %v", err)
			}
		}

		if !isMutationMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		superAuditWriteMu.Lock()
		defer superAuditWriteMu.Unlock()
		beforeID, err := s.Store.MaxAuditLogID(r.Context())
		if err != nil {
			log.Printf("super audit max id: %v", err)
			beforeID = 0
		}
		next.ServeHTTP(w, r)
		if _, err := s.Store.CaptureSuperAuditSince(context.WithoutCancel(r.Context()), beforeID, meta); err != nil {
			log.Printf("capture super audit: %v", err)
		}
	})
}

func (s *Server) WithAuditConsoleV2(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/audit", s.adminOnly(s.adminAuditConsoleV2))
	mux.HandleFunc("POST /admin/remote-notifications/test", s.adminOnly(s.adminTestRemoteNotificationsV2))
	mux.Handle("/", next)
	return mux
}
