package httpserver

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Ltre/FmlySys/internal/store"
)

type medicationPageView struct {
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
	Plans           []store.MedicationPlanFull
	Summary         store.MedicationRangeSummary
	Message         string
	Error           string
}

type medicationPlansView struct {
	Title           string
	ActivePartition string
	AdminUsername   string
	CurrentMember   store.Member
	Permissions     map[string]bool
	Plans           []store.MedicationPlanFull
	Today           string
	Message         string
}

type medicationPlanDetailView struct {
	Title           string
	ActivePartition string
	AdminUsername   string
	CurrentMember   store.Member
	Permissions     map[string]bool
	Members         []store.Member
	Plan            store.MedicationPlanFull
	Date            string
	CanManage       bool
	Deliveries      []store.MedicationNotificationDelivery
	Message         string
	Error           string
}

type medicationCheckinView struct {
	Title           string
	ActivePartition string
	AdminUsername   string
	CurrentMember   store.Member
	Permissions     map[string]bool
	Plan            store.MedicationPlanFull
	Date            string
	Message         string
}

type pushSubscriptionPayload struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256DH string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func medicationLocalLocation() *time.Location {
	if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		return loc
	}
	return time.FixedZone("CST", 8*60*60)
}

func medicationToday() string {
	return time.Now().In(medicationLocalLocation()).Format("2006-01-02")
}

func normalizeRangeDays(raw string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(raw))
	switch n {
	case 7, 14, 30, 90, 180, 365:
		return n
	default:
		return 7
	}
}

func queryMessage(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("message"))
}

func (s *Server) renderMedicationTemplate(w http.ResponseWriter, name string, v any) {
	if err := s.Templates.ExecuteTemplate(w, name, v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) medicationV2(w http.ResponseWriter, r *http.Request) {
	v := medicationPageView{
		Title:           "服药管理",
		ActivePartition: s.PM.ActiveID,
		CurrentMember:   currentMember(r),
		Permissions:     currentPermissions(r),
		MedicationDate:  strings.TrimSpace(r.URL.Query().Get("date")),
		RangeDays:       normalizeRangeDays(r.URL.Query().Get("range")),
		SelectedPatient: parseID(r.URL.Query().Get("member")),
		Message:         queryMessage(r),
	}
	if v.MedicationDate == "" {
		v.MedicationDate = medicationToday()
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
		v.Plans, err = s.Store.MedicationPlansForPatientDate(
			r.Context(),
			v.SelectedPatient,
			v.MedicationDate,
		)
		if err != nil {
			s.fail(w, r, err)
			return
		}
	}

	v.Summary, err = s.Store.MedicationSummaryRangeForPatient(
		r.Context(),
		v.MedicationDate,
		v.RangeDays,
		v.SelectedPatient,
	)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	s.renderMedicationTemplate(w, "medication-v2.html", v)
}

func (s *Server) medicationPlansFlat(w http.ResponseWriter, r *http.Request) {
	plans, err := s.Store.AllMedicationPlans(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}

	v := medicationPlansView{
		Title:           "全部服药计划",
		ActivePartition: s.PM.ActiveID,
		CurrentMember:   currentMember(r),
		Permissions:     currentPermissions(r),
		Plans:           plans,
		Today:           medicationToday(),
		Message:         queryMessage(r),
	}
	s.renderMedicationTemplate(w, "medication-plans.html", v)
}

func (s *Server) medicationPlanDetail(w http.ResponseWriter, r *http.Request) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		date = medicationToday()
	}

	plan, err := s.Store.MedicationPlanFullByID(
		r.Context(),
		parseID(r.PathValue("id")),
		date,
	)
	if err != nil || plan.IsDeleted {
		http.NotFound(w, r)
		return
	}

	member := currentMember(r)
	permissions := currentPermissions(r)
	v := medicationPlanDetailView{
		Title:           "服药计划详情",
		ActivePartition: s.PM.ActiveID,
		CurrentMember:   member,
		Permissions:     permissions,
		Plan:            plan,
		Date:            date,
		CanManage:       store.CanManageMedicationPlan(permissions, member.ID, plan.CreatedBy),
		Message:         queryMessage(r),
	}
	if v.CanManage {
		v.Members, _ = s.familyMembers(r.Context())
	}
	v.Deliveries, _ = s.Store.MedicationNotificationDeliveries(r.Context(), plan.ID, date)

	s.renderMedicationTemplate(w, "medication-plan-detail.html", v)
}

func (s *Server) requireMedicationPlanManage(
	w http.ResponseWriter,
	r *http.Request,
	planID int64,
) (store.MedicationPlanFull, bool) {
	plan, err := s.Store.MedicationPlanFullByID(r.Context(), planID, medicationToday())
	if err != nil || plan.IsDeleted {
		http.NotFound(w, r)
		return store.MedicationPlanFull{}, false
	}
	if !store.CanManageMedicationPlan(
		currentPermissions(r),
		currentMember(r).ID,
		plan.CreatedBy,
	) {
		http.Error(w, "你没有管理该服药计划的权限", http.StatusForbidden)
		return store.MedicationPlanFull{}, false
	}
	return plan, true
}
