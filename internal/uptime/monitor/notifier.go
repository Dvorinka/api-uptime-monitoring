package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type WebhookNotifier struct {
	client *http.Client
}

func NewWebhookNotifier(timeout time.Duration) *WebhookNotifier {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &WebhookNotifier{
		client: &http.Client{Timeout: timeout},
	}
}

type WebhookPayload struct {
	MonitorID      string    `json:"monitor_id"`
	MonitorName    string    `json:"monitor_name"`
	Target         string    `json:"target"`
	PreviousStatus Status    `json:"previous_status"`
	CurrentStatus  Status    `json:"current_status"`
	CheckedAt      time.Time `json:"checked_at"`
	HTTPStatus     int       `json:"http_status,omitempty"`
	ResponseMS     int64     `json:"response_ms,omitempty"`
	Error          string    `json:"error,omitempty"`
}

func (n *WebhookNotifier) Notify(ctx context.Context, endpoint string, payload WebhookPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(data))
	}

	return nil
}
