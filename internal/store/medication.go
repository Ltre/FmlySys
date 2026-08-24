package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

func normalizeMedicationDate(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Now().Format("2006-01-02"), nil
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return "", errors.New("服药日期格式无效")
	}
	return value, nil
}

func normalizeMedicationPlanInput(patientID int64, medicine, dosage, scheduledTime, instructions, startDate string) (int64, string, string, string, string, string, error) {
	medicine = strings.TrimSpace(medicine)
	dosage = strings.TrimSpace(dosage)
	scheduledTime = strings.TrimSpace(scheduledTime)
	instructions = strings.TrimSpace(instructions)
	if patientID <= 0 {
		return 0, "", "", "", "", "", errors.New("请选择服药成员")
	}
	if medicine == "" || utf8.RuneCountInString(medicine) > 120 {
		return 0, "", "", "", "", "", errors.New("药品名称必填且最多 120 个字符")
	}
	if dosage == "" || utf8.RuneCountInString(dosage) > 120 {
		return 0, "", "", "", "", "", errors.New("每次用量必填且最多 120 个字符")
	}
	if _, err := time.Parse("15:04", scheduledTime); err != nil {
		return 0, "", "", "", "", "", errors.New("计划服药时间格式无效")
	}
	if utf8.RuneCountInString(instructions) > 500 {
		return 0, "", "", "", "", "", errors.New("服药说明最多 500 个字符")
	}
	startDate, err := normalizeMedicationDate(startDate)
	if err != nil {
		return 0, "", "", "", "", "", err
	}
	return patientID, medicine, dosage, scheduledTime, instructions, startDate, nil
}

func (s *Store) CreateMedicationPlan(ctx context.Context, actor, patientID int64, medicine, dosage, scheduledTime, instructions, startDate string) error {
	patientID, medicine, dosage, scheduledTime, instructions, startDate, err := normalizeMedicationPlanInput(patientID, medicine, dosage, scheduledTime, instructions, startDate)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM members WHERE id=? AND status='active' AND is_del=0)`, patientID).Scan(&active); err != nil {
		return err
	}
	if active == 0 {
		return errors.New("选择的服药成员不存在或已停用")
	}
	res, err := tx.ExecContext(ctx, `
INSERT INTO medication_plans(patient_member_id,medicine_name,dosage,scheduled_time,instructions,start_date,end_date,created_by,created_at,updated_at)
VALUES(?,?,?,?,?,?,NULL,?,?,?)`, patientID, medicine, dosage, scheduledTime, instructions, startDate, actor, now(), now())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	if err := auditTx(ctx, tx, actor, "create", "medication_plan", id, nil, map[string]any{
		"patient_member_id": patientID, "medicine_name": medicine, "dosage": dosage,
		"scheduled_time": scheduledTime, "instructions": instructions, "start_date": startDate,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MedicationPlansForDate(ctx context.Context, scheduledDate string) ([]MedicationPlan, error) {
	scheduledDate, err := normalizeMedicationDate(scheduledDate)
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT p.id,p.patient_member_id,patient.name,p.medicine_name,p.dosage,p.scheduled_time,p.instructions,
       p.start_date,COALESCE(p.end_date,''),p.created_by,
       COALESCE(r.id,0),COALESCE(r.status,''),COALESCE(r.note,''),COALESCE(r.recorded_by_member_id,0),
       COALESCE(recorder.name,''),COALESCE(r.recorded_at,'')
FROM medication_plans p
JOIN members patient ON patient.id=p.patient_member_id
LEFT JOIN medication_intake_records r ON r.plan_id=p.id AND r.scheduled_date=?
LEFT JOIN members recorder ON recorder.id=r.recorded_by_member_id
WHERE p.start_date<=? AND (p.end_date IS NULL OR p.end_date>=?)
ORDER BY patient.name,p.scheduled_time,p.id`, scheduledDate, scheduledDate, scheduledDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MedicationPlan
	for rows.Next() {
		var plan MedicationPlan
		if err := rows.Scan(
			&plan.ID, &plan.PatientMemberID, &plan.PatientName, &plan.MedicineName, &plan.Dosage,
			&plan.ScheduledTime, &plan.Instructions, &plan.StartDate, &plan.EndDate, &plan.CreatedBy,
			&plan.RecordID, &plan.RecordStatus, &plan.RecordNote, &plan.RecordedBy,
			&plan.RecordedByName, &plan.RecordedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, plan)
	}
	return out, rows.Err()
}

func (s *Store) RecordMedicationIntake(ctx context.Context, actor, planID int64, scheduledDate, status, note string) error {
	if planID <= 0 {
		return errors.New("用药计划不存在")
	}
	var err error
	scheduledDate, err = normalizeMedicationDate(scheduledDate)
	if err != nil {
		return err
	}
	status = strings.TrimSpace(status)
	if status != "taken" && status != "missed" {
		return errors.New("请选择已确认服用或未服用")
	}
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > 500 {
		return errors.New("服药记录备注最多 500 个字符")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var startDate, endDate string
	if err := tx.QueryRowContext(ctx, `SELECT start_date,COALESCE(end_date,'') FROM medication_plans WHERE id=?`, planID).Scan(&startDate, &endDate); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("用药计划不存在")
		}
		return err
	}
	if scheduledDate < startDate || (endDate != "" && scheduledDate > endDate) {
		return errors.New("该日期不在用药计划有效期内")
	}
	old := map[string]any{}
	var recordID, oldRecorder int64
	var oldStatus, oldNote, oldRecordedAt string
	err = tx.QueryRowContext(ctx, `
SELECT id,status,note,recorded_by_member_id,recorded_at
FROM medication_intake_records WHERE plan_id=? AND scheduled_date=?`, planID, scheduledDate).
		Scan(&recordID, &oldStatus, &oldNote, &oldRecorder, &oldRecordedAt)
	action := "update"
	if errors.Is(err, sql.ErrNoRows) {
		action = "create"
		res, insertErr := tx.ExecContext(ctx, `
INSERT INTO medication_intake_records(plan_id,scheduled_date,status,note,recorded_by_member_id,recorded_at,updated_at)
VALUES(?,?,?,?,?,?,?)`, planID, scheduledDate, status, note, actor, now(), now())
		if insertErr != nil {
			return insertErr
		}
		recordID, _ = res.LastInsertId()
		old = nil
	} else if err != nil {
		return err
	} else {
		old = map[string]any{"status": oldStatus, "note": oldNote, "recorded_by_member_id": oldRecorder, "recorded_at": oldRecordedAt}
		if _, err := tx.ExecContext(ctx, `
UPDATE medication_intake_records
SET status=?,note=?,recorded_by_member_id=?,recorded_at=?,updated_at=?
WHERE id=?`, status, note, actor, now(), now(), recordID); err != nil {
			return err
		}
	}
	if err := auditTx(ctx, tx, actor, action, "medication_intake", recordID, old, map[string]any{
		"plan_id": planID, "scheduled_date": scheduledDate, "status": status, "note": note,
		"recorded_by_member_id": actor,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) EndMedicationPlan(ctx context.Context, actor, planID int64, endDate string) error {
	if planID <= 0 {
		return errors.New("用药计划不存在")
	}
	var err error
	endDate, err = normalizeMedicationDate(endDate)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var startDate string
	var oldEnd sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT start_date,end_date FROM medication_plans WHERE id=?`, planID).Scan(&startDate, &oldEnd); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("用药计划不存在")
		}
		return err
	}
	if oldEnd.Valid {
		return errors.New("该用药计划已经停用")
	}
	if endDate < startDate {
		return errors.New("停用日期不能早于计划开始日期")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE medication_plans SET end_date=?,updated_at=? WHERE id=?`, endDate, now(), planID); err != nil {
		return err
	}
	if err := auditTx(ctx, tx, actor, "end", "medication_plan", planID, map[string]any{"end_date": nil}, map[string]any{"end_date": endDate}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MedicationSummaryThrough(ctx context.Context, toDate string) (MedicationSummary, error) {
	toDate, err := normalizeMedicationDate(toDate)
	if err != nil {
		return MedicationSummary{}, err
	}
	end, _ := time.Parse("2006-01-02", toDate)
	fromDate := end.AddDate(0, 0, -6).Format("2006-01-02")
	var scheduled, taken, missed int64
	err = s.DB.QueryRowContext(ctx, `
WITH RECURSIVE dates(d) AS (
    SELECT date(?)
    UNION ALL
    SELECT date(d,'+1 day') FROM dates WHERE d < date(?)
)
SELECT COUNT(p.id),
       COALESCE(SUM(CASE WHEN r.status='taken' THEN 1 ELSE 0 END),0),
       COALESCE(SUM(CASE WHEN r.status='missed' THEN 1 ELSE 0 END),0)
FROM dates
JOIN medication_plans p ON p.start_date<=dates.d AND (p.end_date IS NULL OR p.end_date>=dates.d)
LEFT JOIN medication_intake_records r ON r.plan_id=p.id AND r.scheduled_date=dates.d`, fromDate, toDate).
		Scan(&scheduled, &taken, &missed)
	if err != nil {
		return MedicationSummary{}, err
	}
	unrecorded := scheduled - taken - missed
	if unrecorded < 0 {
		unrecorded = 0
	}
	percent := int64(0)
	if scheduled > 0 {
		percent = taken * 100 / scheduled
	}
	return MedicationSummary{
		FromDate: fromDate, ToDate: toDate,
		Scheduled: int(scheduled), Taken: int(taken), Missed: int(missed),
		Unrecorded: int(unrecorded), TakenPercent: int(percent),
	}, nil
}
