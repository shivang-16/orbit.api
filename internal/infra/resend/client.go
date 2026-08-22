// Package resend is a minimal REST client for Resend's emails API.
// See https://resend.com/docs/api-reference/emails/send-email
package resend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/shivang-16/orbit.api/internal/config"
)

const apiURL = "https://api.resend.com/emails"

type Client struct {
	httpClient *http.Client
	apiKey     string
	from       string
}

func New(cfg config.Config) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		apiKey:     cfg.Resend.APIKey,
		from:       cfg.Resend.FromEmail,
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.apiKey != "" && c.from != ""
}

func (c *Client) From() string {
	if c == nil {
		return ""
	}
	return c.from
}

type SendParams struct {
	To      string
	Subject string
	HTML    string
	Text    string
}

func (c *Client) Send(ctx context.Context, params SendParams) (string, error) {
	if !c.Enabled() {
		return "", fmt.Errorf("resend is not configured")
	}

	body, err := json.Marshal(map[string]any{
		"from":    c.from,
		"to":      []string{params.To},
		"subject": params.Subject,
		"html":    params.HTML,
		"text":    params.Text,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("resend %d: %s", res.StatusCode, bytes.TrimSpace(raw))
	}

	var parsed struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &parsed)
	return parsed.ID, nil
}
