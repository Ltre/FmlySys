package store

import (
	"testing"
	"time"
)

func TestMedicationAutomaticRetryIntervalV4(t *testing.T) {
	last := time.Date(2026, 8, 28, 6, 0, 0, 0, time.UTC)
	if medicationAutomaticRetryAllowedSinceV4(last, last.Add(19*time.Minute+59*time.Second)) {
		t.Fatal("19分59秒时不应允许自动重试")
	}
	if !medicationAutomaticRetryAllowedSinceV4(last, last.Add(20*time.Minute)) {
		t.Fatal("满20分钟时应该允许自动重试")
	}
}
