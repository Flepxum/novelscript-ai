package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Flepxum/novelscript-ai/backend/internal/config"
)

func TestGeneratorSplitChaptersUsesLLMParagraphBoundaries(t *testing.T) {
	var received chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("expected /chat/completions request, got %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		content := `{
			"chapters": [
				{"title": "雨夜书店", "start_paragraph": 1, "confidence": "high", "reason": "开篇建立地点与人物"},
				{"title": "失踪名单", "start_paragraph": 3, "confidence": "high", "reason": "清晨后目标切换为名单"},
				{"title": "旧站台", "start_paragraph": 5, "confidence": "high", "reason": "地点切换到车站并进入高潮"}
			]
		}`
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": content}},
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

	chapters, err := generator.SplitChapters(context.Background(), "雨夜书店", "示例作者", nonStandardNovelText(), 3)
	if err != nil {
		t.Fatalf("split chapters: %v", err)
	}
	if len(chapters) != 3 {
		t.Fatalf("expected 3 chapters, got %d", len(chapters))
	}
	if chapters[1].Title != "失踪名单" || !strings.Contains(chapters[1].Content, "失踪名单") {
		t.Fatalf("unexpected second chapter: %#v", chapters[1])
	}
	if received.ResponseFormat == nil {
		t.Fatal("expected json response_format")
	}
	if !strings.Contains(received.Messages[1].Content, "P0001") || !strings.Contains(received.Messages[1].Content, "不要返回小说正文") {
		t.Fatalf("expected paragraph boundary prompt, got %s", received.Messages[1].Content)
	}
}

func TestAssembleAgentChaptersAddsOpeningBoundary(t *testing.T) {
	paragraphs := []sourceParagraph{
		{Index: 1, Text: "开篇段落。"},
		{Index: 2, Text: "转折段落。"},
		{Index: 3, Text: "高潮段落。"},
	}
	chapters, err := assembleAgentChapters(paragraphs, []chapterBoundary{
		{Title: "转折", StartParagraph: 2},
		{Title: "高潮", StartParagraph: 3},
	}, 3)
	if err != nil {
		t.Fatalf("assemble chapters: %v", err)
	}
	if chapters[0].Title != "开篇" || !strings.Contains(chapters[0].Content, "开篇段落") {
		t.Fatalf("expected opening boundary to be added, got %#v", chapters[0])
	}
}

func nonStandardNovelText() string {
	return `雨水从旧书店的檐角落下，林知夏推门时，门铃没有响。

周衡把一张旧车票压进书脊，像是在藏住某个不能被叫醒的名字。

清晨，许燃带着失踪名单来到书店，名单上每个名字都被撕去了一角。

三人发现名单被拆成三段，而最后一段指向二十年前停运的旧站台。

夜里，废弃车站的广播忽然响起，提示下一班列车即将进站。

林知夏在站台尽头看见内应，也看见周衡多年沉默背后的真相。`
}
