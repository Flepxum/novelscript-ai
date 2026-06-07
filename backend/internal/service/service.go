package service

import (
	"context"
	"errors"
	"log"
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
	cleaned := parser.CleanText(content)
	if cleaned == "" {
		return domain.SourceDocument{}, validationFailed("novel source validation failed", []domain.ValidationIssue{
			{Path: "content", Message: "novel content is empty"},
		})
	}

	chapters, err := parser.SplitChapters(cleaned)
	segmentationMode := "rule_parser"
	if err != nil {
		log.Printf(
			"Source rule chapter split failed, switching to ChapterSegmentationAgent: project_id=%s novel_title=%s source_chars=%d error=%s",
			projectID,
			strings.TrimSpace(novelTitle),
			len([]rune(cleaned)),
			err,
		)
		ctx, cancel := context.WithTimeout(context.Background(), chapterSegmentationTimeout(s.cfg))
		defer cancel()
		agentChapters, agentErr := s.generator.SplitChapters(ctx, novelTitle, author, cleaned, s.cfg.MinChapterCount)
		if agentErr != nil {
			log.Printf(
				"Source chapter segmentation failed: project_id=%s parser_error=%s agent_error=%s",
				projectID,
				err,
				agentErr,
			)
			return domain.SourceDocument{}, validationFailed("novel source segmentation failed", []domain.ValidationIssue{
				{Path: "content", Message: "rule parser failed: " + err.Error()},
				{Path: "llm", Message: "chapter segmentation agent failed: " + agentErr.Error()},
			})
		}
		chapters = agentChapters
		segmentationMode = "chapter_segmentation_agent"
	}
	source := domain.SourceDocument{
		ID:         s.repo.NextID("src"),
		ProjectID:  projectID,
		NovelTitle: strings.TrimSpace(novelTitle),
		Author:     strings.TrimSpace(author),
		Content:    cleaned,
		Chapters:   chapters,
		CreatedAt:  time.Now(),
	}
	s.repo.SaveSource(source)
	log.Printf(
		"Source saved: project_id=%s source_id=%s segmentation_mode=%s chapters=%d source_chars=%d",
		projectID,
		source.ID,
		segmentationMode,
		len(source.Chapters),
		len([]rune(source.Content)),
	)
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

func (s *AppService) GetScriptVersion(projectID, versionID string) (domain.ScriptVersion, error) {
	version, err := s.repo.GetVersion(projectID, versionID)
	if err != nil {
		return domain.ScriptVersion{}, notFound("script version not found")
	}
	return version, nil
}

func (s *AppService) RestoreScriptVersion(projectID, versionID string) (domain.ScriptVersion, error) {
	version, err := s.repo.GetVersion(projectID, versionID)
	if err != nil {
		return domain.ScriptVersion{}, notFound("script version not found")
	}
	restored := domain.ScriptVersion{
		ID:         s.repo.NextID("ver"),
		ProjectID:  projectID,
		YAML:       version.YAML,
		Draft:      version.Draft,
		EditorNote: "恢复版本 " + versionID,
		CreatedAt:  time.Now(),
	}
	s.repo.SaveVersion(restored)
	return restored, nil
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
	log.Printf("Scene regeneration started: project_id=%s scene_id=%s instruction_chars=%d model=%s", projectID, sceneID, len([]rune(instruction)), s.cfg.ModelName)
	current, err := s.repo.LatestVersion(projectID)
	if err != nil {
		log.Printf("Scene regeneration failed: project_id=%s scene_id=%s error=%s", projectID, sceneID, err)
		return domain.ScriptVersion{}, domain.Scene{}, notFound("script version not found")
	}
	next, scene, err := s.generator.RegenerateScene(context.Background(), current.Draft, sceneID, instruction)
	if err != nil {
		log.Printf("Scene regeneration failed: project_id=%s scene_id=%s error=%s", projectID, sceneID, err)
		return domain.ScriptVersion{}, domain.Scene{}, badRequest(err.Error())
	}
	if issues := validator.ValidateDraft(next); len(issues) > 0 {
		log.Printf("Scene regeneration validation failed: project_id=%s scene_id=%s issues=%s", projectID, sceneID, validationIssueSummary(issues))
		repaired, repairErr := s.generator.RepairDraft(context.Background(), next, issues)
		if repairErr != nil {
			log.Printf("Scene regeneration repair failed: project_id=%s scene_id=%s error=%s", projectID, sceneID, repairErr)
			return domain.ScriptVersion{}, domain.Scene{}, validationFailed("regenerated script validation failed and repair failed", issues)
		}
		repairedScene, found := sceneByID(repaired, sceneID)
		if !found {
			log.Printf("Scene regeneration repair missing scene: project_id=%s scene_id=%s", projectID, sceneID)
			return domain.ScriptVersion{}, domain.Scene{}, validationFailed("regenerated script validation failed", []domain.ValidationIssue{
				{Path: "scenes", Message: "repaired draft did not include requested scene"},
			})
		}
		if repairedIssues := validator.ValidateDraft(repaired); len(repairedIssues) > 0 {
			log.Printf("Scene regeneration repaired draft validation failed: project_id=%s scene_id=%s issues=%s", projectID, sceneID, validationIssueSummary(repairedIssues))
			return domain.ScriptVersion{}, domain.Scene{}, validationFailed("regenerated script validation failed", repairedIssues)
		}
		next = repaired
		scene = repairedScene
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
	log.Printf("Scene regeneration succeeded: project_id=%s scene_id=%s version_id=%s", projectID, sceneID, version.ID)
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
	log.Printf(
		"Generation job started: job_id=%s project_id=%s title=%s chapters=%d source_chars=%d target_scenes=%d model=%s structure_mode=%s",
		jobID,
		project.ID,
		project.Title,
		len(source.Chapters),
		len([]rune(source.Content)),
		config.TargetSceneCount,
		s.cfg.ModelName,
		s.cfg.ModelStructureMode,
	)
	stepDelay := time.Duration(s.cfg.JobStepDelayMS) * time.Millisecond
	step := func(status string, progress int, label string) {
		job, err := s.repo.GetJob(jobID)
		if err != nil {
			return
		}
		log.Printf("Generation job step: job_id=%s status=%s progress=%d label=%s", jobID, status, progress, label)
		job.Status = status
		job.Progress = progress
		job.CurrentStep = label
		s.repo.SaveJob(job)
		if stepDelay > 0 {
			time.Sleep(stepDelay)
		}
	}

	step("splitting", 12, "正在确认章节边界和来源引用")
	step("analyzing", 28, "StoryStructureAgent 正在提炼人物、主题和主线冲突")
	step("outlining", 45, "ScenePlannerAgent 正在规划幕结构和智能场数")
	step("generating_scenes", 68, "SceneExpansionAgent 正在逐场生成节拍、动作和对白")

	draft, err := s.generator.Generate(context.Background(), ai.GenerationInput{
		Project: project,
		Source:  source,
		Config:  config,
	})
	if err != nil {
		log.Printf("Generation job failed during LLM generation: job_id=%s project_id=%s error=%s", jobID, project.ID, err)
		s.failJob(jobID, err.Error())
		return
	}

	step("validating", 84, "正在校验 YAML Schema 和引用关系")
	if issues := validator.ValidateDraft(draft); len(issues) > 0 {
		log.Printf("Generation job validation failed before repair: job_id=%s project_id=%s issues=%s", jobID, project.ID, validationIssueSummary(issues))
		step("validating", 88, "ValidationRepairAgent 正在修复结构和引用问题")
		repaired, repairErr := s.generator.RepairDraft(context.Background(), draft, issues)
		if repairErr != nil {
			log.Printf("Generation job repair failed: job_id=%s project_id=%s error=%s", jobID, project.ID, repairErr)
			s.failJob(jobID, "generated script did not pass validation and repair failed: "+repairErr.Error())
			return
		}
		if repairedIssues := validator.ValidateDraft(repaired); len(repairedIssues) > 0 {
			log.Printf("Generation job validation failed after repair: job_id=%s project_id=%s issues=%s", jobID, project.ID, validationIssueSummary(repairedIssues))
			s.failJob(jobID, "generated script did not pass validation: "+validationIssueSummary(repairedIssues))
			return
		}
		draft = repaired
	}

	step("exporting", 94, "正在导出结构化 YAML")
	yamlText, err := exporter.ToYAML(draft)
	if err != nil {
		log.Printf("Generation job YAML export failed: job_id=%s project_id=%s error=%s", jobID, project.ID, err)
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
	log.Printf("Generation job succeeded: job_id=%s project_id=%s version_id=%s scenes=%d", jobID, project.ID, version.ID, len(draft.Scenes))
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
	log.Printf("Generation job marked failed: job_id=%s error=%s", jobID, message)
}

func validationIssueSummary(issues []domain.ValidationIssue) string {
	limit := len(issues)
	if limit > 5 {
		limit = 5
	}
	parts := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		parts = append(parts, issues[i].Path+" "+issues[i].Message)
	}
	if len(issues) > limit {
		parts = append(parts, "...")
	}
	return strings.Join(parts, "; ")
}

func sceneByID(draft domain.ScriptDraft, sceneID string) (domain.Scene, bool) {
	for _, scene := range draft.Scenes {
		if scene.ID == sceneID {
			return scene, true
		}
	}
	return domain.Scene{}, false
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

func chapterSegmentationTimeout(cfg config.Config) time.Duration {
	seconds := cfg.JobTimeoutSeconds
	if seconds <= 0 {
		seconds = cfg.ModelTimeoutSeconds * 2
	}
	if seconds <= 0 {
		seconds = 180
	}
	return time.Duration(seconds) * time.Second
}

func AsAppError(err error) (AppError, bool) {
	var appErr AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return AppError{}, false
}
