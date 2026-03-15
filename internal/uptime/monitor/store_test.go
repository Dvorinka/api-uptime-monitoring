package monitor

import (
	"testing"
	"time"
)

func TestCreateMonitorDefaults(t *testing.T) {
	store := NewStore()

	created, err := store.CreateMonitor(CreateMonitorInput{
		Name:   "Homepage",
		Type:   MonitorTypeHTTP,
		Target: "https://example.com",
	})
	if err != nil {
		t.Fatalf("create monitor: %v", err)
	}

	if created.IntervalSeconds != defaultIntervalSeconds {
		t.Fatalf("expected default interval %d, got %d", defaultIntervalSeconds, created.IntervalSeconds)
	}
	if created.TimeoutSeconds != defaultTimeoutSeconds {
		t.Fatalf("expected default timeout %d, got %d", defaultTimeoutSeconds, created.TimeoutSeconds)
	}
	if !created.CheckSSL {
		t.Fatalf("expected check_ssl to default to true for http monitors")
	}
}

func TestRecordCheckAndResultHistory(t *testing.T) {
	store := NewStore()
	created, err := store.CreateMonitor(CreateMonitorInput{
		Name:   "Homepage",
		Type:   MonitorTypeHTTP,
		Target: "https://example.com",
	})
	if err != nil {
		t.Fatalf("create monitor: %v", err)
	}

	first := CheckResult{
		Status:     StatusUp,
		CheckedAt:  time.Now().UTC(),
		ResponseMS: 120,
		HTTPStatus: 200,
	}
	_, changed, prev, err := store.RecordCheck(created.ID, first)
	if err != nil {
		t.Fatalf("record first result: %v", err)
	}
	if changed {
		t.Fatalf("first result should not count as status change from unknown")
	}
	if prev != StatusUnknown {
		t.Fatalf("expected previous status unknown, got %s", prev)
	}

	second := CheckResult{
		Status:     StatusDown,
		CheckedAt:  time.Now().UTC().Add(10 * time.Second),
		ResponseMS: 900,
		HTTPStatus: 500,
		Error:      "unexpected status code 500",
	}
	updated, changed, prev, err := store.RecordCheck(created.ID, second)
	if err != nil {
		t.Fatalf("record second result: %v", err)
	}
	if !changed {
		t.Fatalf("expected status transition to be detected")
	}
	if prev != StatusUp {
		t.Fatalf("expected previous status up, got %s", prev)
	}
	if updated.LastStatus != StatusDown {
		t.Fatalf("expected updated status down, got %s", updated.LastStatus)
	}

	results, err := store.GetResults(created.ID, 10)
	if err != nil {
		t.Fatalf("read results: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Status != StatusDown {
		t.Fatalf("latest result should be first")
	}
}

func TestSummary(t *testing.T) {
	store := NewStore()
	m1, err := store.CreateMonitor(CreateMonitorInput{
		Name:   "one",
		Type:   MonitorTypeHTTP,
		Target: "https://example.com",
	})
	if err != nil {
		t.Fatalf("create monitor m1: %v", err)
	}
	m2, err := store.CreateMonitor(CreateMonitorInput{
		Name:   "two",
		Type:   MonitorTypeHTTP,
		Target: "https://example.org",
	})
	if err != nil {
		t.Fatalf("create monitor m2: %v", err)
	}

	if _, _, _, err := store.RecordCheck(m1.ID, CheckResult{Status: StatusUp}); err != nil {
		t.Fatalf("record check m1: %v", err)
	}
	if _, _, _, err := store.RecordCheck(m2.ID, CheckResult{Status: StatusDown}); err != nil {
		t.Fatalf("record check m2: %v", err)
	}

	summary := store.Summary()
	if summary.TotalMonitors != 2 {
		t.Fatalf("expected 2 monitors, got %d", summary.TotalMonitors)
	}
	if summary.UpMonitors != 1 || summary.DownMonitors != 1 || summary.UnknownMonitors != 0 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}
