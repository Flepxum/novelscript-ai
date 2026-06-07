package parser

import "testing"

func TestSplitChaptersChinese(t *testing.T) {
	input := `第一章 雨夜
林知夏推开旧书店的门。

第二章 来信
周衡把信藏进书脊。

第三章 火光
许燃在巷口看见火光。`

	chapters, err := SplitChapters(input)
	if err != nil {
		t.Fatalf("SplitChapters returned error: %v", err)
	}
	if len(chapters) != 3 {
		t.Fatalf("expected 3 chapters, got %d", len(chapters))
	}
	if chapters[0].Title != "第一章 雨夜" {
		t.Fatalf("unexpected first title: %s", chapters[0].Title)
	}
}

func TestSplitChaptersEnglish(t *testing.T) {
	input := `Chapter 1 The Door
Someone knocks.

Chapter 2 The Map
The map is incomplete.

Chapter 3 The Choice
The lead chooses to leave.`

	chapters, err := SplitChapters(input)
	if err != nil {
		t.Fatalf("SplitChapters returned error: %v", err)
	}
	if chapters[1].Title != "Chapter 2 The Map" {
		t.Fatalf("unexpected second title: %s", chapters[1].Title)
	}
}

func TestSplitChaptersRejectsTooFewChapters(t *testing.T) {
	_, err := SplitChapters("第一章\n只有一章")
	if err == nil {
		t.Fatal("expected error for fewer than 3 chapters")
	}
}
