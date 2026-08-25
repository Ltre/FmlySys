package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *Store) MedicationPatientRespond(ctx context.Context, actor, planID int64, scheduledDate, response string) error {
	if response != "taken" && response != "later" {
		return errors.New("请选择“我已服药”或“等下再说”")
	}
	if _, err := time.Parse(medicationDateLayout, scheduledDate); err != nil {
		return errors.New("服药日期格式无效")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	plan, err := s.medicationPlanFullByIDTx(ctx, tx, planID)
	if err != nil {
		return err
	}
	if plan.IsDeleted || !plan.ActiveOn(scheduledDate) {
		return errors.New("该日期不在用药计划有效期内")
	}
	if actor != plan.PatientMemberID {
		return errors.New("只有该计划的服药人可以进行签到")
	}
	var oldResponse, oldStatus string
	var oldID int64
	err = tx.QueryRowContext(ctx, `SELECT id,response,verification_status FROM medication_checkins WHERE plan_id=? AND scheduled_date=?`, planID, scheduledDate).Scan(&oldID, &oldResponse, &oldStatus)
	action := "update"
	before := any(map[string]any{"response": oldResponse, "verification_status": oldStatus})
	if errors.Is(err, sql.ErrNoRows) {
		action = "create"
		before = nil
	} else if err != nil {
		return err
	}
	verification := "none"
	if response == "taken" {
		verification = "pending"
	}
	at := now()
	_, err = tx.ExecContext(ctx, `INSERT INTO medication_checkins(plan_id,scheduled_date,patient_member_id,response,response_at,verification_status,verified_by_member_id,verified_at,updated_at) VALUES(?,?,?,?,?,?,NULL,NULL,?) ON CONFLICT(plan_id,scheduled_date) DO UPDATE SET patient_member_id=excluded.patient_member_id,response=excluded.response,response_at=excluded.response_at,verification_status=excluded.verification_status,verified_by_member_id=NULL,verified_at=NULL,updated_at=excluded.updated_at`, planID, scheduledDate, actor, response, at, verification, at)
	if err != nil {
		return err
	}
	if oldID == 0 {
		_ = tx.QueryRowContext(ctx, `SELECT id FROM medication_checkins WHERE plan_id=? AND scheduled_date=?`, planID, scheduledDate).Scan(&oldID)
	}
	if err := auditTx(ctx, tx, actor, action, "medication_checkin", oldID, before, map[string]any{"plan_id": planID, "scheduled_date": scheduledDate, "response": response, "verification_status": verification}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MedicationVerifyClaim(ctx context.Context, actor, planID int64, scheduledDate string, confirmed bool) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var checkinID, patientID int64
	var response, verification string
	if err := tx.QueryRowContext(ctx, `SELECT id,patient_member_id,response,verification_status FROM medication_checkins WHERE plan_id=? AND scheduled_date=?`, planID, scheduledDate).Scan(&checkinID, &patientID, &response, &verification); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("没有待验证的服药记录")
		}
		return err
	}
	if response != "taken" || verification != "pending" {
		return errors.New("当前记录不是待验证的“我已服药”记录")
	}
	newVerification := "rejected"
	intakeStatus := "missed"
	if confirmed {
		newVerification = "confirmed"
		intakeStatus = "taken"
	}
	at := now()
	if _, err := tx.ExecContext(ctx, `UPDATE medication_checkins SET verification_status=?,verified_by_member_id=?,verified_at=?,updated_at=? WHERE id=?`, newVerification, actor, at, at, checkinID); err != nil {
		return err
	}
	var recordID int64
	var oldStatus, oldNote, oldAt string
	err = tx.QueryRowContext(ctx, `SELECT id,status,note,recorded_at FROM medication_intake_records WHERE plan_id=? AND scheduled_date=?`, planID, scheduledDate).Scan(&recordID, &oldStatus, &oldNote, &oldAt)
	intakeAction := "update"
	intakeBefore := any(map[string]any{"status": oldStatus, "note": oldNote, "recorded_at": oldAt})
	if errors.Is(err, sql.ErrNoRows) {
		intakeAction = "create"
		intakeBefore = nil
		res, e := tx.ExecContext(ctx, `INSERT INTO medication_intake_records(plan_id,scheduled_date,status,note,recorded_by_member_id,recorded_at,updated_at) VALUES(?,?,?,?,?,?,?)`, planID, scheduledDate, intakeStatus, "由服药签到验证生成", actor, at, at)
		if e != nil {
			return e
		}
		recordID, _ = res.LastInsertId()
	} else if err != nil {
		return err
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE medication_intake_records SET status=?,note=?,recorded_by_member_id=?,recorded_at=?,updated_at=? WHERE id=?`, intakeStatus, "由服药签到验证生成", actor, at, at, recordID); err != nil {
			return err
		}
	}
	if err := auditTx(ctx, tx, actor, "update", "medication_checkin", checkinID, map[string]any{"verification_status": "pending"}, map[string]any{"verification_status": newVerification, "verified_by_member_id": actor}); err != nil {
		return err
	}
	if err := auditTx(ctx, tx, actor, intakeAction, "medication_intake", recordID, intakeBefore, map[string]any{"plan_id": planID, "scheduled_date": scheduledDate, "status": intakeStatus, "recorded_by_member_id": actor}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MedicationReminderNeeded(ctx context.Context, planID int64, scheduledDate string) (bool, string, error) {
	plan, err := s.MedicationPlanFullByID(ctx, planID, scheduledDate)
	if err != nil {
		return false, "", err
	}
	if !plan.ActiveOn(scheduledDate) {
		return false, "计划当前不在有效期内", nil
	}
	if plan.RecordStatus == "taken" || plan.CheckinStatus == "confirmed" {
		return false, "当天已经确认服药", nil
	}
	if plan.CheckinResponse == "taken" && plan.CheckinStatus == "pending" {
		return false, "服药人已点击“我已服药”，正在等待验证", nil
	}
	return true, "当天尚未完成服药确认", nil
}
