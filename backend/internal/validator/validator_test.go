package validator

import (
	"testing"

	"github.com/Flepxum/novelscript-ai/backend/internal/domain"
)

func TestValidateDraftAcceptsValidDraft(t *testing.T) {
	issues := ValidateDraft(validDraft())
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %#v", issues)
	}
}

func TestValidateDraftRejectsInvalidChapterRef(t *testing.T) {
	draft := validDraft()
	draft.Scenes[0].ChapterRefs = []int{9}

	issues := ValidateDraft(draft)
	if len(issues) == 0 {
		t.Fatal("expected validation issue")
	}
}

func TestValidateDraftRejectsInvalidCharacterRef(t *testing.T) {
	draft := validDraft()
	draft.Scenes[0].Characters = []string{"missing"}

	issues := ValidateDraft(draft)
	if len(issues) == 0 {
		t.Fatal("expected validation issue")
	}
}

func validDraft() domain.ScriptDraft {
	return domain.ScriptDraft{
		SchemaVersion: "1.0",
		Project: domain.ScriptProject{
			Title:            "测试项目",
			AdaptationTarget: "web_series",
			Language:         "zh-CN",
		},
		Source: domain.ScriptSource{
			ChapterCount: 3,
			ChapterRefs:  []int{1, 2, 3},
		},
		World: domain.World{
			Logline: "主角在三章故事中完成选择。",
			Theme:   []string{"选择"},
			Tone:    "克制",
		},
		Characters: []domain.Character{
			{ID: "c01", Name: "主角", Role: "protagonist"},
		},
		Acts: []domain.Act{
			{ID: "act1", Title: "入局", Purpose: "建立冲突", SceneIDs: []string{"s01"}},
		},
		Scenes: []domain.Scene{
			{
				ID:          "s01",
				ActID:       "act1",
				ChapterRefs: []int{1},
				Location:    "旧宅",
				Time:        "夜",
				Purpose:     "引出主线",
				Conflict:    "线索消失",
				Summary:     "主角发现线索。",
				Characters:  []string{"c01"},
				Beats:       []string{"进入旧宅"},
			},
		},
		Continuity: domain.Continuity{},
		Revision:   domain.Revision{Confidence: "medium"},
	}
}
