package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

const MedicationAutomaticRetryIntervalV4 = 20 * time.Minute

func medicationAutomaticRetryAllowedSinceV4(last, nowTime time.Time) bool {
	return nowTime.UTC().Sub(last.UTC()) >= MedicationAutomaticRetryIntervalV4
}

func (s *Store) MedicationAutomaticStageRetryAllowedV4(ctx context.Context, planID int64, date, stage string, nowTime time.Time) (bool, error) {
	var latest sql.NullString
	if err := s.DB.QueryRowContext(ctx, `
SELECT MAX(created_at)
FROM medication_notification_deliveries
WHERE plan_id=? AND scheduled_date=? AND stage=? AND status='failed'`, planID, date, stage).Scan(&latest); err != nil {
		return false, err
	}
	if !latest.Valid || strings.TrimSpace(latest.String) == "" {
		return true, nil
	}
	last, err := time.Parse(time.RFC3339Nano, latest.String)
	if err != nil {
		return true, nil
	}
	return medicationAutomaticRetryAllowedSinceV4(last, nowTime), nil
}
