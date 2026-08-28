package store

import "testing"

func TestMedicationReminderRequiresBothChannels(t *testing.T) {
	cases := []struct {
		stage string
		want  bool
	}{
		{stage: "manual", want: true},
		{stage: "plus2h", want: true},
		{stage: "scheduled", want: false},
		{stage: "plus1h", want: false},
	}
	for _, tc := range cases {
		if got := MedicationReminderRequiresBothChannels(tc.stage); got != tc.want {
			t.Fatalf("stage %s: got %v want %v", tc.stage, got, tc.want)
		}
	}
}
