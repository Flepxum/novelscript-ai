package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/Flepxum/novelscript-ai/backend/internal/config"
	"github.com/Flepxum/novelscript-ai/backend/internal/domain"
)

type chapterBrief struct {
	ChapterIndex    int      `json:"chapter_index"`
	Title           string   `json:"title"`
	Summary         string   `json:"summary"`
	KeyEvents       []string `json:"key_events"`
	Characters      []string `json:"characters"`
	Locations       []string `json:"locations"`
	Timeline        []string `json:"timeline"`
	Conflicts       []string `json:"conflicts"`
	AdaptationHints []string `json:"adaptation_hints,omitempty"`
	OpenQuestions   []string `json:"open_questions,omitempty"`
}

type chapterBriefBatch struct {
	Briefs []chapterBrief `json:"briefs"`
}

type storyBible struct {
	World      domain.World       `json:"world"`
	Characters []domain.Character `json:"characters"`
	Continuity domain.Continuity  `json:"continuity"`
}

type scenePlan struct {
	Acts     []domain.Act    `json:"acts"`
	Scenes   []domain.Scene  `json:"scenes"`
	Revision domain.Revision `json:"revision"`
}

type sceneDraftBatch struct {
	Scenes []domain.Scene `json:"scenes"`
}

type sceneExpansionBatch struct {
	StartIndex int
	Cards      []domain.Scene
}

func (a *ScriptAgent) GenerateDraftMultiAgent(ctx context.Context, input GenerationInput) (domain.ScriptDraft, error) {
	if shouldUseFastPath(input, a.generator.cfg) {
		log.Printf(
			"Script agent pipeline selected: mode=fast_path project_id=%s chapters=%d source_chars=%d max_chars=%d",
			input.Project.ID,
			len(input.Source.Chapters),
			sourceTextCharCount(input.Source.Chapters),
			a.generator.cfg.ModelFastPathMaxChars,
		)
		return a.GenerateDraftDecomposed(ctx, input)
	}

	batchCount := len(chapterBatches(input.Source.Chapters, a.generator.cfg.ModelChapterAnalysisBatchSize))
	log.Printf(
		"Script agent pipeline started: mode=multi_agent project_id=%s chapters=%d chapter_batch_size=%d chapter_batches=%d max_parallel=%d",
		input.Project.ID,
		len(input.Source.Chapters),
		chapterAnalysisBatchSize(a.generator.cfg.ModelChapterAnalysisBatchSize, len(input.Source.Chapters)),
		batchCount,
		agentParallelism(a.generator.cfg.JobMaxParallel, batchCount),
	)

	briefs, err := a.analyzeChapters(ctx, input)
	if err != nil {
		return domain.ScriptDraft{}, err
	}
	bible, err := a.buildStoryBible(ctx, input, briefs)
	if err != nil {
		return domain.ScriptDraft{}, err
	}
	planned, err := a.planScenes(ctx, input, briefs, bible)
	if err != nil {
		return domain.ScriptDraft{}, err
	}

	plan := scriptPlan{
		World:      bible.World,
		Characters: bible.Characters,
		Acts:       planned.Acts,
		Scenes:     planned.Scenes,
		Continuity: bible.Continuity,
		Revision:   planned.Revision,
	}
	normalizePlan(&plan, input)

	scenes, err := a.expandScenes(ctx, input, plan)
	if err != nil {
		return domain.ScriptDraft{}, err
	}
	draft := assembleDraftFromPlan(input, plan, scenes)
	log.Printf(
		"Script agent pipeline succeeded: mode=multi_agent project_id=%s chapter_briefs=%d scenes=%d characters=%d",
		input.Project.ID,
		len(briefs),
		len(draft.Scenes),
		len(draft.Characters),
	)
	return draft, nil
}

func (a *ScriptAgent) analyzeChapters(ctx context.Context, input GenerationInput) ([]chapterBrief, error) {
	if len(input.Source.Chapters) == 0 {
		return nil, fmt.Errorf("source chapters are required")
	}

	batches := chapterBatches(input.Source.Chapters, a.generator.cfg.ModelChapterAnalysisBatchSize)
	parallelism := agentParallelism(a.generator.cfg.JobMaxParallel, len(batches))
	results := make([][]chapterBrief, len(batches))
	errCh := make(chan error, len(batches))
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup

	for batchIndex, batch := range batches {
		wg.Add(1)
		go func(batchIndex int, batch []domain.Chapter) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			log.Printf(
				"Script agent step started: agent=ChapterAnalysisAgent step=analyze_chapter_batch project_id=%s batch=%d/%d chapters=%s",
				input.Project.ID,
				batchIndex+1,
				len(batches),
				chapterRangeLabel(batch),
			)
			content, err := a.generator.callJSONCompletion(ctx, chapterBatchAnalysisMessages(input, batch, a.generator.cfg), "chapter_analysis_batch_json")
			if err != nil {
				errCh <- fmt.Errorf("analyze chapter batch %s: %w", chapterRangeLabel(batch), err)
				return
			}
			briefs, err := a.decodeChapterBriefBatch(ctx, content, input, batch)
			if err != nil {
				errCh <- fmt.Errorf("parse chapter batch %s: %w", chapterRangeLabel(batch), err)
				return
			}
			results[batchIndex] = briefs
			log.Printf(
				"Script agent step succeeded: agent=ChapterAnalysisAgent step=analyze_chapter_batch project_id=%s batch=%d/%d briefs=%d",
				input.Project.ID,
				batchIndex+1,
				len(batches),
				len(briefs),
			)
		}(batchIndex, batch)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return nil, err
		}
	}
	briefs := make([]chapterBrief, 0, len(input.Source.Chapters))
	for _, batchBriefs := range results {
		briefs = append(briefs, batchBriefs...)
	}
	return briefs, nil
}

func (a *ScriptAgent) buildStoryBible(ctx context.Context, input GenerationInput, briefs []chapterBrief) (storyBible, error) {
	log.Printf("Script agent step started: agent=StoryBibleAgent step=build_story_bible project_id=%s chapter_briefs=%d", input.Project.ID, len(briefs))
	content, err := a.generator.callJSONCompletion(ctx, storyBibleMessages(input, briefs, a.generator.cfg), "story_bible_json")
	if err != nil {
		return storyBible{}, fmt.Errorf("build story bible: %w", err)
	}
	bible, err := a.decodeStoryBible(ctx, content, input, briefs)
	if err != nil {
		return storyBible{}, err
	}
	log.Printf("Script agent step succeeded: agent=StoryBibleAgent step=build_story_bible project_id=%s characters=%d themes=%d", input.Project.ID, len(bible.Characters), len(bible.World.Theme))
	return bible, nil
}

func (a *ScriptAgent) planScenes(ctx context.Context, input GenerationInput, briefs []chapterBrief, bible storyBible) (scenePlan, error) {
	log.Printf("Script agent step started: agent=ScenePlannerAgent step=plan_scenes project_id=%s chapter_briefs=%d", input.Project.ID, len(briefs))
	content, err := a.generator.callJSONCompletion(ctx, scenePlanMessages(input, briefs, bible, a.generator.cfg), "scene_plan_json")
	if err != nil {
		return scenePlan{}, fmt.Errorf("plan scenes: %w", err)
	}
	plan, err := a.decodeScenePlan(ctx, content, input, briefs, bible)
	if err != nil {
		return scenePlan{}, err
	}
	log.Printf("Script agent step succeeded: agent=ScenePlannerAgent step=plan_scenes project_id=%s acts=%d scene_cards=%d", input.Project.ID, len(plan.Acts), len(plan.Scenes))
	return plan, nil
}

func (a *ScriptAgent) expandScenes(ctx context.Context, input GenerationInput, plan scriptPlan) ([]domain.Scene, error) {
	if len(plan.Scenes) == 0 {
		return nil, fmt.Errorf("scene plan did not include scenes")
	}

	batches := sceneBatches(plan.Scenes, a.generator.cfg.ModelSceneExpansionBatchSize)
	parallelism := agentParallelism(a.generator.cfg.JobMaxParallel, len(batches))
	scenes := make([]domain.Scene, len(plan.Scenes))
	errCh := make(chan error, len(batches))
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup

	for batchIndex, batch := range batches {
		wg.Add(1)
		go func(batchIndex int, batch sceneExpansionBatch) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			log.Printf(
				"Script agent step started: agent=SceneExpansionAgent step=expand_scene_batch project_id=%s batch=%d/%d scenes=%s",
				input.Project.ID,
				batchIndex+1,
				len(batches),
				sceneBatchRangeLabel(batch.Cards),
			)

			var generated []domain.Scene
			if len(batch.Cards) == 1 {
				sceneCard := batch.Cards[0]
				sceneContent, err := a.generator.callJSONCompletion(ctx, sceneExpansionMessages(input, plan, sceneCard, a.generator.cfg), "agent_scene_json")
				if err != nil {
					errCh <- fmt.Errorf("generate scene %s: %w", sceneCard.ID, err)
					return
				}
				scene, err := a.decodeScene(ctx, sceneContent, input, plan, sceneCard)
				if err != nil {
					errCh <- fmt.Errorf("parse scene %s: %w", sceneCard.ID, err)
					return
				}
				generated = []domain.Scene{scene}
			} else {
				sceneContent, err := a.generator.callJSONCompletion(ctx, sceneBatchExpansionMessages(input, plan, batch.Cards, a.generator.cfg), "agent_scene_batch_json")
				if err != nil {
					errCh <- fmt.Errorf("generate scene batch %s: %w", sceneBatchRangeLabel(batch.Cards), err)
					return
				}
				generated, err = a.decodeSceneBatch(ctx, sceneContent, input, plan, batch.Cards)
				if err != nil {
					errCh <- fmt.Errorf("parse scene batch %s: %w", sceneBatchRangeLabel(batch.Cards), err)
					return
				}
			}

			for offset, scene := range generated {
				scenes[batch.StartIndex+offset] = scene
			}
			log.Printf(
				"Script agent step succeeded: agent=SceneExpansionAgent step=expand_scene_batch project_id=%s batch=%d/%d scenes=%d",
				input.Project.ID,
				batchIndex+1,
				len(batches),
				len(generated),
			)
		}(batchIndex, batch)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return nil, err
		}
	}
	return scenes, nil
}

func (a *ScriptAgent) decodeSceneBatch(ctx context.Context, content string, input GenerationInput, plan scriptPlan, sceneCards []domain.Scene) ([]domain.Scene, error) {
	var batch sceneDraftBatch
	if err := decodeModelJSON(content, &batch); err != nil {
		log.Printf("Script agent parse failed: agent=SceneExpansionAgent step=expand_scene_batch scenes=%s error=%s raw_chars=%d", sceneBatchRangeLabel(sceneCards), err, len([]rune(content)))
		repaired, repairErr := a.repairMalformedJSON(ctx, "expand_scene_batch", content, err, agentJSONRepairMessages(
			"SceneExpansionAgent",
			sceneBatchExpansionRepairContext(input, plan, sceneCards, a.generator.cfg),
			content,
			err,
			[]string{
				"只输出一个场景批次 JSON object，不要 Markdown。",
				"顶层字段只能包含 scenes。",
				"scenes 是数组，必须为输入中的每个 scene.id 返回一个完整 Scene object。",
				"每个 Scene 必须保留 id, act_id, chapter_refs, characters。",
				"不要输出 schema_version、project、source、world、完整 ScriptDraft 或 YAML。",
			},
			a.generator.cfg,
		))
		if repairErr != nil {
			log.Printf("Script agent warning: agent=SceneExpansionAgent step=expand_scene_batch fallback=scene_cards reason=repair_failed error=%s", repairErr)
			return normalizeSceneBatch(nil, sceneCards, plan), nil
		}
		if err := decodeModelJSON(repaired, &batch); err != nil {
			log.Printf("Script agent warning: agent=SceneExpansionAgent step=expand_scene_batch fallback=scene_cards reason=parse_repaired_failed error=%s", err)
			return normalizeSceneBatch(nil, sceneCards, plan), nil
		}
	}
	return normalizeSceneBatch(batch.Scenes, sceneCards, plan), nil
}

func (a *ScriptAgent) decodeChapterBriefBatch(ctx context.Context, content string, input GenerationInput, chapters []domain.Chapter) ([]chapterBrief, error) {
	var batch chapterBriefBatch
	if err := decodeModelJSON(content, &batch); err != nil {
		log.Printf("Script agent parse failed: agent=ChapterAnalysisAgent step=analyze_chapter_batch chapters=%s error=%s raw_chars=%d", chapterRangeLabel(chapters), err, len([]rune(content)))
		repaired, repairErr := a.repairMalformedJSON(ctx, "chapter_analysis_batch", content, err, agentJSONRepairMessages(
			"ChapterAnalysisAgent",
			chapterBatchAnalysisRepairContext(input, chapters, a.generator.cfg),
			content,
			err,
			[]string{
				"只输出一个章节分析批次 JSON object，不要 Markdown。",
				"顶层字段只能包含 briefs。",
				"briefs 是数组，每个元素只能包含 chapter_index, title, summary, key_events, characters, locations, timeline, conflicts, adaptation_hints, open_questions。",
				"必须为输入中的每个 chapter_index 返回一个 brief。",
				"不要输出完整剧本、场景对白或 YAML。",
			},
			a.generator.cfg,
		))
		if repairErr != nil {
			log.Printf("Script agent warning: agent=ChapterAnalysisAgent step=analyze_chapter_batch fallback=source_excerpt reason=repair_failed error=%s", repairErr)
			return normalizeChapterBriefBatch(nil, chapters), nil
		}
		if err := decodeModelJSON(repaired, &batch); err != nil {
			log.Printf("Script agent warning: agent=ChapterAnalysisAgent step=analyze_chapter_batch fallback=source_excerpt reason=parse_repaired_failed error=%s", err)
			return normalizeChapterBriefBatch(nil, chapters), nil
		}
	}
	return normalizeChapterBriefBatch(batch.Briefs, chapters), nil
}

func (a *ScriptAgent) decodeChapterBrief(ctx context.Context, content string, input GenerationInput, chapter domain.Chapter) (chapterBrief, error) {
	var brief chapterBrief
	if err := decodeModelJSON(content, &brief); err != nil {
		log.Printf("Script agent parse failed: agent=ChapterAnalysisAgent step=analyze_chapter chapter=%d error=%s raw_chars=%d", chapter.Index, err, len([]rune(content)))
		repaired, repairErr := a.repairMalformedJSON(ctx, "chapter_analysis", content, err, agentJSONRepairMessages(
			"ChapterAnalysisAgent",
			chapterAnalysisRepairContext(input, chapter, a.generator.cfg),
			content,
			err,
			[]string{
				"只输出一个章节分析 JSON object，不要 Markdown。",
				"顶层字段只能包含 chapter_index, title, summary, key_events, characters, locations, timeline, conflicts, adaptation_hints, open_questions。",
				"chapter_index 必须等于输入章节编号。",
				"不要输出完整剧本、场景对白或 YAML。",
			},
			a.generator.cfg,
		))
		if repairErr != nil {
			log.Printf("Script agent warning: agent=ChapterAnalysisAgent step=analyze_chapter fallback=source_excerpt chapter=%d reason=repair_failed error=%s", chapter.Index, repairErr)
			var fallback chapterBrief
			normalizeChapterBrief(&fallback, chapter)
			return fallback, nil
		}
		if err := decodeModelJSON(repaired, &brief); err != nil {
			log.Printf("Script agent warning: agent=ChapterAnalysisAgent step=analyze_chapter fallback=source_excerpt chapter=%d reason=parse_repaired_failed error=%s", chapter.Index, err)
			var fallback chapterBrief
			normalizeChapterBrief(&fallback, chapter)
			return fallback, nil
		}
	}
	normalizeChapterBrief(&brief, chapter)
	return brief, nil
}

func (a *ScriptAgent) decodeStoryBible(ctx context.Context, content string, input GenerationInput, briefs []chapterBrief) (storyBible, error) {
	var bible storyBible
	if err := decodeModelJSON(content, &bible); err != nil {
		log.Printf("Script agent parse failed: agent=StoryBibleAgent step=build_story_bible error=%s raw_chars=%d", err, len([]rune(content)))
		repaired, repairErr := a.repairMalformedJSON(ctx, "story_bible", content, err, agentJSONRepairMessages(
			"StoryBibleAgent",
			storyBibleRepairContext(input, briefs, a.generator.cfg),
			content,
			err,
			[]string{
				"只输出 StoryBible JSON object，不要 Markdown。",
				"顶层字段只能包含 world, characters, continuity。",
				"characters[].id 必须使用 c01, c02 递增，后续场景只能引用这些 ID。",
				"不要输出 acts、scenes、dialogues 或 YAML。",
			},
			a.generator.cfg,
		))
		if repairErr != nil {
			return storyBible{}, fmt.Errorf("parse story bible: %w; malformed-json repair failed: %v", err, repairErr)
		}
		if err := decodeModelJSON(repaired, &bible); err != nil {
			return storyBible{}, fmt.Errorf("parse repaired story bible: %w", err)
		}
	}
	normalizeStoryBible(&bible, input)
	return bible, nil
}

func (a *ScriptAgent) decodeScenePlan(ctx context.Context, content string, input GenerationInput, briefs []chapterBrief, bible storyBible) (scenePlan, error) {
	var plan scenePlan
	if err := decodeModelJSON(content, &plan); err != nil {
		log.Printf("Script agent parse failed: agent=ScenePlannerAgent step=plan_scenes error=%s raw_chars=%d", err, len([]rune(content)))
		repaired, repairErr := a.repairMalformedJSON(ctx, "scene_plan", content, err, agentJSONRepairMessages(
			"ScenePlannerAgent",
			scenePlanRepairContext(input, briefs, bible, a.generator.cfg),
			content,
			err,
			[]string{
				"只输出 ScenePlan JSON object，不要 Markdown。",
				"顶层字段只能包含 acts, scenes, revision。",
				"scenes 是场景卡，不要写 dialogues。",
				"scenes[].characters 只能引用 StoryBible characters[].id。",
				"acts[].scene_ids 必须引用 scenes[].id。",
			},
			a.generator.cfg,
		))
		if repairErr != nil {
			return scenePlan{}, fmt.Errorf("parse scene plan: %w; malformed-json repair failed: %v", err, repairErr)
		}
		if err := decodeModelJSON(repaired, &plan); err != nil {
			return scenePlan{}, fmt.Errorf("parse repaired scene plan: %w", err)
		}
	}
	normalizeScenePlan(&plan, input)
	return plan, nil
}

func chapterAnalysisMessages(input GenerationInput, chapter domain.Chapter, cfg config.Config) []chatMessage {
	userPrompt := fmt.Sprintf(`请作为 ChapterAnalysisAgent 分析单个小说章节，只做信息提炼，不写剧本。

项目:
- title: %s
- adaptation_target: %s
- language: %s
- style: %s

章节:
- chapter_index: %d
- title: %s
- word_count: %d

章节原文:
%s

输出要求:
1. 只输出 JSON object，不要 Markdown。
2. 顶层字段只能包含 chapter_index, title, summary, key_events, characters, locations, timeline, conflicts, adaptation_hints, open_questions。
3. summary 用 1-3 句话概括本章戏剧功能。
4. key_events 写清楚因果链，不要只写气氛。
5. adaptation_hints 写给后续剧本规划 Agent 的可执行提示。
6. 不要输出场景对白、YAML 或完整剧本。`,
		input.Project.Title,
		input.Project.AdaptationTarget,
		input.Project.Language,
		valueOrDefault(input.Config.Style, "影视化、可表演"),
		chapter.Index,
		chapter.Title,
		chapter.WordCount,
		truncateRunes(strings.TrimSpace(chapter.Content), agentInputBudget(cfg, 3)),
	)
	return []chatMessage{
		{Role: "system", Content: scopedAgentSystemPrompt("ChapterAnalysisAgent", "章节分析 JSON object，顶层只能包含 chapter_index, title, summary, key_events, characters, locations, timeline, conflicts, adaptation_hints, open_questions")},
		{Role: "user", Content: userPrompt},
	}
}

func chapterBatchAnalysisMessages(input GenerationInput, chapters []domain.Chapter, cfg config.Config) []chatMessage {
	userPrompt := fmt.Sprintf(`请作为 ChapterAnalysisAgent 批量分析多个小说章节，只做信息提炼，不写剧本。

项目:
- title: %s
- adaptation_target: %s
- language: %s
- style: %s

批次章节:
%s

输出要求:
1. 只输出 JSON object，不要 Markdown。
2. 顶层字段只能包含 briefs。
3. briefs 是数组，必须为每个输入章节返回一个 brief。
4. 每个 brief 字段只能包含 chapter_index, title, summary, key_events, characters, locations, timeline, conflicts, adaptation_hints, open_questions。
5. summary 用 1-3 句话概括本章戏剧功能。
6. key_events 写清楚因果链，不要只写气氛。
7. adaptation_hints 写给后续剧本规划 Agent 的可执行提示。
8. 不要输出场景对白、YAML 或完整剧本。`,
		input.Project.Title,
		input.Project.AdaptationTarget,
		input.Project.Language,
		valueOrDefault(input.Config.Style, "影视化、可表演"),
		chapterBatchContext(chapters, cfg),
	)
	return []chatMessage{
		{Role: "system", Content: scopedAgentSystemPrompt("ChapterAnalysisAgent", "章节分析批次 JSON object，顶层只能包含 briefs")},
		{Role: "user", Content: userPrompt},
	}
}

func storyBibleMessages(input GenerationInput, briefs []chapterBrief, cfg config.Config) []chatMessage {
	userPrompt := fmt.Sprintf(`请作为 StoryBibleAgent，根据章节分析建立全局故事圣经，只做人设、世界观、连续性，不规划具体场景。

项目:
- title: %s
- adaptation_target: %s
- language: %s
- novel_title: %s
- author: %s
- style: %s

章节分析:
%s

输出要求:
1. 只输出 JSON object，不要 Markdown。
2. 顶层字段只能包含 world, characters, continuity。
3. world 必须包含 logline, theme, tone，可补充 setting。
4. characters[].id 使用 c01、c02 递增；name 使用原文人物名；role/goal/conflict/arc 要服务后续剧本生成。
5. relationships[].target 必须引用 characters[].id。
6. continuity 记录时间线、未解决线索、伏笔和重要道具。
7. 不要输出 acts、scenes、dialogues 或 YAML。`,
		input.Project.Title,
		input.Project.AdaptationTarget,
		input.Project.Language,
		input.Source.NovelTitle,
		input.Source.Author,
		valueOrDefault(input.Config.Style, "影视化、可表演"),
		briefContext(briefs, cfg.ModelMaxInputChars),
	)
	return []chatMessage{
		{Role: "system", Content: scopedAgentSystemPrompt("StoryBibleAgent", "StoryBible JSON object，顶层只能包含 world, characters, continuity")},
		{Role: "user", Content: userPrompt},
	}
}

func scenePlanMessages(input GenerationInput, briefs []chapterBrief, bible storyBible, cfg config.Config) []chatMessage {
	sceneTarget := sceneTargetForInput(input, cfg, "ScenePlannerAgent")
	bibleJSON, _ := json.MarshalIndent(bible, "", "  ")
	userPrompt := fmt.Sprintf(`请作为 ScenePlannerAgent，把故事圣经和章节分析转成剧本场景计划。这个阶段只做场景卡，不写对白。

项目:
- title: %s
- adaptation_target: %s
- language: %s

生成偏好:
- style: %s
- target_scene_count: %s
- dialogue_density: %s
- preserve_original_names: %t

故事圣经:
%s

章节分析:
%s

输出要求:
1. 只输出 JSON object，不要 Markdown。
2. 顶层字段只能包含 acts, scenes, revision。
3. scenes 是场景卡，每个场景必须包含 id, act_id, chapter_refs, location, time, purpose, conflict, summary, characters, beats。
4. 场景卡不要写 dialogues，完整对白会由 SceneExpansionAgent 逐场生成。
5. scenes[].chapter_refs 只能引用输入章节编号。
6. scenes[].characters 只能引用故事圣经 characters[].id。
7. acts[].id 使用 act1、act2；scenes[].id 使用 s01、s02；acts[].scene_ids 必须引用 scenes[].id。
8. revision.editor_notes 写给作者的后续打磨建议。`,
		input.Project.Title,
		input.Project.AdaptationTarget,
		input.Project.Language,
		valueOrDefault(input.Config.Style, "影视化、可表演"),
		sceneTarget,
		valueOrDefault(input.Config.DialogueDensity, "medium"),
		input.Config.PreserveOriginalNames,
		truncateRunes(string(bibleJSON), agentInputBudget(cfg, 3)),
		briefContext(briefs, cfg.ModelMaxInputChars/2),
	)
	return []chatMessage{
		{Role: "system", Content: scopedAgentSystemPrompt("ScenePlannerAgent", "ScenePlan JSON object，顶层只能包含 acts, scenes, revision")},
		{Role: "user", Content: userPrompt},
	}
}

func chapterAnalysisRepairContext(input GenerationInput, chapter domain.Chapter, cfg config.Config) string {
	return fmt.Sprintf("项目: %s\n章节编号: %d\n章节标题: %s\n章节原文:\n%s",
		input.Project.Title,
		chapter.Index,
		chapter.Title,
		truncateRunes(chapter.Content, agentInputBudget(cfg, 3)),
	)
}

func chapterBatchAnalysisRepairContext(input GenerationInput, chapters []domain.Chapter, cfg config.Config) string {
	return fmt.Sprintf("项目: %s\n章节范围: %s\n批次章节:\n%s",
		input.Project.Title,
		chapterRangeLabel(chapters),
		chapterBatchContext(chapters, cfg),
	)
}

func storyBibleRepairContext(input GenerationInput, briefs []chapterBrief, cfg config.Config) string {
	return fmt.Sprintf("项目: %s\n章节分析:\n%s", input.Project.Title, briefContext(briefs, cfg.ModelMaxInputChars))
}

func scenePlanRepairContext(input GenerationInput, briefs []chapterBrief, bible storyBible, cfg config.Config) string {
	bibleJSON, _ := json.MarshalIndent(bible, "", "  ")
	return fmt.Sprintf("项目: %s\n故事圣经:\n%s\n章节分析:\n%s",
		input.Project.Title,
		truncateRunes(string(bibleJSON), agentInputBudget(cfg, 3)),
		briefContext(briefs, cfg.ModelMaxInputChars/2),
	)
}

func agentJSONRepairMessages(agentName string, contextText string, raw string, parseErr error, requirements []string, cfg config.Config) []chatMessage {
	userPrompt := fmt.Sprintf(`上一次 %s 输出的 JSON 解析失败，请作为 JSON 修复 Agent 修复它。

解析错误:
%s

可用上下文:
%s

上一次原始输出:
%s

修复要求:
%s`,
		agentName,
		parseErr,
		contextText,
		truncateRunes(raw, agentInputBudget(cfg, 2)),
		numberedRequirements(requirements),
	)
	return []chatMessage{
		{Role: "system", Content: scopedAgentSystemPrompt("JSONRepairAgent", fmt.Sprintf("%s 的修复后 JSON object", agentName))},
		{Role: "user", Content: userPrompt},
	}
}

func normalizeChapterBrief(brief *chapterBrief, chapter domain.Chapter) {
	brief.ChapterIndex = chapter.Index
	if strings.TrimSpace(brief.Title) == "" {
		brief.Title = chapter.Title
	}
	if strings.TrimSpace(brief.Summary) == "" {
		brief.Summary = truncateRunes(strings.TrimSpace(chapter.Content), 240)
	}
	if len(brief.KeyEvents) == 0 && strings.TrimSpace(brief.Summary) != "" {
		brief.KeyEvents = []string{brief.Summary}
	}
}

func normalizeChapterBriefBatch(briefs []chapterBrief, chapters []domain.Chapter) []chapterBrief {
	briefByChapter := make(map[int]chapterBrief, len(briefs))
	for _, brief := range briefs {
		if brief.ChapterIndex == 0 {
			continue
		}
		if _, exists := briefByChapter[brief.ChapterIndex]; exists {
			log.Printf("Script agent warning: agent=ChapterAnalysisAgent duplicate_brief chapter=%d ignored=true", brief.ChapterIndex)
			continue
		}
		briefByChapter[brief.ChapterIndex] = brief
	}

	normalized := make([]chapterBrief, 0, len(chapters))
	for _, chapter := range chapters {
		brief, ok := briefByChapter[chapter.Index]
		if !ok {
			summary := truncateRunes(strings.TrimSpace(chapter.Content), 240)
			brief = chapterBrief{
				ChapterIndex: chapter.Index,
				Title:        chapter.Title,
				Summary:      summary,
			}
			if summary != "" {
				brief.KeyEvents = []string{summary}
			}
			log.Printf(
				"Script agent warning: agent=ChapterAnalysisAgent missing_brief chapter=%d fallback=source_excerpt",
				chapter.Index,
			)
		}
		normalizeChapterBrief(&brief, chapter)
		normalized = append(normalized, brief)
	}
	return normalized
}

func normalizeStoryBible(bible *storyBible, input GenerationInput) {
	if strings.TrimSpace(bible.World.Tone) == "" {
		bible.World.Tone = valueOrDefault(input.Config.Style, "影视化、可表演")
	}
	seen := map[string]bool{}
	for i := range bible.Characters {
		id := strings.TrimSpace(bible.Characters[i].ID)
		if id == "" || seen[id] {
			id = fmt.Sprintf("c%02d", i+1)
			bible.Characters[i].ID = id
		}
		seen[id] = true
		if strings.TrimSpace(bible.Characters[i].Role) == "" {
			bible.Characters[i].Role = "supporting"
		}
	}
}

func normalizeScenePlan(plan *scenePlan, input GenerationInput) {
	combined := scriptPlan{
		Acts:   plan.Acts,
		Scenes: plan.Scenes,
	}
	normalizePlan(&combined, input)
	plan.Acts = combined.Acts
	plan.Scenes = combined.Scenes
	if strings.TrimSpace(plan.Revision.Confidence) == "" {
		plan.Revision.Confidence = "medium"
	}
}

func briefContext(briefs []chapterBrief, maxChars int) string {
	data, _ := json.MarshalIndent(briefs, "", "  ")
	if maxChars <= 0 {
		maxChars = 24000
	}
	return truncateRunes(string(data), maxChars)
}

func numberedRequirements(requirements []string) string {
	lines := make([]string, 0, len(requirements))
	for i, requirement := range requirements {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, requirement))
	}
	return strings.Join(lines, "\n")
}

func agentInputBudget(cfg config.Config, divisor int) int {
	if divisor <= 0 {
		divisor = 1
	}
	maxChars := cfg.ModelMaxInputChars
	if maxChars <= 0 {
		maxChars = 24000
	}
	budget := maxChars / divisor
	if budget < 4000 {
		return 4000
	}
	return budget
}

func agentParallelism(configured int, total int) int {
	if total <= 1 {
		return 1
	}
	if configured <= 0 {
		configured = 2
	}
	if configured > 6 {
		configured = 6
	}
	if configured > total {
		return total
	}
	return configured
}

func chapterBatches(chapters []domain.Chapter, configuredSize int) [][]domain.Chapter {
	if len(chapters) == 0 {
		return nil
	}
	size := chapterAnalysisBatchSize(configuredSize, len(chapters))
	batches := make([][]domain.Chapter, 0, (len(chapters)+size-1)/size)
	for start := 0; start < len(chapters); start += size {
		end := start + size
		if end > len(chapters) {
			end = len(chapters)
		}
		batches = append(batches, chapters[start:end])
	}
	return batches
}

func chapterAnalysisBatchSize(configured int, total int) int {
	if total <= 0 {
		return 0
	}
	if configured <= 0 {
		configured = 6
	}
	if configured > 10 {
		configured = 10
	}
	if configured > total {
		return total
	}
	return configured
}

func chapterRangeLabel(chapters []domain.Chapter) string {
	if len(chapters) == 0 {
		return "none"
	}
	if len(chapters) == 1 {
		return fmt.Sprintf("%d", chapters[0].Index)
	}
	return fmt.Sprintf("%d-%d", chapters[0].Index, chapters[len(chapters)-1].Index)
}

func chapterBatchContext(chapters []domain.Chapter, cfg config.Config) string {
	if len(chapters) == 0 {
		return ""
	}
	perChapterBudget := agentInputBudget(cfg, 2) / len(chapters)
	if perChapterBudget < 1200 {
		perChapterBudget = 1200
	}

	var builder strings.Builder
	for _, chapter := range chapters {
		builder.WriteString(fmt.Sprintf("\n[chapter %d]\n", chapter.Index))
		builder.WriteString(fmt.Sprintf("chapter_index: %d\n", chapter.Index))
		builder.WriteString(fmt.Sprintf("title: %s\n", chapter.Title))
		builder.WriteString(fmt.Sprintf("word_count: %d\n", chapter.WordCount))
		builder.WriteString("content:\n")
		builder.WriteString(truncateRunes(strings.TrimSpace(chapter.Content), perChapterBudget))
		builder.WriteString("\n")
	}
	return builder.String()
}

func sceneBatches(sceneCards []domain.Scene, configuredSize int) []sceneExpansionBatch {
	if len(sceneCards) == 0 {
		return nil
	}
	size := sceneExpansionBatchSize(configuredSize, len(sceneCards))
	batches := make([]sceneExpansionBatch, 0, (len(sceneCards)+size-1)/size)
	for start := 0; start < len(sceneCards); start += size {
		end := start + size
		if end > len(sceneCards) {
			end = len(sceneCards)
		}
		batches = append(batches, sceneExpansionBatch{
			StartIndex: start,
			Cards:      sceneCards[start:end],
		})
	}
	return batches
}

func sceneExpansionBatchSize(configured int, total int) int {
	if total <= 0 {
		return 0
	}
	if configured <= 0 {
		configured = 3
	}
	if configured > 5 {
		configured = 5
	}
	if configured > total {
		return total
	}
	return configured
}

func sceneBatchRangeLabel(sceneCards []domain.Scene) string {
	if len(sceneCards) == 0 {
		return "none"
	}
	if len(sceneCards) == 1 {
		return sceneCards[0].ID
	}
	return fmt.Sprintf("%s-%s", sceneCards[0].ID, sceneCards[len(sceneCards)-1].ID)
}
