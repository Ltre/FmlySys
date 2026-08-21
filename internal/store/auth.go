package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const MemberSessionTTL = 30 * 24 * time.Hour
const JoinAccessTTL = 30 * time.Minute

type PermissionDef struct {
	Key   string
	Label string
}

var PermissionCatalog = []PermissionDef{
	{Key: "assets.view", Label: "查看公共资产"},
	{Key: "assets.self_change", Label: "登记本人公共资产增减"},
	{Key: "expenses.create", Label: "新增公共消费"},
	{Key: "expenses.edit", Label: "编辑公共消费"},
	{Key: "transfers.create", Label: "登记成员间转账"},
	{Key: "reimbursements.create", Label: "登记报销"},
	{Key: "matters.view", Label: "查看家族事务"},
	{Key: "matters.manage", Label: "管理家族事务"},
	{Key: "share.view", Label: "查看家族共享资料"},
	{Key: "share.manage", Label: "管理家族共享资料"},
}

var DefaultMemberPermissions = []string{
	"assets.view",
	"assets.self_change",
	"expenses.create",
	"transfers.create",
	"reimbursements.create",
	"matters.view",
	"share.view",
}

type JoinRequest struct {
	ID                                                                     int64
	OpenID, UnionID, Nickname, RealName, Relation, Status, RejectionReason string
	RequestedAt, ReviewedAt                                                string
}

type WeChatResolve struct {
	MemberID   int64
	JoinToken  string
	JoinStatus string
}

func PermissionAllowed(key string) bool {
	for _, p := range PermissionCatalog {
		if p.Key == key {
			return true
		}
	}
	return false
}

func IsDefaultPermission(key string) bool {
	for _, p := range DefaultMemberPermissions {
		if p == key {
			return true
		}
	}
	return false
}

func normalizePermissions(perms []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		if !PermissionAllowed(p) {
			return nil, fmt.Errorf("未知成员权限：%s", p)
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

func (s *Store) SetMemberPermissions(ctx context.Context, memberID int64, perms []string) error {
	perms, err := normalizePermissions(perms)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := setPermissionsTx(ctx, tx, memberID, perms); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SetMemberPermissionsAudited(ctx context.Context, auditActor, memberID int64, perms []string) error {
	perms, err := normalizePermissions(perms)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := setPermissionsTx(ctx, tx, memberID, perms); err != nil {
		return err
	}
	if err := auditTx(ctx, tx, auditActor, "update_permissions", "member", memberID, nil, map[string]any{"permissions": perms}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GrantAllPermissions(ctx context.Context, memberID int64) error {
	all := make([]string, 0, len(PermissionCatalog))
	for _, p := range PermissionCatalog {
		all = append(all, p.Key)
	}
	return s.SetMemberPermissions(ctx, memberID, all)
}

func setPermissionsTx(ctx context.Context, tx *sql.Tx, memberID int64, perms []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM member_permissions WHERE member_id=?`, memberID); err != nil {
		return err
	}
	for _, p := range perms {
		if _, err := tx.ExecContext(ctx, `INSERT INTO member_permissions(member_id,permission,created_at) VALUES(?,?,?)`, memberID, p, now()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) MemberPermissions(ctx context.Context, memberID int64) (map[string]bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT permission FROM member_permissions WHERE member_id=? ORDER BY permission`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out[p] = true
	}
	return out, rows.Err()
}

func (s *Store) AllMemberPermissions(ctx context.Context) (map[int64]map[string]bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT member_id,permission FROM member_permissions ORDER BY member_id,permission`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]map[string]bool{}
	for rows.Next() {
		var id int64
		var p string
		if err := rows.Scan(&id, &p); err != nil {
			return nil, err
		}
		if out[id] == nil {
			out[id] = map[string]bool{}
		}
		out[id][p] = true
	}
	return out, rows.Err()
}

func (s *Store) CreateMemberWithPermissions(ctx context.Context, auditActor int64, name, relation string, perms []string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("成员姓名不能为空")
	}
	perms, err := normalizePermissions(perms)
	if err != nil {
		return 0, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO members(name,relation,status,created_at) VALUES(?,?,'active',?)`, name, relation, now())
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if err := setPermissionsTx(ctx, tx, id, perms); err != nil {
		return 0, err
	}
	if err := auditTx(ctx, tx, auditActor, "create", "member", id, nil, map[string]any{"name": name, "relation": relation, "permissions": perms}); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) CreateMemberSession(ctx context.Context, memberID int64) (string, error) {
	raw, hash, err := memberToken()
	if err != nil {
		return "", err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO member_sessions(token_hash,member_id,expires_at,created_at,last_seen_at) VALUES(?,?,?,?,?)`, hash, memberID, time.Now().UTC().Add(MemberSessionTTL).Format(time.RFC3339Nano), now(), now())
	return raw, err
}

func (s *Store) MemberFromSession(ctx context.Context, raw string) (Member, map[string]bool, error) {
	if raw == "" {
		return Member{}, nil, sql.ErrNoRows
	}
	hash := memberTokenHash(raw)
	var m Member
	var expires string
	err := s.DB.QueryRowContext(ctx, `SELECT m.id,m.name,m.relation,s.expires_at FROM member_sessions s JOIN members m ON m.id=s.member_id WHERE s.token_hash=? AND m.status='active'`, hash).Scan(&m.ID, &m.Name, &m.Relation, &expires)
	if err != nil {
		return Member{}, nil, err
	}
	t, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || time.Now().After(t) {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM member_sessions WHERE token_hash=?`, hash)
		return Member{}, nil, sql.ErrNoRows
	}
	perms, err := s.MemberPermissions(ctx, m.ID)
	if err != nil {
		return Member{}, nil, err
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE member_sessions SET last_seen_at=? WHERE token_hash=?`, now(), hash)
	return m, perms, nil
}

func (s *Store) DeleteMemberSession(ctx context.Context, raw string) {
	if raw != "" {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM member_sessions WHERE token_hash=?`, memberTokenHash(raw))
	}
}

func (s *Store) ResolveWeChat(ctx context.Context, openID, unionID, nickname string) (WeChatResolve, error) {
	openID = strings.TrimSpace(openID)
	if openID == "" {
		return WeChatResolve{}, errors.New("微信 OpenID 为空")
	}
	var member sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT member_id FROM wechat_identities WHERE openid=?`, openID).Scan(&member)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return WeChatResolve{}, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		_, err = s.DB.ExecContext(ctx, `INSERT INTO wechat_identities(openid,unionid,nickname,member_id,created_at,updated_at) VALUES(?,?,?,NULL,?,?)`, openID, unionID, nickname, now(), now())
	} else {
		_, err = s.DB.ExecContext(ctx, `UPDATE wechat_identities SET unionid=?,nickname=?,updated_at=? WHERE openid=?`, unionID, nickname, now(), openID)
	}
	if err != nil {
		return WeChatResolve{}, err
	}
	if member.Valid && member.Int64 > 0 {
		return WeChatResolve{MemberID: member.Int64}, nil
	}

	raw, hash, err := memberToken()
	if err != nil {
		return WeChatResolve{}, err
	}
	expires := time.Now().UTC().Add(JoinAccessTTL).Format(time.RFC3339Nano)
	var status string
	err = s.DB.QueryRowContext(ctx, `SELECT status FROM join_requests WHERE openid=?`, openID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		status = "draft"
		_, err = s.DB.ExecContext(ctx, `INSERT INTO join_requests(openid,unionid,nickname,status,access_token_hash,access_token_expires_at,created_at,updated_at) VALUES(?,?,?,'draft',?,?,?,?)`, openID, unionID, nickname, hash, expires, now(), now())
	} else if err == nil {
		_, err = s.DB.ExecContext(ctx, `UPDATE join_requests SET unionid=?,nickname=?,access_token_hash=?,access_token_expires_at=?,updated_at=? WHERE openid=?`, unionID, nickname, hash, expires, now(), openID)
	}
	if err != nil {
		return WeChatResolve{}, err
	}
	return WeChatResolve{JoinToken: raw, JoinStatus: status}, nil
}

func (s *Store) JoinRequestByToken(ctx context.Context, raw string) (JoinRequest, error) {
	if raw == "" {
		return JoinRequest{}, sql.ErrNoRows
	}
	var v JoinRequest
	var expires string
	err := s.DB.QueryRowContext(ctx, `SELECT id,openid,unionid,nickname,real_name,relation,status,rejection_reason,requested_at,reviewed_at,access_token_expires_at FROM join_requests WHERE access_token_hash=?`, memberTokenHash(raw)).Scan(&v.ID, &v.OpenID, &v.UnionID, &v.Nickname, &v.RealName, &v.Relation, &v.Status, &v.RejectionReason, &v.RequestedAt, &v.ReviewedAt, &expires)
	if err != nil {
		return JoinRequest{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || time.Now().After(t) {
		return JoinRequest{}, sql.ErrNoRows
	}
	return v, nil
}

func (s *Store) SubmitJoinRequest(ctx context.Context, raw, realName, relation string) error {
	req, err := s.JoinRequestByToken(ctx, raw)
	if err != nil {
		return err
	}
	if req.Status == "approved" {
		return errors.New("该微信身份已经审核通过，请重新扫码登录")
	}
	if req.Status == "pending" {
		return nil
	}
	realName = strings.TrimSpace(realName)
	if realName == "" {
		return errors.New("请填写真实姓名")
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE join_requests SET real_name=?,relation=?,status='pending',rejection_reason='',requested_at=?,updated_at=? WHERE id=?`, realName, strings.TrimSpace(relation), now(), now(), req.ID)
	return err
}

func (s *Store) PendingJoinRequests(ctx context.Context) ([]JoinRequest, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,openid,unionid,nickname,real_name,relation,status,rejection_reason,requested_at,reviewed_at FROM join_requests WHERE status='pending' ORDER BY requested_at,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []JoinRequest
	for rows.Next() {
		var v JoinRequest
		if err := rows.Scan(&v.ID, &v.OpenID, &v.UnionID, &v.Nickname, &v.RealName, &v.Relation, &v.Status, &v.RejectionReason, &v.RequestedAt, &v.ReviewedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) ApproveJoinRequest(ctx context.Context, auditActor, requestID, existingMemberID int64, newName, newRelation string, perms []string, reviewer string) (int64, error) {
	perms, err := normalizePermissions(perms)
	if err != nil {
		return 0, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var openID, realName, relation, status string
	if err := tx.QueryRowContext(ctx, `SELECT openid,real_name,relation,status FROM join_requests WHERE id=?`, requestID).Scan(&openID, &realName, &relation, &status); err != nil {
		return 0, err
	}
	if status != "pending" {
		return 0, errors.New("该申请已经处理")
	}
	memberID := existingMemberID
	if memberID == 0 {
		name := strings.TrimSpace(newName)
		if name == "" {
			name = realName
		}
		rel := strings.TrimSpace(newRelation)
		if rel == "" {
			rel = relation
		}
		if name == "" {
			return 0, errors.New("新建成员姓名不能为空")
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO members(name,relation,status,created_at) VALUES(?,?,'active',?)`, name, rel, now())
		if err != nil {
			return 0, err
		}
		memberID, _ = res.LastInsertId()
	} else {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM members WHERE id=? AND status='active'`, memberID).Scan(&n); err != nil || n != 1 {
			return 0, errors.New("选择的成员不存在")
		}
	}
	if err := setPermissionsTx(ctx, tx, memberID, perms); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE wechat_identities SET member_id=?,updated_at=? WHERE openid=?`, memberID, now(), openID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE join_requests SET status='approved',reviewed_at=?,reviewed_by=?,updated_at=? WHERE id=?`, now(), reviewer, now(), requestID); err != nil {
		return 0, err
	}
	if err := auditTx(ctx, tx, auditActor, "approve", "join_request", requestID, nil, map[string]any{"member_id": memberID, "permissions": perms, "reviewer": reviewer}); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return memberID, nil
}

func (s *Store) RejectJoinRequest(ctx context.Context, auditActor, requestID int64, reviewer, reason string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE join_requests SET status='rejected',rejection_reason=?,reviewed_at=?,reviewed_by=?,updated_at=? WHERE id=? AND status='pending'`, strings.TrimSpace(reason), now(), reviewer, now(), requestID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return errors.New("该申请已经处理")
	}
	if err := auditTx(ctx, tx, auditActor, "reject", "join_request", requestID, nil, map[string]any{"reviewer": reviewer, "reason": reason}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FamilyArchives(ctx context.Context) ([]Archive, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,title,category,content,visibility,created_at FROM archives WHERE visibility='family' ORDER BY created_at DESC,id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Archive
	for rows.Next() {
		var v Archive
		if err := rows.Scan(&v.ID, &v.Title, &v.Category, &v.Content, &v.Visibility, &v.CreatedAt); err != nil {
			return nil, err
		}
		atts, err := s.Attachments(ctx, v.ID)
		if err != nil {
			return nil, err
		}
		v.Attachments = atts
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) FamilyAttachmentPath(ctx context.Context, id int64, uploadDir string) (string, string, error) {
	var storage, original string
	err := s.DB.QueryRowContext(ctx, `SELECT a.storage_name,a.original_name FROM attachments a JOIN archives ar ON ar.id=a.archive_id WHERE a.id=? AND ar.visibility='family'`, id).Scan(&storage, &original)
	if err != nil {
		return "", "", err
	}
	return filepath.Join(uploadDir, storage), original, nil
}

func memberToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, memberTokenHash(raw), nil
}

func memberTokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
