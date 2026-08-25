package store

import (
	"context"
	"encoding/json"
	"strings"
)

type MemberAccessLog struct {
	ID         int64
	IPAddress  string
	MemberID   int64
	MemberName string
	Method     string
	Path       string
	AccessedAt string
}

type SuperAuditLog struct {
	ID             int64
	AuditLogID     int64
	Surface        string
	IPAddress      string
	MemberID       int64
	MemberName     string
	AdminUsername  string
	Operation      string
	DataCategory   string
	OriginalAction string
	EntityID       int64
	BeforeJSON     string
	AfterJSON      string
	RequestMethod  string
	RequestPath    string
	OperationTime  string
}

type SuperAuditRequestMeta struct {
	Surface       string
	IPAddress     string
	MemberID      int64
	MemberName    string
	AdminUsername string
	RequestMethod string
	RequestPath   string
}

func normalizeSuperAuditOperation(action string) string {
	action = strings.ToLower(strings.TrimSpace(action))
	switch {
	case strings.Contains(action, "delete"), strings.Contains(action, "remove"), strings.Contains(action, "revoke"):
		return "delete"
	case strings.Contains(action, "create"), strings.Contains(action, "insert"), strings.Contains(action, "add"), strings.Contains(action, "register"):
		return "create"
	default:
		return "update"
	}
}

func (s *Store) RecordMemberAccess(ctx context.Context, ip string, member Member, method, path string) error {
	if member.ID <= 0 { return nil }
	_, err := s.DB.ExecContext(ctx, `INSERT INTO member_access_logs(ip_address,member_id,member_name,method,path,accessed_at) VALUES(?,?,?,?,?,?)`, strings.TrimSpace(ip), member.ID, member.Name, method, path, now())
	return err
}
func (s *Store) MaxAuditLogID(ctx context.Context) (int64,error){var id int64;err:=s.DB.QueryRowContext(ctx,`SELECT COALESCE(MAX(id),0) FROM audit_logs`).Scan(&id);return id,err}
func (s *Store) CaptureSuperAuditSince(ctx context.Context, afterID int64, meta SuperAuditRequestMeta) (int,error) {
	if meta.Surface!="admin"{meta.Surface="front"}
	rows,err:=s.DB.QueryContext(ctx,`SELECT a.id,a.action,a.entity_type,COALESCE(a.entity_id,0),COALESCE(a.before_json,''),COALESCE(a.after_json,''),a.created_at,COALESCE(a.actor_member_id,0),COALESCE(m.name,'') FROM audit_logs a LEFT JOIN members m ON m.id=a.actor_member_id WHERE a.id>? ORDER BY a.id`,afterID);if err!=nil{return 0,err};defer rows.Close()
	type rowValue struct{id,entityID,actorID int64;action,entityType,beforeJSON,afterJSON,createdAt,actorName string};var values []rowValue
	for rows.Next(){var v rowValue;if err:=rows.Scan(&v.id,&v.action,&v.entityType,&v.entityID,&v.beforeJSON,&v.afterJSON,&v.createdAt,&v.actorID,&v.actorName);err!=nil{return 0,err};values=append(values,v)};if err:=rows.Err();err!=nil{return 0,err};if len(values)==0{return 0,nil}
	tx,err:=s.DB.BeginTx(ctx,nil);if err!=nil{return 0,err};defer tx.Rollback()
	for _,v:=range values{memberID:=meta.MemberID;memberName:=meta.MemberName;if meta.Surface=="front"&&memberID==0&&v.actorID>0{memberID,memberName=v.actorID,v.actorName};_,err:=tx.ExecContext(ctx,`INSERT OR IGNORE INTO super_audit_logs(audit_log_id,surface,ip_address,member_id,member_name,admin_username,operation,data_category,original_action,entity_id,before_json,after_json,request_method,request_path,operation_time) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,v.id,meta.Surface,meta.IPAddress,nullID(memberID),memberName,meta.AdminUsername,normalizeSuperAuditOperation(v.action),v.entityType,v.action,nullID(v.entityID),nullTextAudit(v.beforeJSON),nullTextAudit(v.afterJSON),meta.RequestMethod,meta.RequestPath,v.createdAt);if err!=nil{return 0,err}}
	if err:=tx.Commit();err!=nil{return 0,err};return len(values),nil
}
func nullTextAudit(v string)any{if strings.TrimSpace(v)==""||v=="null"{return nil};return v}
func (s *Store) RecordFallbackSuperAudit(ctx context.Context,meta SuperAuditRequestMeta,category,action string,after any)error{if meta.Surface!="admin"{meta.Surface="front"};b,_:=json.Marshal(after);_,err:=s.DB.ExecContext(ctx,`INSERT INTO super_audit_logs(surface,ip_address,member_id,member_name,admin_username,operation,data_category,original_action,before_json,after_json,request_method,request_path,operation_time) VALUES(?,?,?,?,?,?,?,?,NULL,?,?,?,?)`,meta.Surface,meta.IPAddress,nullID(meta.MemberID),meta.MemberName,meta.AdminUsername,normalizeSuperAuditOperation(action),category,action,string(b),meta.RequestMethod,meta.RequestPath,now());return err}
func (s *Store) MemberAccessLogs(ctx context.Context,limit int)([]MemberAccessLog,error){if limit<=0||limit>2000{limit=500};rows,err:=s.DB.QueryContext(ctx,`SELECT id,ip_address,member_id,member_name,method,path,accessed_at FROM member_access_logs ORDER BY accessed_at DESC,id DESC LIMIT ?`,limit);if err!=nil{return nil,err};defer rows.Close();var out []MemberAccessLog;for rows.Next(){var v MemberAccessLog;if err:=rows.Scan(&v.ID,&v.IPAddress,&v.MemberID,&v.MemberName,&v.Method,&v.Path,&v.AccessedAt);err!=nil{return nil,err};out=append(out,v)};return out,rows.Err()}
func (s *Store) SuperAuditLogs(ctx context.Context,surface string,limit int)([]SuperAuditLog,error){if surface!="admin"{surface="front"};if limit<=0||limit>2000{limit=500};rows,err:=s.DB.QueryContext(ctx,`SELECT id,COALESCE(audit_log_id,0),surface,ip_address,COALESCE(member_id,0),member_name,admin_username,operation,data_category,original_action,COALESCE(entity_id,0),COALESCE(before_json,''),COALESCE(after_json,''),request_method,request_path,operation_time FROM super_audit_logs WHERE surface=? ORDER BY operation_time DESC,id DESC LIMIT ?`,surface,limit);if err!=nil{return nil,err};defer rows.Close();var out []SuperAuditLog;for rows.Next(){var v SuperAuditLog;if err:=rows.Scan(&v.ID,&v.AuditLogID,&v.Surface,&v.IPAddress,&v.MemberID,&v.MemberName,&v.AdminUsername,&v.Operation,&v.DataCategory,&v.OriginalAction,&v.EntityID,&v.BeforeJSON,&v.AfterJSON,&v.RequestMethod,&v.RequestPath,&v.OperationTime);err!=nil{return nil,err};out=append(out,v)};return out,rows.Err()}
func (s *Store) AuditChange(ctx context.Context,actor int64,action,entity string,entityID int64,before,after any)error{return auditExec(ctx,s.DB,actor,action,entity,entityID,before,after)}
