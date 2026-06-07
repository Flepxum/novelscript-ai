package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Flepxum/novelscript-ai/backend/internal/config"
	"github.com/Flepxum/novelscript-ai/backend/internal/domain"
)

type GenerationInput struct {
	Project domain.Project
	Source  domain.SourceDocument
	Config  domain.GenerationConfig
}

type Generator struct {
	cfg config.Config
}

func NewGenerator(cfg config.Config) *Generator {
	return &Generator{cfg: cfg}
}

func (g *Generator) Generate(ctx context.Context, input GenerationInput) (domain.ScriptDraft, error) {
	return NewScriptAgent(g).GenerateDraft(ctx, input)
}

func (g *Generator) RegenerateScene(ctx context.Context, draft domain.ScriptDraft, sceneID string, instruction string) (domain.ScriptDraft, domain.Scene, error) {
	return NewScriptAgent(g).RegenerateScene(ctx, draft, sceneID, instruction)
}

func (g *Generator) RepairDraft(ctx context.Context, draft domain.ScriptDraft, issues []domain.ValidationIssue) (domain.ScriptDraft, error) {
	return NewScriptAgent(g).RepairDraft(ctx, draft, issues)
}

func generationMessages(input GenerationInput, cfg config.Config) []chatMessage {
	sceneTarget := sceneTargetForInput(input, cfg, "ScriptDraftAgent")

	userPrompt := fmt.Sprintf(`请把小说章节改编为结构化剧本 JSON。

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
1. 只输出一个 JSON object，不要 Markdown，不要解释文字。
2. JSON 顶层字段必须是 schema_version, project, source, world, characters, acts, scenes, continuity, revision。
3. schema_version 必须是 "1.0"。
4. character.id 使用 c01、c02 递增；act.id 使用 act1、act2；scene.id 使用 s01、s02 递增。
5. scenes[].chapter_refs 只能引用输入章节编号；scenes[].characters 只能引用 characters[].id；acts[].scene_ids 必须引用存在的场景。
6. 改编要保留原小说人物、关键事件和因果关系；需要合理推断时写入 ai_assumptions。
7. 每场戏必须包含可表演的 purpose、conflict、summary、beats，并根据 dialogue_density 生成对白。
8. revision.editor_notes 写给作者的可执行打磨建议，不能写系统实现说明。`,
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
		{Role: "system", Content: scriptAgentSystemPrompt()},
		{Role: "user", Content: userPrompt},
	}
}

func regenerateSceneMessages(draft domain.ScriptDraft, scene domain.Scene, instruction string, cfg config.Config) []chatMessage {
	if instruction == "" {
		instruction = "提升场景目标、冲突推进和对白可表演性，同时保持剧情连续性。"
	}
	draftJSON, _ := json.MarshalIndent(draft, "", "  ")
	sceneJSON, _ := json.MarshalIndent(scene, "", "  ")
	if maxChars := cfg.ModelMaxInputChars; maxChars > 0 {
		draftJSON = []byte(truncateRunes(string(draftJSON), maxChars))
	}

	userPrompt := fmt.Sprintf(`请基于当前剧本 JSON 重写指定场景，并返回完整的更新后剧本 JSON。

重写指令:
%s

需要重写的场景:
%s

当前完整剧本:
%s

输出要求:
1. 只输出完整 ScriptDraft JSON object，不要 Markdown，不要解释文字。
2. 保持 schema_version、project、source、characters、acts、continuity 的连续性。
3. 必须保留 scene_id=%s，更新该场景的 purpose、conflict、summary、beats、dialogues、notes 或 ai_assumptions。
4. 不要删除其他场景，不要破坏 acts[].scene_ids、scenes[].characters、chapter_refs 的引用。`,
		instruction,
		string(sceneJSON),
		string(draftJSON),
		scene.ID,
	)

	return []chatMessage{
		{Role: "system", Content: scriptAgentSystemPrompt()},
		{Role: "user", Content: userPrompt},
	}
}

func repairDraftMessages(draft domain.ScriptDraft, issues []domain.ValidationIssue) []chatMessage {
	draftJSON, _ := json.MarshalIndent(draft, "", "  ")
	issuesJSON, _ := json.MarshalIndent(issues, "", "  ")

	userPrompt := fmt.Sprintf(`请修复以下 ScriptDraft JSON，使其通过后端校验，并返回完整的修复后 JSON。

校验问题:
%s

当前 ScriptDraft:
%s

修复要求:
1. 只输出完整 ScriptDraft JSON object，不要 Markdown，不要解释文字。
2. 不要删除有效剧情内容，只修复缺失字段、错误引用、重复 ID、空数组和不一致的 acts/scenes/characters/chapter_refs。
3. characters[].id 必须被 scenes[].characters 正确引用；scenes[].id 必须被 acts[].scene_ids 正确引用。
4. scenes[].chapter_refs 必须引用 source.chapter_refs 中存在的章节。`,
		string(issuesJSON),
		string(draftJSON),
	)

	return []chatMessage{
		{Role: "system", Content: scriptAgentSystemPrompt()},
		{Role: "user", Content: userPrompt},
	}
}

func scriptAgentSystemPrompt() string {
	return `你是 NovelScript ScriptDraft Agent，一个专业的小说改编剧本助理。当前任务只允许输出完整 ScriptDraft JSON。你遵守用户给出的 Schema 字段、ID 规则和引用规则。你不会编造无法从原文或合理推断中支撑的核心剧情。你的输出必须是严格 JSON，不能包含 Markdown、注释、寒暄或解释。`
}

func scopedAgentSystemPrompt(agentName string, outputContract string) string {
	return fmt.Sprintf(`你是 NovelScript %s。当前任务只允许输出 %s。你必须服从用户消息里的阶段边界和字段契约，不要输出其他阶段的数据。严禁输出 schema_version、完整 ScriptDraft、YAML、Markdown、注释、寒暄或解释，除非用户消息明确要求完整 ScriptDraft。`, agentName, outputContract)
}

func decodeModelJSON(content string, target any) error {
	jsonText, err := extractJSONObject(content)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(jsonText))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func extractJSONObject(content string) (string, error) {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```JSON")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end < start {
		return "", fmt.Errorf("llm response did not contain a JSON object")
	}
	return content[start : end+1], nil
}

func normalizeGeneratedDraft(draft *domain.ScriptDraft, input GenerationInput) {
	draft.SchemaVersion = "1.0"
	draft.Project.Title = input.Project.Title
	draft.Project.AdaptationTarget = input.Project.AdaptationTarget
	draft.Project.Language = input.Project.Language
	draft.Source.NovelTitle = input.Source.NovelTitle
	draft.Source.Author = input.Source.Author
	draft.Source.ChapterCount = len(input.Source.Chapters)
	draft.Source.ChapterRefs = chapterRefs(input.Source.Chapters)
	if strings.TrimSpace(draft.Revision.GeneratedAt) == "" {
		draft.Revision.GeneratedAt = time.Now().Format(time.RFC3339)
	}
	if strings.TrimSpace(draft.Revision.Confidence) == "" {
		draft.Revision.Confidence = "medium"
	}
}

func normalizeRegeneratedDraft(next *domain.ScriptDraft, current domain.ScriptDraft) {
	if strings.TrimSpace(next.SchemaVersion) == "" {
		next.SchemaVersion = current.SchemaVersion
	}
	if strings.TrimSpace(next.SchemaVersion) == "" {
		next.SchemaVersion = "1.0"
	}
	if strings.TrimSpace(next.Project.Title) == "" {
		next.Project.Title = current.Project.Title
	}
	if strings.TrimSpace(next.Project.AdaptationTarget) == "" {
		next.Project.AdaptationTarget = current.Project.AdaptationTarget
	}
	if strings.TrimSpace(next.Project.Language) == "" {
		next.Project.Language = current.Project.Language
	}
	if len(next.Project.Genre) == 0 {
		next.Project.Genre = current.Project.Genre
	}
	if next.Source.ChapterCount == 0 {
		next.Source.ChapterCount = current.Source.ChapterCount
	}
	if len(next.Source.ChapterRefs) == 0 {
		next.Source.ChapterRefs = current.Source.ChapterRefs
	}
	if strings.TrimSpace(next.Source.NovelTitle) == "" {
		next.Source.NovelTitle = current.Source.NovelTitle
	}
	if strings.TrimSpace(next.Source.Author) == "" {
		next.Source.Author = current.Source.Author
	}
	if strings.TrimSpace(next.Revision.GeneratedAt) == "" {
		next.Revision.GeneratedAt = time.Now().Format(time.RFC3339)
	}
	if strings.TrimSpace(next.Revision.Confidence) == "" {
		next.Revision.Confidence = "medium"
	}
}

func buildChapterContext(chapters []domain.Chapter, maxChars int) string {
	var builder strings.Builder
	for _, chapter := range chapters {
		builder.WriteString(fmt.Sprintf("\n[chapter %d] %s\n", chapter.Index, chapter.Title))
		builder.WriteString(strings.TrimSpace(chapter.Content))
		builder.WriteString("\n")
	}
	if maxChars <= 0 {
		maxChars = 24000
	}
	return truncateRunes(builder.String(), maxChars)
}

func sourceTextCharCount(chapters []domain.Chapter) int {
	total := 0
	for _, chapter := range chapters {
		total += len([]rune(strings.TrimSpace(chapter.Content)))
	}
	return total
}

func sceneTargetForInput(input GenerationInput, cfg config.Config, agentName string) string {
	if input.Config.TargetSceneCount > 0 {
		return fmt.Sprintf("%d", input.Config.TargetSceneCount)
	}
	if shouldUseFastPath(input, cfg) {
		chapterCount := len(input.Source.Chapters)
		minScenes := 3
		maxScenes := chapterCount
		if maxScenes < minScenes {
			maxScenes = minScenes
		}
		if maxScenes > 5 {
			maxScenes = 5
		}
		return fmt.Sprintf("%d-%d（短篇快速初稿，合并弱冲突段落，避免过度拆场）", minScenes, maxScenes)
	}
	return fmt.Sprintf("由 %s 根据章节密度、冲突强度和改编目标决定", agentName)
}

func shouldUseFastPath(input GenerationInput, cfg config.Config) bool {
	maxChars := cfg.ModelFastPathMaxChars
	if maxChars <= 0 {
		return false
	}
	if len(input.Source.Chapters) == 0 {
		return false
	}
	return sourceTextCharCount(input.Source.Chapters) <= maxChars
}

func truncateRunes(input string, maxChars int) string {
	if maxChars <= 0 {
		return input
	}
	runes := []rune(input)
	if len(runes) <= maxChars {
		return input
	}
	return string(runes[:maxChars]) + "\n\n[输入已按 MODEL_MAX_INPUT_CHARS 截断，请只基于可见内容生成。]"
}

func chapterRefs(chapters []domain.Chapter) []int {
	refs := make([]int, len(chapters))
	for i, chapter := range chapters {
		refs[i] = chapter.Index
	}
	return refs
}

func findScene(draft domain.ScriptDraft, sceneID string) (domain.Scene, bool) {
	for _, scene := range draft.Scenes {
		if scene.ID == sceneID {
			return scene, true
		}
	}
	return domain.Scene{}, false
}

func valueOrDefault(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
