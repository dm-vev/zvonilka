package translation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	domaintranslation "github.com/dm-vev/zvonilka/internal/domain/translation"
	"github.com/dm-vev/zvonilka/internal/platform/config"
)

// HTTPProvider adapts a JSON-over-HTTP translation backend.
type HTTPProvider struct {
	endpointURL  string
	apiKey       string
	providerName string
	maxTextBytes int
	client       *http.Client
}

type translateRequest struct {
	Text           string `json:"text"`
	SourceLanguage string `json:"source_language,omitempty"`
	TargetLanguage string `json:"target_language"`
}

type translateResponse struct {
	TranslatedText         string `json:"translated_text"`
	DetectedSourceLanguage string `json:"detected_source_language"`
	Provider               string `json:"provider"`
}

// NewHTTPProvider constructs the configured HTTP translation provider.
func NewHTTPProvider(cfg config.TranslationConfig) (*HTTPProvider, error) {
	endpointURL := strings.TrimSpace(cfg.EndpointURL)
	if endpointURL == "" || cfg.Timeout <= 0 || cfg.MaxTextBytes <= 0 {
		return nil, domaintranslation.ErrInvalidInput
	}

	providerName := strings.TrimSpace(cfg.ProviderName)
	if providerName == "" {
		providerName = "translation-http"
	}

	return &HTTPProvider{
		endpointURL:  endpointURL,
		apiKey:       strings.TrimSpace(cfg.APIKey),
		providerName: providerName,
		maxTextBytes: cfg.MaxTextBytes,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}, nil
}

// Translate calls the remote HTTP backend and normalizes the response.
func (p *HTTPProvider) Translate(
	ctx context.Context,
	request domaintranslation.ProviderRequest,
) (domaintranslation.ProviderResult, error) {
	if p == nil || p.client == nil {
		return domaintranslation.ProviderResult{}, domaintranslation.ErrInvalidInput
	}

	request.Text = strings.TrimSpace(request.Text)
	request.SourceLanguage = strings.TrimSpace(request.SourceLanguage)
	request.TargetLanguage = strings.TrimSpace(request.TargetLanguage)
	if request.Text == "" || request.TargetLanguage == "" {
		return domaintranslation.ProviderResult{}, domaintranslation.ErrInvalidInput
	}
	if len(request.Text) > p.maxTextBytes {
		return domaintranslation.ProviderResult{}, domaintranslation.ErrInvalidInput
	}

	payload, err := json.Marshal(translateRequest{
		Text:           request.Text,
		SourceLanguage: request.SourceLanguage,
		TargetLanguage: request.TargetLanguage,
	})
	if err != nil {
		return domaintranslation.ProviderResult{}, fmt.Errorf("marshal translation request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpointURL, bytes.NewReader(payload))
	if err != nil {
		return domaintranslation.ProviderResult{}, fmt.Errorf("build translation request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return domaintranslation.ProviderResult{}, fmt.Errorf("perform translation request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(p.maxTextBytes)+1024))
	if err != nil {
		return domaintranslation.ProviderResult{}, fmt.Errorf("read translation response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return domaintranslation.ProviderResult{}, fmt.Errorf("translation provider returned %s", resp.Status)
	}

	var decoded translateResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return domaintranslation.ProviderResult{}, fmt.Errorf("decode translation response: %w", err)
	}
	decoded.TranslatedText = strings.TrimSpace(decoded.TranslatedText)
	decoded.DetectedSourceLanguage = strings.TrimSpace(decoded.DetectedSourceLanguage)
	decoded.Provider = strings.TrimSpace(decoded.Provider)
	if decoded.TranslatedText == "" {
		return domaintranslation.ProviderResult{}, domaintranslation.ErrInvalidInput
	}
	if decoded.Provider == "" {
		decoded.Provider = p.providerName
	}

	return domaintranslation.ProviderResult{
		TranslatedText: decoded.TranslatedText,
		SourceLanguage: decoded.DetectedSourceLanguage,
		Provider:       decoded.Provider,
	}, nil
}

var _ domaintranslation.Provider = (*HTTPProvider)(nil)
