package store

import (
	"context"
	"strings"
)

const AuditPageSize = 200

func normalizeAuditPage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}

func (s *Store) MemberAccessLogsPage(ctx context.Context, page int) ([]MemberAccessLog, int, error) {
	page = normalizeAuditPage(page)
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM member_access_logs`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT id,ip_address,member_id,member_name,method,path,accessed_at
FROM member_access_logs
ORDER BY accessed_at DESC,id DESC
LIMIT ? OFFSET ?`, AuditPageSize, (page-1)*AuditPageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []MemberAccessLog
	for rows.Next() {
		var v MemberAccessLog
		if err := rows.Scan(&v.ID, &v.IPAddress, &v.MemberID, &v.MemberName, &v.Method, &v.Path, &v.AccessedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

func (s *Store) SuperAuditLogsPage(ctx context.Context, surface string, page int) ([]SuperAuditLog, int, error) {
	if surface != "admin" {
		surface = "front"
	}
	page = normalizeAuditPage(page)
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM super_audit_logs WHERE surface=?`, surface).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT id,COALESCE(audit_log_id,0),surface,ip_address,COALESCE(member_id,0),
       member_name,admin_username,operation,data_category,original_action,
       COALESCE(entity_id,0),COALESCE(before_json,''),COALESCE(after_json,''),
       request_method,request_path,operation_time
FROM super_audit_logs
WHERE surface=?
ORDER BY operation_time DESC,id DESC
LIMIT ? OFFSET ?`, surface, AuditPageSize, (page-1)*AuditPageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []SuperAuditLog
	for rows.Next() {
		var v SuperAuditLog
		if err := rows.Scan(
			&v.ID,
			&v.AuditLogID,
			&v.Surface,
			&v.IPAddress,
			&v.MemberID,
			&v.MemberName,
			&v.AdminUsername,
			&v.Operation,
			&v.DataCategory,
			&v.OriginalAction,
			&v.EntityID,
			&v.BeforeJSON,
			&v.AfterJSON,
			&v.RequestMethod,
			&v.RequestPath,
			&v.OperationTime,
		); err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

func (s *Store) DeleteFallbackSuperAudits(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM super_audit_logs WHERE audit_log_id IS NULL`)
	return err
}

func IsLikelyFrontendResourcePath(path string) bool {
	path = strings.ToLower(strings.TrimSpace(path))
	if path == "" {
		return false
	}
	if strings.HasPrefix(path, "/static/") || path == "/healthz" || path == "/favicon.ico" || path == "/medication-sw.js" {
		return true
	}
	if q := strings.IndexByte(path, '?'); q >= 0 {
		path = path[:q]
	}
	for _, ext := range []string{".css", ".js", ".map", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".woff", ".woff2", ".ttf"} {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}
