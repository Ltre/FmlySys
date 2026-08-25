package httpserver

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Ltre/FmlySys/internal/store"
)

func (s *Server) requireMedicationPlanManageV3(w http.ResponseWriter, r *http.Request, planID int64, date string) (store.MedicationPlanV3, bool) {
	plan, err := s.Store.MedicationPlanV3ByID(r.Context(), planID, date)
	if err != nil || plan.IsDeleted {
		http.NotFound(w, r)
		return store.MedicationPlanV3{}, false
	}
	if !store.CanManageMedicationPlan(currentPermissions(r), currentMember(r).ID, plan.CreatedBy) {
		http.Error(w, "你没有管理该服药计划的权限", http.StatusForbidden)
		return store.MedicationPlanV3{}, false
	}
	return plan, true
}

func (s *Server) createMedicationPlanV3(w http.ResponseWriter, r *http.Request) {
	if !currentPermissions(r)["medication.manage_self"] {
		http.Error(w, "你没有创建服药计划的权限", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	patientID := parseID(r.FormValue("patient_member_id"))
	timezone := strings.TrimSpace(r.FormValue("timezone"))
	if timezone == "" {
		timezone = requestTimezone(r)
	}
	id, err := s.Store.CreateMedicationPlanV3(
		r.Context(),
		currentMember(r).ID,
		patientID,
		r.FormValue("medicine_name"),
		r.FormValue("dosage"),
		r.FormValue("scheduled_time"),
		r.FormValue("instructions"),
		r.FormValue("start_date"),
		r.FormValue("end_date"),
		timezone,
	)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/medication?member="+strconv.FormatInt(patientID, 10)+"&message="+url.QueryEscape("服药计划已创建")+"#medication-plan-"+strconv.FormatInt(id, 10))
}

func (s *Server) updateMedicationPlanV3(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	date := medicationDateForRequest(r)
	plan, ok := s.requireMedicationPlanManageV3(w, r, id, date)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	timezone := strings.TrimSpace(r.FormValue("timezone"))
	if timezone == "" {
		timezone = plan.Timezone
	}
	if err := s.Store.UpdateMedicationPlanV3(
		r.Context(),
		currentMember(r).ID,
		id,
		parseID(r.FormValue("patient_member_id")),
		r.FormValue("medicine_name"),
		r.FormValue("dosage"),
		r.FormValue("scheduled_time"),
		r.FormValue("instructions"),
		r.FormValue("start_date"),
		r.FormValue("end_date"),
		timezone,
	); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/medication/plans/"+strconv.FormatInt(id, 10)+"?message="+url.QueryEscape("服药计划已保存"))
}

func (s *Server) endMedicationPlanV3(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	date := medicationDateForRequest(r)
	plan, ok := s.requireMedicationPlanManageV3(w, r, id, date)
	if !ok {
		return
	}
	if plan.GloballyClosed(time.Now()) {
		redirect(w, r, "/medication/plans/"+strconv.FormatInt(id, 10)+"?message="+url.QueryEscape("该计划已经结束或过期"))
		return
	}
	if err := s.Store.EndMedicationPlanNow(r.Context(), currentMember(r).ID, id, plan.Timezone); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/medication/plans/"+strconv.FormatInt(id, 10)+"?message="+url.QueryEscape("服药计划已立即结束"))
}

func (s *Server) saveMedicationIntakeV3(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	date := strings.TrimSpace(r.FormValue("scheduled_date"))
	if date == "" {
		date = medicationDateForRequest(r)
	}
	if _, ok := s.requireMedicationPlanManageV3(w, r, id, date); !ok {
		return
	}
	_, actionable, reason, err := s.Store.MedicationPlanActionableV3(r.Context(), id, date, time.Now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !actionable {
		http.Error(w, "不能登记服药情况："+reason, http.StatusConflict)
		return
	}
	if err := s.Store.RecordMedicationIntake(r.Context(), currentMember(r).ID, id, date, r.FormValue("status"), r.FormValue("note")); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/medication/plans/"+strconv.FormatInt(id, 10)+"?date="+url.QueryEscape(date)+"&message="+url.QueryEscape("服药情况已保存"))
}

func (s *Server) verifyMedicationClaimV3(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	date := strings.TrimSpace(r.FormValue("scheduled_date"))
	if _, ok := s.requireMedicationPlanManageV3(w, r, id, date); !ok {
		return
	}
	_, actionable, reason, err := s.Store.MedicationPlanActionableV3(r.Context(), id, date, time.Now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !actionable {
		http.Error(w, "不能验证服药情况："+reason, http.StatusConflict)
		return
	}
	confirmed := r.FormValue("result") == "confirmed"
	if err := s.Store.MedicationVerifyClaim(r.Context(), currentMember(r).ID, id, date, confirmed); err != nil {
		s.fail(w, r, err)
		return
	}
	message := "已确认服药"
	if !confirmed {
		message = "已判定并没有服药；到达后续提醒节点时仍会继续提醒"
	}
	redirect(w, r, "/medication/plans/"+strconv.FormatInt(id, 10)+"?date="+url.QueryEscape(date)+"&message="+url.QueryEscape(message))
}

func (s *Server) medicationCheckinRespondV3(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	planID := parseID(r.FormValue("plan_id"))
	date := strings.TrimSpace(r.FormValue("scheduled_date"))
	response := r.FormValue("response")
	plan, actionable, reason, err := s.Store.MedicationPlanActionableV3(r.Context(), planID, date, time.Now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if plan.PatientMemberID != currentMember(r).ID {
		http.Error(w, "只有该计划的服药人可以签到", http.StatusForbidden)
		return
	}
	if !actionable {
		http.Error(w, "不能签到："+reason, http.StatusConflict)
		return
	}
	if err := s.Store.MedicationPatientRespond(r.Context(), currentMember(r).ID, planID, date, response); err != nil {
		s.fail(w, r, err)
		return
	}

	message := "已记录“等下再说”，若仍未完成确认会在后续节点再次提醒"
	if response == "taken" {
		message = "已提交“我已服药”，当前进入待验证状态；管理者尚未确认期间不会继续自动催促"
	} else if response == "later" {
		s.notifyMedicationManagersLater(r, plan, date)
	}
	redirect(w, r, "/medication/checkin?plan="+strconv.FormatInt(planID, 10)+"&date="+url.QueryEscape(date)+"&message="+url.QueryEscape(message))
}

func (s *Server) notifyMedicationManagersLater(r *http.Request, plan store.MedicationPlanV3, date string) {
	managers, err := s.Store.MedicationPlanManagers(r.Context(), plan.ID, currentMember(r).ID)
	if err != nil {
		log.Printf("medication later managers: %v", err)
		return
	}
	for _, manager := range managers {
		title := "服药人选择了“等下再说”"
		body := fmt.Sprintf("%s 暂未服用 %s（%s），计划时间 %s。", plan.PatientName, plan.MedicineName, plan.Dosage, plan.ScheduledTime)
		link := "/medication/plans/" + strconv.FormatInt(plan.ID, 10) + "?date=" + url.QueryEscape(date)
		notificationID, err := s.Store.CreateMemberNotification(
			r.Context(), currentMember(r).ID, manager.ID, "medication_later", title, body, link, plan.ID, date,
		)
		if err != nil {
			log.Printf("create medication later notification manager=%d: %v", manager.ID, err)
			continue
		}
		if err := s.dispatchNotificationCenterPush(r.Context(), manager.ID, notificationID, title, body); err != nil {
			log.Printf("push medication later notification manager=%d: %v", manager.ID, err)
		}
	}
}

func (s *Server) manualMedicationReminderV3(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	plan, ok := s.requireMedicationPlanManageV3(w, r, id, medicationDateForRequest(r))
	if !ok {
		return
	}
	date := plan.LocalDate(time.Now())
	needed, reason, err := s.Store.MedicationReminderNeededV3(r.Context(), id, date, time.Now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !needed {
		redirect(w, r, "/medication/plans/"+strconv.FormatInt(id, 10)+"?date="+date+"&message="+url.QueryEscape("未推送："+reason))
		return
	}

	title := "服药提醒：" + plan.MedicineName
	body := fmt.Sprintf("%s：请服用 %s（%s），计划时间 %s。", plan.PatientName, plan.MedicineName, plan.Dosage, plan.ScheduledTime)
	checkinLink := "/medication/checkin?plan=" + strconv.FormatInt(plan.ID, 10) + "&date=" + url.QueryEscape(date)
	notificationID, err := s.Store.CreateMemberNotification(
		r.Context(), currentMember(r).ID, plan.PatientMemberID, "medication_manual", title, body, checkinLink, plan.ID, date,
	)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.dispatchManualMedicationReminderV3(r.Context(), plan, date, notificationID, title, body); err != nil {
		redirect(w, r, "/medication/plans/"+strconv.FormatInt(id, 10)+"?date="+date+"&message="+url.QueryEscape("通知中心已写入，但外部推送失败："+err.Error()))
		return
	}
	redirect(w, r, "/medication/plans/"+strconv.FormatInt(id, 10)+"?date="+date+"&message="+url.QueryEscape("已发送提醒，并写入服药人的通知中心"))
}
