package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

const DefaultMedicationTimezone = "Asia/Shanghai"

type MedicationPlanV3 struct {
	MedicationPlanFull
	Timezone    string
	EndedAt     string
	EndedBy     int64
	EndedByName string
}

func NormalizeMedicationTimezone(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultMedicationTimezone
	}
	if _, err := time.LoadLocation(raw); err != nil {
		return DefaultMedicationTimezone
	}
	return raw
}

func medicationTimezoneLocation(raw string) *time.Location {
	name := NormalizeMedicationTimezone(raw)
	if loc, err := time.LoadLocation(name); err == nil {
		return loc
	}
	return time.FixedZone("UTC+8", 8*60*60)
}

func (p MedicationPlanV3) LocalDate(nowTime time.Time) string {
	return nowTime.In(medicationTimezoneLocation(p.Timezone)).Format(medicationDateLayout)
}

func (p MedicationPlanV3) GloballyClosed(nowTime time.Time) bool {
	if p.IsDeleted || p.EndedAt != "" {
		return true
	}
	today := p.LocalDate(nowTime)
	return p.EndDate != "" && today > p.EndDate
}

func (p MedicationPlanV3) ActiveOnV3(date string) bool {
	if p.IsDeleted || p.EndedAt != "" {
		return false
	}
	return date >= p.StartDate && (p.EndDate == "" || date <= p.EndDate)
}

func (p MedicationPlanV3) StatusAt(nowTime time.Time) string {
	if p.IsDeleted {
		return "已删除"
	}
	if p.EndedAt != "" {
		return "已手动结束"
	}
	today := p.LocalDate(nowTime)
	if p.EndDate != "" && today > p.EndDate {
		return "已过期"
	}
	if today < p.StartDate {
		return "未开始"
	}
	return "进行中"
}

func (p MedicationPlanV3) ActionableForDate(nowTime time.Time, date string) bool {
	if p.GloballyClosed(nowTime) {
		return false
	}
	return p.ActiveOnV3(date)
}

func (s *Store) MedicationPlanV3ByID(ctx context.Context, id int64, scheduledDate string) (MedicationPlanV3, error) {
	base, err := s.MedicationPlanFullByID(ctx, id, scheduledDate)
	if err != nil {
		return MedicationPlanV3{}, err
	}
	v := MedicationPlanV3{MedicationPlanFull: base, Timezone: DefaultMedicationTimezone}
	if err := s.DB.QueryRowContext(ctx, `
SELECT COALESCE(timezone,'Asia/Shanghai'),COALESCE(ended_at,''),COALESCE(ended_by,0),COALESCE(m.name,'')
FROM medication_plans p
LEFT JOIN members m ON m.id=p.ended_by
WHERE p.id=?`, id).Scan(&v.Timezone, &v.EndedAt, &v.EndedBy, &v.EndedByName); err != nil {
		return MedicationPlanV3{}, err
	}
	v.Timezone = NormalizeMedicationTimezone(v.Timezone)
	return v, nil
}

func (s *Store) medicationPlanV3IDs(ctx context.Context, where string, args ...any) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM medication_plans WHERE `+where+` ORDER BY patient_member_id,start_date,scheduled_time,id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) MedicationPlansForPatientV3(ctx context.Context, patientID int64, date string) ([]MedicationPlanV3, error) {
	if patientID <= 0 {
		return nil, nil
	}
	ids, err := s.medicationPlanV3IDs(ctx, `patient_member_id=? AND is_deleted=0`, patientID)
	if err != nil {
		return nil, err
	}
	out := make([]MedicationPlanV3, 0, len(ids))
	for _, id := range ids {
		p, err := s.MedicationPlanV3ByID(ctx, id, date)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Store) AllMedicationPlansV3(ctx context.Context, date string) ([]MedicationPlanV3, error) {
	ids, err := s.medicationPlanV3IDs(ctx, `is_deleted=0`)
	if err != nil {
		return nil, err
	}
	out := make([]MedicationPlanV3, 0, len(ids))
	for _, id := range ids {
		p, err := s.MedicationPlanV3ByID(ctx, id, date)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Store) CreateMedicationPlanV3(
	ctx context.Context,
	actor, patientID int64,
	medicine, dosage, scheduledTime, instructions, startDate, endDate, timezone string,
) (int64, error) {
	timezone = NormalizeMedicationTimezone(timezone)
	if strings.TrimSpace(startDate) == "" {
		startDate = time.Now().In(medicationTimezoneLocation(timezone)).Format(medicationDateLayout)
	}
	in, err := validateMedicationPlanFields(patientID, medicine, dosage, scheduledTime, instructions, startDate, endDate)
	if err != nil {
		return 0, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM members WHERE id=? AND status='active' AND is_del=0)`, patientID).Scan(&active); err != nil {
		return 0, err
	}
	if active == 0 {
		return 0, errors.New("选择的服药成员不存在或已停用")
	}
	res, err := tx.ExecContext(ctx, `
INSERT INTO medication_plans(
    patient_member_id,medicine_name,dosage,scheduled_time,instructions,start_date,end_date,
    created_by,created_at,updated_at,is_deleted,timezone
)
VALUES(?,?,?,?,?,?,?,?,?,?,0,?)`, in.PatientMemberID, in.MedicineName, in.Dosage, in.ScheduledTime,
		in.Instructions, in.StartDate, nullText(in.EndDate), actor, now(), now(), timezone)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if err := auditTx(ctx, tx, actor, "create", "medication_plan", id, nil, map[string]any{
		"patient_member_id": in.PatientMemberID,
		"medicine_name":     in.MedicineName,
		"dosage":            in.Dosage,
		"scheduled_time":    in.ScheduledTime,
		"instructions":      in.Instructions,
		"start_date":        in.StartDate,
		"end_date":          nullText(in.EndDate),
		"timezone":          timezone,
	}); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) UpdateMedicationPlanV3(
	ctx context.Context,
	actor, id, patientID int64,
	medicine, dosage, scheduledTime, instructions, startDate, endDate, timezone string,
) error {
	timezone = NormalizeMedicationTimezone(timezone)
	in, err := validateMedicationPlanFields(patientID, medicine, dosage, scheduledTime, instructions, startDate, endDate)
	if err != nil {
		return err
	}
	old, err := s.MedicationPlanV3ByID(ctx, id, startDate)
	if err != nil {
		return err
	}
	if old.IsDeleted {
		return errors.New("该用药计划已标记删除")
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
	if _, err := tx.ExecContext(ctx, `
UPDATE medication_plans
SET patient_member_id=?,medicine_name=?,dosage=?,scheduled_time=?,instructions=?,start_date=?,end_date=?,timezone=?,updated_at=?
WHERE id=? AND is_deleted=0`, in.PatientMemberID, in.MedicineName, in.Dosage, in.ScheduledTime,
		in.Instructions, in.StartDate, nullText(in.EndDate), timezone, now(), id); err != nil {
		return err
	}
	before := map[string]any{
		"patient_member_id": old.PatientMemberID,
		"medicine_name":     old.MedicineName,
		"dosage":            old.Dosage,
		"scheduled_time":    old.ScheduledTime,
		"instructions":      old.Instructions,
		"start_date":        old.StartDate,
		"end_date":          nullText(old.EndDate),
		"timezone":          old.Timezone,
	}
	after := map[string]any{
		"patient_member_id": in.PatientMemberID,
		"medicine_name":     in.MedicineName,
		"dosage":            in.Dosage,
		"scheduled_time":    in.ScheduledTime,
		"instructions":      in.Instructions,
		"start_date":        in.StartDate,
		"end_date":          nullText(in.EndDate),
		"timezone":          timezone,
	}
	if err := auditTx(ctx, tx, actor, "update", "medication_plan", id, before, after); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) EndMedicationPlanNow(ctx context.Context, actor, id int64, timezone string) error {
	timezone = NormalizeMedicationTimezone(timezone)
	nowTime := time.Now()
	localDate := nowTime.In(medicationTimezoneLocation(timezone)).Format(medicationDateLayout)
	old, err := s.MedicationPlanV3ByID(ctx, id, localDate)
	if err != nil {
		return err
	}
	if old.IsDeleted {
		return errors.New("该用药计划已标记删除")
	}
	if old.EndedAt != "" {
		return errors.New("该用药计划已经结束")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	endedAt := now()
	newEndDate := old.EndDate
	if localDate >= old.StartDate && (newEndDate == "" || localDate < newEndDate) {
		newEndDate = localDate
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE medication_plans
SET end_date=?,ended_at=?,ended_by=?,updated_at=?
WHERE id=? AND is_deleted=0`, nullText(newEndDate), endedAt, actor, endedAt, id); err != nil {
		return err
	}
	if err := auditTx(ctx, tx, actor, "update", "medication_plan", id,
		map[string]any{"end_date": nullText(old.EndDate), "ended_at": nil, "ended_by": nil},
		map[string]any{"end_date": nullText(newEndDate), "ended_at": endedAt, "ended_by": actor}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MedicationPlanActionableV3(ctx context.Context, planID int64, selectedDate string, nowTime time.Time) (MedicationPlanV3, bool, string, error) {
	p, err := s.MedicationPlanV3ByID(ctx, planID, selectedDate)
	if err != nil {
		return MedicationPlanV3{}, false, "", err
	}
	if p.IsDeleted {
		return p, false, "计划已删除", nil
	}
	if p.EndedAt != "" {
		return p, false, "计划已手动结束", nil
	}
	today := p.LocalDate(nowTime)
	if p.EndDate != "" && today > p.EndDate {
		return p, false, "计划已经过期", nil
	}
	if selectedDate < p.StartDate {
		return p, false, "所选日期早于计划开始日期", nil
	}
	if p.EndDate != "" && selectedDate > p.EndDate {
		return p, false, "所选日期晚于计划结束日期", nil
	}
	return p, true, "", nil
}

func (s *Store) MedicationReminderNeededV3(ctx context.Context, planID int64, scheduledDate string, nowTime time.Time) (bool, string, error) {
	p, actionable, reason, err := s.MedicationPlanActionableV3(ctx, planID, scheduledDate, nowTime)
	if err != nil || !actionable {
		return false, reason, err
	}
	if p.RecordStatus == "taken" || p.CheckinStatus == "confirmed" {
		return false, "当天已经确认服药", nil
	}
	if p.CheckinResponse == "taken" && p.CheckinStatus == "pending" {
		return false, "服药人已点击“我已服药”，正在等待验证", nil
	}
	return true, "当天尚未完成服药确认", nil
}

func (s *Store) MedicationReminderCandidatesV3(ctx context.Context, dateHint string) ([]MedicationPlanV3, error) {
	ids, err := s.medicationPlanV3IDs(ctx, `is_deleted=0 AND ended_at IS NULL`)
	if err != nil {
		return nil, err
	}
	out := make([]MedicationPlanV3, 0, len(ids))
	for _, id := range ids {
		p, err := s.MedicationPlanV3ByID(ctx, id, dateHint)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func MedicationStageForTimeV3(nowTime, scheduled time.Time) string {
	if nowTime.Before(scheduled) {
		return ""
	}
	if nowTime.Before(scheduled.Add(time.Hour)) {
		return "scheduled"
	}
	if nowTime.Before(scheduled.Add(2 * time.Hour)) {
		return "plus1h"
	}
	return "plus2h"
}

func (s *Store) MedicationAutomaticStageSentV3(ctx context.Context, planID int64, date, stage string) (bool, error) {
	var exists int
	err := s.DB.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1 FROM medication_notification_deliveries
    WHERE plan_id=? AND scheduled_date=? AND stage=? AND status='sent'
)`, planID, date, stage).Scan(&exists)
	return exists != 0, err
}

func (s *Store) MedicationAutomaticStageRetryAllowedV3(ctx context.Context, planID int64, date, stage string, nowTime time.Time) (bool, error) {
	var latest sql.NullString
	if err := s.DB.QueryRowContext(ctx, `
SELECT MAX(created_at)
FROM medication_notification_deliveries
WHERE plan_id=? AND scheduled_date=? AND stage=?`, planID, date, stage).Scan(&latest); err != nil {
		return false, err
	}
	if !latest.Valid || strings.TrimSpace(latest.String) == "" {
		return true, nil
	}
	last, err := time.Parse(time.RFC3339Nano, latest.String)
	if err != nil {
		return true, nil
	}
	return nowTime.UTC().Sub(last.UTC()) >= time.Minute, nil
}

func (s *Store) MedicationSummaryRangeForPatientV3(ctx context.Context, toDate string, days int, patientID int64) (MedicationRangeSummary, error) {
	if days != 7 && days != 14 && days != 30 && days != 90 && days != 180 && days != 365 {
		days = 7
	}
	end, err := time.Parse(medicationDateLayout, toDate)
	if err != nil {
		return MedicationRangeSummary{}, errors.New("统计结束日期格式无效")
	}
	fromDate := end.AddDate(0, 0, -(days - 1)).Format(medicationDateLayout)

	where := `is_deleted=0`
	args := []any{}
	if patientID > 0 {
		where += ` AND patient_member_id=?`
		args = append(args, patientID)
	}
	ids, err := s.medicationPlanV3IDs(ctx, where, args...)
	if err != nil {
		return MedicationRangeSummary{}, err
	}
	plans := make([]MedicationPlanV3, 0, len(ids))
	for _, id := range ids {
		p, err := s.MedicationPlanV3ByID(ctx, id, toDate)
		if err != nil {
			return MedicationRangeSummary{}, err
		}
		plans = append(plans, p)
	}

	records := map[string]string{}
	rows, err := s.DB.QueryContext(ctx, `
SELECT plan_id,scheduled_date,status
FROM medication_intake_records
WHERE scheduled_date>=? AND scheduled_date<=?`, fromDate, toDate)
	if err != nil {
		return MedicationRangeSummary{}, err
	}
	for rows.Next() {
		var planID int64
		var date, status string
		if err := rows.Scan(&planID, &date, &status); err != nil {
			rows.Close()
			return MedicationRangeSummary{}, err
		}
		records[strconv.FormatInt(planID, 10)+"|"+date] = status
	}
	if err := rows.Close(); err != nil {
		return MedicationRangeSummary{}, err
	}

	isScheduled := func(p MedicationPlanV3, date string) bool {
		if date < p.StartDate || (p.EndDate != "" && date > p.EndDate) {
			return false
		}
		if p.EndedAt == "" {
			return true
		}
		ended, err := time.Parse(time.RFC3339Nano, p.EndedAt)
		if err != nil {
			return false
		}
		loc := medicationTimezoneLocation(p.Timezone)
		endedLocal := ended.In(loc)
		endedDate := endedLocal.Format(medicationDateLayout)
		if date < endedDate {
			return true
		}
		if date > endedDate {
			return false
		}
		scheduled, err := time.ParseInLocation(medicationDateLayout+" 15:04", date+" "+p.ScheduledTime, loc)
		return err == nil && !scheduled.After(endedLocal)
	}

	summary := MedicationRangeSummary{FromDate: fromDate, ToDate: toDate, Days: days}
	for d := end.AddDate(0, 0, -(days - 1)); !d.After(end); d = d.AddDate(0, 0, 1) {
		date := d.Format(medicationDateLayout)
		for _, p := range plans {
			if !isScheduled(p, date) {
				continue
			}
			summary.Scheduled++
			key := strconv.FormatInt(p.ID, 10) + "|" + date
			switch records[key] {
			case "taken":
				summary.Taken++
			case "missed":
				summary.Missed++
			default:
				summary.Unrecorded++
			}
		}
	}
	if summary.Scheduled > 0 {
		summary.TakenPercent = summary.Taken * 100 / summary.Scheduled
	}
	return summary, nil
}
