package httpserver

import (
	"testing"
	"time"
)

func TestMedicationOverviewMonthStart(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, loc)

	got, err := medicationOverviewMonthStart("", now, loc)
	if err != nil || got.Format("2006-01-02") != "2026-08-01" {
		t.Fatalf("default month: got=%v err=%v", got, err)
	}
	got, err = medicationOverviewMonthStart("2026-07", now, loc)
	if err != nil || got.Format("2006-01-02") != "2026-07-01" {
		t.Fatalf("selected month: got=%v err=%v", got, err)
	}
	for _, raw := range []string{"2026-7", "2026-13", "abc", "2026-07-01"} {
		if _, err := medicationOverviewMonthStart(raw, now, loc); err == nil {
			t.Fatalf("expected invalid month %q", raw)
		}
	}
}
