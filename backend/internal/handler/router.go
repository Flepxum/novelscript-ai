package handler

import (
	"encoding/json"
	"net/http"

	"github.com/Flepxum/novelscript-ai/backend/internal/domain"
	"github.com/Flepxum/novelscript-ai/backend/internal/service"
	"github.com/gin-gonic/gin"
)

type Router struct {
	service *service.AppService
}

func NewRouter(app *service.AppService) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), cors())

	r := Router{service: app}
	api := router.Group("/api/v1")
	api.GET("/health", r.health)
	api.GET("/projects", r.listProjects)
	api.POST("/projects", r.createProject)
	api.GET("/projects/:projectId", r.getProject)
	api.POST("/projects/:projectId/source", r.saveSource)
	api.PUT("/projects/:projectId/chapters", r.updateChapters)
	api.POST("/projects/:projectId/generate", r.generate)
	api.GET("/jobs/:jobId", r.getJob)
	api.GET("/projects/:projectId/script", r.getScript)
	api.PUT("/projects/:projectId/script", r.saveScript)
	api.GET("/projects/:projectId/script/versions", r.listVersions)
	api.POST("/projects/:projectId/script/regenerate", r.regenerateScene)
	api.GET("/schema/script", r.schema)
	api.GET("/schema/yaml", r.schema)

	return router
}

func (r Router) health(c *gin.Context) {
	ok(c, map[string]string{"status": "ok"})
}

func (r Router) listProjects(c *gin.Context) {
	ok(c, r.service.ListProjects())
}

func (r Router) createProject(c *gin.Context) {
	var req struct {
		Title            string `json:"title"`
		AdaptationTarget string `json:"adaptation_target"`
		Language         string `json:"language"`
	}
	if !bind(c, &req) {
		return
	}
	project, err := r.service.CreateProject(req.Title, req.AdaptationTarget, req.Language)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, project)
}

func (r Router) getProject(c *gin.Context) {
	project, err := r.service.GetProject(c.Param("projectId"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, project)
}

func (r Router) saveSource(c *gin.Context) {
	var req struct {
		NovelTitle string `json:"novel_title"`
		Author     string `json:"author"`
		Content    string `json:"content"`
	}
	if !bind(c, &req) {
		return
	}
	source, err := r.service.SaveSource(c.Param("projectId"), req.NovelTitle, req.Author, req.Content)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, map[string]any{
		"source_id":     source.ID,
		"chapter_count": len(source.Chapters),
		"chapters":      source.Chapters,
	})
}

func (r Router) updateChapters(c *gin.Context) {
	var req struct {
		Chapters []domain.Chapter `json:"chapters"`
	}
	if !bind(c, &req) {
		return
	}
	source, err := r.service.UpdateChapters(c.Param("projectId"), req.Chapters)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, map[string]any{
		"source_id":     source.ID,
		"chapter_count": len(source.Chapters),
		"chapters":      source.Chapters,
	})
}

func (r Router) generate(c *gin.Context) {
	var req domain.GenerationConfig
	if !bind(c, &req) {
		return
	}
	job, err := r.service.StartGeneration(c.Param("projectId"), req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, map[string]any{
		"job_id":       job.ID,
		"id":           job.ID,
		"status":       job.Status,
		"progress":     job.Progress,
		"current_step": job.CurrentStep,
	})
}

func (r Router) getJob(c *gin.Context) {
	job, err := r.service.GetJob(c.Param("jobId"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, job)
}

func (r Router) getScript(c *gin.Context) {
	version, err := r.service.GetScript(c.Param("projectId"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, map[string]any{
		"version_id": version.ID,
		"yaml":       version.YAML,
		"draft":      version.Draft,
		"created_at": version.CreatedAt,
	})
}

func (r Router) saveScript(c *gin.Context) {
	var req struct {
		YAML       string `json:"yaml"`
		EditorNote string `json:"editor_note"`
	}
	if !bind(c, &req) {
		return
	}
	version, _, err := r.service.SaveScript(c.Param("projectId"), req.YAML, req.EditorNote)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, map[string]any{
		"version_id": version.ID,
		"yaml":       version.YAML,
		"draft":      version.Draft,
		"created_at": version.CreatedAt,
	})
}

func (r Router) listVersions(c *gin.Context) {
	versions := r.service.ListVersions(c.Param("projectId"))
	items := make([]map[string]any, len(versions))
	for i, version := range versions {
		items[i] = map[string]any{
			"version_id":  version.ID,
			"created_at":  version.CreatedAt,
			"editor_note": version.EditorNote,
		}
	}
	ok(c, items)
}

func (r Router) regenerateScene(c *gin.Context) {
	var req struct {
		Scope       string `json:"scope"`
		SceneID     string `json:"scene_id"`
		Instruction string `json:"instruction"`
	}
	if !bind(c, &req) {
		return
	}
	version, scene, err := r.service.RegenerateScene(c.Param("projectId"), req.SceneID, req.Instruction)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, map[string]any{
		"version_id": version.ID,
		"scene":      scene,
		"yaml":       version.YAML,
		"draft":      version.Draft,
	})
}

func (r Router) schema(c *gin.Context) {
	data, err := r.service.ReadSchema()
	if err != nil {
		fail(c, err)
		return
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		c.Data(http.StatusOK, "application/json; charset=utf-8", data)
		return
	}
	ok(c, decoded)
}

func bind(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "bad_request",
				"message": "invalid request body",
				"details": []domain.ValidationIssue{{Path: "body", Message: err.Error()}},
			},
		})
		return false
	}
	return true
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"data": data})
}

func fail(c *gin.Context, err error) {
	if appErr, matched := service.AsAppError(err); matched {
		c.JSON(appErr.Status, gin.H{"error": appErr})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{
		"error": gin.H{
			"code":    "internal_error",
			"message": err.Error(),
		},
	})
}

func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
