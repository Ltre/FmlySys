package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

func medicationOverviewMonthStart(raw string, now time.Time, loc *time.Location) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc), nil
	}
	monthStart, err := time.ParseInLocation("2006-01", raw, loc)
	if err != nil || monthStart.Format("2006-01") != raw {
		return time.Time{}, errors.New("月份格式无效，应为 YYYY-MM")
	}
	return monthStart, nil
}

func (s *Server) medicationMonthOverviewV3(w http.ResponseWriter, r *http.Request) {
	tz := requestTimezone(r)
	loc := timezoneLocation(tz)
	nowTime := time.Now().In(loc)
	monthStart, err := medicationOverviewMonthStart(r.URL.Query().Get("month"), nowTime, loc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

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
