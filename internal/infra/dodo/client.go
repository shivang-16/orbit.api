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
}

type CheckoutSession struct {
	CheckoutURL string `json:"checkout_url"`
	SessionID   string `json:"session_id"`
}

func (c *Client) CreateCheckoutSession(ctx context.Context, params CreateCheckoutSessionParams) (*CheckoutSession, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("DODO_PAYMENTS_API_KEY is required")
	}

	body, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("encode checkout params: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/checkouts", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build checkout request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call dodo checkout: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read dodo response: %w", err)
	}

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("dodo checkout failed (%d): %s", resp.StatusCode, string(raw))
	}

	var session CheckoutSession
	if err := json.Unmarshal(raw, &session); err != nil {
		return nil, fmt.Errorf("decode dodo response: %w", err)
	}
	if session.CheckoutURL == "" {
		return nil, fmt.Errorf("dodo did not return a checkout url")
	}
	return &session, nil
}
