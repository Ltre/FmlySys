package httpserver

import (
	"net/http"
	"strings"
)

func medicationAwarePermissions(perms []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(perms)+1)
	for _, p := range perms {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	if (seen["medication.manage_self"] || seen["medication.manage_others"]) && !seen["medication.view"] {
		out = append(out, "medication.view")
	}
	return out
}

func (s *Server) adminCreateMemberMedicationAware(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	if _, err := s.Store.CreateMemberWithPermissions(
		r.Context(),
		s.DevActorID,
		r.FormValue("name"),
		r.FormValue("relation"),
		medicationAwarePermissions(formPermissions(r)),
	); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/admin/authorities")
}

func (s *Server) adminSetPermissionsMedicationAware(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.SetMemberPermissionsAudited(
		r.Context(),
		s.DevActorID,
		parseID(r.PathValue("id")),
		medicationAwarePermissions(formPermissions(r)),
	); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/admin/authorities#member-"+r.PathValue("id"))
}

func (s *Server) adminApproveJoinMedicationAware(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	_, err := s.Store.ApproveJoinRequest(
		r.Context(),
		s.DevActorID,
		parseID(r.PathValue("id")),
		parseID(r.FormValue("member_id")),
		r.FormValue("new_name"),
		r.FormValue("new_relation"),
		medicationAwarePermissions(formPermissions(r)),
		currentAdmin(r).Username,
	)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/admin")
}

func (s *Server) WithMedicationEnhancements(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /admin/members", s.adminOnly(s.adminCreateMemberMedicationAware))
	mux.HandleFunc("POST /admin/members/{id}/permissions", s.adminOnly(s.adminSetPermissionsMedicationAware))
	mux.HandleFunc("POST /admin/join/{id}/approve", s.adminOnly(s.adminApproveJoinMedicationAware))
	mux.HandleFunc("GET /medication-sw.js", s.medicationServiceWorker)
	mux.HandleFunc("GET /medication", s.member("medication.view", s.medicationV2))
	mux.HandleFunc("GET /medication/plans", s.member("medication.view", s.medicationPlansFlat))
	mux.HandleFunc("GET /medication/plans/{id}", s.member("medication.view", s.medicationPlanDetail))
	mux.HandleFunc("POST /medication/plans", s.member("medication.view", s.createMedicationPlanV2))
	mux.HandleFunc("POST /medication/plans/{id}", s.member("medication.view", s.updateMedicationPlanV2))
	mux.HandleFunc("POST /medication/plans/{id}/delete", s.member("medication.view", s.deleteMedicationPlanV2))
	mux.HandleFunc("POST /medication/plans/{id}/end", s.member("medication.view", s.endMedicationPlanV2))
	mux.HandleFunc("POST /medication/plans/{id}/records", s.member("medication.view", s.saveMedicationIntakeV2))
	mux.HandleFunc("POST /medication/plans/{id}/verify", s.member("medication.view", s.verifyMedicationClaim))
	mux.HandleFunc("POST /medication/plans/{id}/notify", s.member("medication.view", s.manualMedicationReminder))
	mux.HandleFunc("GET /medication/checkin", s.member("", s.medicationCheckin))
	mux.HandleFunc("POST /medication/checkin", s.member("", s.medicationCheckinRespond))
	mux.HandleFunc("GET /medication/push/public-key", s.member("", s.medicationPushPublicKey))
	mux.HandleFunc("POST /medication/push/subscribe", s.member("", s.medicationPushSubscribe))
	mux.Handle("/", next)
	return mux
}
