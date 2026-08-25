package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func scanMedicationPlans(rows *sql.Rows) ([]MedicationPlanFull, error) {
	defer rows.Close()
	var out []MedicationPlanFull
	for rows.Next() {
		var p MedicationPlanFull
		if err := rows.Scan(
			&p.ID,
			&p.PatientMemberID,
			&p.PatientName,
			&p.MedicineName,
			&p.Dosage,
			&p.ScheduledTime,
			&p.Instructions,
			&p.StartDate,
			&p.EndDate,
			&p.CreatedBy,
			&p.CreatorName,
			&p.CreatedAt,
			&p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) AllMedicationPlans(ctx context.Context) ([]MedicationPlanFull, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT p.id, p.patient_member_id, patient.name, p.medicine_name, p.dosage,
       p.scheduled_time, p.instructions, p.start_date, COALESCE(p.end_date,''),
       p.created_by, COALESCE(creator.name,''), p.created_at, p.updated_at
FROM medication_plans p
JOIN members patient ON patient.id=p.patient_member_id
LEFT JOIN members creator ON creator.id=p.created_by
WHERE p.is_deleted=0
ORDER BY patient.name, p.start_date, p.scheduled_time, p.id`)
	if err != nil {
		return nil, err
	}
	return scanMedicationPlans(rows)
}

func (s *Store) MedicationPatients(ctx context.Context) ([]Member, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT m.id, m.name, m.relation
FROM members m
WHERE EXISTS(
    SELECT 1
    FROM medication_plans p
    WHERE p.patient_member_id=m.id AND p.is_deleted=0
)
ORDER BY m.name, m.id`)
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

func (s *Store) MedicationPlansForPatientDate(ctx context.Context, patientID int64, date string) ([]MedicationPlanFull, error) {
	if patientID <= 0 {
		return nil, nil
	}
	if _, err := time.Parse(medicationDateLayout, date); err != nil {
		return nil, errors.New("服药日期格式无效")
	}

	rows, err := s.DB.QueryContext(ctx, `
SELECT p.id, p.patient_member_id, patient.name, p.medicine_name, p.dosage,
       p.scheduled_time, p.instructions, p.start_date, COALESCE(p.end_date,''),
       p.created_by, COALESCE(creator.name,''), p.created_at, p.updated_at
FROM medication_plans p
JOIN members patient ON patient.id=p.patient_member_id
LEFT JOIN members creator ON creator.id=p.created_by
WHERE p.patient_member_id=?
  AND p.is_deleted=0
  AND p.start_date<=?
  AND (p.end_date IS NULL OR p.end_date>=?)
ORDER BY p.scheduled_time, p.id`, patientID, date, date)
	if err != nil {
		return nil, err
	}

	plans, err := scanMedicationPlans(rows)
	if err != nil {
		return nil, err
	}
	for i := range plans {
		plan, err := s.MedicationPlanFullByID(ctx, plans[i].ID, date)
		if err == nil {
			plans[i] = plan
		}
	}
	return plans, nil
}

func (s *Store) MedicationSummaryRange(ctx context.Context, toDate string, days int) (MedicationRangeSummary, error) {
	return s.MedicationSummaryRangeForPatient(ctx, toDate, days, 0)
}

func (s *Store) MedicationSummaryRangeForPatient(ctx context.Context, toDate string, days int, patientID int64) (MedicationRangeSummary, error) {
	if days != 7 && days != 14 && days != 30 && days != 90 && days != 180 && days != 365 {
		days = 7
	}
	end, err := time.Parse(medicationDateLayout, toDate)
	if err != nil {
		return MedicationRangeSummary{}, errors.New("统计结束日期格式无效")
	}
	fromDate := end.AddDate(0, 0, -(days - 1)).Format(medicationDateLayout)

	var scheduled, taken, missed int
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
JOIN medication_plans p
  ON p.is_deleted=0
 AND p.start_date<=dates.d
 AND (p.end_date IS NULL OR p.end_date>=dates.d)
 AND (?=0 OR p.patient_member_id=?)
LEFT JOIN medication_intake_records r
  ON r.plan_id=p.id AND r.scheduled_date=dates.d`, fromDate, toDate, patientID, patientID).
		Scan(&scheduled, &taken, &missed)
	if err != nil {
		return MedicationRangeSummary{}, err
	}

	unrecorded := scheduled - taken - missed
	if unrecorded < 0 {
		unrecorded = 0
	}
	percent := 0
	if scheduled > 0 {
		percent = taken * 100 / scheduled
	}
	return MedicationRangeSummary{
		FromDate:     fromDate,
		ToDate:       toDate,
		Days:         days,
		Scheduled:    scheduled,
		Taken:        taken,
		Missed:       missed,
		Unrecorded:   unrecorded,
		TakenPercent: percent,
	}, nil
}
