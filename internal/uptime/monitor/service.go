package monitor

import (
	"context"
	"log"
	"sync"
	"time"
)

type Service struct {
	store        *Store
	checker      *Checker
	notifier     *WebhookNotifier
	pollInterval time.Duration
	logger       *log.Logger
}

func NewService(store *Store, checker *Checker, notifier *WebhookNotifier, pollInterval time.Duration, logger *log.Logger) *Service {
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	return &Service{
		store:        store,
		checker:      checker,
		notifier:     notifier,
		pollInterval: pollInterval,
		logger:       logger,
	}
}

func (s *Service) Start(ctx context.Context) {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.RunDueChecks(ctx)
		}
	}
}

func (s *Service) RunDueChecks(ctx context.Context) int {
	due := s.store.ListDueMonitors(time.Now().UTC())
	return s.runChecks(ctx, due)
}

func (s *Service) RunAllChecks(ctx context.Context) int {
	monitors := s.store.ListMonitors()
	return s.runChecks(ctx, monitors)
}

func (s *Service) RunCheckForMonitor(ctx context.Context, monitorID string) (CheckResult, error) {
	monitor, ok := s.store.GetMonitor(monitorID)
	if !ok {
		return CheckResult{}, ErrMonitorNotFound
	}
	result := s.checker.RunCheck(ctx, monitor)
	result.MonitorID = monitorID
	if err := s.recordResultAndNotify(ctx, monitor, result); err != nil {
		return CheckResult{}, err
	}
	return result, nil
}

func (s *Service) runChecks(ctx context.Context, monitors []Monitor) int {
	if len(monitors) == 0 {
		return 0
	}

	var wg sync.WaitGroup
	for _, monitor := range monitors {
		monitor := monitor
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := s.checker.RunCheck(ctx, monitor)
			if err := s.recordResultAndNotify(ctx, monitor, result); err != nil && s.logger != nil {
				s.logger.Printf("check failure monitor=%s err=%v", monitor.ID, err)
			}
		}()
	}
	wg.Wait()
	return len(monitors)
}

func (s *Service) recordResultAndNotify(ctx context.Context, monitor Monitor, result CheckResult) error {
	updated, changed, prev, err := s.store.RecordCheck(monitor.ID, result)
	if err != nil {
		return err
	}

	if !changed || updated.WebhookURL == "" || s.notifier == nil {
		return nil
	}

	notifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	payload := WebhookPayload{
		MonitorID:      monitor.ID,
		MonitorName:    monitor.Name,
		Target:         monitor.Target,
		PreviousStatus: prev,
		CurrentStatus:  result.Status,
		CheckedAt:      result.CheckedAt,
		HTTPStatus:     result.HTTPStatus,
		ResponseMS:     result.ResponseMS,
		Error:          result.Error,
	}
	if err := s.notifier.Notify(notifyCtx, updated.WebhookURL, payload); err != nil && s.logger != nil {
		s.logger.Printf("webhook failure monitor=%s err=%v", monitor.ID, err)
	}

	return nil
}
