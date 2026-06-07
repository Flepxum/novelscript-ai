package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/Flepxum/novelscript-ai/backend/internal/config"
	"github.com/Flepxum/novelscript-ai/backend/internal/domain"
)

type ScriptAgent struct {
	generator *Generator
}

func NewScriptAgent(generator *Generator) *ScriptAgent {
	return &ScriptAgent{generator: generator}
}

func (a *ScriptAgent) GenerateDraft(ctx context.Context, input GenerationInput) (domain.ScriptDraft, error) {
	log.Printf("Script agent step started: step=generate_draft project_id=%s chapters=%d", input.Project.ID, len(input.Source.Chapters))
	content, err := a.generator.callChatCompletion(ctx, generationMessages(input, a.generator.cfg))
	if err != nil {
		return domain.ScriptDraft{}, err
	}

	draft, err := a.decodeGeneratedDraft(ctx, content, input)
	if err != nil {
		return domain.ScriptDraft{}, err
	}
	log.Printf("Script agent step succeeded: step=generate_draft project_id=%s scenes=%d characters=%d", input.Project.ID, len(draft.Scenes), len(draft.Characters))
	return draft, nil
}

func (a *ScriptAgent) RegenerateScene(ctx context.Context, draft domain.ScriptDraft, sceneID string, instruction string) (domain.ScriptDraft, domain.Scene, error) {
	currentScene, ok := findScene(draft, sceneID)
	if !ok {
		return draft, domain.Scene{}, fmt.Errorf("scene %s not found", sceneID)
	}

	log.Printf("Script agent step started: step=regenerate_scene scene_id=%s instruction_chars=%d", sceneID, len([]rune(instruction)))
	content, err := a.generator.callChatCompletion(ctx, regenerateSceneMessages(draft, currentScene, strings.TrimSpace(instruction), a.generator.cfg))
	if err != nil {
		return draft, domain.Scene{}, err
	}

	next, err := a.decodeRegeneratedDraft(ctx, content, draft)
	if err != nil {
		return draft, domain.Scene{}, err
	}

	updatedScene, ok := findScene(next, sceneID)
	if !ok {
		return draft, domain.Scene{}, fmt.Errorf("llm response did not include scene %s", sceneID)
	}
	log.Printf("Script agent step succeeded: step=regenerate_scene scene_id=%s scenes=%d", sceneID, len(next.Scenes))
	return next, updatedScene, nil
}

func (a *ScriptAgent) RepairDraft(ctx context.Context, draft domain.ScriptDraft, issues []domain.ValidationIssue) (domain.ScriptDraft, error) {
	log.Printf("Script agent step started: step=repair_validation issues=%d", len(issues))
	content, err := a.generator.callChatCompletion(ctx, repairDraftMessages(draft, issues))
	if err != nil {
		return domain.ScriptDraft{}, err
	}

	next, err := a.decodeValidationRepairDraft(ctx, content, draft, issues)
	if err != nil {
		return domain.ScriptDraft{}, err
	}
	log.Printf("Script agent step succeeded: step=repair_validation scenes=%d characters=%d", len(next.Scenes), len(next.Characters))
	return next, nil
}

func (a *ScriptAgent) decodeGeneratedDraft(ctx context.Context, content string, input GenerationInput) (domain.ScriptDraft, error) {
	var draft domain.ScriptDraft
	if err := decodeModelJSON(content, &draft); err != nil {
		log.Printf("Script agent parse failed: step=generate_draft error=%s raw_chars=%d", err, len([]rune(content)))
		repaired, repairErr := a.repairMalformedJSON(ctx, "generate_draft", content, err, malformedGenerationRepairMessages(input, content, err, a.generator.cfg))
		if repairErr != nil {
			return domain.ScriptDraft{}, fmt.Errorf("parse llm script draft: %w; malformed-json repair failed: %v", err, repairErr)
		}
		if err := decodeModelJSON(repaired, &draft); err != nil {
			return domain.ScriptDraft{}, fmt.Errorf("parse repaired llm script draft: %w", err)
		}
	}
	normalizeGeneratedDraft(&draft, input)
	return draft, nil
}

func (a *ScriptAgent) decodeRegeneratedDraft(ctx context.Context, content string, current domain.ScriptDraft) (domain.ScriptDraft, error) {
	var next domain.ScriptDraft
	if err := decodeModelJSON(content, &next); err != nil {
		log.Printf("Script agent parse failed: step=regenerate_scene error=%s raw_chars=%d", err, len([]rune(content)))
		repaired, repairErr := a.repairMalformedJSON(ctx, "regenerate_scene", content, err, malformedDraftRepairMessages(current, content, err, a.generator.cfg))
		if repairErr != nil {
			return domain.ScriptDraft{}, fmt.Errorf("parse llm regenerated script draft: %w; malformed-json repair failed: %v", err, repairErr)
		}
		if err := decodeModelJSON(repaired, &next); err != nil {
			return domain.ScriptDraft{}, fmt.Errorf("parse repaired llm regenerated script draft: %w", err)
		}
	}
	normalizeRegeneratedDraft(&next, current)
	return next, nil
}

func (a *ScriptAgent) decodeValidationRepairDraft(ctx context.Context, content string, current domain.ScriptDraft, issues []domain.ValidationIssue) (domain.ScriptDraft, error) {
	var next domain.ScriptDraft
	if err := decodeModelJSON(content, &next); err != nil {
		log.Printf("Script agent parse failed: step=repair_validation error=%s raw_chars=%d", err, len([]rune(content)))
		repaired, repairErr := a.repairMalformedJSON(ctx, "repair_validation", content, err, malformedValidationRepairMessages(current, issues, content, err, a.generator.cfg))
		if repairErr != nil {
			return domain.ScriptDraft{}, fmt.Errorf("parse llm repaired script draft: %w; malformed-json repair failed: %v", err, repairErr)
		}
		if err := decodeModelJSON(repaired, &next); err != nil {
			return domain.ScriptDraft{}, fmt.Errorf("parse repaired validation script draft: %w", err)
		}
	}
	normalizeRegeneratedDraft(&next, current)
	return next, nil
}

func (a *ScriptAgent) repairMalformedJSON(ctx context.Context, step string, raw string, parseErr error, messages []chatMessage) (string, error) {
	log.Printf("Script agent step started: step=repair_malformed_json source_step=%s parse_error=%s raw_chars=%d", step, parseErr, len([]rune(raw)))
	repaired, err := a.generator.callChatCompletion(ctx, messages)
	if err != nil {
		log.Printf("Script agent malformed JSON repair failed: source_step=%s error=%s", step, err)
		return "", err
	}
	log.Printf("Script agent malformed JSON repair response received: source_step=%s chars=%d", step, len([]rune(repaired)))
	return repaired, nil
}

func malformedGenerationRepairMessages(input GenerationInput, raw string, parseErr error, cfg config.Config) []chatMessage {
	context := fmt.Sprintf(`项目:
- title: %s
- adaptation_target: %s
- language: %s
- novel_title: %s
- author: %s

章节原文:
%s`,
		input.Project.Title,
		input.Project.AdaptationTarget,
		input.Project.Language,
		input.Source.NovelTitle,
		input.Source.Author,
		buildChapterContext(input.Source.Chapters, cfg.ModelMaxInputChars/2),
	)
	return malformedJSONRepairMessages(context, raw, parseErr, cfg)
}

func malformedDraftRepairMessages(current domain.ScriptDraft, raw string, parseErr error, cfg config.Config) []chatMessage {
	currentJSON, _ := json.MarshalIndent(current, "", "  ")
	return malformedJSONRepairMessages("当前完整 ScriptDraft:\n"+truncateRunes(string(currentJSON), cfg.ModelMaxInputChars/2), raw, parseErr, cfg)
}

func malformedValidationRepairMessages(current domain.ScriptDraft, issues []domain.ValidationIssue, raw string, parseErr error, cfg config.Config) []chatMessage {
	currentJSON, _ := json.MarshalIndent(current, "", "  ")
	issuesJSON, _ := json.MarshalIndent(issues, "", "  ")
	context := fmt.Sprintf("校验问题:\n%s\n\n当前完整 ScriptDraft:\n%s", string(issuesJSON), truncateRunes(string(currentJSON), cfg.ModelMaxInputChars/2))
	return malformedJSONRepairMessages(context, raw, parseErr, cfg)
}

func malformedJSONRepairMessages(context string, raw string, parseErr error, cfg config.Config) []chatMessage {
	maxRawChars := cfg.ModelMaxInputChars / 2
	if maxRawChars <= 0 {
		maxRawChars = 12000
	}
	userPrompt := fmt.Sprintf(`上一次模型输出的 ScriptDraft JSON 解析失败，请作为 JSON 修复 agent 修复它。

解析错误:
%s

可用上下文:
%s

上一次原始输出:
%s

修复要求:
1. 只输出一个完整、可解析的 ScriptDraft JSON object，不要 Markdown，不要解释。
2. 如果上一次输出被截断，请根据上下文补齐缺失的闭合结构和必要字段。
3. 顶层字段必须包含 schema_version, project, source, world, characters, acts, scenes, continuity, revision。
4. 保持 ID 规则：characters 使用 c01，acts 使用 act1，scenes 使用 s01。
5. 输出尽量紧凑，避免无关长文本，优先保证 JSON 完整和引用正确。`,
		parseErr,
		context,
		truncateRunes(raw, maxRawChars),
	)

	return []chatMessage{
		{Role: "system", Content: scriptAgentSystemPrompt()},
		{Role: "user", Content: userPrompt},
	}
}
