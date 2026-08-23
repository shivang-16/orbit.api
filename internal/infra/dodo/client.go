// Package dodo is a minimal REST client for Dodo Payments — just enough to
// create a hosted checkout session. See https://docs.dodopayments.com.
package dodo

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

type Client struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string
}

func New(cfg config.Config) *Client {
	baseURL := "https://test.dodopayments.com"
	if cfg.Dodo.Env == "live" {
		baseURL = "https://live.dodopayments.com"
	}
	return &Client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		apiKey:     cfg.Dodo.APIKey,
		baseURL:    baseURL,
	}
}

type CartItem struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

type Customer struct {
	Email string `json:"email,omitempty"`
}

type CreateCheckoutSessionParams struct {
	ProductCart []CartItem        `json:"product_cart"`
	ReturnURL   string            `json:"return_url,omitempty"`
	Customer    *Customer         `json:"customer,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	// MandateMinAmountInrPaise overrides Dodo's ₹15,000 INR e-mandate floor
	// so Indian-card renewals authorize near the actual plan charge.
	MandateMinAmountInrPaise *int `json:"mandate_min_amount_inr_paise,omitempty"`
}

type CheckoutSession struct {
	CheckoutURL string `json:"checkout_url"`
	SessionID   string `json:"session_id"`
}

func (c *Client) CreateCheckoutSession(ctx context.Context, params CreateCheckoutSessionParams) (*CheckoutSession, error) {
	var session CheckoutSession
	if err := c.doJSON(ctx, http.MethodPost, "/checkouts", params, &session); err != nil {
		return nil, err
	}
	if session.CheckoutURL == "" {
		return nil, fmt.Errorf("dodo did not return a checkout url")
	}
	return &session, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	raw, _, err := c.do(ctx, method, path, body, "application/json")
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode dodo response: %w", err)
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, accept string) ([]byte, string, error) {
	if c.apiKey == "" {
		return nil, "", fmt.Errorf("DODO_PAYMENTS_API_KEY is required")
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, "", fmt.Errorf("encode dodo request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, "", fmt.Errorf("build dodo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("call dodo: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read dodo response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("dodo request failed (%d): %s", resp.StatusCode, string(raw))
	}
	return raw, resp.Header.Get("Content-Type"), nil
}
