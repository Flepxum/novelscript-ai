package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	AppEnv               string
	Port                 string
	APIBasePath          string
	PublicBaseURL        string
	LogLevel             string
	RequestBodyLimitMB   int
	CORSAllowedOrigins   []string
	RepositoryType       string
	SQLitePath           string
	DataDir              string
	ExportDir            string
	MinChapterCount      int
	MaxChapterCount      int
	MaxChapterChars      int
	JobStepDelayMS       int
	JobTimeoutSeconds    int
	JobMaxParallel       int
	SchemaPath           string
	ModelProvider        string
	ModelBaseURL         string
	ModelAPIKey          string
	ModelOrgID           string
	ModelProjectID       string
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
	loadDotenvFiles()

	return Config{
		AppEnv:               getenv("APP_ENV", "development"),
		Port:                 getenv("PORT", "8080"),
		APIBasePath:          getenv("API_BASE_PATH", "/api/v1"),
		PublicBaseURL:        getenv("PUBLIC_BASE_URL", "http://localhost:8080"),
		LogLevel:             getenv("LOG_LEVEL", "info"),
		RequestBodyLimitMB:   getenvInt("REQUEST_BODY_LIMIT_MB", 20),
		CORSAllowedOrigins:   getenvCSV("CORS_ALLOWED_ORIGINS", []string{"http://localhost:5173"}),
		RepositoryType:       getenv("REPOSITORY_TYPE", "memory"),
		SQLitePath:           getenv("SQLITE_PATH", "../data/novelscript.db"),
		DataDir:              getenv("DATA_DIR", "../data"),
		ExportDir:            getenv("EXPORT_DIR", "../data/exports"),
		MinChapterCount:      getenvInt("MIN_CHAPTER_COUNT", 3),
		MaxChapterCount:      getenvInt("MAX_CHAPTER_COUNT", 80),
		MaxChapterChars:      getenvInt("MAX_CHAPTER_CHARS", 20000),
		JobStepDelayMS:       getenvInt("JOB_STEP_DELAY_MS", 180),
		JobTimeoutSeconds:    getenvInt("JOB_TIMEOUT_SECONDS", 180),
		JobMaxParallel:       getenvInt("JOB_MAX_PARALLEL", 2),
		SchemaPath:           getenv("SCRIPT_SCHEMA_PATH", "../schemas/script.schema.json"),
		ModelProvider:        getenv("MODEL_PROVIDER", ""),
		ModelBaseURL:         getenv("MODEL_BASE_URL", ""),
		ModelAPIKey:          getenv("MODEL_API_KEY", ""),
		ModelOrgID:           getenv("MODEL_ORG_ID", ""),
		ModelProjectID:       getenv("MODEL_PROJECT_ID", ""),
		ModelName:            getenv("MODEL_NAME", ""),
		ModelTimeoutSeconds:  getenvInt("MODEL_TIMEOUT_SECONDS", 120),
		ModelMaxRetries:      getenvInt("MODEL_MAX_RETRIES", 2),
		ModelMaxInputChars:   getenvInt("MODEL_MAX_INPUT_CHARS", 24000),
		ModelMaxOutputTokens: getenvInt("MODEL_MAX_OUTPUT_TOKENS", 6000),
		ModelTemperature:     getenvFloat("MODEL_TEMPERATURE", 1),
		ModelStructureMode:   getenv("MODEL_STRUCTURE_MODE", "json_schema"),
		ModelPromptVersion:   getenv("MODEL_PROMPT_VERSION", "script-draft-v1"),
	}
}

func loadDotenvFiles() {
	processEnv := existingProcessEnv()
	for _, path := range dotenvCandidates() {
		loadDotenvFile(path, processEnv)
	}
}

func existingProcessEnv() map[string]bool {
	result := map[string]bool{}
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok && strings.TrimSpace(value) != "" {
			result[key] = true
		}
	}
	return result
}

func dotenvCandidates() []string {
	wd, err := os.Getwd()
	if err != nil {
		return []string{".env", filepath.Join("backend", ".env")}
	}

	var candidates []string
	if filepath.Base(wd) == "backend" {
		candidates = append(candidates, filepath.Join(wd, "..", ".env"), filepath.Join(wd, ".env"))
	} else {
		candidates = append(candidates, filepath.Join(wd, ".env"), filepath.Join(wd, "backend", ".env"))
	}

	seen := map[string]bool{}
	unique := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		cleaned := filepath.Clean(candidate)
		if !seen[cleaned] {
			seen[cleaned] = true
			unique = append(unique, cleaned)
		}
	}
	return unique
}

func loadDotenvFile(path string, processEnv map[string]bool) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := parseDotenvLine(scanner.Text())
		if !ok || processEnv[key] {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

func parseDotenvLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimSpace(strings.TrimPrefix(line, "export "))

	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", false
	}
	return key, cleanDotenvValue(value), true
}

func cleanDotenvValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		quote := value[0]
		if (quote == '"' || quote == '\'') && value[len(value)-1] == quote {
			value = value[1 : len(value)-1]
			if quote == '"' {
				value = strings.ReplaceAll(value, `\n`, "\n")
				value = strings.ReplaceAll(value, `\"`, `"`)
				value = strings.ReplaceAll(value, `\\`, `\`)
			}
			return value
		}
	}
	return stripInlineComment(value)
}

func stripInlineComment(value string) string {
	for i, char := range value {
		if char == '#' && i > 0 && isWhitespace(value[i-1]) {
			return strings.TrimSpace(value[:i])
		}
	}
	return strings.TrimSpace(value)
}

func isWhitespace(char byte) bool {
	return char == ' ' || char == '\t'
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getenvCSV(key string, fallback []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return fallback
	}
	return result
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
