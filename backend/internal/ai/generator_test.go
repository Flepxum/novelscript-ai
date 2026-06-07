package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Flepxum/novelscript-ai/backend/internal/config"
	"github.com/Flepxum/novelscript-ai/backend/internal/domain"
)

func TestGeneratorGenerateCallsOpenAICompatibleChatCompletion(t *testing.T) {
	var received chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("expected /v1/chat/completions request, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("expected bearer token header, got %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		content, err := json.Marshal(validDraft())
		if err != nil {
			t.Fatalf("marshal draft: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": string(content)}},
			},
		})
	}))
	defer server.Close()

	generator := NewGenerator(config.Config{
		ModelBaseURL:         server.URL + "/v1",
		ModelAPIKey:          "test-key",
		ModelName:            "script-pro",
		ModelTimeoutSeconds:  3,
		ModelMaxRetries:      0,
		ModelMaxOutputTokens: 2000,
		ModelTemperature:     0.4,
		ModelStructureMode:   "json_object",
		ModelMaxInputChars:   12000,
		ModelPromptVersion:   "script-draft-v1",
	})

	draft, err := generator.Generate(context.Background(), generationInput())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if received.Model != "script-pro" {
		t.Fatalf("expected model script-pro, got %s", received.Model)
	}
	if len(received.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(received.Messages))
	}
	if !strings.Contains(received.Messages[1].Content, "[chapter 1]") {
		t.Fatalf("expected prompt to include chapter text, got %s", received.Messages[1].Content)
	}
	if draft.Project.Title != "雨夜书店改编" {
		t.Fatalf("expected project metadata to be normalized, got %s", draft.Project.Title)
	}
	if draft.Source.ChapterCount != 3 || len(draft.Source.ChapterRefs) != 3 {
		t.Fatalf("expected source metadata from input, got %#v", draft.Source)
	}
	if draft.Scenes[0].Dialogues[0].Line == "" {
		t.Fatal("expected scene dialogue from llm response")
	}
}

func TestGeneratorGenerateRequiresModelConfig(t *testing.T) {
	generator := NewGenerator(config.Config{})

	_, err := generator.Generate(context.Background(), generationInput())
	if err == nil {
		t.Fatal("expected missing config error")
	}
	if !strings.Contains(err.Error(), "MODEL_BASE_URL") || !strings.Contains(err.Error(), "MODEL_API_KEY") || !strings.Contains(err.Error(), "MODEL_NAME") {
		t.Fatalf("expected missing config details, got %v", err)
	}
}

func TestGeneratorMultiAgentPipelineUsesSpecializedAgents(t *testing.T) {
	var mu sync.Mutex
	counts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var received chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(received.Messages) != 2 {
			t.Fatalf("expected system and user messages, got %#v", received.Messages)
		}
		prompt := received.Messages[1].Content

		switch {
		case strings.Contains(prompt, "ChapterAnalysisAgent"):
			mu.Lock()
			counts["chapter"]++
			mu.Unlock()
			respondJSON(w, chapterBriefBatch{
				Briefs: []chapterBrief{
					{
						ChapterIndex:    1,
						Title:           "第一章分析",
						Summary:         "本章推动调查前进。",
						KeyEvents:       []string{"发现关键线索"},
						Characters:      []string{"林知夏", "周衡"},
						Locations:       []string{"旧书店"},
						Timeline:        []string{"雨夜"},
						Conflicts:       []string{"是否继续追查"},
						AdaptationHints: []string{"保留线索递进"},
					},
					{
						ChapterIndex:    2,
						Title:           "第二章分析",
						Summary:         "名单让调查范围扩大。",
						KeyEvents:       []string{"失踪名单被确认"},
						Characters:      []string{"林知夏", "许燃"},
						Locations:       []string{"旧书店"},
						Timeline:        []string{"次日"},
						Conflicts:       []string{"名单来源不明"},
						AdaptationHints: []string{"压缩为线索升级场"},
					},
					{
						ChapterIndex:    3,
						Title:           "第三章分析",
						Summary:         "旧站台揭开内应线索。",
						KeyEvents:       []string{"找到最后一页名单"},
						Characters:      []string{"林知夏", "周衡"},
						Locations:       []string{"旧站台"},
						Timeline:        []string{"深夜"},
						Conflicts:       []string{"内应身份曝光"},
						AdaptationHints: []string{"作为第一集结尾反转"},
					},
				},
			})
		case strings.Contains(prompt, "StoryBibleAgent"):
			mu.Lock()
			counts["bible"]++
			mu.Unlock()
			respondJSON(w, storyBible{
				World: domain.World{
					Logline: "一名编辑在雨夜书店追查失踪名单。",
					Theme:   []string{"真相", "信任"},
					Tone:    "克制、悬疑、影视化",
					Setting: "旧书店与旧站台",
				},
				Characters: []domain.Character{
					{ID: "c01", Name: "林知夏", Role: "protagonist", Goal: "查明名单真相"},
					{ID: "c02", Name: "周衡", Role: "keeper", Conflict: "隐瞒旧案"},
				},
				Continuity: domain.Continuity{
					Timeline: []string{"雨夜开始调查"},
					Props:    []string{"旧车票"},
				},
			})
		case strings.Contains(prompt, "ScenePlannerAgent"):
			mu.Lock()
			counts["planner"]++
			mu.Unlock()
			respondJSON(w, scenePlan{
				Acts: []domain.Act{
					{ID: "act1", Title: "入局", Purpose: "建立调查目标", SceneIDs: []string{"s01"}},
				},
				Scenes: []domain.Scene{
					{
						ID:          "s01",
						ActID:       "act1",
						ChapterRefs: []int{1, 2, 3},
						Location:    "旧书店",
						Time:        "雨夜",
						Purpose:     "让主角确认名单线索",
						Conflict:    "周衡阻止林知夏继续追查",
						Summary:     "林知夏在旧书店发现名单和旧车票有关。",
						Characters:  []string{"c01", "c02"},
						Beats:       []string{"林知夏进店", "周衡隐瞒", "两人交换条件"},
					},
				},
				Revision: domain.Revision{Confidence: "medium", EditorNotes: []string{"可继续强化周衡动机。"}},
			})
		case strings.Contains(prompt, "完整 Scene JSON"):
			mu.Lock()
			counts["scene"]++
			mu.Unlock()
			scene := validDraft().Scenes[0]
			scene.ChapterRefs = []int{1, 2, 3}
			respondJSON(w, scene)
		default:
			t.Fatalf("unexpected prompt: %s", prompt)
		}
	}))
	defer server.Close()

	generator := NewGenerator(config.Config{
		ModelBaseURL:         server.URL,
		ModelAPIKey:          "test-key",
		ModelName:            "script-pro",
		ModelTimeoutSeconds:  3,
		ModelMaxRetries:      0,
		ModelStructureMode:   "json_object",
		ModelMaxOutputTokens: 2000,
		ModelMaxInputChars:   12000,
		ModelAgentPipeline:   "multi_agent",
		JobMaxParallel:       1,
	})

	draft, err := generator.Generate(context.Background(), generationInput())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(draft.Scenes) != 1 || draft.Scenes[0].ID != "s01" {
		t.Fatalf("expected expanded scene draft, got %#v", draft.Scenes)
	}
	if counts["chapter"] != 1 || counts["bible"] != 1 || counts["planner"] != 1 || counts["scene"] != 1 {
		t.Fatalf("unexpected agent call counts: %#v", counts)
	}
}

func TestChapterBatchesRespectConfiguredSize(t *testing.T) {
	chapters := []domain.Chapter{
		{Index: 1},
		{Index: 2},
		{Index: 3},
		{Index: 4},
		{Index: 5},
		{Index: 6},
		{Index: 7},
	}

	batches := chapterBatches(chapters, 3)
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches, got %d", len(batches))
	}
	if chapterRangeLabel(batches[0]) != "1-3" || chapterRangeLabel(batches[1]) != "4-6" || chapterRangeLabel(batches[2]) != "7" {
		t.Fatalf("unexpected batch ranges: %s, %s, %s", chapterRangeLabel(batches[0]), chapterRangeLabel(batches[1]), chapterRangeLabel(batches[2]))
	}
	if chapterAnalysisBatchSize(50, len(chapters)) != len(chapters) {
		t.Fatal("expected configured size to be capped by total chapter count")
	}
	if chapterAnalysisBatchSize(0, 12) != 6 {
		t.Fatal("expected default chapter analysis batch size to be 6")
	}
}

func TestChapterBriefBatchFallsBackWhenModelReturnsScriptDraftShape(t *testing.T) {
	input := generationInput()
	agent := NewScriptAgent(NewGenerator(config.Config{}))
	raw, err := json.Marshal(validDraft())
	if err != nil {
		t.Fatalf("marshal draft: %v", err)
	}

	briefs, err := agent.decodeChapterBriefBatch(context.Background(), string(raw), input, input.Source.Chapters)
	if err != nil {
		t.Fatalf("decode chapter brief batch: %v", err)
	}
	if len(briefs) != len(input.Source.Chapters) {
		t.Fatalf("expected fallback briefs for all chapters, got %d", len(briefs))
	}
	for index, brief := range briefs {
		if brief.ChapterIndex != input.Source.Chapters[index].Index {
			t.Fatalf("expected chapter index %d, got %d", input.Source.Chapters[index].Index, brief.ChapterIndex)
		}
		if strings.TrimSpace(brief.Summary) == "" {
			t.Fatal("expected fallback summary from source excerpt")
		}
	}
}

func TestGeneratorFastPathUsesStructurePlanAndSceneBatch(t *testing.T) {
	var mu sync.Mutex
	counts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var received chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		prompt := received.Messages[1].Content

		switch {
		case strings.Contains(prompt, "剧本生成计划"):
			mu.Lock()
			counts["plan"]++
			mu.Unlock()
			draft := validDraft()
			second := draft.Scenes[0]
			second.ID = "s02"
			second.Summary = "林知夏和周衡前往旧站台确认名单线索。"
			second.ChapterRefs = []int{2, 3}
			respondJSON(w, scriptPlan{
				World:      draft.World,
				Characters: draft.Characters,
				Acts: []domain.Act{
					{ID: "act1", Title: "入局", Purpose: "压缩短篇核心冲突", SceneIDs: []string{"s01", "s02"}},
				},
				Scenes:     []domain.Scene{draft.Scenes[0], second},
				Continuity: draft.Continuity,
				Revision:   draft.Revision,
			})
		case strings.Contains(prompt, "批量生成完整 Scene JSON"):
			mu.Lock()
			counts["scene_batch"]++
			mu.Unlock()
			draft := validDraft()
			second := draft.Scenes[0]
			second.ID = "s02"
			second.Summary = "林知夏和周衡前往旧站台确认名单线索。"
			second.ChapterRefs = []int{2, 3}
			respondJSON(w, sceneDraftBatch{Scenes: []domain.Scene{draft.Scenes[0], second}})
		default:
			t.Fatalf("unexpected prompt: %s", prompt)
		}
	}))
	defer server.Close()

	generator := NewGenerator(config.Config{
		ModelBaseURL:                 server.URL,
		ModelAPIKey:                  "test-key",
		ModelName:                    "script-pro",
		ModelTimeoutSeconds:          3,
		ModelMaxRetries:              0,
		ModelStructureMode:           "json_object",
		ModelMaxOutputTokens:         2000,
		ModelMaxInputChars:           12000,
		ModelAgentPipeline:           "multi_agent",
		ModelFastPathMaxChars:        5000,
		ModelSceneExpansionBatchSize: 3,
		JobMaxParallel:               1,
	})

	draft, err := generator.Generate(context.Background(), generationInput())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(draft.Scenes) != 2 {
		t.Fatalf("expected 2 generated scenes, got %#v", draft.Scenes)
	}
	if counts["plan"] != 1 || counts["scene_batch"] != 1 {
		t.Fatalf("unexpected fast path counts: %#v", counts)
	}
}

func TestGeneratorGenerateAcceptsArrayContentResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content, err := json.Marshal(validDraft())
		if err != nil {
			t.Fatalf("marshal draft: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"finish_reason": "stop",
					"message": map[string]any{
						"role": "assistant",
						"content": []map[string]string{
							{"type": "text", "text": string(content)},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	generator := NewGenerator(config.Config{
		ModelBaseURL:         server.URL,
		ModelAPIKey:          "test-key",
		ModelName:            "script-pro",
		ModelTimeoutSeconds:  3,
		ModelMaxRetries:      0,
		ModelStructureMode:   "json_object",
		ModelMaxOutputTokens: 2000,
		ModelMaxInputChars:   12000,
	})

	draft, err := generator.Generate(context.Background(), generationInput())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if draft.Scenes[0].ID != "s01" {
		t.Fatalf("expected draft from array content, got %#v", draft.Scenes)
	}
}

func TestGeneratorGenerateReportsMissingContentWithFinishReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"finish_reason": "stop",
					"message": map[string]string{
						"role":    "assistant",
						"content": "",
					},
				},
			},
		})
	}))
	defer server.Close()

	generator := NewGenerator(config.Config{
		ModelBaseURL:         server.URL,
		ModelAPIKey:          "test-key",
		ModelName:            "script-pro",
		ModelTimeoutSeconds:  3,
		ModelMaxRetries:      0,
		ModelStructureMode:   "json_object",
		ModelMaxOutputTokens: 2000,
		ModelMaxInputChars:   12000,
	})

	_, err := generator.Generate(context.Background(), generationInput())
	if err == nil {
		t.Fatal("expected missing content error")
	}
	if !strings.Contains(err.Error(), "finish_reason=stop") {
		t.Fatalf("expected finish reason in error, got %v", err)
	}
}

func TestGeneratorRepairsMalformedJSONResponse(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]string{"role": "assistant", "content": `{"schema_version":"1.0"`}},
				},
			})
			return
		}

		var received chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode repair request: %v", err)
		}
		if !strings.Contains(received.Messages[1].Content, "JSON 解析失败") && !strings.Contains(received.Messages[1].Content, "解析错误") {
			t.Fatalf("expected repair prompt, got %s", received.Messages[1].Content)
		}

		content, err := json.Marshal(validDraft())
		if err != nil {
			t.Fatalf("marshal draft: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": string(content)}},
			},
		})
	}))
	defer server.Close()

	generator := NewGenerator(config.Config{
		ModelBaseURL:         server.URL,
		ModelAPIKey:          "test-key",
		ModelName:            "script-pro",
		ModelTimeoutSeconds:  3,
		ModelMaxRetries:      0,
		ModelStructureMode:   "json_object",
		ModelMaxOutputTokens: 2000,
		ModelMaxInputChars:   12000,
	})

	draft, err := generator.Generate(context.Background(), generationInput())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("expected generation and repair requests, got %d", requestCount)
	}
	if draft.Scenes[0].ID != "s01" {
		t.Fatalf("expected repaired draft, got %#v", draft.Scenes)
	}
}

func TestGeneratorFallsBackToDecomposedAgentWhenFullDraftHitsLengthLimit(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		var received chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		switch requestCount {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{
						"finish_reason": "length",
						"message": map[string]string{
							"role":    "assistant",
							"content": "",
						},
					},
				},
			})
		case 2:
			if !strings.Contains(received.Messages[1].Content, "剧本生成计划") {
				t.Fatalf("expected plan prompt, got %s", received.Messages[1].Content)
			}
			plan := validPlan()
			content, err := json.Marshal(plan)
			if err != nil {
				t.Fatalf("marshal plan: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]string{"role": "assistant", "content": string(content)}},
				},
			})
		case 3:
			if !strings.Contains(received.Messages[1].Content, "完整 Scene JSON") {
				t.Fatalf("expected scene prompt, got %s", received.Messages[1].Content)
			}
			content, err := json.Marshal(validDraft().Scenes[0])
			if err != nil {
				t.Fatalf("marshal scene: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]string{"role": "assistant", "content": string(content)}},
				},
			})
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	defer server.Close()

	generator := NewGenerator(config.Config{
		ModelBaseURL:         server.URL,
		ModelAPIKey:          "test-key",
		ModelName:            "script-pro",
		ModelTimeoutSeconds:  3,
		ModelMaxRetries:      0,
		ModelStructureMode:   "json_object",
		ModelMaxOutputTokens: 2000,
		ModelMaxInputChars:   12000,
	})

	draft, err := generator.Generate(context.Background(), generationInput())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if requestCount != 3 {
		t.Fatalf("expected full generation, plan, and scene requests, got %d", requestCount)
	}
	if len(draft.Scenes) != 1 || draft.Scenes[0].ID != "s01" {
		t.Fatalf("expected decomposed scene draft, got %#v", draft.Scenes)
	}
	if draft.Project.Title != "雨夜书店改编" || draft.Source.ChapterCount != 3 {
		t.Fatalf("expected normalized metadata, got project=%#v source=%#v", draft.Project, draft.Source)
	}
}

func TestGeneratorRegenerateSceneCallsLLMAndReturnsUpdatedScene(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("expected /chat/completions request, got %s", r.URL.Path)
		}

		next := validDraft()
		next.Scenes[0].Conflict = "重写后的冲突更集中"
		next.Scenes[0].Dialogues = append(next.Scenes[0].Dialogues, domain.Dialogue{Speaker: "c01", Line: "我必须现在做决定。"})
		content, err := json.Marshal(next)
		if err != nil {
			t.Fatalf("marshal draft: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": string(content)}},
			},
		})
	}))
	defer server.Close()

	generator := NewGenerator(config.Config{
		ModelBaseURL:         server.URL,
		ModelAPIKey:          "test-key",
		ModelName:            "script-pro",
		ModelTimeoutSeconds:  3,
		ModelMaxRetries:      0,
		ModelStructureMode:   "json_object",
		ModelMaxOutputTokens: 2000,
		ModelMaxInputChars:   12000,
	})

	next, scene, err := generator.RegenerateScene(context.Background(), validDraft(), "s01", "加强冲突")
	if err != nil {
		t.Fatalf("regenerate scene: %v", err)
	}
	if scene.Conflict != "重写后的冲突更集中" {
		t.Fatalf("expected updated scene conflict, got %s", scene.Conflict)
	}
	if next.Scenes[0].Dialogues[len(next.Scenes[0].Dialogues)-1].Line != "我必须现在做决定。" {
		t.Fatal("expected updated dialogue")
	}
}

func TestGeneratorCorrectsModelsBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("expected /v1/chat/completions request, got %s", r.URL.Path)
		}
		content, err := json.Marshal(validDraft())
		if err != nil {
			t.Fatalf("marshal draft: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": string(content)}},
			},
		})
	}))
	defer server.Close()

	generator := NewGenerator(config.Config{
		ModelBaseURL:         server.URL + "/v1/models",
		ModelAPIKey:          "test-key",
		ModelName:            "script-pro",
		ModelTimeoutSeconds:  3,
		ModelStructureMode:   "json_object",
		ModelMaxOutputTokens: 2000,
		ModelMaxInputChars:   12000,
	})

	if _, err := generator.Generate(context.Background(), generationInput()); err != nil {
		t.Fatalf("generate: %v", err)
	}
}

func TestGeneratorRepairDraftSendsValidationIssuesToLLM(t *testing.T) {
	var received chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		content, err := json.Marshal(validDraft())
		if err != nil {
			t.Fatalf("marshal draft: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": string(content)}},
			},
		})
	}))
	defer server.Close()

	generator := NewGenerator(config.Config{
		ModelBaseURL:         server.URL,
		ModelAPIKey:          "test-key",
		ModelName:            "script-pro",
		ModelTimeoutSeconds:  3,
		ModelStructureMode:   "json_object",
		ModelMaxOutputTokens: 2000,
		ModelMaxInputChars:   12000,
	})

	issues := []domain.ValidationIssue{{Path: "scenes[0].characters[0]", Message: "must reference an existing character"}}
	repaired, err := generator.RepairDraft(context.Background(), validDraft(), issues)
	if err != nil {
		t.Fatalf("repair draft: %v", err)
	}
	if repaired.Scenes[0].ID != "s01" {
		t.Fatalf("expected repaired draft, got %#v", repaired.Scenes)
	}
	if len(received.Messages) != 2 || !strings.Contains(received.Messages[1].Content, "scenes[0].characters[0]") {
		t.Fatalf("expected repair prompt to include validation issue, got %#v", received.Messages)
	}
}

func validPlan() scriptPlan {
	draft := validDraft()
	return scriptPlan{
		World:      draft.World,
		Characters: draft.Characters,
		Acts:       draft.Acts,
		Scenes:     []domain.Scene{draft.Scenes[0]},
		Continuity: draft.Continuity,
		Revision:   draft.Revision,
	}
}

func generationInput() GenerationInput {
	return GenerationInput{
		Project: domain.Project{
			Title:            "雨夜书店改编",
			AdaptationTarget: "web_series",
			Language:         "zh-CN",
		},
		Source: domain.SourceDocument{
			NovelTitle: "雨夜书店",
			Author:     "示例作者",
			Chapters: []domain.Chapter{
				{Index: 1, Title: "第一章 雨夜来客", Content: "林知夏推开旧书店的门，发现周衡藏起一张旧车票。"},
				{Index: 2, Title: "第二章 失踪名单", Content: "许燃带来失踪名单，三人确认名单被拆成数段。"},
				{Index: 3, Title: "第三章 旧站台", Content: "他们在废弃车站找到最后一页名单，也发现内应。"},
			},
		},
		Config: domain.GenerationConfig{
			Style:                 "克制、悬疑、影视化",
			TargetSceneCount:      3,
			DialogueDensity:       "medium",
			PreserveOriginalNames: true,
		},
	}
}

func validDraft() domain.ScriptDraft {
	return domain.ScriptDraft{
		SchemaVersion: "1.0",
		Project: domain.ScriptProject{
			Title:            "模型返回标题",
			AdaptationTarget: "web_series",
			Language:         "zh-CN",
			Genre:            []string{"悬疑"},
		},
		Source: domain.ScriptSource{
			NovelTitle:   "模型返回小说名",
			Author:       "模型返回作者",
			ChapterCount: 3,
			ChapterRefs:  []int{1, 2, 3},
		},
		World: domain.World{
			Logline: "一名编辑在雨夜书店追查失踪名单，并被迫面对旧案真相。",
			Theme:   []string{"真相", "信任"},
			Tone:    "克制、悬疑、影视化",
			Setting: "旧书店与废弃车站",
		},
		Characters: []domain.Character{
			{ID: "c01", Name: "林知夏", Role: "protagonist", Traits: []string{"敏锐"}},
			{ID: "c02", Name: "周衡", Role: "keeper", Traits: []string{"谨慎"}},
		},
		Acts: []domain.Act{
			{ID: "act1", Title: "入局", Purpose: "建立雨夜书店的秘密", SceneIDs: []string{"s01"}},
		},
		Scenes: []domain.Scene{
			{
				ID:          "s01",
				ActID:       "act1",
				ChapterRefs: []int{1},
				Location:    "旧书店",
				Time:        "雨夜",
				Purpose:     "让林知夏发现旧车票",
				Conflict:    "周衡试图阻止她继续追查",
				Summary:     "林知夏在雨夜进入书店，发现周衡藏起关键车票。",
				Characters:  []string{"c01", "c02"},
				Beats:       []string{"林知夏进店", "周衡遮掩车票", "两人达成临时交易"},
				Dialogues: []domain.Dialogue{
					{Speaker: "c01", Line: "这张车票不是普通旧物。"},
					{Speaker: "c02", Line: "知道得太早，对你没有好处。"},
				},
			},
		},
		Continuity: domain.Continuity{
			Timeline: []string{"雨夜书店事件开启调查"},
			Props:    []string{"旧车票"},
		},
		Revision: domain.Revision{
			GeneratedAt: "2026-06-07T00:00:00Z",
			Confidence:  "medium",
			EditorNotes: []string{"建议进一步打磨周衡的动机。"},
		},
	}
}

func respondJSON(w http.ResponseWriter, payload any) {
	content, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]string{"role": "assistant", "content": string(content)}},
		},
	})
}
