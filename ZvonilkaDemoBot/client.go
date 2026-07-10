package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type botAPIClient struct {
	baseURL string
	token   string
	http    *http.Client
}

type botAPIResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
}

func newBotAPIClient(baseURL, token string) *botAPIClient {
	return &botAPIClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token),
		http: &http.Client{
			Timeout: 45 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        16,
				MaxIdleConnsPerHost: 8,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (c *botAPIClient) call(ctx context.Context, method string, params map[string]any, result any) error {
	if c == nil || c.http == nil || c.baseURL == "" || c.token == "" {
		return fmt.Errorf("bot api client is not configured")
	}

	payload, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encode %s request: %w", method, err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/bot"+c.token+"/"+method,
		bytes.NewReader(payload),
	)
	if err != nil {
		return fmt.Errorf("create %s request: %w", method, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("call %s: %w", method, err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("read %s response: %w", method, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("call %s: http %s: %s", method, response.Status, strings.TrimSpace(string(body)))
	}

	var envelope botAPIResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode %s response: %w", method, err)
	}
	if !envelope.OK {
		if envelope.ErrorCode != 0 {
			return fmt.Errorf("call %s: api %d: %s", method, envelope.ErrorCode, envelope.Description)
		}
		return fmt.Errorf("call %s: api error: %s", method, envelope.Description)
	}
	if result == nil || len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, result); err != nil {
		return fmt.Errorf("decode %s result: %w", method, err)
	}

	return nil
}

func (c *botAPIClient) callRaw(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	var result json.RawMessage
	if err := c.call(ctx, method, params, &result); err != nil {
		return nil, err
	}
	return result, nil
}
