package monitor

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	defaultIntervalSeconds = 60
	defaultTimeoutSeconds  = 10
	maxSavedResults        = 200
)

var (
	ErrMonitorNotFound = errors.New("monitor not found")
)

type Store struct {
	mu       sync.RWMutex
	monitors map[string]Monitor
	results  map[string][]CheckResult
}

func NewStore() *Store {
	return &Store{
		monitors: make(map[string]Monitor),
		results:  make(map[string][]CheckResult),
	}
}

func (s *Store) CreateMonitor(input CreateMonitorInput) (Monitor, error) {
	if err := validateCreateMonitorInput(input); err != nil {
		return Monitor{}, err
	}

	now := time.Now().UTC()
	checkSSL := false
	if input.Type == MonitorTypeHTTP {
		checkSSL = true
	}
	if input.CheckSSL != nil {
		checkSSL = *input.CheckSSL
	}

	interval := input.IntervalSeconds
	if interval == 0 {
		interval = defaultIntervalSeconds
	}

	timeout := input.TimeoutSeconds
	if timeout == 0 {
		timeout = defaultTimeoutSeconds
	}

	monitor := Monitor{
		ID:              newID(),
		Name:            strings.TrimSpace(input.Name),
		Type:            input.Type,
		Target:          strings.TrimSpace(input.Target),
		IntervalSeconds: interval,
		TimeoutSeconds:  timeout,
		ExpectedStatus:  input.ExpectedStatus,
		WebhookURL:      strings.TrimSpace(input.WebhookURL),
		CheckSSL:        checkSSL,
		CreatedAt:       now,
		UpdatedAt:       now,
		LastStatus:      StatusUnknown,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.monitors[monitor.ID] = monitor
	s.results[monitor.ID] = make([]CheckResult, 0, 16)

	return monitor, nil
}

func (s *Store) GetMonitor(id string) (Monitor, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	monitor, ok := s.monitors[id]
	return monitor, ok
}

func (s *Store) ListMonitors() []Monitor {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Monitor, 0, len(s.monitors))
	for _, monitor := range s.monitors {
		out = append(out, monitor)
	}

	slices.SortFunc(out, func(a, b Monitor) int {
		return strings.Compare(a.ID, b.ID)
	})

	return out
}

func (s *Store) Summary() Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	summary := Summary{
		TotalMonitors: len(s.monitors),
	}

	for _, m := range s.monitors {
		switch m.LastStatus {
		case StatusUp:
			summary.UpMonitors++
		case StatusDown:
			summary.DownMonitors++
		default:
			summary.UnknownMonitors++
		}
	}

	return summary
}

func (s *Store) DeleteMonitor(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.monitors[id]; !ok {
		return false
	}

	delete(s.monitors, id)
	delete(s.results, id)
	return true
}

func (s *Store) RecordCheck(monitorID string, result CheckResult) (Monitor, bool, Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	monitor, ok := s.monitors[monitorID]
	if !ok {
		return Monitor{}, false, StatusUnknown, ErrMonitorNotFound
	}

	if result.ID == "" {
		result.ID = newID()
	}
	if result.CheckedAt.IsZero() {
		result.CheckedAt = time.Now().UTC()
	}

	result.MonitorID = monitorID
	history := append(s.results[monitorID], result)
	if len(history) > maxSavedResults {
		history = history[len(history)-maxSavedResults:]
	}
	s.results[monitorID] = history

	prevStatus := monitor.LastStatus
	monitor.LastCheckAt = result.CheckedAt
	monitor.LastStatus = result.Status
	monitor.LastError = result.Error
	monitor.LastResponseMS = result.ResponseMS
	monitor.LastHTTPStatus = result.HTTPStatus
	monitor.LastSSLExpires = result.SSLExpires
	monitor.UpdatedAt = time.Now().UTC()
	s.monitors[monitorID] = monitor

	changed := prevStatus != StatusUnknown && prevStatus != result.Status
	return monitor, changed, prevStatus, nil
}

func (s *Store) GetResults(monitorID string, limit int) ([]CheckResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	history, ok := s.results[monitorID]
	if !ok {
		return nil, ErrMonitorNotFound
	}

	if limit <= 0 || limit > maxSavedResults {
		limit = 50
	}
	if limit > len(history) {
		limit = len(history)
	}

	out := make([]CheckResult, 0, limit)
	for i := len(history) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, history[i])
	}
	return out, nil
}

func (s *Store) ListDueMonitors(now time.Time) []Monitor {
	s.mu.RLock()
	defer s.mu.RUnlock()

	due := make([]Monitor, 0)
	for _, monitor := range s.monitors {
		if monitor.LastCheckAt.IsZero() {
			due = append(due, monitor)
			continue
		}
		if now.Sub(monitor.LastCheckAt) >= time.Duration(monitor.IntervalSeconds)*time.Second {
			due = append(due, monitor)
		}
	}
	return due
}

func newID() string {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw)
}

func validateCreateMonitorInput(input CreateMonitorInput) error {
	if strings.TrimSpace(input.Name) == "" {
		return errors.New("name is required")
	}

	switch input.Type {
	case MonitorTypeHTTP, MonitorTypePing:
	default:
		return errors.New("type must be one of: http, ping")
	}

	target := strings.TrimSpace(input.Target)
	if target == "" {
		return errors.New("target is required")
	}

	switch input.Type {
	case MonitorTypeHTTP:
		parsed, err := url.Parse(target)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return errors.New("http target must be a valid absolute URL")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return errors.New("http target scheme must be http or https")
		}
	case MonitorTypePing:
		if strings.Contains(target, "://") {
			return errors.New("ping target must be host or host:port")
		}
		host, _, err := net.SplitHostPort(target)
		if err != nil {
			host = target
		}
		if strings.TrimSpace(host) == "" {
			return errors.New("ping target host is invalid")
		}
	}

	if input.IntervalSeconds < 0 || input.IntervalSeconds > 86400 {
		return errors.New("interval_seconds must be between 10 and 86400")
	}
	if input.IntervalSeconds > 0 && input.IntervalSeconds < 10 {
		return errors.New("interval_seconds must be at least 10")
	}

	if input.TimeoutSeconds < 0 || input.TimeoutSeconds > 120 {
		return errors.New("timeout_seconds must be between 1 and 120")
	}
	if input.TimeoutSeconds > 0 && input.TimeoutSeconds < 1 {
		return errors.New("timeout_seconds must be at least 1")
	}

	if input.WebhookURL != "" {
		u, err := url.Parse(input.WebhookURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return errors.New("webhook_url must be a valid URL")
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return errors.New("webhook_url scheme must be http or https")
		}
	}

	if input.ExpectedStatus < 0 || input.ExpectedStatus > 599 {
		return errors.New("expected_status must be in range 100-599 when set")
	}
	if input.ExpectedStatus > 0 && input.ExpectedStatus < 100 {
		return errors.New("expected_status must be in range 100-599 when set")
	}

	return nil
}
