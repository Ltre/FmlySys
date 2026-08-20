package store

import (
	"context"
	"errors"
)

type MatterInput struct {
	ParentID                                             int64
	Title, Type, Description, Status, StartDate, DueDate string
	OwnerMemberID                                        int64
}

func (s *Store) CreateMatter(ctx context.Context, actor int64, in MatterInput) error {
	if in.Title == "" {
		return errors.New("事务标题不能为空")
	}
	if in.Status == "" {
		in.Status = "planned"
	}
	if in.Type == "" {
		in.Type = "general"
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
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
	rows, err := s.DB.QueryContext(ctx, `SELECT m.id,m.parent_id,COALESCE(p.title,''),m.title,m.matter_type,m.description,m.status,COALESCE(m.start_date,''),COALESCE(m.due_date,''),COALESCE(o.name,''),COALESCE((SELECT SUM(e.amount_cent) FROM public_expenses e WHERE e.matter_id=m.id AND e.status='active'),0) FROM matters m LEFT JOIN matters p ON p.id=m.parent_id LEFT JOIN members o ON o.id=m.owner_member_id ORDER BY m.status='done',m.due_date IS NULL,m.due_date,m.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Matter
	for rows.Next() {
		var v Matter
		var parent *int64
		if err := rows.Scan(&v.ID, &parent, &v.ParentTitle, &v.Title, &v.Type, &v.Description, &v.Status, &v.StartDate, &v.DueDate, &v.OwnerName, &v.ExpenseCent); err != nil {
			return nil, err
		}
		v.ParentID = parent
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
