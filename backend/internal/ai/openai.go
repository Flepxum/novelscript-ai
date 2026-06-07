package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	Temperature    float64       `json:"temperature,omitempty"`
	MaxTokens      int           `json:"max_tokens,omitempty"`
	ResponseFormat any           `json:"response_format,omitempty"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func (g *Generator) callChatCompletion(ctx context.Context, messages []chatMessage) (string, error) {
	if err := g.requireModelConfig(); err != nil {
		return "", err
	}

	endpoint, err := chatCompletionEndpoint(g.cfg.ModelBaseURL)
	if err != nil {
		return "", err
	}

	payload := chatCompletionRequest{
		Model:          strings.TrimSpace(g.cfg.ModelName),
		Messages:       messages,
		Temperature:    g.cfg.ModelTemperature,
		MaxTokens:      g.cfg.ModelMaxOutputTokens,
		ResponseFormat: g.responseFormat(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	attempts := g.cfg.ModelMaxRetries + 1
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		content, retryable, err := g.sendChatCompletionRequest(ctx, endpoint, body)
		if err == nil {
			return content, nil
		}
		lastErr = err
		if !retryable || attempt == attempts {
			break
		}
		wait := time.Duration(attempt) * 500 * time.Millisecond
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(wait):
		}
	}
	return "", lastErr
}

func (g *Generator) sendChatCompletionRequest(ctx context.Context, endpoint string, body []byte) (string, bool, error) {
	timeout := modelRequestTimeout(g.cfg.ModelTimeoutSeconds)
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", false, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(g.cfg.ModelAPIKey))
	if strings.TrimSpace(g.cfg.ModelOrgID) != "" {
		request.Header.Set("OpenAI-Organization", strings.TrimSpace(g.cfg.ModelOrgID))
	}
	if strings.TrimSpace(g.cfg.ModelProjectID) != "" {
		request.Header.Set("OpenAI-Project", strings.TrimSpace(g.cfg.ModelProjectID))
	}

	response, err := (&http.Client{Timeout: timeout}).Do(request)
	if err != nil {
		return "", true, fmt.Errorf("llm request failed: %w", err)
	}
	defer response.Body.Close()

	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if readErr != nil {
		return "", true, fmt.Errorf("read llm response failed: %w", readErr)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		retryable := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError
		return "", retryable, fmt.Errorf("llm request failed: status=%d detail=%s", response.StatusCode, compactLogDetail(string(responseBody), 500))
	}

	var decoded chatCompletionResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return "", false, fmt.Errorf("parse llm response failed: %w", err)
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return "", false, fmt.Errorf("llm response did not include message content")
	}
	return decoded.Choices[0].Message.Content, false, nil
}

func (g *Generator) requireModelConfig() error {
	missing := missingModelConfig(g.cfg)
	if len(missing) > 0 {
		return fmt.Errorf("llm config incomplete: set %s", strings.Join(missing, ", "))
	}
	return nil
}

func (g *Generator) responseFormat() any {
	mode := strings.ToLower(strings.TrimSpace(g.cfg.ModelStructureMode))
	switch mode {
	case "", "json_object":
		return map[string]string{"type": "json_object"}
	case "json_schema":
		schema, err := g.loadScriptSchema()
		if err != nil {
			return map[string]string{"type": "json_object"}
		}
		return map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "script_draft",
				"strict": false,
				"schema": schema,
			},
		}
	case "none", "text":
		return nil
	default:
		return map[string]string{"type": mode}
	}
}

func (g *Generator) loadScriptSchema() (map[string]any, error) {
	path := strings.TrimSpace(g.cfg.SchemaPath)
	if path == "" {
		return nil, fmt.Errorf("SCRIPT_SCHEMA_PATH is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, err
	}
	return schema, nil
}

func chatCompletionEndpoint(baseURL string) (string, error) {
	return openAIEndpoint(baseURL, "/chat/completions")
}

func modelRequestTimeout(configuredSeconds int) time.Duration {
	if configuredSeconds <= 0 {
		configuredSeconds = 120
	}
	return time.Duration(configuredSeconds) * time.Second
}
