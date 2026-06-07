package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsBackendDotenvFile(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeDotenv(t, filepath.Join(root, "backend", ".env"), `
MODEL_BASE_URL="https://example.test/v1"
MODEL_API_KEY='file-key'
MODEL_NAME=script-pro
MODEL_TIMEOUT_SECONDS=9
MODEL_TEMPERATURE=0.7
`)
	t.Chdir(root)
	t.Setenv("MODEL_BASE_URL", "")
	t.Setenv("MODEL_API_KEY", "")
	t.Setenv("MODEL_NAME", "")
	t.Setenv("MODEL_TIMEOUT_SECONDS", "")
	t.Setenv("MODEL_TEMPERATURE", "")

	cfg := Load()

	if cfg.ModelBaseURL != "https://example.test/v1" {
		t.Fatalf("expected model base url from .env, got %q", cfg.ModelBaseURL)
	}
	if cfg.ModelAPIKey != "file-key" {
		t.Fatalf("expected model api key from .env")
	}
	if cfg.ModelName != "script-pro" {
		t.Fatalf("expected model name from .env, got %q", cfg.ModelName)
	}
	if cfg.ModelTimeoutSeconds != 9 {
		t.Fatalf("expected timeout from .env, got %d", cfg.ModelTimeoutSeconds)
	}
	if cfg.ModelTemperature != 0.7 {
		t.Fatalf("expected temperature from .env, got %f", cfg.ModelTemperature)
	}
}

func TestLoadKeepsProcessEnvPriorityOverDotenv(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeDotenv(t, filepath.Join(root, "backend", ".env"), `
MODEL_NAME=file-model
MODEL_BASE_URL=https://file.example/v1
`)
	t.Chdir(root)
	t.Setenv("MODEL_NAME", "process-model")
	t.Setenv("MODEL_BASE_URL", "")

	cfg := Load()

	if cfg.ModelName != "process-model" {
		t.Fatalf("expected process env to win, got %q", cfg.ModelName)
	}
	if cfg.ModelBaseURL != "https://file.example/v1" {
		t.Fatalf("expected missing process env to fall back to .env, got %q", cfg.ModelBaseURL)
	}
}

func TestParseDotenvLineSupportsExportAndComments(t *testing.T) {
	key, value, ok := parseDotenvLine(`export MODEL_NAME=script-pro # local model`)
	if !ok {
		t.Fatal("expected dotenv line to parse")
	}
	if key != "MODEL_NAME" || value != "script-pro" {
		t.Fatalf("unexpected dotenv pair: %s=%s", key, value)
	}
}

func writeDotenv(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
