package store

import "context"

func MedicationReminderRequiresBothChannels(stage string) bool {
	return stage == "manual" || stage == "plus2h"
}

func (s *Store) MedicationAutomaticChannelSentV3(ctx context.Context, planID int64, date, stage, channel string) (bool, error) {
	var exists int
	err := s.DB.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1
    FROM medication_notification_deliveries
    WHERE plan_id=? AND scheduled_date=? AND stage=? AND channel=? AND status='sent'
)`, planID, date, stage, channel).Scan(&exists)
	return exists != 0, err
}

func (s *Store) MedicationAutomaticStageCompleteV3(ctx context.Context, planID int64, date, stage string) (bool, error) {
	if stage == "plus2h" {
		pwaSent, err := s.MedicationAutomaticChannelSentV3(ctx, planID, date, stage, "pwa")
		if err != nil || !pwaSent {
			return false, err
		}
		termuxSent, err := s.MedicationAutomaticChannelSentV3(ctx, planID, date, stage, "termux")
		if err != nil {
			return false, err
		}
		return termuxSent, nil
	}
	return s.MedicationAutomaticStageSentV3(ctx, planID, date, stage)
}
