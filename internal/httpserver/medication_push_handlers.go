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
	webassets "github.com/Ltre/FmlySys/web"
)

func (s *Server) medicationPushPublicKey(w http.ResponseWriter, r *http.Request) {
	key, err := loadOrCreateMedicationVAPIDKey(s.Config.DataDir)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"public_key": medicationVAPIDPublicKeyBase64(key)})
}

func (s *Server) medicationPushSubscribe(w http.ResponseWriter, r *http.Request) {
	var payload pushSubscriptionPayload
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&payload); err != nil {
		http.Error(w, "PWA 推送订阅数据无效", http.StatusBadRequest)
		return
	}
	m := currentMember(r)
	if err := s.Store.SaveMedicationPushSubscription(r.Context(), m.ID, payload.Endpoint, payload.Keys.P256DH, payload.Keys.Auth, r.UserAgent()); err != nil {
		s.fail(w, r, err)
		return
	}
	_ = s.Store.AuditChange(r.Context(), m.ID, "create", "medication_push_subscription", 0, nil, map[string]any{"member_id": m.ID, "endpoint_host": endpointHost(payload.Endpoint)})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func endpointHost(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	return u.Host
}

func (s *Server) medicationServiceWorker(w http.ResponseWriter, r *http.Request) {
	raw, err := webassets.FS.ReadFile("static/medication-sw.js")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Service-Worker-Allowed", "/")
	_, _ = w.Write(raw)
}

func (s *Server) manualMedicationReminder(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	plan, ok := s.requireMedicationPlanManage(w, r, id)
	if !ok {
		return
	}
	date := medicationToday()
	needed, reason, err := s.Store.MedicationReminderNeeded(r.Context(), id, date)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !needed {
		redirect(w, r, "/medication/plans/"+strconv.FormatInt(id, 10)+"?date="+date+"&message="+url.QueryEscape("未推送："+reason))
		return
	}
	if err := s.dispatchMedicationReminder(r.Context(), plan, date, "manual"); err != nil {
		redirect(w, r, "/medication/plans/"+strconv.FormatInt(id, 10)+"?date="+date+"&message="+url.QueryEscape("推送失败："+err.Error()))
		return
	}
	redirect(w, r, "/medication/plans/"+strconv.FormatInt(id, 10)+"?date="+date+"&message="+url.QueryEscape("已向服药人发送提醒"))
}

func (s *Server) dispatchMedicationReminder(ctx context.Context, plan store.MedicationPlanFull, date, stage string) error {
	title := "服药提醒"
	body := fmt.Sprintf("%s：现在请服用 %s（%s）", plan.PatientName, plan.MedicineName, plan.Dosage)
	payload, _ := json.Marshal(map[string]any{"title": title, "body": body, "voice": body, "url": "/medication/checkin?plan=" + strconv.FormatInt(plan.ID, 10) + "&date=" + url.QueryEscape(date)})
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

func (s *Server) runMedicationReminderSweep(ctx context.Context, nowTime time.Time) {
	loc := medicationLocalLocation()
	nowTime = nowTime.In(loc)
	date := nowTime.Format("2006-01-02")
	plans, err := s.Store.MedicationActivePlansForDate(ctx, date)
	if err != nil {
		log.Printf("medication reminder plans: %v", err)
		return
	}
	for _, plan := range plans {
		scheduled, err := time.ParseInLocation("2006-01-02 15:04", date+" "+plan.ScheduledTime, loc)
		if err != nil {
			continue
		}
		stage := store.MedicationStageForTime(nowTime, scheduled)
		if stage == "" {
			continue
		}
		attempted, err := s.Store.MedicationAutomaticStageAttempted(ctx, plan.ID, date, stage)
		if err != nil || attempted {
			continue
		}
		needed, _, err := s.Store.MedicationReminderNeeded(ctx, plan.ID, date)
		if err != nil || !needed {
			continue
		}
		if err := s.dispatchMedicationReminder(ctx, plan, date, stage); err != nil {
			log.Printf("medication reminder plan=%d stage=%s: %v", plan.ID, stage, err)
		}
	}
}

func (s *Server) StartMedicationReminderLoop(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		s.runMedicationReminderSweep(ctx, time.Now())
		for {
			select {
			case <-ctx.Done():
				return
			case nowTime := <-ticker.C:
				s.runMedicationReminderSweep(ctx, nowTime)
			}
		}
	}()
}
