package store

import (
	"context"
	"database/sql"
	"errors"
)

type MemberNotification struct {
	ID                int64
	RecipientMemberID int64
	ActorMemberID     int64
	ActorName         string
	Kind              string
	Title             string
	Body              string
	Link              string
	PlanID            int64
	ScheduledDate     string
	ReadAt            string
	CreatedAt         string
}

func (s *Store) CreateMemberNotification(
	ctx context.Context,
	actorMemberID, recipientMemberID int64,
	kind, title, body, link string,
	planID int64,
	scheduledDate string,
) (int64, error) {
	if recipientMemberID <= 0 {
		return 0, errors.New("通知接收成员无效")
	}
	if kind != "medication_manual" && kind != "medication_later" {
		return 0, errors.New("通知类型无效")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM members WHERE id=? AND status='active' AND is_del=0)`, recipientMemberID).Scan(&active); err != nil {
		return 0, err
	}
	if active == 0 {
		return 0, errors.New("通知接收成员不存在或已停用")
	}
	var actor any
	if actorMemberID > 0 {
		actor = actorMemberID
	}
	var plan any
	if planID > 0 {
		plan = planID
	}
	res, err := tx.ExecContext(ctx, `
INSERT INTO member_notifications(
    recipient_member_id,actor_member_id,kind,title,body,link,plan_id,scheduled_date,created_at
)
VALUES(?,?,?,?,?,?,?,?,?)`, recipientMemberID, actor, kind, title, body, link, plan, scheduledDate, now())
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if err := auditTx(ctx, tx, actorMemberID, "create", "member_notification", id, nil, map[string]any{
		"recipient_member_id": recipientMemberID,
		"kind":                kind,
		"title":               title,
		"link":                link,
		"plan_id":             plan,
		"scheduled_date":      scheduledDate,
	}); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) MemberNotifications(ctx context.Context, memberID int64, limit int) ([]MemberNotification, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT n.id,n.recipient_member_id,COALESCE(n.actor_member_id,0),COALESCE(a.name,''),
       n.kind,n.title,n.body,n.link,COALESCE(n.plan_id,0),n.scheduled_date,
       COALESCE(n.read_at,''),n.created_at
FROM member_notifications n
LEFT JOIN members a ON a.id=n.actor_member_id
WHERE n.recipient_member_id=?
ORDER BY n.created_at DESC,n.id DESC
LIMIT ?`, memberID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemberNotification
	for rows.Next() {
		var n MemberNotification
		if err := rows.Scan(
			&n.ID,
			&n.RecipientMemberID,
			&n.ActorMemberID,
			&n.ActorName,
			&n.Kind,
			&n.Title,
			&n.Body,
			&n.Link,
			&n.PlanID,
			&n.ScheduledDate,
			&n.ReadAt,
			&n.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) MemberNotificationByID(ctx context.Context, memberID, notificationID int64) (MemberNotification, error) {
	var n MemberNotification
	err := s.DB.QueryRowContext(ctx, `
SELECT n.id,n.recipient_member_id,COALESCE(n.actor_member_id,0),COALESCE(a.name,''),
       n.kind,n.title,n.body,n.link,COALESCE(n.plan_id,0),n.scheduled_date,
       COALESCE(n.read_at,''),n.created_at
FROM member_notifications n
LEFT JOIN members a ON a.id=n.actor_member_id
WHERE n.id=? AND n.recipient_member_id=?`, notificationID, memberID).Scan(
		&n.ID,
		&n.RecipientMemberID,
		&n.ActorMemberID,
		&n.ActorName,
		&n.Kind,
		&n.Title,
		&n.Body,
		&n.Link,
		&n.PlanID,
		&n.ScheduledDate,
		&n.ReadAt,
		&n.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return MemberNotification{}, errors.New("通知不存在")
	}
	return n, err
}

func (s *Store) MarkMemberNotificationRead(ctx context.Context, actorMemberID, notificationID int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldRead string
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(read_at,'') FROM member_notifications WHERE id=? AND recipient_member_id=?`, notificationID, actorMemberID).Scan(&oldRead); err != nil {
		return err
	}
	if oldRead != "" {
		return tx.Commit()
	}
	readAt := now()
	if _, err := tx.ExecContext(ctx, `UPDATE member_notifications SET read_at=? WHERE id=? AND recipient_member_id=?`, readAt, notificationID, actorMemberID); err != nil {
		return err
	}
	if err := auditTx(ctx, tx, actorMemberID, "update", "member_notification", notificationID,
		map[string]any{"read_at": nil}, map[string]any{"read_at": readAt}); err != nil {
		return err
	}
	return tx.Commit()
}

// MedicationPlanManagers returns active members who can manage this exact plan
// under the manage_self/manage_others split. excludeMemberID is normally the
// patient who clicked "等下再说", to avoid notifying the same person about
// their own response.
func (s *Store) MedicationPlanManagers(ctx context.Context, planID, excludeMemberID int64) ([]Member, error) {
	var creatorID int64
	if err := s.DB.QueryRowContext(ctx, `SELECT created_by FROM medication_plans WHERE id=? AND is_deleted=0`, planID).Scan(&creatorID); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT DISTINCT m.id,m.name,m.relation
FROM members m
WHERE m.status='active' AND m.is_del=0 AND m.id<>?
  AND (
      (m.id=? AND EXISTS(
          SELECT 1 FROM member_permissions p
          WHERE p.member_id=m.id AND p.permission='medication.manage_self'
      ))
      OR
      (m.id<>? AND EXISTS(
          SELECT 1 FROM member_permissions p
          WHERE p.member_id=m.id AND p.permission='medication.manage_others'
      ))
  )
ORDER BY m.name,m.id`, excludeMemberID, creatorID, creatorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.Name, &m.Relation); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
