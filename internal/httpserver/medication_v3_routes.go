package httpserver

import "net/http"

func (s *Server) WithMedicationV3(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /medication", s.member("medication.view", s.medicationV3))
	mux.HandleFunc("GET /medication/plans", s.member("medication.view", s.medicationPlansFlatV3))
	mux.HandleFunc("GET /medication/plans/{id}", s.member("medication.view", s.medicationPlanDetailV3))
	mux.HandleFunc("POST /medication/plans", s.member("medication.view", s.createMedicationPlanV3))
	mux.HandleFunc("POST /medication/plans/{id}", s.member("medication.view", s.updateMedicationPlanV3))
	mux.HandleFunc("POST /medication/plans/{id}/end", s.member("medication.view", s.endMedicationPlanV3))
	mux.HandleFunc("POST /medication/plans/{id}/records", s.member("medication.view", s.saveMedicationIntakeV3))
	mux.HandleFunc("POST /medication/plans/{id}/verify", s.member("medication.view", s.verifyMedicationClaimV3))
	mux.HandleFunc("POST /medication/plans/{id}/notify", s.member("medication.view", s.manualMedicationReminderV3))
	mux.HandleFunc("GET /medication/checkin", s.member("", s.medicationCheckinV3))
	mux.HandleFunc("POST /medication/checkin", s.member("", s.medicationCheckinRespondV3))
	mux.Handle("/", next)
	return mux
}
