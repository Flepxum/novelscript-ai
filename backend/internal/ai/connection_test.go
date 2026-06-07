package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Flepxum/novelscript-ai/backend/internal/config"
)

func TestCheckLLMConnectionSkipsWhenConfigIsIncomplete(t *testing.T) {
	result := CheckLLMConnection(context.Background(), config.Config{})

	if result.Configured {
		t.Fatal("expected incomplete config to skip the connection check")
	}
	expected := "MODEL_BASE_URL,MODEL_API_KEY,MODEL_NAME"
	actual := strings.Join(result.Missing, ",")
	if actual != expected {
		t.Fatalf("expected missing config %q, got %q", expected, actual)
	}
}

func TestCheckLLMConnectionSucceedsAndFindsModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("expected /v1/models request, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("expected bearer token header, got %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("OpenAI-Organization") != "org-1" {
			t.Fatalf("expected organization header, got %q", r.Header.Get("OpenAI-Organization"))
		}
		if r.Header.Get("OpenAI-Project") != "project-1" {
			t.Fatalf("expected project header, got %q", r.Header.Get("OpenAI-Project"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"script-pro"},{"id":"script-lite"}]}`))
	}))
	defer server.Close()

	result := CheckLLMConnection(context.Background(), config.Config{
		ModelBaseURL:        server.URL + "/v1",
		ModelAPIKey:         "test-key",
		ModelOrgID:          "org-1",
		ModelProjectID:      "project-1",
		ModelName:           "script-pro",
		ModelTimeoutSeconds: 3,
	})

	if !result.Configured {
		t.Fatal("expected connection check to run")
	}
	if !result.OK {
		t.Fatalf("expected connection check to succeed, got %q", result.Message)
	}
	if !result.ModelChecked || !result.ModelFound {
		t.Fatalf("expected configured model to be found, checked=%t found=%t", result.ModelChecked, result.ModelFound)
	}
}

func TestCheckLLMConnectionReportsMissingModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("expected /models request, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"another-model"}]}`))
	}))
	defer server.Close()

	result := CheckLLMConnection(context.Background(), config.Config{
		ModelBaseURL:        server.URL,
		ModelAPIKey:         "test-key",
		ModelName:           "script-pro",
		ModelTimeoutSeconds: 3,
	})

	if !result.OK {
		t.Fatalf("expected provider to be reachable, got %q", result.Message)
	}
	if !result.ModelChecked {
		t.Fatal("expected model list to be checked")
	}
	if result.ModelFound {
		t.Fatal("expected configured model to be absent")
	}
}

func TestCheckLLMConnectionAcceptsBaseURLThatAlreadyPointsToModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("expected /v1/models request, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"script-pro"}]}`))
	}))
	defer server.Close()

	result := CheckLLMConnection(context.Background(), config.Config{
		ModelBaseURL:        server.URL + "/v1/models",
		ModelAPIKey:         "test-key",
		ModelName:           "script-pro",
		ModelTimeoutSeconds: 3,
	})

	if !result.OK || !result.ModelFound {
		t.Fatalf("expected connection check to find model, ok=%t found=%t message=%q", result.OK, result.ModelFound, result.Message)
	}
}

func TestCheckLLMConnectionCorrectsChatCompletionsBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("expected /v1/models request, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"script-pro"}]}`))
	}))
	defer server.Close()

	result := CheckLLMConnection(context.Background(), config.Config{
		ModelBaseURL:        server.URL + "/v1/chat/completions",
		ModelAPIKey:         "test-key",
		ModelName:           "script-pro",
		ModelTimeoutSeconds: 3,
	})

	if !result.OK || !result.ModelFound {
		t.Fatalf("expected connection check to find model, ok=%t found=%t message=%q", result.OK, result.ModelFound, result.Message)
	}
}

func TestCheckLLMConnectionReportsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	result := CheckLLMConnection(context.Background(), config.Config{
		ModelBaseURL:        server.URL,
		ModelAPIKey:         "test-key",
		ModelName:           "script-pro",
		ModelTimeoutSeconds: 3,
	})

	if result.OK {
		t.Fatal("expected connection check to fail")
	}
	if result.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", result.StatusCode)
	}
	if !strings.Contains(result.Message, "invalid api key") {
		t.Fatalf("expected failure detail to include response body, got %q", result.Message)
	}
}
