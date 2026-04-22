package ai

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

type Input struct {
	PostTitle string
	Content   string
	VisitorID string
}

type Decision struct {
	Status string
	Reason string
}

type RuntimeConfig struct {
	BaseURL      string
	APIKey       string
	Model        string
	SystemPrompt string
}

type Reviewer interface {
	ReviewComment(ctx context.Context, input Input) (Decision, error)
}

type OpenAICompatibleReviewer struct {
	mu     sync.RWMutex
	cfg    RuntimeConfig
	client *http.Client
}

func NewReviewer(cfg RuntimeConfig) *OpenAICompatibleReviewer {
	return &OpenAICompatibleReviewer{
		cfg: cfg,
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (r *OpenAICompatibleReviewer) Update(cfg RuntimeConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfg = cfg
}

func (r *OpenAICompatibleReviewer) ReviewComment(ctx context.Context, input Input) (Decision, error) {
	r.mu.RLock()
	cfg := r.cfg
	r.mu.RUnlock()

	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.Model) == "" {
		return Decision{
			Status: "pending",
			Reason: "llm-not-configured",
		}, nil
	}

	systemPrompt := strings.TrimSpace(cfg.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = "You are a blog moderation assistant."
	}

	requestBody := chatRequest{
		Model: cfg.Model,
		Messages: []chatMessage{
			{
				Role:    "system",
				Content: systemPrompt + "\nReturn strict JSON only: {\"status\":\"approved|rejected|pending\",\"reason\":\"short explanation\"}.",
			},
			{
				Role:    "user",
				Content: buildModerationPrompt(input),
			},
		},
		Temperature: 0.1,
		MaxTokens:   200,
		ResponseFormat: &responseFormat{
			Type: "json_object",
		},
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return Decision{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatCompletionsURL(cfg.BaseURL), bytes.NewReader(body))
	if err != nil {
		return Decision{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := r.client.Do(req)
	if err != nil {
		return Decision{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Decision{}, err
	}
	if resp.StatusCode >= 400 {
		return Decision{}, fmt.Errorf("llm request failed: %s", strings.TrimSpace(string(raw)))
	}

	var payload chatResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Decision{}, err
	}
	if len(payload.Choices) == 0 {
		return Decision{}, errors.New("llm returned no choices")
	}

	content := strings.TrimSpace(payload.Choices[0].Message.Content)
	if content == "" {
		return Decision{}, errors.New("llm returned empty content")
	}

	var decision Decision
	if err := json.Unmarshal([]byte(extractJSONObject(content)), &decision); err != nil {
		return Decision{}, fmt.Errorf("invalid moderation json: %w", err)
	}

	decision.Status = normalizeStatus(decision.Status)
	decision.Reason = strings.TrimSpace(decision.Reason)
	if decision.Reason == "" {
		decision.Reason = "llm-reviewed"
	}
	return decision, nil
}

func buildModerationPrompt(input Input) string {
	return fmt.Sprintf(`Review this blog comment for moderation.

Post title: %s
Visitor ID: %s
Comment:
%s

Return:
- "approved" when the comment is normal discussion and safe to show after human review
- "rejected" when it is spam, abusive, illegal, malicious, explicit advertising, or clearly unsafe
- "pending" when it is ambiguous and needs manual attention

Do not add markdown. JSON only.`, input.PostTitle, input.VisitorID, input.Content)
}

func normalizeStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "approved":
		return "approved"
	case "rejected":
		return "rejected"
	default:
		return "pending"
	}
}

func chatCompletionsURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(trimmed, "/chat/completions") {
		return trimmed
	}
	return trimmed + "/chat/completions"
}

func extractJSONObject(input string) string {
	start := strings.IndexByte(input, '{')
	end := strings.LastIndexByte(input, '}')
	if start >= 0 && end > start {
		return input[start : end+1]
	}
	return input
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Temperature    float64         `json:"temperature,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}
