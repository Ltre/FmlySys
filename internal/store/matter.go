package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type MatterInput struct {
	ParentID                                             int64
	Title, Type, Description, Status, StartDate, DueDate string
	OwnerMemberID                                        int64
}

func normalizeMatterInput(in MatterInput) (MatterInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Type = strings.TrimSpace(in.Type)
	in.Description = strings.TrimSpace(in.Description)
	in.Status = strings.TrimSpace(in.Status)
	in.StartDate = strings.TrimSpace(in.StartDate)
	in.DueDate = strings.TrimSpace(in.DueDate)
	if in.Title == "" {
		return in, errors.New("事务标题不能为空")
	}
	if in.Status == "" {
		in.Status = "planned"
	}
	if in.Status != "planned" && in.Status != "active" && in.Status != "done" && in.Status != "cancelled" {
		return in, errors.New("事务状态无效")
	}
	if in.Type == "" {
		in.Type = "general"
	}
	for _, item := range []struct {
		label string
		value string
	}{{"开始日期", in.StartDate}, {"截止日期", in.DueDate}} {
		if item.value == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", item.value); err != nil {
			return in, errors.New(item.label + "格式无效")
		}
	}
	if in.StartDate != "" && in.DueDate != "" && in.StartDate > in.DueDate {
		return in, errors.New("截止日期不能早于开始日期")
	}
	return in, nil
}

func validateMatterReferences(ctx context.Context, tx *sql.Tx, matterID int64, in MatterInput) error {
	if in.ParentID > 0 {
		if in.ParentID == matterID {
			return errors.New("事务不能把自己设为父事务")
		}
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM matters WHERE id=?)`, in.ParentID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return errors.New("选择的父事务不存在")
		}
		if matterID > 0 {
			var cyclic int
			if err := tx.QueryRowContext(ctx, `
WITH RECURSIVE descendants(id) AS (
    SELECT id FROM matters WHERE parent_id=?
    UNION ALL
    SELECT m.id FROM matters m JOIN descendants d ON m.parent_id=d.id
)
SELECT EXISTS(SELECT 1 FROM descendants WHERE id=?)`, matterID, in.ParentID).Scan(&cyclic); err != nil {
				return err
			}
			if cyclic != 0 {
				return errors.New("不能把下级事务设为父事务")
			}
		}
	}
	if in.OwnerMemberID > 0 {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM members WHERE id=? AND status='active' AND is_del=0)`, in.OwnerMemberID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return errors.New("选择的负责人不存在或已停用")
		}
	}
	return nil
}

func (s *Store) CreateMatter(ctx context.Context, actor int64, in MatterInput) error {
	var err error
	in, err = normalizeMatterInput(in)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := validateMatterReferences(ctx, tx, 0, in); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO matters(parent_id,title,matter_type,description,status,start_date,due_date,owner_member_id,created_by,created_at,updated_at,version) VALUES(?,?,?,?,?,?,?,?,?,?,?,1)`, nullID(in.ParentID), in.Title, in.Type, in.Description, in.Status, nullText(in.StartDate), nullText(in.DueDate), nullID(in.OwnerMemberID), actor, now(), now())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	if err := auditTx(ctx, tx, actor, "create", "matter", id, nil, map[string]any{"title": in.Title, "status": in.Status}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UpdateMatter(ctx context.Context, actor, matterID int64, in MatterInput) error {
	if matterID <= 0 {
		return errors.New("事务不存在")
	}
	var err error
	in, err = normalizeMatterInput(in)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var old MatterInput
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(parent_id,0),title,matter_type,description,status,
       COALESCE(start_date,''),COALESCE(due_date,''),COALESCE(owner_member_id,0)
FROM matters WHERE id=?`, matterID).Scan(&old.ParentID, &old.Title, &old.Type, &old.Description, &old.Status, &old.StartDate, &old.DueDate, &old.OwnerMemberID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("事务不存在")
		}
		return err
	}
	if err := validateMatterReferences(ctx, tx, matterID, in); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE matters
SET parent_id=?,title=?,matter_type=?,description=?,status=?,start_date=?,due_date=?,owner_member_id=?,updated_at=?,version=version+1
WHERE id=?`, nullID(in.ParentID), in.Title, in.Type, in.Description, in.Status, nullText(in.StartDate), nullText(in.DueDate), nullID(in.OwnerMemberID), now(), matterID); err != nil {
		return err
	}
	if err := auditTx(ctx, tx, actor, "update", "matter", matterID, old, in); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SetMatterStatus(ctx context.Context, actor, id int64, status string) error {
	if status != "planned" && status != "active" && status != "done" && status != "cancelled" {
		return errors.New("事务状态无效")
	}
	var old string
	if err := s.DB.QueryRowContext(ctx, `SELECT status FROM matters WHERE id=?`, id).Scan(&old); err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE matters SET status=?,updated_at=?,version=version+1 WHERE id=?`, status, now(), id); err != nil {
		return err
	}
	if err := auditTx(ctx, tx, actor, "update", "matter", id, map[string]any{"status": old}, map[string]any{"status": status}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Matters(ctx context.Context) ([]Matter, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT m.id,m.parent_id,COALESCE(p.title,''),m.title,m.matter_type,m.description,m.status,COALESCE(m.start_date,''),COALESCE(m.due_date,''),COALESCE(m.owner_member_id,0),COALESCE(o.name,''),COALESCE((SELECT SUM(e.amount_cent) FROM public_expenses e WHERE e.matter_id=m.id AND e.status='active'),0) FROM matters m LEFT JOIN matters p ON p.id=m.parent_id LEFT JOIN members o ON o.id=m.owner_member_id ORDER BY m.status='done',m.due_date IS NULL,m.due_date,m.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Matter
	for rows.Next() {
		var v Matter
		var parent *int64
		if err := rows.Scan(&v.ID, &parent, &v.ParentTitle, &v.Title, &v.Type, &v.Description, &v.Status, &v.StartDate, &v.DueDate, &v.OwnerMemberID, &v.OwnerName, &v.ExpenseCent); err != nil {
			return nil, err
		}
		v.ParentID = parent
		if parent != nil {
			v.ParentIDValue = *parent
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func nullText(v string) any {
	if v == "" {
		return nil
	}
	return v
}
