package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Ltre/FmlySys/internal/store"
)

func (s *Server) dispatchMedicationReminderTargetV3(
	ctx context.Context,
	plan store.MedicationPlanV3,
	date, stage, targetURL, title, body string,
) error {
	payload, _ := json.Marshal(map[string]any{
		"title": title,
		"body":  body,
		"voice": body,
		"url":   targetURL,
	})

	key, keyErr := loadOrCreateMedicationVAPIDKey(s.Config.DataDir)
	pwaSent := false
	var pwaErrors []string
	if keyErr == nil {
		subs, err := s.Store.MedicationPushSubscriptions(ctx, plan.PatientMemberID)
		if err != nil {
			pwaErrors = append(pwaErrors, err.Error())
		} else {
			for _, sub := range subs {
				statusCode, sendErr := sendWebPush(ctx, key, sub.Endpoint, sub.P256DH, sub.Auth, payload)
				if sendErr == nil {
					pwaSent = true
					_ = s.Store.RecordMedicationNotificationDelivery(ctx, plan.ID, date, stage, "pwa", "sent", sub.Endpoint)
					continue
				}
				_ = s.Store.RecordMedicationNotificationDelivery(ctx, plan.ID, date, stage, "pwa", "failed", sendErr.Error())
				pwaErrors = append(pwaErrors, sendErr.Error())
				if statusCode == http.StatusNotFound || statusCode == http.StatusGone {
					_ = s.Store.DeleteMedicationPushSubscription(ctx, sub.ID)
				}
			}
		}
	} else {
		pwaErrors = append(pwaErrors, keyErr.Error())
	}
	if pwaSent {
		return nil
	}

	if err := sendTermuxMedicationNotification(s.Config.DataDir, title, body); err == nil {
		_ = s.Store.RecordMedicationNotificationDelivery(ctx, plan.ID, date, stage, "termux", "sent", "FRP STCP SSH")
		return nil
	} else {
		_ = s.Store.RecordMedicationNotificationDelivery(ctx, plan.ID, date, stage, "termux", "failed", err.Error())
		if len(pwaErrors) == 0 {
			return err
		}
		return fmt.Errorf("PWA 未成功投递（%s）；Termux 兜底也失败（%v）", strings.Join(pwaErrors, "; "), err)
	}
}

func (s *Server) dispatchAutomaticMedicationReminderV3(ctx context.Context, plan store.MedicationPlanV3, date, stage string) error {
	title := "服药提醒"
	body := fmt.Sprintf("%s：现在请服用 %s（%s）", plan.PatientName, plan.MedicineName, plan.Dosage)
	target := "/medication/checkin?plan=" + strconv.FormatInt(plan.ID, 10) + "&date=" + url.QueryEscape(date)
	return s.dispatchMedicationReminderTargetV3(ctx, plan, date, stage, target, title, body)
}

func (s *Server) dispatchManualMedicationReminderV3(
	ctx context.Context,
	plan store.MedicationPlanV3,
	date string,
	notificationID int64,
	title, body string,
) error {
	// Manual reminders intentionally open the notification-center detail first;
	// that page contains the exact check-in link requested by the user.
	target := "/notifications/" + strconv.FormatInt(notificationID, 10)
	return s.dispatchMedicationReminderTargetV3(ctx, plan, date, "manual", target, title, body)
}

func (s *Server) runMedicationReminderSweepV3(ctx context.Context, nowTime time.Time) {
	plans, err := s.Store.MedicationReminderCandidatesV3(ctx, medicationDateInTimezone(nowTime, defaultSystemTimezone))
	if err != nil {
		log.Printf("medication reminder candidates v3: %v", err)
		return
	}

	for _, plan := range plans {
		loc := timezoneLocation(plan.Timezone)
		localNow := nowTime.In(loc)
		date := localNow.Format("2006-01-02")
		if !plan.ActiveOnV3(date) {
			continue
		}
		scheduled, err := time.ParseInLocation("2006-01-02 15:04", date+" "+plan.ScheduledTime, loc)
		if err != nil {
			log.Printf("medication reminder parse plan=%d timezone=%s: %v", plan.ID, plan.Timezone, err)
			continue
		}
		stage := store.MedicationStageForTimeV3(localNow, scheduled)
		if stage == "" {
			continue
		}
		sent, err := s.Store.MedicationAutomaticStageSentV3(ctx, plan.ID, date, stage)
		if err != nil || sent {
			continue
		}
		retryAllowed, err := s.Store.MedicationAutomaticStageRetryAllowedV3(ctx, plan.ID, date, stage, nowTime)
		if err != nil || !retryAllowed {
			continue
		}
		needed, _, err := s.Store.MedicationReminderNeededV3(ctx, plan.ID, date, nowTime)
		if err != nil || !needed {
			continue
		}
		if err := s.dispatchAutomaticMedicationReminderV3(ctx, plan, date, stage); err != nil {
			log.Printf("medication reminder v3 plan=%d date=%s stage=%s timezone=%s: %v", plan.ID, date, stage, plan.Timezone, err)
		}
	}
}

func (s *Server) StartMedicationReminderLoopV3(ctx context.Context) {
	go func() {
		s.runMedicationReminderSweepV3(ctx, time.Now())
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case nowTime := <-ticker.C:
				s.runMedicationReminderSweepV3(ctx, nowTime)
			}
		}
	}()
}
