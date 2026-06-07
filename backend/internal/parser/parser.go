package parser

import (
	"errors"
	"regexp"
	"strings"
	"unicode"

	"github.com/Flepxum/novelscript-ai/backend/internal/domain"
)

var chapterHeading = regexp.MustCompile(`^\s*((第[一二三四五六七八九十百千万〇零两0-9]+[章节回卷部篇].*)|(Chapter\s+[0-9IVXLCDM]+.*)|(CHAPTER\s+[0-9IVXLCDM]+.*)|([0-9]{1,3}[.、]\s*.+))\s*$`)

func CleanText(input string) string {
	text := strings.ReplaceAll(input, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimPrefix(text, "\ufeff")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRightFunc(line, unicode.IsSpace)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func SplitChapters(input string) ([]domain.Chapter, error) {
	text := CleanText(input)
	if text == "" {
		return nil, errors.New("novel content is empty")
	}

	lines := strings.Split(text, "\n")
	var chapters []domain.Chapter
	var title string
	var buffer []string

	flush := func() {
		body := strings.TrimSpace(strings.Join(buffer, "\n"))
		if title == "" && body == "" {
			return
		}
		if title == "" {
			title = "未命名章节"
		}
		chapters = append(chapters, domain.Chapter{
			Index:     len(chapters) + 1,
			Title:     title,
			Content:   body,
			WordCount: CountWords(body),
		})
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if chapterHeading.MatchString(trimmed) {
			flush()
			title = trimmed
			buffer = nil
			continue
		}
		buffer = append(buffer, line)
	}
	flush()

	if len(chapters) < 3 {
		return nil, errors.New("at least 3 chapters are required")
	}
	return chapters, nil
}

func CountWords(input string) int {
	count := 0
	inASCIIWord := false
	for _, r := range input {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			inASCIIWord = false
			continue
		}
		if unicode.Is(unicode.Han, r) {
			count++
			inASCIIWord = false
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if !inASCIIWord {
				count++
				inASCIIWord = true
			}
			continue
		}
		inASCIIWord = false
	}
	return count
}
