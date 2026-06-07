package ai

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Flepxum/novelscript-ai/backend/internal/domain"
)

type GenerationInput struct {
	Project domain.Project
	Source  domain.SourceDocument
	Config  domain.GenerationConfig
}

type Generator struct{}

func NewGenerator() *Generator {
	return &Generator{}
}

func (g *Generator) Generate(ctx context.Context, input GenerationInput) (domain.ScriptDraft, error) {
	_ = ctx
	chapters := input.Source.Chapters
	sceneCount := input.Config.TargetSceneCount
	if sceneCount <= 0 {
		sceneCount = len(chapters) * 2
	}
	if sceneCount < 3 {
		sceneCount = 3
	}
	if sceneCount > 18 {
		sceneCount = 18
	}

	names := extractNames(input.Source.Content)
	characters := buildCharacters(names)
	acts := []domain.Act{
		{ID: "act1", Title: "入局", Purpose: "建立人物目标、世界规则和首个外部冲突。"},
		{ID: "act2", Title: "追索", Purpose: "让线索升级，推动人物关系和主要矛盾持续加压。"},
		{ID: "act3", Title: "抉择", Purpose: "集中爆发核心冲突，给出阶段性答案并留下可扩展伏笔。"},
	}

	scenes := make([]domain.Scene, 0, sceneCount)
	for i := 0; i < sceneCount; i++ {
		chapter := chapters[i%len(chapters)]
		actIndex := actIndexFor(i, sceneCount)
		sceneID := fmt.Sprintf("s%02d", i+1)
		acts[actIndex].SceneIDs = append(acts[actIndex].SceneIDs, sceneID)
		lead := characters[0]
		counterpart := characters[(i+1)%len(characters)]
		passage := compactExcerpt(chapter.Content, 72)
		scenes = append(scenes, domain.Scene{
			ID:          sceneID,
			ActID:       acts[actIndex].ID,
			ChapterRefs: []int{chapter.Index},
			Location:    inferLocation(chapter.Content, chapter.Index),
			Time:        []string{"清晨", "午后", "夜"}[i%3],
			Purpose:     scenePurpose(actIndex, chapter.Title),
			Conflict:    sceneConflict(actIndex),
			Summary:     fmt.Sprintf("根据「%s」改编：%s", chapter.Title, passage),
			Characters:  []string{lead.ID, counterpart.ID},
			Beats: []string{
				fmt.Sprintf("%s进入与「%s」相关的关键情境。", lead.Name, chapter.Title),
				"角色通过行动暴露真实目标，信息差被逐步放大。",
				"场景以一个未解决的选择收束，推动下一场继续升级。",
			},
			Dialogues: []domain.Dialogue{
				{Speaker: lead.ID, Line: fmt.Sprintf("这不是巧合，%s里一定还藏着另一层线索。", chapter.Title)},
				{Speaker: counterpart.ID, Line: "如果现在退开，后面的真相就永远不会轮到我们说出口。"},
			},
			Notes: []string{
				"本场由后端生成编排层产出，后续可接入模型配置替换生成策略。",
				"保留章节来源，便于作者回看原文继续打磨。",
			},
			AIAssumptions: []string{
				"章节中的叙述被压缩为可表演的行动和对白。",
			},
		})
	}

	chapterRefs := make([]int, len(chapters))
	for i, chapter := range chapters {
		chapterRefs[i] = chapter.Index
	}

	return domain.ScriptDraft{
		SchemaVersion: "1.0",
		Project: domain.ScriptProject{
			Title:            input.Project.Title,
			AdaptationTarget: input.Project.AdaptationTarget,
			Language:         input.Project.Language,
			Genre:            []string{"AI 改编", "剧情"},
		},
		Source: domain.ScriptSource{
			NovelTitle:   input.Source.NovelTitle,
			Author:       input.Source.Author,
			ChapterCount: len(chapters),
			ChapterRefs:  chapterRefs,
		},
		World: domain.World{
			Logline: fmt.Sprintf("围绕「%s」展开，主角在连续事件中追索真相并完成关键选择。", input.Project.Title),
			Theme:   []string{"选择", "真相", "代价"},
			Tone:    defaultTone(input.Config.Style),
			Setting: "由原小说章节自动归纳的影视化叙事空间。",
		},
		Characters: characters,
		Acts:       acts,
		Scenes:     scenes,
		Continuity: domain.Continuity{
			Timeline: []string{
				"第一幕建立事件入口，第二幕扩大冲突，第三幕完成阶段性选择。",
				"所有场景均保留 chapter_refs，支持回溯原小说章节。",
			},
			OpenThreads: []string{
				"反复出现的线索需要在后续版本中明确最终归属。",
			},
			Foreshadowing: []string{
				"第一章出现的异常细节可在第三幕作为反转依据。",
			},
			Props: []string{"原文关键物件", "未解线索"},
		},
		Revision: domain.Revision{
			GeneratedAt: time.Now().Format(time.RFC3339),
			Confidence:  "medium",
			EditorNotes: []string{
				"这是一份可编辑初稿，建议作者优先调整人物动机和场景节奏。",
			},
			AIAssumptions: []string{
				"当前生成策略基于章节文本做规则化压缩，模型接入后可替换为真实推理生成。",
			},
		},
	}, nil
}

func (g *Generator) RegenerateScene(ctx context.Context, draft domain.ScriptDraft, sceneID string, instruction string) (domain.ScriptDraft, domain.Scene, error) {
	_ = ctx
	for i, scene := range draft.Scenes {
		if scene.ID != sceneID {
			continue
		}
		note := "局部重写：强化场景目标、冲突和可表演动作。"
		if strings.TrimSpace(instruction) != "" {
			note = "局部重写指令：" + strings.TrimSpace(instruction)
		}
		scene.Conflict = scene.Conflict + "；重写后冲突更集中。"
		scene.Summary = scene.Summary + " 重写版本强调角色当下选择，让信息通过行动和对白释放。"
		scene.Beats = append(scene.Beats, "根据重写指令补充一个更明确的转折点。")
		scene.Dialogues = append(scene.Dialogues, domain.Dialogue{
			Speaker: scene.Characters[0],
			Line:    "我现在要的不是解释，是一个能继续走下去的理由。",
		})
		scene.Notes = append(scene.Notes, note)
		draft.Scenes[i] = scene
		draft.Revision.GeneratedAt = time.Now().Format(time.RFC3339)
		draft.Revision.EditorNotes = append(draft.Revision.EditorNotes, fmt.Sprintf("%s 已完成局部重写。", sceneID))
		return draft, scene, nil
	}
	return draft, domain.Scene{}, fmt.Errorf("scene %s not found", sceneID)
}

func extractNames(content string) []string {
	re := regexp.MustCompile(`([\p{Han}]{2,4})(?:说|问|道|推|看|站|走|笑|沉默|转身|抬头|低声)`)
	matches := re.FindAllStringSubmatch(content, -1)
	counts := map[string]int{}
	for _, match := range matches {
		name := strings.TrimSpace(match[1])
		if len([]rune(name)) < 2 || isStopName(name) {
			continue
		}
		counts[name]++
	}
	type pair struct {
		Name  string
		Count int
	}
	var pairs []pair
	for name, count := range counts {
		pairs = append(pairs, pair{name, count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Count == pairs[j].Count {
			return pairs[i].Name < pairs[j].Name
		}
		return pairs[i].Count > pairs[j].Count
	})
	names := make([]string, 0, 3)
	for _, item := range pairs {
		names = append(names, item.Name)
		if len(names) == 3 {
			break
		}
	}
	fallbacks := []string{"主角", "对手", "同盟者"}
	for len(names) < 3 {
		names = append(names, fallbacks[len(names)])
	}
	return names
}

func isStopName(name string) bool {
	stops := map[string]bool{
		"有人": true, "众人": true, "所有人": true, "这个人": true, "年轻人": true, "老人": true,
	}
	return stops[name]
}

func buildCharacters(names []string) []domain.Character {
	roles := []string{"protagonist", "antagonist", "supporting"}
	traits := [][]string{
		{"敏锐", "克制", "行动导向"},
		{"谨慎", "有秘密", "制造阻力"},
		{"观察力强", "情感直接", "推动关系"},
	}
	characters := make([]domain.Character, 0, len(names))
	for i, name := range names {
		characters = append(characters, domain.Character{
			ID:       fmt.Sprintf("c%02d", i+1),
			Name:     name,
			Role:     roles[i%len(roles)],
			Traits:   traits[i%len(traits)],
			Goal:     "在事件推进中争取主动权。",
			Conflict: "个人目标与外部压力持续冲突。",
			Arc:      "从被动卷入到主动选择。",
		})
	}
	return characters
}

func actIndexFor(sceneIndex, sceneCount int) int {
	third := (sceneIndex * 3) / sceneCount
	if third > 2 {
		return 2
	}
	return third
}

func compactExcerpt(content string, limit int) string {
	text := strings.Join(strings.Fields(content), "")
	if text == "" {
		return "原章节提供了该场景的主要事件和情绪线索。"
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

func inferLocation(content string, chapterIndex int) string {
	locationHints := []string{"书店", "街", "巷", "院", "宅", "桥", "城", "房间", "码头", "车站"}
	for _, hint := range locationHints {
		if strings.Contains(content, hint) {
			return hint
		}
	}
	return fmt.Sprintf("第%d章主要场景", chapterIndex)
}

func scenePurpose(actIndex int, chapterTitle string) string {
	switch actIndex {
	case 0:
		return "建立人物处境并引出主线钩子：" + chapterTitle
	case 1:
		return "推动调查与关系变化，让冲突进一步升级。"
	default:
		return "迫使角色做出选择，并留下下一轮打磨空间。"
	}
}

func sceneConflict(actIndex int) string {
	switch actIndex {
	case 0:
		return "角色想弄清事实，但关键线索被遮蔽。"
	case 1:
		return "角色掌握的信息彼此矛盾，信任关系开始松动。"
	default:
		return "真相逼近，角色必须在安全与行动之间选择。"
	}
}

func defaultTone(style string) string {
	if strings.TrimSpace(style) != "" {
		return style
	}
	return "克制、悬疑、影视化"
}
