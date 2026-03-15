package monitor

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Checker struct{}

func NewChecker() *Checker {
	return &Checker{}
}

func (c *Checker) RunCheck(ctx context.Context, monitor Monitor) CheckResult {
	timeout := time.Duration(monitor.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultTimeoutSeconds * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch monitor.Type {
	case MonitorTypeHTTP:
		return c.runHTTPCheck(ctx, timeout, monitor)
	case MonitorTypePing:
		return c.runPingCheck(ctx, timeout, monitor)
	default:
		return CheckResult{
			Status:    StatusDown,
			CheckedAt: time.Now().UTC(),
			Error:     "unsupported monitor type",
		}
	}
}

func (c *Checker) runHTTPCheck(ctx context.Context, timeout time.Duration, monitor Monitor) CheckResult {
	result := CheckResult{
		Status:    StatusDown,
		CheckedAt: time.Now().UTC(),
	}

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, monitor.Target, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	result.ResponseMS = time.Since(start).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	result.HTTPStatus = resp.StatusCode
	if monitor.ExpectedStatus > 0 {
		if resp.StatusCode == monitor.ExpectedStatus {
			result.Status = StatusUp
		} else {
			result.Error = fmt.Sprintf("expected %d, got %d", monitor.ExpectedStatus, resp.StatusCode)
		}
	} else if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		result.Status = StatusUp
	} else {
		result.Error = fmt.Sprintf("unexpected status code %d", resp.StatusCode)
	}

	if monitor.CheckSSL && strings.HasPrefix(strings.ToLower(monitor.Target), "https://") {
		sslExpiry, sslErr := fetchSSLExpiry(ctx, timeout, monitor.Target)
		if sslErr != nil {
			result.Status = StatusDown
			result.Error = appendError(result.Error, "ssl check failed: "+sslErr.Error())
			return result
		}
		result.SSLExpires = &sslExpiry
		if sslExpiry.Before(time.Now().UTC()) {
			result.Status = StatusDown
			result.Error = appendError(result.Error, "ssl certificate has expired")
		}
	}

	return result
}

func (c *Checker) runPingCheck(ctx context.Context, timeout time.Duration, monitor Monitor) CheckResult {
	result := CheckResult{
		Status:    StatusDown,
		CheckedAt: time.Now().UTC(),
	}

	target, err := normalizePingTarget(monitor.Target)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	start := time.Now()
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", target)
	result.ResponseMS = time.Since(start).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	_ = conn.Close()

	result.Status = StatusUp
	return result
}

func fetchSSLExpiry(ctx context.Context, timeout time.Duration, target string) (time.Time, error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return time.Time{}, err
	}
	host := parsed.Host
	if host == "" {
		return time.Time{}, errors.New("target host is empty")
	}
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "443")
	}

	dialer := &net.Dialer{Timeout: timeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", host, &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: parsed.Hostname(),
	})
	if err != nil {
		return time.Time{}, err
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return time.Time{}, errors.New("no peer certificates")
	}

	select {
	case <-ctx.Done():
		return time.Time{}, ctx.Err()
	default:
	}

	return certs[0].NotAfter.UTC(), nil
}

func normalizePingTarget(target string) (string, error) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return "", errors.New("ping target is empty")
	}
	if strings.Contains(trimmed, "://") {
		return "", errors.New("ping target must be host or host:port")
	}

	if _, _, err := net.SplitHostPort(trimmed); err == nil {
		return trimmed, nil
	}
	return net.JoinHostPort(trimmed, "443"), nil
}

func appendError(current, next string) string {
	if current == "" {
		return next
	}
	return current + "; " + next
}
