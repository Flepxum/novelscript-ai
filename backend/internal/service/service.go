package service

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/Flepxum/novelscript-ai/backend/internal/ai"
	"github.com/Flepxum/novelscript-ai/backend/internal/config"
	"github.com/Flepxum/novelscript-ai/backend/internal/domain"
	"github.com/Flepxum/novelscript-ai/backend/internal/exporter"
	"github.com/Flepxum/novelscript-ai/backend/internal/parser"
	"github.com/Flepxum/novelscript-ai/backend/internal/repository"
	"github.com/Flepxum/novelscript-ai/backend/internal/validator"
)

type AppService struct {
	repo      *repository.MemoryRepository
	generator *ai.Generator
	cfg       config.Config
}

func NewAppService(repo *repository.MemoryRepository, generator *ai.Generator, cfg config.Config) *AppService {
	return &AppService{
		repo:      repo,
		generator: generator,
		cfg:       cfg,
	}
}

func (s *AppService) CreateProject(title, target, language string) (domain.Project, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return domain.Project{}, badRequest("project title is required")
	}
	if target == "" {
		target = "web_series"
	}
	if language == "" {
		language = "zh-CN"
	}
	project := domain.Project{
		ID:               s.repo.NextID("proj"),
		Title:            title,
		AdaptationTarget: target,
		Language:         language,
		CreatedAt:        time.Now(),
	}
	s.repo.SaveProject(project)
	return project, nil
}

func (s *AppService) ListProjects() []domain.Project {
	return s.repo.ListProjects()
}

func (s *AppService) GetProject(projectID string) (map[string]any, error) {
	project, err := s.repo.GetProject(projectID)
	if err != nil {
		return nil, notFound("project not found")
	}
	result := map[string]any{
		"id":                project.ID,
		"title":             project.Title,
		"adaptation_target": project.AdaptationTarget,
		"language":          project.Language,
		"created_at":        project.CreatedAt,
	}
	if source, err := s.repo.GetSource(projectID); err == nil {
		result["source"] = map[string]any{
			"source_id":     source.ID,
			"novel_title":   source.NovelTitle,
			"author":        source.Author,
			"chapter_count": len(source.Chapters),
			"chapters":      chapterSummaries(source.Chapters),
		}
	}
	if latest, err := s.repo.LatestVersion(projectID); err == nil {
		result["current_version"] = map[string]any{
			"version_id": latest.ID,
			"created_at": latest.CreatedAt,
		}
	}
	return result, nil
}

func (s *AppService) SaveSource(projectID, novelTitle, author, content string) (domain.SourceDocument, error) {
	if _, err := s.repo.GetProject(projectID); err != nil {
		return domain.SourceDocument{}, notFound("project not found")
	}
	chapters, err := parser.SplitChapters(content)
	if err != nil {
		return domain.SourceDocument{}, validationFailed("novel source validation failed", []domain.ValidationIssue{
			{Path: "content", Message: err.Error()},
		})
	}
	source := domain.SourceDocument{
		ID:         s.repo.NextID("src"),
		ProjectID:  projectID,
		NovelTitle: strings.TrimSpace(novelTitle),
		Author:     strings.TrimSpace(author),
		Content:    parser.CleanText(content),
		Chapters:   chapters,
		CreatedAt:  time.Now(),
	}
	s.repo.SaveSource(source)
	return source, nil
}

func (s *AppService) UpdateChapters(projectID string, chapters []domain.Chapter) (domain.SourceDocument, error) {
	source, err := s.repo.GetSource(projectID)
	if err != nil {
		return domain.SourceDocument{}, notFound("source document not found")
	}
	if len(chapters) < 3 {
		return domain.SourceDocument{}, validationFailed("chapter validation failed", []domain.ValidationIssue{
			{Path: "chapters", Message: "at least 3 chapters are required"},
		})
	}
	for i := range chapters {
		chapters[i].Index = i + 1
		chapters[i].Title = strings.TrimSpace(chapters[i].Title)
		chapters[i].Content = parser.CleanText(chapters[i].Content)
		chapters[i].WordCount = parser.CountWords(chapters[i].Content)
		if chapters[i].Title == "" {
			chapters[i].Title = "未命名章节"
		}
	}
	source.Chapters = chapters
	source.Content = joinChapters(chapters)
	s.repo.SaveSource(source)
	return source, nil
}

func (s *AppService) StartGeneration(projectID string, config domain.GenerationConfig) (domain.GenerationJob, error) {
	project, err := s.repo.GetProject(projectID)
	if err != nil {
		return domain.GenerationJob{}, notFound("project not found")
	}
	source, err := s.repo.GetSource(projectID)
	if err != nil {
		return domain.GenerationJob{}, validationFailed("source document is required before generation", []domain.ValidationIssue{
			{Path: "source", Message: "upload or paste a novel with at least 3 chapters first"},
		})
	}
	job := domain.GenerationJob{
		ID:          s.repo.NextID("job"),
		ProjectID:   projectID,
		Status:      "queued",
		Progress:    0,
		CurrentStep: "任务已进入队列",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	s.repo.SaveJob(job)
	go s.runGeneration(job.ID, project, source, config)
	return job, nil
}

func (s *AppService) GetJob(jobID string) (domain.GenerationJob, error) {
	job, err := s.repo.GetJob(jobID)
	if err != nil {
		return domain.GenerationJob{}, notFound("job not found")
	}
	return job, nil
}

func (s *AppService) GetScript(projectID string) (domain.ScriptVersion, error) {
	version, err := s.repo.LatestVersion(projectID)
	if err != nil {
		return domain.ScriptVersion{}, notFound("script version not found")
	}
	return version, nil
}

func (s *AppService) SaveScript(projectID, yamlText, editorNote string) (domain.ScriptVersion, []domain.ValidationIssue, error) {
	if strings.TrimSpace(yamlText) == "" {
		return domain.ScriptVersion{}, nil, badRequest("yaml is required")
	}
	draft, err := exporter.FromYAML(yamlText)
	if err != nil {
		return domain.ScriptVersion{}, nil, validationFailed("script yaml parse failed", []domain.ValidationIssue{
			{Path: "yaml", Message: err.Error()},
		})
	}
	issues := validator.ValidateDraft(draft)
	if len(issues) > 0 {
		return domain.ScriptVersion{}, issues, validationFailed("script schema validation failed", issues)
	}
	normalized, err := exporter.ToYAML(draft)
	if err != nil {
		return domain.ScriptVersion{}, nil, internalError(err.Error())
	}
	version := domain.ScriptVersion{
		ID:         s.repo.NextID("ver"),
		ProjectID:  projectID,
		YAML:       normalized,
		Draft:      draft,
		EditorNote: strings.TrimSpace(editorNote),
		CreatedAt:  time.Now(),
	}
	s.repo.SaveVersion(version)
	return version, nil, nil
}

func (s *AppService) RegenerateScene(projectID, sceneID, instruction string) (domain.ScriptVersion, domain.Scene, error) {
	current, err := s.repo.LatestVersion(projectID)
	if err != nil {
		return domain.ScriptVersion{}, domain.Scene{}, notFound("script version not found")
	}
	next, scene, err := s.generator.RegenerateScene(context.Background(), current.Draft, sceneID, instruction)
	if err != nil {
		return domain.ScriptVersion{}, domain.Scene{}, badRequest(err.Error())
	}
	if issues := validator.ValidateDraft(next); len(issues) > 0 {
		return domain.ScriptVersion{}, domain.Scene{}, validationFailed("regenerated script validation failed", issues)
	}
	yamlText, err := exporter.ToYAML(next)
	if err != nil {
		return domain.ScriptVersion{}, domain.Scene{}, internalError(err.Error())
	}
	version := domain.ScriptVersion{
		ID:         s.repo.NextID("ver"),
		ProjectID:  projectID,
		YAML:       yamlText,
		Draft:      next,
		EditorNote: "局部重写 " + sceneID,
		CreatedAt:  time.Now(),
	}
	s.repo.SaveVersion(version)
	return version, scene, nil
}

func (s *AppService) ListVersions(projectID string) []domain.ScriptVersion {
	return s.repo.ListVersions(projectID)
}

func (s *AppService) ReadSchema() ([]byte, error) {
	data, err := os.ReadFile(s.cfg.SchemaPath)
	if err != nil {
		return nil, internalError(err.Error())
	}
	return data, nil
}

func (s *AppService) runGeneration(jobID string, project domain.Project, source domain.SourceDocument, config domain.GenerationConfig) {
	step := func(status string, progress int, label string) {
		job, err := s.repo.GetJob(jobID)
		if err != nil {
			return
		}
		job.Status = status
		job.Progress = progress
		job.CurrentStep = label
		s.repo.SaveJob(job)
		time.Sleep(180 * time.Millisecond)
	}

	step("splitting", 12, "正在清洗文本并确认章节边界")
	step("analyzing", 28, "正在提炼角色、主题和主线冲突")
	step("outlining", 45, "正在规划幕结构和场景列表")
	step("generating_scenes", 68, "正在生成场景节拍、动作和对白")

	draft, err := s.generator.Generate(context.Background(), ai.GenerationInput{
		Project: project,
		Source:  source,
		Config:  config,
	})
	if err != nil {
		s.failJob(jobID, err.Error())
		return
	}

	step("validating", 84, "正在校验 YAML Schema 和引用关系")
	if issues := validator.ValidateDraft(draft); len(issues) > 0 {
		s.failJob(jobID, "generated script did not pass validation")
		return
	}

	step("exporting", 94, "正在导出结构化 YAML")
	yamlText, err := exporter.ToYAML(draft)
	if err != nil {
		s.failJob(jobID, err.Error())
		return
	}
	version := domain.ScriptVersion{
		ID:        s.repo.NextID("ver"),
		ProjectID: project.ID,
		YAML:      yamlText,
		Draft:     draft,
		CreatedAt: time.Now(),
	}
	s.repo.SaveVersion(version)

	now := time.Now()
	job, err := s.repo.GetJob(jobID)
	if err != nil {
		return
	}
	job.Status = "succeeded"
	job.Progress = 100
	job.CurrentStep = "剧本初稿已生成"
	job.CompletedAt = &now
	s.repo.SaveJob(job)
}

func (s *AppService) failJob(jobID, message string) {
	job, err := s.repo.GetJob(jobID)
	if err != nil {
		return
	}
	now := time.Now()
	job.Status = "failed"
	job.Progress = 100
	job.CurrentStep = "生成失败"
	job.Error = &message
	job.CompletedAt = &now
	s.repo.SaveJob(job)
}

func chapterSummaries(chapters []domain.Chapter) []map[string]any {
	result := make([]map[string]any, len(chapters))
	for i, chapter := range chapters {
		result[i] = map[string]any{
			"index":      chapter.Index,
			"title":      chapter.Title,
			"word_count": chapter.WordCount,
		}
	}
	return result
}

func joinChapters(chapters []domain.Chapter) string {
	var parts []string
	for _, chapter := range chapters {
		parts = append(parts, chapter.Title+"\n"+chapter.Content)
	}
	return strings.Join(parts, "\n\n")
}

func AsAppError(err error) (AppError, bool) {
	var appErr AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return AppError{}, false
}
