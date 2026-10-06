package starlings

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestGuardReportsManualVsAutomatic(t *testing.T) {
	g := NewGuard()
	g.Observe("lookup", GuardManual, 30*time.Microsecond, nil)
	g.Observe("lookup", GuardAutomatic, 10*time.Microsecond, nil)
	g.Observe("lookup", GuardAutomatic, 12*time.Microsecond, nil)
	report := g.Report()
	if len(report.Metrics) != 2 || len(report.Advice) != 1 {
		t.Fatalf("report = %+v", report)
	}
	if !strings.Contains(report.Advice[0].Recommendation, "automatic is faster") {
		t.Fatalf("advice = %q", report.Advice[0].Recommendation)
	}
	if !strings.Contains(report.String(), "Starlings Guard") {
		t.Fatal("formatted report lacks heading")
	}
}

func TestGuardCompareDetectsMismatchAndKeepsManualResult(t *testing.T) {
	g := NewGuard()
	result, err := GuardCompare(g, "pure", func(a, b int) bool { return a == b },
		func() (int, error) { return 1, nil },
		func() (int, error) { return 2, nil })
	if err != nil || result != 1 {
		t.Fatalf("result=%d err=%v", result, err)
	}
	report := g.Report()
	if len(report.Advice) != 1 || report.Advice[0].Mismatches != 1 {
		t.Fatalf("comparison report = %+v", report)
	}

	want := errors.New("manual failed")
	err = g.Measure("side-effect", GuardManual, func() error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("Measure error = %v", err)
	}
	for _, metric := range g.Report().Metrics {
		if metric.Feature == "side-effect" && metric.Errors != 1 {
			t.Fatalf("errors = %d", metric.Errors)
		}
	}
}

func TestGuardBoundsFeatureLabels(t *testing.T) {
	g := NewGuard(WithGuardMaxFeatures(1))
	g.Observe("one", GuardManual, time.Microsecond, nil)
	g.Observe("two", GuardManual, time.Microsecond, nil)
	report := g.Report()
	if report.DroppedSamples != 1 {
		t.Fatalf("dropped samples = %d", report.DroppedSamples)
	}
	if len(report.Metrics) != 2 {
		t.Fatalf("bounded metrics = %+v", report.Metrics)
	}
}
