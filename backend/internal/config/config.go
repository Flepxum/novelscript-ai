package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port                 string
	SchemaPath           string
	ModelProvider        string
	ModelBaseURL         string
	ModelAPIKey          string
	ModelName            string
	ModelTimeoutSeconds  int
	ModelMaxRetries      int
	ModelMaxInputChars   int
	ModelMaxOutputTokens int
	ModelTemperature     float64
	ModelStructureMode   string
	ModelPromptVersion   string
}

func Load() Config {
	return Config{
		Port:                 getenv("PORT", "8080"),
		SchemaPath:           getenv("SCRIPT_SCHEMA_PATH", "../schemas/script.schema.json"),
		ModelProvider:        getenv("MODEL_PROVIDER", ""),
		ModelBaseURL:         getenv("MODEL_BASE_URL", ""),
		ModelAPIKey:          getenv("MODEL_API_KEY", ""),
		ModelName:            getenv("MODEL_NAME", ""),
		ModelTimeoutSeconds:  getenvInt("MODEL_TIMEOUT_SECONDS", 120),
		ModelMaxRetries:      getenvInt("MODEL_MAX_RETRIES", 2),
		ModelMaxInputChars:   getenvInt("MODEL_MAX_INPUT_CHARS", 24000),
		ModelMaxOutputTokens: getenvInt("MODEL_MAX_OUTPUT_TOKENS", 6000),
		ModelTemperature:     getenvFloat("MODEL_TEMPERATURE", 0.4),
		ModelStructureMode:   getenv("MODEL_STRUCTURE_MODE", "json_schema"),
		ModelPromptVersion:   getenv("MODEL_PROMPT_VERSION", "script-draft-v1"),
	}
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvFloat(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
