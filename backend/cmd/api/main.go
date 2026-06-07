package main

import (
	"log"

	"github.com/Flepxum/novelscript-ai/backend/internal/ai"
	"github.com/Flepxum/novelscript-ai/backend/internal/config"
	"github.com/Flepxum/novelscript-ai/backend/internal/handler"
	"github.com/Flepxum/novelscript-ai/backend/internal/repository"
	"github.com/Flepxum/novelscript-ai/backend/internal/service"
)

func main() {
	cfg := config.Load()
	repo := repository.NewMemoryRepository()
	generator := ai.NewGenerator()
	app := service.NewAppService(repo, generator, cfg)
	router := handler.NewRouter(app)

	addr := ":" + cfg.Port
	log.Printf("NovelScript AI API listening on http://localhost%s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatal(err)
	}
}
