package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type RuntimeConfig struct {
	Enabled    bool
	BaseURL    string
	APIKey     string
	Model      string
	Dimensions int
	Timeout    time.Duration
}

type OpenAICompatibleEmbedder struct {
	mu     sync.RWMutex
	cfg    RuntimeConfig
	client *http.Client
}

func New(cfg RuntimeConfig) *OpenAICompatibleEmbedder {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &OpenAICompatibleEmbedder{
		cfg: cfg,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (e *OpenAICompatibleEmbedder) Update(cfg RuntimeConfig) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cfg = cfg
	e.client = &http.Client{Timeout: timeout}
}

func (e *OpenAICompatibleEmbedder) Config() RuntimeConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cfg
}

func (e *OpenAICompatibleEmbedder) Ready() bool {
	cfg := e.Config()
	return cfg.Enabled &&
		strings.TrimSpace(cfg.BaseURL) != "" &&
		strings.TrimSpace(cfg.APIKey) != "" &&
		strings.TrimSpace(cfg.Model) != ""
}

func (e *OpenAICompatibleEmbedder) EmbedText(ctx context.Context, input string) ([]float32, error) {
	cfg := e.Config()
	if !cfg.Enabled {
		return nil, errors.New("semantic search disabled")
	}
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("embedding not configured")
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, errors.New("embedding input is empty")
	}

	requestBody := embeddingRequest{
		Model: cfg.Model,
		Input: input,
	}
	if cfg.Dimensions > 0 {
		requestBody.Dimensions = cfg.Dimensions
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, embeddingsURL(cfg.BaseURL), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("embedding request failed: %s", strings.TrimSpace(string(raw)))
	}

	var payload embeddingResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if len(payload.Data) == 0 {
		return nil, errors.New("embedding response contained no data")
	}
	embedding := payload.Data[0].Embedding
	if len(embedding) == 0 {
		return nil, errors.New("embedding response contained empty vector")
	}
	if cfg.Dimensions > 0 && len(embedding) != cfg.Dimensions {
		return nil, fmt.Errorf("embedding dimension mismatch: got %d want %d", len(embedding), cfg.Dimensions)
	}
	return embedding, nil
}

func embeddingsURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(trimmed, "/embeddings") {
		return trimmed
	}
	return trimmed + "/embeddings"
}

type embeddingRequest struct {
	Model      string `json:"model"`
	Input      string `json:"input"`
	Dimensions int    `json:"dimensions,omitempty"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}
