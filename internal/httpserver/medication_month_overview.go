package httpserver

import (
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) medicationMonthOverviewV3(w http.ResponseWriter, r *http.Request) {
	tz := requestTimezone(r)
	loc := timezoneLocation(tz)
	nowTime := time.Now().In(loc)
	monthStart := time.Date(nowTime.Year(), nowTime.Month(), 1, 0, 0, 0, 0, loc)

	overview, err := s.Store.MedicationMonthOverviewForPatientV3(
		r.Context(),
		parseID(r.URL.Query().Get("member")),
		monthStart.Format("2006-01-02"),
		nowTime.Format("2006-01-02"),
		tz,
	)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(overview)
}
