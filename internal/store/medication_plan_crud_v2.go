package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

func (s *Store) CreateMedicationPlanEnhanced(
	ctx context.Context,
	actor, patientID int64,
	medicine, dosage, scheduledTime, instructions, startDate, endDate string,
) (int64, error) {
	in, err := validateMedicationPlanFields(
		patientID,
		medicine,
		dosage,
		scheduledTime,
		instructions,
		startDate,
		endDate,
	)
	if err != nil {
		return 0, err
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var active int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1
    FROM members
    WHERE id=? AND status='active' AND is_del=0
)`, patientID).Scan(&active); err != nil {
		return 0, err
	}
	if active == 0 {
		return 0, errors.New("选择的服药成员不存在或已停用")
	}

	res, err := tx.ExecContext(ctx, `
INSERT INTO medication_plans(
    patient_member_id, medicine_name, dosage, scheduled_time, instructions,
    start_date, end_date, created_by, created_at, updated_at, is_deleted
)
VALUES(?,?,?,?,?,?,?,?,?,?,0)`,
		in.PatientMemberID,
		in.MedicineName,
		in.Dosage,
		in.ScheduledTime,
		in.Instructions,
		in.StartDate,
		nullText(in.EndDate),
		actor,
		now(),
		now(),
	)
	if err != nil {
		return 0, err
	}

	id, _ := res.LastInsertId()
	after := map[string]any{
		"patient_member_id": in.PatientMemberID,
		"medicine_name":     in.MedicineName,
		"dosage":             in.Dosage,
		"scheduled_time":     in.ScheduledTime,
		"instructions":       in.Instructions,
		"start_date":         in.StartDate,
		"end_date":           nullText(in.EndDate),
	}
	if err := auditTx(ctx, tx, actor, "create", "medication_plan", id, nil, after); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) medicationPlanFullByIDTx(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id int64) (MedicationPlanFull, error) {
	var plan MedicationPlanFull
	var deleted int
	err := q.QueryRowContext(ctx, `
SELECT p.id, p.patient_member_id, patient.name, p.medicine_name, p.dosage,
       p.scheduled_time, p.instructions, p.start_date, COALESCE(p.end_date,''),
       p.created_by, COALESCE(creator.name,''), p.created_at, p.updated_at,
       p.is_deleted, COALESCE(p.deleted_at,'')
FROM medication_plans p
JOIN members patient ON patient.id=p.patient_member_id
LEFT JOIN members creator ON creator.id=p.created_by
WHERE p.id=?`, id).Scan(
		&plan.ID,
		&plan.PatientMemberID,
		&plan.PatientName,
		&plan.MedicineName,
		&plan.Dosage,
		&plan.ScheduledTime,
		&plan.Instructions,
		&plan.StartDate,
		&plan.EndDate,
		&plan.CreatedBy,
		&plan.CreatorName,
		&plan.CreatedAt,
		&plan.UpdatedAt,
		&deleted,
		&plan.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MedicationPlanFull{}, errors.New("用药计划不存在")
		}
		return MedicationPlanFull{}, err
	}
	plan.IsDeleted = deleted != 0
	return plan, nil
}

func (s *Store) MedicationPlanFullByID(ctx context.Context, id int64, scheduledDate string) (MedicationPlanFull, error) {
	plan, err := s.medicationPlanFullByIDTx(ctx, s.DB, id)
	if err != nil {
		return plan, err
	}
	if scheduledDate == "" {
		scheduledDate = time.Now().Format(medicationDateLayout)
	}

	_ = s.DB.QueryRowContext(ctx, `
SELECT COALESCE(r.id,0), COALESCE(r.status,''), COALESCE(r.note,''),
       COALESCE(r.recorded_by_member_id,0), COALESCE(rec.name,''),
       COALESCE(r.recorded_at,'')
FROM medication_plans p
LEFT JOIN medication_intake_records r
  ON r.plan_id=p.id AND r.scheduled_date=?
LEFT JOIN members rec ON rec.id=r.recorded_by_member_id
WHERE p.id=?`, scheduledDate, id).Scan(
		&plan.RecordID,
		&plan.RecordStatus,
		&plan.RecordNote,
		&plan.RecordedBy,
		&plan.RecordedByName,
		&plan.RecordedAt,
	)

	_ = s.DB.QueryRowContext(ctx, `
SELECT COALESCE(c.response,''), COALESCE(c.verification_status,''),
       COALESCE(c.response_at,''), COALESCE(v.name,''), COALESCE(c.verified_at,'')
FROM medication_plans p
LEFT JOIN medication_checkins c
  ON c.plan_id=p.id AND c.scheduled_date=?
LEFT JOIN members v ON v.id=c.verified_by_member_id
WHERE p.id=?`, scheduledDate, id).Scan(
		&plan.CheckinResponse,
		&plan.CheckinStatus,
		&plan.CheckinAt,
		&plan.VerifiedByName,
		&plan.VerifiedAt,
	)
	return plan, nil
}

func (s *Store) MedicationPlanCreatorID(ctx context.Context, id int64) (int64, error) {
	var creator int64
	err := s.DB.QueryRowContext(
		ctx,
		`SELECT created_by FROM medication_plans WHERE id=? AND is_deleted=0`,
		id,
	).Scan(&creator)
	return creator, err
}

func (s *Store) UpdateMedicationPlanEnhanced(
	ctx context.Context,
	actor, id, patientID int64,
	medicine, dosage, scheduledTime, instructions, startDate, endDate string,
) error {
	in, err := validateMedicationPlanFields(
		patientID,
		medicine,
		dosage,
		scheduledTime,
		instructions,
		startDate,
		endDate,
	)
	if err != nil {
		return err
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	old, err := s.medicationPlanFullByIDTx(ctx, tx, id)
	if err != nil {
		return err
	}
	if old.IsDeleted {
		return errors.New("该用药计划已标记删除")
	}

	var active int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1
    FROM members
    WHERE id=? AND status='active' AND is_del=0
)`, patientID).Scan(&active); err != nil {
		return err
	}
	if active == 0 {
		return errors.New("选择的服药成员不存在或已停用")
	}

	_, err = tx.ExecContext(ctx, `
UPDATE medication_plans
SET patient_member_id=?, medicine_name=?, dosage=?, scheduled_time=?,
    instructions=?, start_date=?, end_date=?, updated_at=?
WHERE id=? AND is_deleted=0`,
		in.PatientMemberID,
		in.MedicineName,
		in.Dosage,
		in.ScheduledTime,
		in.Instructions,
		in.StartDate,
		nullText(in.EndDate),
		now(),
		id,
	)
	if err != nil {
		return err
	}

	before := map[string]any{
		"patient_member_id": old.PatientMemberID,
		"medicine_name":     old.MedicineName,
		"dosage":             old.Dosage,
		"scheduled_time":     old.ScheduledTime,
		"instructions":       old.Instructions,
		"start_date":         old.StartDate,
		"end_date":           nullText(old.EndDate),
	}
	after := map[string]any{
		"patient_member_id": in.PatientMemberID,
		"medicine_name":     in.MedicineName,
		"dosage":             in.Dosage,
		"scheduled_time":     in.ScheduledTime,
		"instructions":       in.Instructions,
		"start_date":         in.StartDate,
		"end_date":           nullText(in.EndDate),
	}
	if err := auditTx(ctx, tx, actor, "update", "medication_plan", id, before, after); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SoftDeleteMedicationPlan(ctx context.Context, actor, id int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	old, err := s.medicationPlanFullByIDTx(ctx, tx, id)
	if err != nil {
		return err
	}
	if old.IsDeleted {
		return errors.New("该用药计划已标记删除")
	}

	deletedAt := now()
	if _, err := tx.ExecContext(ctx, `
UPDATE medication_plans
SET is_deleted=1, deleted_at=?, deleted_by=?, updated_at=?
WHERE id=?`, deletedAt, actor, deletedAt, id); err != nil {
		return err
	}

	before := map[string]any{
		"is_deleted":        0,
		"patient_member_id": old.PatientMemberID,
		"medicine_name":     old.MedicineName,
	}
	after := map[string]any{
		"is_deleted": 1,
		"deleted_at": deletedAt,
		"deleted_by": actor,
	}
	if err := auditTx(ctx, tx, actor, "delete", "medication_plan", id, before, after); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) EndMedicationPlanEnhanced(ctx context.Context, actor, id int64, endDate string) error {
	endDate = strings.TrimSpace(endDate)
	if endDate == "" {
		endDate = time.Now().Format(medicationDateLayout)
	}
	if _, err := time.Parse(medicationDateLayout, endDate); err != nil {
		return errors.New("结束日期格式无效")
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	old, err := s.medicationPlanFullByIDTx(ctx, tx, id)
	if err != nil {
		return err
	}
	if old.IsDeleted {
		return errors.New("该用药计划已标记删除")
	}
	if endDate < old.StartDate {
		return errors.New("结束日期不能早于开始日期")
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE medication_plans SET end_date=?,updated_at=? WHERE id=?`,
		endDate,
		now(),
		id,
	); err != nil {
		return err
	}
	if err := auditTx(
		ctx,
		tx,
		actor,
		"update",
		"medication_plan",
		id,
		map[string]any{"end_date": nullText(old.EndDate)},
		map[string]any{"end_date": endDate},
	); err != nil {
		return err
	}
	return tx.Commit()
}
