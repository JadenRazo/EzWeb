package health

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type WebhookSender struct {
	URL    string
	Format string
	Client *http.Client
}

func NewWebhookSender(url, format string) *WebhookSender {
	return &WebhookSender{
		URL:    url,
		Format: format,
		Client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (ws *WebhookSender) SendAlert(domain string, failures int, lastError string) error {
	if ws.URL == "" {
		return nil
	}

	var payload []byte
	var err error

	switch ws.Format {
	case "slack":
		payload, err = json.Marshal(map[string]string{
			"text": fmt.Sprintf("*%s* is DOWN — %d consecutive failures\nLast error: %s", domain, failures, lastError),
		})
	default:
		payload, err = json.Marshal(map[string]interface{}{
			"embeds": []map[string]interface{}{
				{
					"title":       fmt.Sprintf("Site Down: %s", domain),
					"description": fmt.Sprintf("%d consecutive health check failures\n\nLast error: %s", failures, lastError),
					"color":       16711680,
					"timestamp":   time.Now().UTC().Format(time.RFC3339),
				},
			},
		})
	}
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	resp, err := ws.Client.Post(ws.URL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func (ws *WebhookSender) SendRecovery(domain string) error {
	if ws.URL == "" {
		return nil
	}

	var payload []byte
	var err error

	switch ws.Format {
	case "slack":
		payload, err = json.Marshal(map[string]string{
			"text": fmt.Sprintf("*%s* is back UP", domain),
		})
	default:
		payload, err = json.Marshal(map[string]interface{}{
			"embeds": []map[string]interface{}{
				{
					"title":       fmt.Sprintf("Site Recovered: %s", domain),
					"description": "Site is responding normally again.",
					"color":       65280,
					"timestamp":   time.Now().UTC().Format(time.RFC3339),
				},
			},
		})
	}
	if err != nil {
		return err
	}

	resp, err := ws.Client.Post(ws.URL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
