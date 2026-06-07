package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/Flepxum/novelscript-ai/backend/internal/ai"
	"github.com/Flepxum/novelscript-ai/backend/internal/config"
	"github.com/Flepxum/novelscript-ai/backend/internal/handler"
	"github.com/Flepxum/novelscript-ai/backend/internal/repository"
	"github.com/Flepxum/novelscript-ai/backend/internal/service"
)

func main() {
	cfg := config.Load()
	logStartupConfig(cfg)
	logLLMConnection(cfg)

	repo := repository.NewMemoryRepository()
	generator := ai.NewGenerator(cfg)
	app := service.NewAppService(repo, generator, cfg)
	router := handler.NewRouter(app)

	addr := ":" + cfg.Port
	log.Printf("NovelScript AI API listening on http://localhost%s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatal(err)
	}
}

func logStartupConfig(cfg config.Config) {
	log.Printf(
		"Startup config: env=%s log_level=%s port=%s api_base_path=%s public_base_url=%s repository=%s schema=%s cors_origins=%s",
		valueOrUnset(cfg.AppEnv),
		valueOrUnset(cfg.LogLevel),
		valueOrUnset(cfg.Port),
		valueOrUnset(cfg.APIBasePath),
		valueOrUnset(cfg.PublicBaseURL),
		valueOrUnset(cfg.RepositoryType),
		valueOrUnset(cfg.SchemaPath),
		valueOrUnset(strings.Join(cfg.CORSAllowedOrigins, ",")),
	)
	log.Printf(
		"LLM config: provider=%s base_url=%s model=%s api_key_configured=%t org_configured=%t project_configured=%t timeout=%ds retries=%d structure_mode=%s agent_pipeline=%s prompt_version=%s",
		valueOrUnset(cfg.ModelProvider),
		valueOrUnset(cfg.ModelBaseURL),
		valueOrUnset(cfg.ModelName),
		isConfigured(cfg.ModelAPIKey),
		isConfigured(cfg.ModelOrgID),
		isConfigured(cfg.ModelProjectID),
		cfg.ModelTimeoutSeconds,
		cfg.ModelMaxRetries,
		valueOrUnset(cfg.ModelStructureMode),
		valueOrUnset(cfg.ModelAgentPipeline),
		valueOrUnset(cfg.ModelPromptVersion),
	)
}

func logLLMConnection(cfg config.Config) {
	result := ai.CheckLLMConnection(context.Background(), cfg)
	if !result.Configured {
		log.Printf("LLM connection check skipped: missing required config %s", strings.Join(result.Missing, ", "))
		return
	}

	latency := result.Duration.Round(time.Millisecond)
	if result.OK {
		modelStatus := "not_checked"
		if result.ModelChecked && result.ModelFound {
			modelStatus = "found"
		}
		if result.ModelChecked && !result.ModelFound {
			modelStatus = "not_found"
			log.Printf(
				"LLM provider reachable but configured model was not found: endpoint=%s status=%d model=%s model_status=%s latency=%s",
				valueOrUnset(result.Endpoint),
				result.StatusCode,
				valueOrUnset(cfg.ModelName),
				modelStatus,
				latency,
			)
			return
		}
		log.Printf(
			"LLM connection check succeeded: endpoint=%s status=%d model=%s model_status=%s latency=%s",
			valueOrUnset(result.Endpoint),
			result.StatusCode,
			valueOrUnset(cfg.ModelName),
			modelStatus,
			latency,
		)
		return
	}

	if result.StatusCode > 0 {
		log.Printf(
			"LLM connection check failed: endpoint=%s status=%d latency=%s detail=%s",
			valueOrUnset(result.Endpoint),
			result.StatusCode,
			latency,
			valueOrUnset(result.Message),
		)
		return
	}
	log.Printf(
		"LLM connection check failed: endpoint=%s latency=%s detail=%s",
		valueOrUnset(result.Endpoint),
		latency,
		valueOrUnset(result.Message),
	)
}

func isConfigured(value string) bool {
	return strings.TrimSpace(value) != ""
}

func valueOrUnset(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "(unset)"
	}
	return value
}
