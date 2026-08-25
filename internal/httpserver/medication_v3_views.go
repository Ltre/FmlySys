package httpserver

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Ltre/FmlySys/internal/store"
)

type medicationPlanCardV3 struct {
	Plan         store.MedicationPlanV3
	Status       string
	Actionable   bool
	ActionReason string
	CanManage    bool
}

type medicationPageV3View struct {
	Title           string
	ActivePartition string
	AdminUsername   string
	CurrentMember   store.Member
	Permissions     map[string]bool
	Members         []store.Member
	Patients        []store.Member
	SelectedPatient int64
	MedicationDate  string
	RangeDays       int
	Plans           []medicationPlanCardV3
	Summary         store.MedicationRangeSummary
	Message         string
	HasClosedPlans  bool
	Timezone        string
}

type medicationPlansV3View struct {
	Title           string
	ActivePartition string
	AdminUsername   string
	CurrentMember   store.Member
	Permissions     map[string]bool
	Plans           []medicationPlanCardV3
	Timezone        string
	Message         string
}

type medicationPlanDetailV3View struct {
	Title           string
	ActivePartition string
	AdminUsername   string
	CurrentMember   store.Member
	Permissions     map[string]bool
	Members         []store.Member
	Plan            store.MedicationPlanV3
	Status          string
	Date            string
	CanManage       bool
	Actionable      bool
	ActionReason    string
	Deliveries      []store.MedicationNotificationDelivery
	Message         string
	Timezone        string
}

type medicationCheckinV3View struct {
	Title           string
	ActivePartition string
	AdminUsername   string
	CurrentMember   store.Member
	Permissions     map[string]bool
	Plan            store.MedicationPlanV3
	Status          string
	Date            string
	Actionable      bool
	ActionReason    string
	Message         string
	Timezone        string
}

func medicationV3Date(r *http.Request) string {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		date = medicationDateForRequest(r)
	}
	return date
}

func (s *Server) medicationV3(w http.ResponseWriter, r *http.Request) {
	date := medicationV3Date(r)
	v := medicationPageV3View{
		Title:           "服药管理",
		ActivePartition: s.PM.ActiveID,
		CurrentMember:   currentMember(r),
		Permissions:     currentPermissions(r),
		MedicationDate:  date,
		RangeDays:       normalizeRangeDays(r.URL.Query().Get("range")),
		SelectedPatient: parseID(r.URL.Query().Get("member")),
		Message:         queryMessage(r),
		Timezone:        requestTimezone(r),
	}
	var err error
	v.Patients, err = s.Store.MedicationPatients(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if v.Permissions["medication.manage_self"] || v.Permissions["medication.manage_others"] {
		v.Members, err = s.familyMembers(r.Context())
		if err != nil {
			s.fail(w, r, err)
			return
		}
	}
	if v.SelectedPatient == 0 {
		for _, patient := range v.Patients {
			if patient.ID == v.CurrentMember.ID {
				v.SelectedPatient = patient.ID
				break
			}
		}
		if v.SelectedPatient == 0 && len(v.Patients) > 0 {
			v.SelectedPatient = v.Patients[0].ID
		}
	}
	if v.SelectedPatient > 0 {
		plans, err := s.Store.MedicationPlansForPatientV3(r.Context(), v.SelectedPatient, date)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		nowTime := time.Now()
		for _, plan := range plans {
			_, actionable, reason, _ := s.Store.MedicationPlanActionableV3(r.Context(), plan.ID, date, nowTime)
			status := plan.StatusAt(nowTime)
			if plan.GloballyClosed(nowTime) {
				v.HasClosedPlans = true
			}
			v.Plans = append(v.Plans, medicationPlanCardV3{
				Plan:         plan,
				Status:       status,
				Actionable:   actionable,
				ActionReason: reason,
				CanManage:    store.CanManageMedicationPlan(v.Permissions, v.CurrentMember.ID, plan.CreatedBy),
			})
		}
	}
	v.Summary, err = s.Store.MedicationSummaryRangeForPatientV3(r.Context(), date, v.RangeDays, v.SelectedPatient)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Templates.ExecuteTemplate(w, "medication-v3.html", v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) medicationPlansFlatV3(w http.ResponseWriter, r *http.Request) {
	date := medicationDateForRequest(r)
	plans, err := s.Store.AllMedicationPlansV3(r.Context(), date)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	member := currentMember(r)
	perms := currentPermissions(r)
	v := medicationPlansV3View{
		Title:           "全部服药计划",
		ActivePartition: s.PM.ActiveID,
		CurrentMember:   member,
		Permissions:     perms,
		Timezone:        requestTimezone(r),
		Message:         queryMessage(r),
	}
	nowTime := time.Now()
	for _, plan := range plans {
		_, actionable, reason, _ := s.Store.MedicationPlanActionableV3(r.Context(), plan.ID, date, nowTime)
		v.Plans = append(v.Plans, medicationPlanCardV3{
			Plan:         plan,
			Status:       plan.StatusAt(nowTime),
			Actionable:   actionable,
			ActionReason: reason,
			CanManage:    store.CanManageMedicationPlan(perms, member.ID, plan.CreatedBy),
		})
	}
	if err := s.Templates.ExecuteTemplate(w, "medication-plans-v3.html", v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) medicationPlanDetailV3(w http.ResponseWriter, r *http.Request) {
	date := medicationV3Date(r)
	id := parseID(r.PathValue("id"))
	plan, err := s.Store.MedicationPlanV3ByID(r.Context(), id, date)
	if err != nil || plan.IsDeleted {
		http.NotFound(w, r)
		return
	}
	member := currentMember(r)
	perms := currentPermissions(r)
	_, actionable, reason, _ := s.Store.MedicationPlanActionableV3(r.Context(), id, date, time.Now())
	v := medicationPlanDetailV3View{
		Title:           "服药计划详情",
		ActivePartition: s.PM.ActiveID,
		CurrentMember:   member,
		Permissions:     perms,
		Plan:            plan,
		Status:          plan.StatusAt(time.Now()),
		Date:            date,
		CanManage:       store.CanManageMedicationPlan(perms, member.ID, plan.CreatedBy),
		Actionable:      actionable,
		ActionReason:    reason,
		Message:         queryMessage(r),
		Timezone:        requestTimezone(r),
	}
	if v.CanManage {
		v.Members, _ = s.familyMembers(r.Context())
	}
	v.Deliveries, _ = s.Store.MedicationNotificationDeliveries(r.Context(), plan.ID, date)
	if err := s.Templates.ExecuteTemplate(w, "medication-plan-detail-v3.html", v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) medicationCheckinV3(w http.ResponseWriter, r *http.Request) {
	date := medicationV3Date(r)
	planID := parseID(r.URL.Query().Get("plan"))
	plan, err := s.Store.MedicationPlanV3ByID(r.Context(), planID, date)
	if err != nil || plan.IsDeleted || plan.PatientMemberID != currentMember(r).ID {
		http.NotFound(w, r)
		return
	}
	_, actionable, reason, _ := s.Store.MedicationPlanActionableV3(r.Context(), planID, date, time.Now())
	v := medicationCheckinV3View{
		Title:           "服药签到",
		ActivePartition: s.PM.ActiveID,
		CurrentMember:   currentMember(r),
		Permissions:     currentPermissions(r),
		Plan:            plan,
		Status:          plan.StatusAt(time.Now()),
		Date:            date,
		Actionable:      actionable,
		ActionReason:    reason,
		Message:         queryMessage(r),
		Timezone:        requestTimezone(r),
	}
	if err := s.Templates.ExecuteTemplate(w, "medication-checkin-v3.html", v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func medicationRedirectPlan(id int64, date, message string) string {
	to := "/medication/plans/" + strconv.FormatInt(id, 10)
	if date != "" {
		to += "?date=" + urlQueryEscape(date)
		if message != "" {
			to += "&message=" + urlQueryEscape(message)
		}
	} else if message != "" {
		to += "?message=" + urlQueryEscape(message)
	}
	return to
}

func urlQueryEscape(v string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(v, "%", "%25"), " ", "+"), "#", "%23")
}
