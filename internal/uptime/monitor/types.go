package monitor

import "time"

type MonitorType string

const (
	MonitorTypeHTTP MonitorType = "http"
	MonitorTypePing MonitorType = "ping"
)

type Status string

const (
	StatusUnknown Status = "unknown"
	StatusUp      Status = "up"
	StatusDown    Status = "down"
)

type Monitor struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	Type            MonitorType `json:"type"`
	Target          string      `json:"target"`
	IntervalSeconds int         `json:"interval_seconds"`
	TimeoutSeconds  int         `json:"timeout_seconds"`
	ExpectedStatus  int         `json:"expected_status,omitempty"`
	WebhookURL      string      `json:"webhook_url,omitempty"`
	CheckSSL        bool        `json:"check_ssl"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
	LastCheckAt     time.Time   `json:"last_check_at,omitempty"`
	LastStatus      Status      `json:"last_status"`
	LastError       string      `json:"last_error,omitempty"`
	LastResponseMS  int64       `json:"last_response_ms,omitempty"`
	LastHTTPStatus  int         `json:"last_http_status,omitempty"`
	LastSSLExpires  *time.Time  `json:"last_ssl_expires,omitempty"`
}

type CreateMonitorInput struct {
	Name            string
	Type            MonitorType
	Target          string
	IntervalSeconds int
	TimeoutSeconds  int
	ExpectedStatus  int
	WebhookURL      string
	CheckSSL        *bool
}

type CheckResult struct {
	ID         string     `json:"id"`
	MonitorID  string     `json:"monitor_id"`
	Status     Status     `json:"status"`
	HTTPStatus int        `json:"http_status,omitempty"`
	ResponseMS int64      `json:"response_ms"`
	CheckedAt  time.Time  `json:"checked_at"`
	Error      string     `json:"error,omitempty"`
	SSLExpires *time.Time `json:"ssl_expires,omitempty"`
}

type Summary struct {
	TotalMonitors   int `json:"total_monitors"`
	UnknownMonitors int `json:"unknown_monitors"`
	UpMonitors      int `json:"up_monitors"`
	DownMonitors    int `json:"down_monitors"`
}
