package httpserver

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (s *Server) createMedicationPlanV2(w http.ResponseWriter, r *http.Request) {
	if !currentPermissions(r)["medication.manage_self"] {
		http.Error(w, "你没有创建服药计划的权限", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	patientID := parseID(r.FormValue("patient_member_id"))
	id, err := s.Store.CreateMedicationPlanEnhanced(
		r.Context(),
		currentMember(r).ID,
		patientID,
		r.FormValue("medicine_name"),
		r.FormValue("dosage"),
		r.FormValue("scheduled_time"),
		r.FormValue("instructions"),
		r.FormValue("start_date"),
		r.FormValue("end_date"),
	)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/medication?member="+strconv.FormatInt(patientID, 10)+"&message="+url.QueryEscape("服药计划已创建")+"#medication-plan-"+strconv.FormatInt(id, 10))
}

func (s *Server) updateMedicationPlanV2(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	if _, ok := s.requireMedicationPlanManage(w, r, id); !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.UpdateMedicationPlanEnhanced(
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
	); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/medication/plans/"+strconv.FormatInt(id, 10)+"?message="+url.QueryEscape("服药计划已保存"))
}

func (s *Server) deleteMedicationPlanV2(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	if _, ok := s.requireMedicationPlanManage(w, r, id); !ok {
		return
	}
	if err := s.Store.SoftDeleteMedicationPlan(r.Context(), currentMember(r).ID, id); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/medication/plans?message="+url.QueryEscape("服药计划已标记删除，历史审计仍保留"))
}

func (s *Server) endMedicationPlanV2(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	if _, ok := s.requireMedicationPlanManage(w, r, id); !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.EndMedicationPlanEnhanced(r.Context(), currentMember(r).ID, id, r.FormValue("end_date")); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/medication/plans/"+strconv.FormatInt(id, 10)+"?message="+url.QueryEscape("服药计划已结束"))
}

func (s *Server) saveMedicationIntakeV2(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	if _, ok := s.requireMedicationPlanManage(w, r, id); !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	date := strings.TrimSpace(r.FormValue("scheduled_date"))
	if date == "" {
		date = medicationToday()
	}
	if err := s.Store.RecordMedicationIntake(r.Context(), currentMember(r).ID, id, date, r.FormValue("status"), r.FormValue("note")); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/medication/plans/"+strconv.FormatInt(id, 10)+"?date="+url.QueryEscape(date)+"&message="+url.QueryEscape("服药情况已保存"))
}

func (s *Server) medicationCheckin(w http.ResponseWriter, r *http.Request) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		date = medicationToday()
	}
	planID := parseID(r.URL.Query().Get("plan"))
	plan, err := s.Store.MedicationPlanFullByID(r.Context(), planID, date)
	if err != nil || plan.IsDeleted || plan.PatientMemberID != currentMember(r).ID {
		http.NotFound(w, r)
		return
	}
	v := medicationCheckinView{
		Title:           "服药签到",
		ActivePartition: s.PM.ActiveID,
		CurrentMember:   currentMember(r),
		Permissions:     currentPermissions(r),
		Plan:            plan,
		Date:            date,
		Message:         queryMessage(r),
	}
	s.renderMedicationTemplate(w, "medication-checkin.html", v)
}

func (s *Server) medicationCheckinRespond(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	planID := parseID(r.FormValue("plan_id"))
	date := strings.TrimSpace(r.FormValue("scheduled_date"))
	response := r.FormValue("response")
	if err := s.Store.MedicationPatientRespond(r.Context(), currentMember(r).ID, planID, date, response); err != nil {
		s.fail(w, r, err)
		return
	}
	message := "已记录“等下再说”，若仍未完成确认会在后续节点再次提醒"
	if response == "taken" {
		message = "已提交“我已服药”，当前进入待验证状态；不会因为管理者尚未确认而继续自动催促"
	}
	redirect(w, r, "/medication/checkin?plan="+strconv.FormatInt(planID, 10)+"&date="+url.QueryEscape(date)+"&message="+url.QueryEscape(message))
}

func (s *Server) verifyMedicationClaim(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	if _, ok := s.requireMedicationPlanManage(w, r, id); !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	date := strings.TrimSpace(r.FormValue("scheduled_date"))
	confirmed := r.FormValue("result") == "confirmed"
	if err := s.Store.MedicationVerifyClaim(r.Context(), currentMember(r).ID, id, date, confirmed); err != nil {
		s.fail(w, r, err)
		return
	}
	message := "已确认服药"
	if !confirmed {
		message = "已判定并没有服药；若到达后续提醒节点，系统仍会再次通知服药人"
	}
	redirect(w, r, "/medication/plans/"+strconv.FormatInt(id, 10)+"?date="+url.QueryEscape(date)+"&message="+url.QueryEscape(message))
}
