package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Flepxum/novelscript-ai/backend/internal/config"
)

type LLMConnectionCheck struct {
	Configured   bool
	OK           bool
	Endpoint     string
	Missing      []string
	StatusCode   int
	ModelChecked bool
	ModelFound   bool
	Duration     time.Duration
	Message      string
}

func CheckLLMConnection(ctx context.Context, cfg config.Config) LLMConnectionCheck {
	missing := missingModelConfig(cfg)
	if len(missing) > 0 {
		return LLMConnectionCheck{
			Configured: false,
			Missing:    missing,
			Message:    "required model configuration is incomplete",
		}
	}

	endpoint, err := modelListEndpoint(cfg.ModelBaseURL)
	if err != nil {
		return LLMConnectionCheck{
			Configured: true,
			Message:    err.Error(),
		}
	}

	timeout := startupCheckTimeout(cfg.ModelTimeoutSeconds)
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(checkCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return LLMConnectionCheck{
			Configured: true,
			Endpoint:   endpoint,
			Message:    err.Error(),
		}
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.ModelAPIKey))
	if strings.TrimSpace(cfg.ModelOrgID) != "" {
		request.Header.Set("OpenAI-Organization", strings.TrimSpace(cfg.ModelOrgID))
	}
	if strings.TrimSpace(cfg.ModelProjectID) != "" {
		request.Header.Set("OpenAI-Project", strings.TrimSpace(cfg.ModelProjectID))
	}

	startedAt := time.Now()
	response, err := (&http.Client{Timeout: timeout}).Do(request)
	duration := time.Since(startedAt)
	if err != nil {
		return LLMConnectionCheck{
			Configured: true,
			Endpoint:   endpoint,
			Duration:   duration,
			Message:    err.Error(),
		}
	}
	defer response.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(response.Body, 16*1024))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message := response.Status
		if readErr == nil && len(body) > 0 {
			message = compactLogDetail(string(body), 320)
		}
		return LLMConnectionCheck{
			Configured: true,
			Endpoint:   endpoint,
			StatusCode: response.StatusCode,
			Duration:   duration,
			Message:    message,
		}
	}

	modelChecked, modelFound := responseContainsModel(body, cfg.ModelName)
	message := "provider reachable"
	if readErr != nil {
		message = "provider reachable, but response body could not be read"
	}
	return LLMConnectionCheck{
		Configured:   true,
		OK:           true,
		Endpoint:     endpoint,
		StatusCode:   response.StatusCode,
		ModelChecked: modelChecked,
		ModelFound:   modelFound,
		Duration:     duration,
		Message:      message,
	}
}

func missingModelConfig(cfg config.Config) []string {
	required := []struct {
		key   string
		value string
	}{
		{key: "MODEL_BASE_URL", value: cfg.ModelBaseURL},
		{key: "MODEL_API_KEY", value: cfg.ModelAPIKey},
		{key: "MODEL_NAME", value: cfg.ModelName},
	}
	missing := make([]string, 0, len(required))
	for _, item := range required {
		if strings.TrimSpace(item.value) == "" {
			missing = append(missing, item.key)
		}
	}
	return missing
}

func modelListEndpoint(baseURL string) (string, error) {
	trimmed := strings.TrimSpace(baseURL)
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid MODEL_BASE_URL: expected an absolute OpenAI-compatible base URL")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(path, "/models") {
		path += "/models"
	}
	parsed.Path = path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func startupCheckTimeout(configuredSeconds int) time.Duration {
	if configuredSeconds <= 0 || configuredSeconds > 10 {
		return 10 * time.Second
	}
	return time.Duration(configuredSeconds) * time.Second
}

func responseContainsModel(body []byte, modelName string) (bool, bool) {
	modelName = strings.TrimSpace(modelName)
	if len(body) == 0 || modelName == "" {
		return false, false
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return false, false
	}

	var identifiers []string
	collectModelIdentifiers(payload, &identifiers)
	if len(identifiers) == 0 {
		return false, false
	}
	for _, identifier := range identifiers {
		if strings.TrimSpace(identifier) == modelName {
			return true, true
		}
	}
	return true, false
}

func collectModelIdentifiers(value any, identifiers *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isModelIdentifierKey(key) {
				if modelID, ok := child.(string); ok {
					*identifiers = append(*identifiers, modelID)
					continue
				}
			}
			collectModelIdentifiers(child, identifiers)
		}
	case []any:
		for _, child := range typed {
			collectModelIdentifiers(child, identifiers)
		}
	}
}

func isModelIdentifierKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "id", "name", "model":
		return true
	default:
		return false
	}
}

func compactLogDetail(detail string, limit int) string {
	detail = strings.Join(strings.Fields(detail), " ")
	if len(detail) <= limit {
		return detail
	}
	return detail[:limit] + "..."
}
