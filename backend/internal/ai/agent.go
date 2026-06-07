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

type scriptPlan struct {
	World      domain.World       `json:"world"`
	Characters []domain.Character `json:"characters"`
	Acts       []domain.Act       `json:"acts"`
	Scenes     []domain.Scene     `json:"scenes"`
	Continuity domain.Continuity  `json:"continuity"`
	Revision   domain.Revision    `json:"revision"`
}

func NewScriptAgent(generator *Generator) *ScriptAgent {
	return &ScriptAgent{generator: generator}
}

func (a *ScriptAgent) GenerateDraft(ctx context.Context, input GenerationInput) (domain.ScriptDraft, error) {
	if a.useMultiAgentPipeline() {
		log.Printf("Script agent pipeline selected: mode=%s project_id=%s chapters=%d", valueOrUnknown(a.generator.cfg.ModelAgentPipeline), input.Project.ID, len(input.Source.Chapters))
		return a.GenerateDraftMultiAgent(ctx, input)
	}

	log.Printf("Script agent step started: step=generate_draft project_id=%s chapters=%d", input.Project.ID, len(input.Source.Chapters))
	content, err := a.generator.callChatCompletion(ctx, generationMessages(input, a.generator.cfg))
	if err != nil {
		if isLengthLimitedLLMError(err) {
			log.Printf("Script agent switching to decomposed generation: project_id=%s reason=%s", input.Project.ID, err)
			return a.GenerateDraftDecomposed(ctx, input)
		}
		return domain.ScriptDraft{}, err
	}

	draft, err := a.decodeGeneratedDraft(ctx, content, input)
	if err != nil {
		if isRepairableGenerationParseError(err) {
			log.Printf("Script agent switching to decomposed generation after parse failure: project_id=%s error=%s", input.Project.ID, err)
			return a.GenerateDraftDecomposed(ctx, input)
		}
		return domain.ScriptDraft{}, err
	}
	log.Printf("Script agent step succeeded: step=generate_draft project_id=%s scenes=%d characters=%d", input.Project.ID, len(draft.Scenes), len(draft.Characters))
	return draft, nil
}

func (a *ScriptAgent) useMultiAgentPipeline() bool {
	switch strings.ToLower(strings.TrimSpace(a.generator.cfg.ModelAgentPipeline)) {
	case "multi_agent", "decomposed", "agents", "production":
		return true
	default:
		return false
	}
}

func (a *ScriptAgent) GenerateDraftDecomposed(ctx context.Context, input GenerationInput) (domain.ScriptDraft, error) {
	log.Printf("Script agent step started: agent=StoryStructureAgent step=decomposed_plan project_id=%s chapters=%d", input.Project.ID, len(input.Source.Chapters))
	content, err := a.generator.callJSONCompletion(ctx, planMessages(input, a.generator.cfg), "agent_plan_json")
	var plan scriptPlan
	if err != nil {
		log.Printf("Script agent warning: agent=StoryStructureAgent step=decomposed_plan fallback=local_scene_plan project_id=%s reason=request_failed error=%s", input.Project.ID, err)
		plan = fallbackScriptPlan(input)
	} else {
		plan, err = a.decodePlan(ctx, content, input)
		if err != nil {
			log.Printf("Script agent warning: agent=StoryStructureAgent step=decomposed_plan fallback=local_scene_plan project_id=%s reason=parse_failed error=%s", input.Project.ID, err)
			plan = fallbackScriptPlan(input)
		}
	}
	log.Printf("Script agent step succeeded: agent=StoryStructureAgent step=decomposed_plan project_id=%s scene_cards=%d characters=%d", input.Project.ID, len(plan.Scenes), len(plan.Characters))

	scenes, err := a.expandScenes(ctx, input, plan)
	if err != nil {
		return domain.ScriptDraft{}, err
	}

	draft := assembleDraftFromPlan(input, plan, scenes)
	log.Printf("Script agent step succeeded: agent=DraftAssembler step=decomposed_draft project_id=%s scenes=%d characters=%d", input.Project.ID, len(draft.Scenes), len(draft.Characters))
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

func (a *ScriptAgent) decodePlan(ctx context.Context, content string, input GenerationInput) (scriptPlan, error) {
	var plan scriptPlan
	if err := decodeModelJSON(content, &plan); err != nil {
		log.Printf("Script agent parse failed: step=decomposed_plan error=%s raw_chars=%d", err, len([]rune(content)))
		repaired, repairErr := a.repairMalformedJSON(ctx, "decomposed_plan", content, err, malformedPlanRepairMessages(input, content, err, a.generator.cfg))
		if repairErr != nil {
			return scriptPlan{}, fmt.Errorf("parse script plan: %w; malformed-json repair failed: %v", err, repairErr)
		}
		if err := decodeModelJSON(repaired, &plan); err != nil {
			return scriptPlan{}, fmt.Errorf("parse repaired script plan: %w", err)
		}
	}
	normalizePlan(&plan, input)
	return plan, nil
}

func (a *ScriptAgent) decodeScene(ctx context.Context, content string, input GenerationInput, plan scriptPlan, sceneCard domain.Scene) (domain.Scene, error) {
	var scene domain.Scene
	if err := decodeModelJSON(content, &scene); err != nil {
		log.Printf("Script agent parse failed: step=expand_scene scene_id=%s error=%s raw_chars=%d", sceneCard.ID, err, len([]rune(content)))
		repaired, repairErr := a.repairMalformedJSON(ctx, "expand_scene", content, err, malformedSceneRepairMessages(input, plan, sceneCard, content, err, a.generator.cfg))
		if repairErr != nil {
			return domain.Scene{}, fmt.Errorf("parse scene: %w; malformed-json repair failed: %v", err, repairErr)
		}
		if err := decodeModelJSON(repaired, &scene); err != nil {
			return domain.Scene{}, fmt.Errorf("parse repaired scene: %w", err)
		}
	}
	normalizeExpandedScene(&scene, sceneCard, plan)
	return scene, nil
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

func malformedSceneRepairMessages(input GenerationInput, plan scriptPlan, sceneCard domain.Scene, raw string, parseErr error, cfg config.Config) []chatMessage {
	planJSON, _ := json.MarshalIndent(plan, "", "  ")
	cardJSON, _ := json.MarshalIndent(sceneCard, "", "  ")
	userPrompt := fmt.Sprintf(`上一次模型输出的 Scene JSON 解析失败，请作为 JSON 修复 agent 修复它。

解析错误:
%s

项目: %s
小说: %s
场景卡:
%s

剧本计划:
%s

上一次原始输出:
%s

修复要求:
1. 只输出一个完整、可解析的 Scene JSON object，不要 Markdown，不要解释。
2. 必须保留场景卡中的 id, act_id, chapter_refs, characters。
3. 必须包含 location, time, purpose, conflict, summary, beats。
4. dialogues 的 speaker 必须引用 characters 中已有 ID。`,
		parseErr,
		input.Project.Title,
		input.Source.NovelTitle,
		string(cardJSON),
		truncateRunes(string(planJSON), cfg.ModelMaxInputChars/2),
		truncateRunes(raw, cfg.ModelMaxInputChars/2),
	)
	return []chatMessage{
		{Role: "system", Content: scopedAgentSystemPrompt("SceneJSONRepairAgent", "一个完整 Scene JSON object")},
		{Role: "user", Content: userPrompt},
	}
}

func malformedPlanRepairMessages(input GenerationInput, raw string, parseErr error, cfg config.Config) []chatMessage {
	maxRawChars := cfg.ModelMaxInputChars / 2
	if maxRawChars <= 0 {
		maxRawChars = 12000
	}
	userPrompt := fmt.Sprintf(`上一次模型输出的剧本计划 JSON 解析失败，请作为 JSON 修复 agent 修复它。

解析错误:
%s

项目:
- title: %s
- adaptation_target: %s
- language: %s
- novel_title: %s
- author: %s

章节原文:
%s

上一次原始输出:
%s

修复要求:
1. 只输出一个完整、可解析的剧本计划 JSON object，不要 Markdown，不要解释。
2. 顶层字段只能包含 world, characters, acts, scenes, continuity, revision。
3. scenes 是场景卡片列表，每个场景卡必须包含 id, act_id, chapter_refs, location, time, purpose, conflict, summary, characters, beats。
4. 场景卡不要写 dialogues，完整对白会由后续 scene agent 逐场生成。
5. acts[].scene_ids 必须引用 scenes[].id。`,
		parseErr,
		input.Project.Title,
		input.Project.AdaptationTarget,
		input.Project.Language,
		input.Source.NovelTitle,
		input.Source.Author,
		buildChapterContext(input.Source.Chapters, cfg.ModelMaxInputChars/2),
		truncateRunes(raw, maxRawChars),
	)
	return []chatMessage{
		{Role: "system", Content: scopedAgentSystemPrompt("PlanJSONRepairAgent", "剧本计划 JSON object，顶层只能包含 world, characters, acts, scenes, continuity, revision")},
		{Role: "user", Content: userPrompt},
	}
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

func planMessages(input GenerationInput, cfg config.Config) []chatMessage {
	sceneTarget := sceneTargetForInput(input, cfg, "StoryStructureAgent")
	userPrompt := fmt.Sprintf(`请把小说章节改编为剧本生成计划 JSON。这个计划只做结构规划，不写完整对白。

项目:
- title: %s
- adaptation_target: %s
- language: %s
- novel_title: %s
- author: %s

生成偏好:
- style: %s
- target_scene_count: %s
- dialogue_density: %s
- preserve_original_names: %t

章节原文:
%s

输出要求:
1. 只输出 JSON object，不要 Markdown。
2. 顶层字段只能包含 world, characters, acts, scenes, continuity, revision。
3. scenes 是场景卡片列表，每个场景卡必须包含 id, act_id, chapter_refs, location, time, purpose, conflict, summary, characters, beats。
4. 场景卡不要写 dialogues，完整对白会由后续 scene agent 逐场生成。
5. characters[].id 使用 c01、c02；acts[].id 使用 act1；scenes[].id 使用 s01。
6. acts[].scene_ids 必须引用 scenes[].id。`,
		input.Project.Title,
		input.Project.AdaptationTarget,
		input.Project.Language,
		input.Source.NovelTitle,
		input.Source.Author,
		valueOrDefault(input.Config.Style, "影视化、可表演"),
		sceneTarget,
		valueOrDefault(input.Config.DialogueDensity, "medium"),
		input.Config.PreserveOriginalNames,
		buildChapterContext(input.Source.Chapters, cfg.ModelMaxInputChars),
	)
	return []chatMessage{
		{Role: "system", Content: scopedAgentSystemPrompt("StoryStructureAgent", "剧本计划 JSON object，顶层只能包含 world, characters, acts, scenes, continuity, revision")},
		{Role: "user", Content: userPrompt},
	}
}

func sceneExpansionMessages(input GenerationInput, plan scriptPlan, sceneCard domain.Scene, cfg config.Config) []chatMessage {
	planJSON, _ := json.MarshalIndent(plan, "", "  ")
	cardJSON, _ := json.MarshalIndent(sceneCard, "", "  ")
	userPrompt := fmt.Sprintf(`请根据剧本计划和指定场景卡，生成一个完整 Scene JSON object。

项目: %s
小说: %s
风格: %s
对白密度: %s

指定场景卡:
%s

完整剧本计划:
%s

相关章节原文:
%s

输出要求:
1. 只输出一个 Scene JSON object，不要 Markdown。
2. 必须保留场景卡中的 id, act_id, chapter_refs, characters。
3. 必须包含 location, time, purpose, conflict, summary, beats。
4. 根据对白密度补充 dialogues，每句 speaker 必须引用 characters 中已有 ID。
5. 可补充 notes 和 ai_assumptions，但不要新增无法追溯的核心剧情。`,
		input.Project.Title,
		input.Source.NovelTitle,
		valueOrDefault(input.Config.Style, "影视化、可表演"),
		valueOrDefault(input.Config.DialogueDensity, "medium"),
		string(cardJSON),
		truncateRunes(string(planJSON), cfg.ModelMaxInputChars/3),
		chapterContextForRefs(input.Source.Chapters, sceneCard.ChapterRefs, cfg.ModelMaxInputChars/2),
	)
	return []chatMessage{
		{Role: "system", Content: scopedAgentSystemPrompt("SceneExpansionAgent", "一个完整 Scene JSON object")},
		{Role: "user", Content: userPrompt},
	}
}

func sceneBatchExpansionMessages(input GenerationInput, plan scriptPlan, sceneCards []domain.Scene, cfg config.Config) []chatMessage {
	planJSON, _ := json.MarshalIndent(plan, "", "  ")
	cardsJSON, _ := json.MarshalIndent(sceneCards, "", "  ")
	userPrompt := fmt.Sprintf(`请根据剧本计划和一组场景卡，批量生成完整 Scene JSON objects。

项目: %s
小说: %s
风格: %s
对白密度: %s

场景卡数组:
%s

完整剧本计划:
%s

相关章节原文:
%s

输出要求:
1. 只输出 JSON object，不要 Markdown。
2. 顶层字段只能包含 scenes。
3. scenes 必须是数组，并为每个输入场景卡返回一个完整 Scene object。
4. 每个 Scene 必须保留场景卡中的 id, act_id, chapter_refs, characters。
5. 每个 Scene 必须包含 location, time, purpose, conflict, summary, beats。
6. 根据对白密度补充 dialogues，每句 speaker 必须引用 characters 中已有 ID。
7. 不要输出 schema_version、project、source、world、完整 ScriptDraft 或 YAML。`,
		input.Project.Title,
		input.Source.NovelTitle,
		valueOrDefault(input.Config.Style, "影视化、可表演"),
		valueOrDefault(input.Config.DialogueDensity, "medium"),
		string(cardsJSON),
		truncateRunes(string(planJSON), cfg.ModelMaxInputChars/3),
		chapterContextForRefs(input.Source.Chapters, sceneRefsFromCards(sceneCards), cfg.ModelMaxInputChars/2),
	)
	return []chatMessage{
		{Role: "system", Content: scopedAgentSystemPrompt("SceneExpansionAgent", "场景批次 JSON object，顶层只能包含 scenes")},
		{Role: "user", Content: userPrompt},
	}
}

func normalizePlan(plan *scriptPlan, input GenerationInput) {
	if strings.TrimSpace(plan.World.Tone) == "" {
		plan.World.Tone = valueOrDefault(input.Config.Style, "影视化、可表演")
	}
	if len(plan.Acts) == 0 {
		plan.Acts = []domain.Act{{ID: "act1", Title: "第一幕", Purpose: "承接原小说主要冲突"}}
	}
	for i := range plan.Scenes {
		if strings.TrimSpace(plan.Scenes[i].ID) == "" {
			plan.Scenes[i].ID = fmt.Sprintf("s%02d", i+1)
		}
		if strings.TrimSpace(plan.Scenes[i].ActID) == "" {
			plan.Scenes[i].ActID = plan.Acts[0].ID
		}
		if len(plan.Scenes[i].ChapterRefs) == 0 {
			plan.Scenes[i].ChapterRefs = []int{input.Source.Chapters[minInt(i, len(input.Source.Chapters)-1)].Index}
		}
	}
	rebuildActSceneRefs(plan)
	if strings.TrimSpace(plan.Revision.Confidence) == "" {
		plan.Revision.Confidence = "medium"
	}
}

func normalizeExpandedScene(scene *domain.Scene, sceneCard domain.Scene, plan scriptPlan) {
	if strings.TrimSpace(scene.ID) == "" {
		scene.ID = sceneCard.ID
	}
	if strings.TrimSpace(scene.ActID) == "" {
		scene.ActID = sceneCard.ActID
	}
	if len(scene.ChapterRefs) == 0 {
		scene.ChapterRefs = sceneCard.ChapterRefs
	}
	if len(scene.Characters) == 0 {
		scene.Characters = sceneCard.Characters
	}
	if strings.TrimSpace(scene.Location) == "" {
		scene.Location = sceneCard.Location
	}
	if strings.TrimSpace(scene.Time) == "" {
		scene.Time = sceneCard.Time
	}
	if strings.TrimSpace(scene.Purpose) == "" {
		scene.Purpose = sceneCard.Purpose
	}
	if strings.TrimSpace(scene.Conflict) == "" {
		scene.Conflict = sceneCard.Conflict
	}
	if strings.TrimSpace(scene.Summary) == "" {
		scene.Summary = sceneCard.Summary
	}
	if len(scene.Beats) == 0 {
		scene.Beats = sceneCard.Beats
	}
	if len(scene.Characters) == 0 && len(plan.Characters) > 0 {
		scene.Characters = []string{plan.Characters[0].ID}
	}
}

func normalizeSceneBatch(scenes []domain.Scene, sceneCards []domain.Scene, plan scriptPlan) []domain.Scene {
	sceneByID := make(map[string]domain.Scene, len(scenes))
	for _, scene := range scenes {
		if strings.TrimSpace(scene.ID) == "" {
			continue
		}
		if _, exists := sceneByID[scene.ID]; exists {
			log.Printf("Script agent warning: agent=SceneExpansionAgent duplicate_scene scene_id=%s ignored=true", scene.ID)
			continue
		}
		sceneByID[scene.ID] = scene
	}

	normalized := make([]domain.Scene, 0, len(sceneCards))
	for _, card := range sceneCards {
		scene, ok := sceneByID[card.ID]
		if !ok {
			scene = card
			scene.Notes = append(scene.Notes, "SceneExpansionAgent 未返回该场景，后端使用场景卡兜底。")
			log.Printf("Script agent warning: agent=SceneExpansionAgent missing_scene scene_id=%s fallback=scene_card", card.ID)
		}
		normalizeExpandedScene(&scene, card, plan)
		normalized = append(normalized, scene)
	}
	return normalized
}

func assembleDraftFromPlan(input GenerationInput, plan scriptPlan, scenes []domain.Scene) domain.ScriptDraft {
	draft := domain.ScriptDraft{
		SchemaVersion: "1.0",
		Project: domain.ScriptProject{
			Title:            input.Project.Title,
			AdaptationTarget: input.Project.AdaptationTarget,
			Language:         input.Project.Language,
			Genre:            plan.ProjectGenres(),
		},
		Source: domain.ScriptSource{
			NovelTitle:   input.Source.NovelTitle,
			Author:       input.Source.Author,
			ChapterCount: len(input.Source.Chapters),
			ChapterRefs:  chapterRefs(input.Source.Chapters),
		},
		World:      plan.World,
		Characters: plan.Characters,
		Acts:       plan.Acts,
		Scenes:     scenes,
		Continuity: plan.Continuity,
		Revision:   plan.Revision,
	}
	normalizeGeneratedDraft(&draft, input)
	return draft
}

func (plan scriptPlan) ProjectGenres() []string {
	if len(plan.World.Theme) == 0 {
		return nil
	}
	return []string{"AI 改编"}
}

func rebuildActSceneRefs(plan *scriptPlan) {
	sceneIDsByAct := map[string][]string{}
	for _, scene := range plan.Scenes {
		sceneIDsByAct[scene.ActID] = append(sceneIDsByAct[scene.ActID], scene.ID)
	}
	for i := range plan.Acts {
		if refs := sceneIDsByAct[plan.Acts[i].ID]; len(refs) > 0 {
			plan.Acts[i].SceneIDs = refs
		}
	}
}

func sceneRefsFromCards(sceneCards []domain.Scene) []int {
	seen := map[int]bool{}
	var refs []int
	for _, card := range sceneCards {
		for _, ref := range card.ChapterRefs {
			if !seen[ref] {
				seen[ref] = true
				refs = append(refs, ref)
			}
		}
	}
	return refs
}

func sceneBatchExpansionRepairContext(input GenerationInput, plan scriptPlan, sceneCards []domain.Scene, cfg config.Config) string {
	planJSON, _ := json.MarshalIndent(plan, "", "  ")
	cardsJSON, _ := json.MarshalIndent(sceneCards, "", "  ")
	return fmt.Sprintf("项目: %s\n场景范围: %s\n场景卡:\n%s\n剧本计划:\n%s\n相关章节:\n%s",
		input.Project.Title,
		sceneBatchRangeLabel(sceneCards),
		string(cardsJSON),
		truncateRunes(string(planJSON), agentInputBudget(cfg, 3)),
		chapterContextForRefs(input.Source.Chapters, sceneRefsFromCards(sceneCards), agentInputBudget(cfg, 3)),
	)
}

func chapterContextForRefs(chapters []domain.Chapter, refs []int, maxChars int) string {
	if len(refs) == 0 {
		return buildChapterContext(chapters, maxChars)
	}
	refSet := map[int]bool{}
	for _, ref := range refs {
		refSet[ref] = true
	}
	var selected []domain.Chapter
	for _, chapter := range chapters {
		if refSet[chapter.Index] {
			selected = append(selected, chapter)
		}
	}
	if len(selected) == 0 {
		selected = chapters
	}
	return buildChapterContext(selected, maxChars)
}

func isLengthLimitedLLMError(err error) bool {
	text := err.Error()
	return strings.Contains(text, "finish_reason=length") || strings.Contains(text, "finish_reason=max_tokens")
}

func isRepairableGenerationParseError(err error) bool {
	text := err.Error()
	return strings.Contains(text, "unexpected EOF") || strings.Contains(text, "repair failed")
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
