package store

import (
	"testing"
	"time"
)

func TestMedicationMonthDayState(t *testing.T) {
	today := "2026-08-28"
	cases := []struct {
		name             string
		date             string
		scheduled, taken int
		want             string
	}{
		{"过去无计划", "2026-08-01", 0, 0, "none"},
		{"今天保持白色", today, 3, 0, "open"},
		{"今天全部完成仍保持白色", today, 3, 3, "open"},
		{"未来保持白色", "2026-08-29", 3, 0, "open"},
		{"过去全部完成", "2026-08-20", 3, 3, "complete"},
		{"过去部分完成", "2026-08-20", 3, 1, "partial"},
		{"过去全部未完成", "2026-08-20", 3, 0, "missed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := medicationMonthDayState(tc.date, today, tc.scheduled, tc.taken); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestMedicationPlanOccurrenceV3ManualEndBoundary(t *testing.T) {
	plan := MedicationPlanV3{
		MedicationPlanFull: MedicationPlanFull{
			StartDate:     "2026-08-01",
			EndDate:       "2026-08-28",
			ScheduledTime: "08:00",
		},
		Timezone: "Asia/Shanghai",
		EndedAt:  "2026-08-28T01:00:00Z", // UTC+8 09:00，08:00 的计划已经发生。
	}
	if _, ok := medicationPlanOccurrenceV3(plan, "2026-08-28"); !ok {
		t.Fatal("手动结束发生在计划时间之后，当天计划应计入月度安排")
	}
	plan.ScheduledTime = "10:00"
	if _, ok := medicationPlanOccurrenceV3(plan, "2026-08-28"); ok {
		t.Fatal("手动结束发生在计划时间之前，当天尚未发生的计划不应计入月度安排")
	}
}

func TestMedicationPlanOccurrenceV3CanConvertToDisplayTimezone(t *testing.T) {
	plan := MedicationPlanV3{
		MedicationPlanFull: MedicationPlanFull{
			StartDate:     "2026-08-01",
			ScheduledTime: "23:30",
		},
		Timezone: "Asia/Shanghai",
	}
	occurrence, ok := medicationPlanOccurrenceV3(plan, "2026-08-28")
	if !ok {
		t.Fatal("计划发生时间应可构造")
	}
	tokyo, _ := time.LoadLocation("Asia/Tokyo")
	if got := occurrence.In(tokyo).Format("2006-01-02"); got != "2026-08-29" {
		t.Fatalf("设备时区日期 got %s want 2026-08-29", got)
	}
}
