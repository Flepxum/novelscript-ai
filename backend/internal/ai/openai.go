package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
		Message      chatCompletionMessage `json:"message"`
		Delta        chatCompletionMessage `json:"delta"`
		FinishReason string                `json:"finish_reason"`
	} `json:"choices"`
}

type chatCompletionMessage struct {
	Role             string `json:"role"`
	Content          any    `json:"content"`
	Refusal          string `json:"refusal"`
	ReasoningContent string `json:"reasoning_content"`
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
		content, retryable, err := g.sendChatCompletionRequest(ctx, endpoint, body, attempt, attempts, messages)
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

func (g *Generator) sendChatCompletionRequest(ctx context.Context, endpoint string, body []byte, attempt int, attempts int, messages []chatMessage) (string, bool, error) {
	timeout := modelRequestTimeout(g.cfg.ModelTimeoutSeconds)
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	startedAt := time.Now()
	log.Printf(
		"LLM request started: endpoint=%s model=%s attempt=%d/%d messages=%d prompt_chars=%d max_tokens=%d temperature=%.3g response_format=%s timeout=%s",
		endpoint,
		strings.TrimSpace(g.cfg.ModelName),
		attempt,
		attempts,
		len(messages),
		messageCharCount(messages),
		g.cfg.ModelMaxOutputTokens,
		g.cfg.ModelTemperature,
		responseFormatName(g.cfg.ModelStructureMode),
		timeout,
	)

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
	latency := time.Since(startedAt).Round(time.Millisecond)
	if err != nil {
		log.Printf("LLM request transport failed: endpoint=%s model=%s attempt=%d/%d latency=%s error=%s", endpoint, strings.TrimSpace(g.cfg.ModelName), attempt, attempts, latency, err)
		return "", true, fmt.Errorf("llm request failed: %w", err)
	}
	defer response.Body.Close()

	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if readErr != nil {
		log.Printf("LLM response read failed: endpoint=%s model=%s status=%d latency=%s error=%s", endpoint, strings.TrimSpace(g.cfg.ModelName), response.StatusCode, latency, readErr)
		return "", true, fmt.Errorf("read llm response failed: %w", readErr)
	}
	log.Printf("LLM response received: endpoint=%s model=%s status=%d latency=%s bytes=%d", endpoint, strings.TrimSpace(g.cfg.ModelName), response.StatusCode, latency, len(responseBody))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		retryable := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError
		log.Printf("LLM request failed: endpoint=%s model=%s status=%d retryable=%t detail=%s", endpoint, strings.TrimSpace(g.cfg.ModelName), response.StatusCode, retryable, compactLogDetail(string(responseBody), 800))
		return "", retryable, fmt.Errorf("llm request failed: status=%d detail=%s", response.StatusCode, compactLogDetail(string(responseBody), 500))
	}

	var decoded chatCompletionResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		log.Printf("LLM response parse failed: endpoint=%s model=%s status=%d latency=%s body=%s", endpoint, strings.TrimSpace(g.cfg.ModelName), response.StatusCode, latency, compactLogDetail(string(responseBody), 1200))
		return "", false, fmt.Errorf("parse llm response failed: %w", err)
	}
	if len(decoded.Choices) == 0 {
		log.Printf("LLM response missing choices: endpoint=%s model=%s status=%d latency=%s body=%s", endpoint, strings.TrimSpace(g.cfg.ModelName), response.StatusCode, latency, compactLogDetail(string(responseBody), 1200))
		return "", false, fmt.Errorf("llm response did not include choices; raw response summary was logged on backend")
	}
	content := extractChatContent(decoded.Choices[0].Message)
	if content == "" {
		content = extractChatContent(decoded.Choices[0].Delta)
	}
	if content == "" {
		finishReason := strings.TrimSpace(decoded.Choices[0].FinishReason)
		refusal := strings.TrimSpace(decoded.Choices[0].Message.Refusal)
		log.Printf(
			"LLM response missing message content: endpoint=%s model=%s status=%d latency=%s finish_reason=%s refusal=%s body=%s",
			endpoint,
			strings.TrimSpace(g.cfg.ModelName),
			response.StatusCode,
			latency,
			valueOrUnknown(finishReason),
			valueOrUnknown(refusal),
			compactLogDetail(string(responseBody), 1600),
		)
		if refusal != "" {
			return "", false, fmt.Errorf("llm response refusal: %s", refusal)
		}
		return "", false, fmt.Errorf("llm response did not include message content (finish_reason=%s); raw response summary was logged on backend", valueOrUnknown(finishReason))
	}
	log.Printf("LLM response content extracted: endpoint=%s model=%s chars=%d finish_reason=%s", endpoint, strings.TrimSpace(g.cfg.ModelName), len([]rune(content)), valueOrUnknown(decoded.Choices[0].FinishReason))
	return content, false, nil
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

func extractChatContent(message chatCompletionMessage) string {
	switch content := message.Content.(type) {
	case string:
		return strings.TrimSpace(content)
	case []any:
		var parts []string
		for _, part := range content {
			text := extractContentPartText(part)
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	case map[string]any:
		return extractContentPartText(content)
	default:
		return ""
	}
}

func extractContentPartText(part any) string {
	switch typed := part.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, key := range []string{"text", "content"} {
			if value, ok := typed[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
		if nested, ok := typed["text"].(map[string]any); ok {
			if value, ok := nested["value"].(string); ok {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func messageCharCount(messages []chatMessage) int {
	total := 0
	for _, message := range messages {
		total += len([]rune(message.Content))
	}
	return total
}

func responseFormatName(mode string) string {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return "json_object"
	}
	return mode
}

func valueOrUnknown(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}
