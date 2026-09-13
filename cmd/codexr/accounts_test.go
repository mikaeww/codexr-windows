package main

import (
	"testing"

	"github.com/mikaeww/codexr-windows/internal/mux"
)

func TestShortColumnShowsFiveHourWindow(t *testing.T) {
	shortMinutes := int64(300)
	weeklyMinutes := int64(10_080)
	snapshot := mux.AccountSnapshot{RateLimits: &mux.RateLimits{
		Primary:   &mux.RateLimitWindow{UsedPercent: 18, WindowDurationMins: &shortMinutes},
		Secondary: &mux.RateLimitWindow{UsedPercent: 42, WindowDurationMins: &weeklyMinutes},
	}}

	if got := shortColumn(snapshot); got != "18% used" {
		t.Fatalf("shortColumn() = %q, want %q", got, "18% used")
	}
}
