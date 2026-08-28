package store

import (
	"context"
	"errors"
	"strconv"
	"time"
)

type MedicationMonthDayOverview struct {
	Date      string `json:"date"`
	Day       int    `json:"day"`
	Scheduled int    `json:"scheduled"`
	Taken     int    `json:"taken"`
	State     string `json:"state"`
}

type MedicationMonthOverview struct {
	Month         string                       `json:"month"`
	Today         string                       `json:"today"`
	WeekdayOffset int                          `json:"weekday_offset"`
	Days          []MedicationMonthDayOverview `json:"days"`
}

func medicationMonthDayState(date, today string, scheduled, taken int) string {
	if scheduled <= 0 {
		return "none"
	}
	if date >= today {
		return "open"
	}
	if taken >= scheduled {
		return "complete"
	}
	if taken > 0 {
		return "partial"
	}
	return "missed"
}

func medicationPlanOccurrenceV3(p MedicationPlanV3, localDate string) (time.Time, bool) {
	if localDate < p.StartDate || (p.EndDate != "" && localDate > p.EndDate) {
		return time.Time{}, false
	}
	loc := medicationTimezoneLocation(p.Timezone)
	scheduled, err := time.ParseInLocation(medicationDateLayout+" 15:04", localDate+" "+p.ScheduledTime, loc)
	if err != nil {
		return time.Time{}, false
	}
	if p.EndedAt != "" {
		ended, err := time.Parse(time.RFC3339Nano, p.EndedAt)
		if err != nil || scheduled.After(ended) {
			return time.Time{}, false
		}
	}
	return scheduled, true
}

func (s *Store) MedicationMonthOverviewForPatientV3(ctx context.Context, patientID int64, monthStart, today, displayTimezone string) (MedicationMonthOverview, error) {
	displayLoc := medicationTimezoneLocation(displayTimezone)
	start, err := time.ParseInLocation(medicationDateLayout, monthStart, displayLoc)
	if err != nil || start.Day() != 1 {
		return MedicationMonthOverview{}, errors.New("月份起始日期无效")
	}
	if _, err := time.ParseInLocation(medicationDateLayout, today, displayLoc); err != nil {
		return MedicationMonthOverview{}, errors.New("当前日期无效")
	}
	endExclusive := start.AddDate(0, 1, 0)
	end := endExclusive.AddDate(0, 0, -1)

	overview := MedicationMonthOverview{
		Month:         start.Format("2006-01"),
		Today:         today,
		WeekdayOffset: (int(start.Weekday()) + 6) % 7,
		Days:          make([]MedicationMonthDayOverview, 0, end.Day()),
	}
	dayIndex := map[string]int{}
	for d := start; d.Before(endExclusive); d = d.AddDate(0, 0, 1) {
		date := d.Format(medicationDateLayout)
		dayIndex[date] = len(overview.Days)
		overview.Days = append(overview.Days, MedicationMonthDayOverview{Date: date, Day: d.Day(), State: "none"})
	}
	if patientID <= 0 {
		return overview, nil
	}

	ids, err := s.medicationPlanV3IDs(ctx, `patient_member_id=? AND is_deleted=0`, patientID)
	if err != nil {
		return MedicationMonthOverview{}, err
	}
	plans := make([]MedicationPlanV3, 0, len(ids))
	for _, id := range ids {
		p, err := s.MedicationPlanV3ByID(ctx, id, monthStart)
		if err != nil {
			return MedicationMonthOverview{}, err
		}
		plans = append(plans, p)
	}

	// scheduled_date is stored in each plan's own timezone. Fetch the patient's
	// completed records and join them to occurrences by plan-local date below.
	taken := map[string]bool{}
	rows, err := s.DB.QueryContext(ctx, `
SELECT r.plan_id,r.scheduled_date
FROM medication_intake_records r
JOIN medication_plans p ON p.id=r.plan_id
WHERE p.patient_member_id=?
  AND p.is_deleted=0
  AND r.status='taken'`, patientID)
	if err != nil {
		return MedicationMonthOverview{}, err
	}
	for rows.Next() {
		var planID int64
		var date string
		if err := rows.Scan(&planID, &date); err != nil {
			rows.Close()
			return MedicationMonthOverview{}, err
		}
		taken[strconv.FormatInt(planID, 10)+"|"+date] = true
	}
	if err := rows.Close(); err != nil {
		return MedicationMonthOverview{}, err
	}

	for _, p := range plans {
		planLoc := medicationTimezoneLocation(p.Timezone)
		firstPlanDate := start.In(planLoc).Format(medicationDateLayout)
		lastPlanDate := endExclusive.Add(-time.Nanosecond).In(planLoc).Format(medicationDateLayout)
		first, err := time.ParseInLocation(medicationDateLayout, firstPlanDate, planLoc)
		if err != nil {
			continue
		}
		for d := first; d.Format(medicationDateLayout) <= lastPlanDate; d = d.AddDate(0, 0, 1) {
			localDate := d.Format(medicationDateLayout)
			occurrence, ok := medicationPlanOccurrenceV3(p, localDate)
			if !ok || occurrence.Before(start) || !occurrence.Before(endExclusive) {
				continue
			}
			displayDate := occurrence.In(displayLoc).Format(medicationDateLayout)
			idx, ok := dayIndex[displayDate]
			if !ok {
				continue
			}
			overview.Days[idx].Scheduled++
			if taken[strconv.FormatInt(p.ID, 10)+"|"+localDate] {
				overview.Days[idx].Taken++
			}
		}
	}

	for i := range overview.Days {
		day := &overview.Days[i]
		day.State = medicationMonthDayState(day.Date, today, day.Scheduled, day.Taken)
	}
	return overview, nil
}
