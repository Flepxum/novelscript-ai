package ai

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/Flepxum/novelscript-ai/backend/internal/domain"
	"github.com/Flepxum/novelscript-ai/backend/internal/parser"
)

type ChapterSegmentationAgent struct {
	generator *Generator
}

type sourceParagraph struct {
	Index int
	Text  string
}

type paragraphWindow struct {
	StartIndex int
	EndIndex   int
	Items      []sourceParagraph
}

type chapterSegmentationResponse struct {
	Chapters []chapterBoundary `json:"chapters"`
}

type chapterBoundary struct {
	Title          string `json:"title"`
	StartParagraph int    `json:"start_paragraph"`
	EndParagraph   int    `json:"end_paragraph,omitempty"`
	Confidence     string `json:"confidence,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

func NewChapterSegmentationAgent(generator *Generator) *ChapterSegmentationAgent {
	return &ChapterSegmentationAgent{generator: generator}
}

func (g *Generator) SplitChapters(ctx context.Context, novelTitle, author, content string, minChapters int) ([]domain.Chapter, error) {
	return NewChapterSegmentationAgent(g).SplitChapters(ctx, novelTitle, author, content, minChapters)
}

func (a *ChapterSegmentationAgent) SplitChapters(ctx context.Context, novelTitle, author, content string, minChapters int) ([]domain.Chapter, error) {
	cleaned := parser.CleanText(content)
	if cleaned == "" {
		return nil, fmt.Errorf("novel content is empty")
	}
	if minChapters <= 0 {
		minChapters = 3
	}

	paragraphs := buildSourceParagraphs(cleaned, minChapters)
	if len(paragraphs) < minChapters {
		return nil, fmt.Errorf("not enough narrative paragraphs for agent segmentation: got %d, need at least %d", len(paragraphs), minChapters)
	}

	windows := buildParagraphWindows(paragraphs, a.generator.cfg.ModelMaxInputChars)
	log.Printf(
		"Chapter segmentation agent started: novel_title=%s author=%s paragraphs=%d windows=%d min_chapters=%d",
		valueOrUnknown(novelTitle),
		valueOrUnknown(author),
		len(paragraphs),
		len(windows),
		minChapters,
	)

	var boundaries []chapterBoundary
	for index, window := range windows {
		log.Printf(
			"Chapter segmentation agent step started: step=detect_boundaries window=%d/%d paragraph_range=%d-%d",
			index+1,
			len(windows),
			window.StartIndex,
			window.EndIndex,
		)
		content, err := a.generator.callJSONCompletion(ctx, chapterSegmentationMessages(novelTitle, author, paragraphs, window, minChapters), "chapter_segmentation_json")
		if err != nil {
			return nil, fmt.Errorf("chapter segmentation window %d: %w", index+1, err)
		}
		result, err := a.decodeSegmentationResponse(ctx, novelTitle, author, paragraphs, window, minChapters, content)
		if err != nil {
			return nil, fmt.Errorf("parse chapter segmentation window %d: %w", index+1, err)
		}
		boundaries = append(boundaries, result.Chapters...)
		log.Printf(
			"Chapter segmentation agent step succeeded: step=detect_boundaries window=%d/%d boundaries=%d",
			index+1,
			len(windows),
			len(result.Chapters),
		)
	}

	chapters, err := assembleAgentChapters(paragraphs, boundaries, minChapters)
	if err != nil {
		log.Printf("Chapter segmentation agent boundary assembly failed: error=%s boundaries=%d", err, len(boundaries))
		repaired, repairErr := a.repairChapterBoundaries(ctx, novelTitle, author, paragraphs, boundaries, minChapters, err)
		if repairErr != nil {
			return nil, fmt.Errorf("%w; boundary repair failed: %v", err, repairErr)
		}
		chapters, err = assembleAgentChapters(paragraphs, repaired.Chapters, minChapters)
		if err != nil {
			return nil, fmt.Errorf("assemble repaired chapter boundaries: %w", err)
		}
	}

	log.Printf("Chapter segmentation agent succeeded: chapters=%d paragraphs=%d", len(chapters), len(paragraphs))
	return chapters, nil
}

func (a *ChapterSegmentationAgent) decodeSegmentationResponse(
	ctx context.Context,
	novelTitle string,
	author string,
	paragraphs []sourceParagraph,
	window paragraphWindow,
	minChapters int,
	raw string,
) (chapterSegmentationResponse, error) {
	var result chapterSegmentationResponse
	if err := decodeModelJSON(raw, &result); err != nil {
		log.Printf(
			"Chapter segmentation agent parse failed: step=detect_boundaries paragraph_range=%d-%d error=%s raw_chars=%d",
			window.StartIndex,
			window.EndIndex,
			err,
			len([]rune(raw)),
		)
		repaired, repairErr := a.generator.callJSONCompletion(
			ctx,
			malformedChapterSegmentationMessages(novelTitle, author, paragraphs, window, minChapters, raw, err),
			"chapter_segmentation_repair_json",
		)
		if repairErr != nil {
			return chapterSegmentationResponse{}, fmt.Errorf("parse chapter segmentation json: %w; malformed-json repair failed: %v", err, repairErr)
		}
		if err := decodeModelJSON(repaired, &result); err != nil {
			return chapterSegmentationResponse{}, fmt.Errorf("parse repaired chapter segmentation json: %w", err)
		}
	}
	return result, nil
}

func (a *ChapterSegmentationAgent) repairChapterBoundaries(
	ctx context.Context,
	novelTitle string,
	author string,
	paragraphs []sourceParagraph,
	boundaries []chapterBoundary,
	minChapters int,
	assemblyErr error,
) (chapterSegmentationResponse, error) {
	log.Printf("Chapter segmentation agent step started: step=repair_boundaries min_chapters=%d error=%s", minChapters, assemblyErr)
	content, err := a.generator.callJSONCompletion(
		ctx,
		chapterBoundaryRepairMessages(novelTitle, author, paragraphs, boundaries, minChapters, assemblyErr),
		"chapter_boundary_repair_json",
	)
	if err != nil {
		return chapterSegmentationResponse{}, err
	}
	var result chapterSegmentationResponse
	if err := decodeModelJSON(content, &result); err != nil {
		return chapterSegmentationResponse{}, err
	}
	log.Printf("Chapter segmentation agent step succeeded: step=repair_boundaries boundaries=%d", len(result.Chapters))
	return result, nil
}

func chapterSegmentationMessages(novelTitle, author string, all []sourceParagraph, window paragraphWindow, minChapters int) []chatMessage {
	userPrompt := fmt.Sprintf(`请作为 Chapter Segmentation Agent，根据段落编号识别小说章节边界。

小说:
- title: %s
- author: %s
- total_paragraphs: %d
- current_window: %d-%d
- minimum_required_chapters: %d

任务要求:
1. 只返回 JSON object，不要 Markdown，不要解释。
2. 不要返回小说正文，只返回章节标题和段落边界。
3. 如果原文没有标准“第 X 章”标题，请根据时间跳转、地点变化、叙事视角、目标变化、重大转折智能推断章节起点。
4. start_paragraph 必须使用下面的全局段落编号，且必须落在 current_window 内。
5. 如果一个章节从窗口之前已经开始，不要重复返回它；只返回起点在当前窗口内的章节。
6. JSON 顶层字段只能是 chapters。
7. chapters 中每项字段为 title, start_paragraph, end_paragraph, confidence, reason。

段落索引:
%s`,
		valueOrUnknown(novelTitle),
		valueOrUnknown(author),
		len(all),
		window.StartIndex,
		window.EndIndex,
		minChapters,
		renderParagraphMap(window.Items),
	)
	return []chatMessage{
		{Role: "system", Content: chapterSegmentationSystemPrompt()},
		{Role: "user", Content: userPrompt},
	}
}

func malformedChapterSegmentationMessages(
	novelTitle string,
	author string,
	paragraphs []sourceParagraph,
	window paragraphWindow,
	minChapters int,
	raw string,
	parseErr error,
) []chatMessage {
	userPrompt := fmt.Sprintf(`上一次 Chapter Segmentation Agent 输出无法解析，请修复为严格 JSON。

解析错误:
%s

小说:
- title: %s
- author: %s
- total_paragraphs: %d
- current_window: %d-%d
- minimum_required_chapters: %d

段落索引:
%s

上一次原始输出:
%s

修复要求:
1. 只输出 JSON object，不要 Markdown。
2. 顶层字段只能是 chapters。
3. chapters[].start_paragraph 必须是整数，且落在 current_window 内。
4. 不要返回正文。`,
		parseErr,
		valueOrUnknown(novelTitle),
		valueOrUnknown(author),
		len(paragraphs),
		window.StartIndex,
		window.EndIndex,
		minChapters,
		renderParagraphMap(window.Items),
		truncateRunes(raw, 8000),
	)
	return []chatMessage{
		{Role: "system", Content: chapterSegmentationSystemPrompt()},
		{Role: "user", Content: userPrompt},
	}
}

func chapterBoundaryRepairMessages(
	novelTitle string,
	author string,
	paragraphs []sourceParagraph,
	boundaries []chapterBoundary,
	minChapters int,
	assemblyErr error,
) []chatMessage {
	boundaryLines := make([]string, 0, len(boundaries))
	for _, boundary := range boundaries {
		boundaryLines = append(boundaryLines, fmt.Sprintf(
			`{"title":"%s","start_paragraph":%d,"confidence":"%s"}`,
			escapePromptString(boundary.Title),
			boundary.StartParagraph,
			escapePromptString(boundary.Confidence),
		))
	}
	userPrompt := fmt.Sprintf(`请作为 Chapter Boundary Repair Agent，修复章节边界列表。

小说:
- title: %s
- author: %s
- total_paragraphs: %d
- minimum_required_chapters: %d

组装错误:
%s

已有边界:
[%s]

全局段落索引:
%s

修复要求:
1. 只输出 JSON object，不要 Markdown。
2. 顶层字段只能是 chapters。
3. 至少返回 %d 个章节边界。
4. start_paragraph 必须是 1 到 %d 之间的整数。
5. 第一个章节必须从 start_paragraph=1 开始。
6. 不要返回小说正文。`,
		valueOrUnknown(novelTitle),
		valueOrUnknown(author),
		len(paragraphs),
		minChapters,
		assemblyErr,
		strings.Join(boundaryLines, ","),
		renderParagraphMap(limitParagraphMap(paragraphs, 180)),
		minChapters,
		len(paragraphs),
	)
	return []chatMessage{
		{Role: "system", Content: chapterSegmentationSystemPrompt()},
		{Role: "user", Content: userPrompt},
	}
}

func chapterSegmentationSystemPrompt() string {
	return `你是 NovelScript Chapter Segmentation Agent，专门把长篇、不规范小说输入切分成可追溯章节。你只输出严格 JSON。你不能复述小说正文，不能编造不存在的段落编号，不能输出 Markdown。`
}

func buildSourceParagraphs(content string, minChapters int) []sourceParagraph {
	blocks := splitBlankSeparatedBlocks(content)
	lines := splitNonEmptyLines(content)
	if len(blocks) < minChapters && len(lines) >= minChapters {
		blocks = lines
	}
	if len(blocks) < minChapters {
		blocks = splitIntoSentenceBlocks(content, 480)
	}

	paragraphs := make([]sourceParagraph, 0, len(blocks))
	for _, block := range blocks {
		trimmed := strings.TrimSpace(block)
		if trimmed == "" {
			continue
		}
		paragraphs = append(paragraphs, sourceParagraph{
			Index: len(paragraphs) + 1,
			Text:  trimmed,
		})
	}
	return paragraphs
}

func splitBlankSeparatedBlocks(content string) []string {
	var blocks []string
	var buffer []string
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "" {
			if len(buffer) > 0 {
				blocks = append(blocks, strings.Join(buffer, "\n"))
				buffer = nil
			}
			continue
		}
		buffer = append(buffer, line)
	}
	if len(buffer) > 0 {
		blocks = append(blocks, strings.Join(buffer, "\n"))
	}
	return blocks
}

func splitNonEmptyLines(content string) []string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

func splitIntoSentenceBlocks(content string, maxRunes int) []string {
	if maxRunes <= 0 {
		maxRunes = 480
	}
	var blocks []string
	var builder strings.Builder
	count := 0
	flush := func() {
		text := strings.TrimSpace(builder.String())
		if text != "" {
			blocks = append(blocks, text)
		}
		builder.Reset()
		count = 0
	}

	for _, r := range content {
		builder.WriteRune(r)
		count++
		if count >= maxRunes || (count >= 120 && isSentenceBoundary(r)) {
			flush()
		}
	}
	flush()
	return blocks
}

func isSentenceBoundary(r rune) bool {
	switch r {
	case '。', '！', '？', '；', '.', '!', '?', ';':
		return true
	default:
		return false
	}
}

func buildParagraphWindows(paragraphs []sourceParagraph, maxInputChars int) []paragraphWindow {
	if len(paragraphs) == 0 {
		return nil
	}
	maxMapChars := maxInputChars * 2 / 3
	if maxMapChars <= 0 {
		maxMapChars = 16000
	}
	if maxMapChars < 6000 {
		maxMapChars = 6000
	}

	var windows []paragraphWindow
	for start := 0; start < len(paragraphs); {
		end := start
		chars := 0
		for end < len(paragraphs) {
			line := renderParagraphLine(paragraphs[end])
			if end > start && chars+len([]rune(line)) > maxMapChars {
				break
			}
			chars += len([]rune(line))
			end++
		}
		if end <= start {
			end = start + 1
		}
		items := paragraphs[start:end]
		windows = append(windows, paragraphWindow{
			StartIndex: items[0].Index,
			EndIndex:   items[len(items)-1].Index,
			Items:      items,
		})
		if end >= len(paragraphs) {
			break
		}
		overlap := 8
		next := end - overlap
		if next <= start {
			next = end
		}
		start = next
	}
	return windows
}

func renderParagraphMap(paragraphs []sourceParagraph) string {
	var builder strings.Builder
	for _, paragraph := range paragraphs {
		builder.WriteString(renderParagraphLine(paragraph))
	}
	return builder.String()
}

func renderParagraphLine(paragraph sourceParagraph) string {
	return fmt.Sprintf("P%04d | %s\n", paragraph.Index, compactParagraphPreview(paragraph.Text))
}

func compactParagraphPreview(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	return truncateRunes(text, 260)
}

func assembleAgentChapters(paragraphs []sourceParagraph, boundaries []chapterBoundary, minChapters int) ([]domain.Chapter, error) {
	normalized := normalizeChapterBoundaries(boundaries, len(paragraphs))
	if len(normalized) < minChapters {
		return nil, fmt.Errorf("chapter segmentation returned %d chapters, need at least %d", len(normalized), minChapters)
	}

	chapters := make([]domain.Chapter, 0, len(normalized))
	for index, boundary := range normalized {
		start := boundary.StartParagraph
		end := len(paragraphs)
		if index+1 < len(normalized) {
			end = normalized[index+1].StartParagraph - 1
		}
		if start > end || start < 1 || end > len(paragraphs) {
			continue
		}

		contentBlocks := make([]string, 0, end-start+1)
		for _, paragraph := range paragraphs[start-1 : end] {
			contentBlocks = append(contentBlocks, paragraph.Text)
		}
		title := strings.TrimSpace(boundary.Title)
		if title == "" {
			title = fmt.Sprintf("第%d章", len(chapters)+1)
		}
		body := strings.TrimSpace(strings.Join(contentBlocks, "\n\n"))
		chapters = append(chapters, domain.Chapter{
			Index:     len(chapters) + 1,
			Title:     title,
			Content:   body,
			WordCount: parser.CountWords(body),
		})
	}
	if len(chapters) < minChapters {
		return nil, fmt.Errorf("assembled %d chapters after filtering empty boundaries, need at least %d", len(chapters), minChapters)
	}
	return chapters, nil
}

func normalizeChapterBoundaries(boundaries []chapterBoundary, paragraphCount int) []chapterBoundary {
	byStart := map[int]chapterBoundary{}
	for _, boundary := range boundaries {
		if boundary.StartParagraph < 1 || boundary.StartParagraph > paragraphCount {
			continue
		}
		current, exists := byStart[boundary.StartParagraph]
		if !exists || boundaryRank(boundary) > boundaryRank(current) {
			byStart[boundary.StartParagraph] = boundary
		}
	}
	if _, exists := byStart[1]; !exists && paragraphCount > 0 {
		byStart[1] = chapterBoundary{
			Title:          "开篇",
			StartParagraph: 1,
			Confidence:     "medium",
			Reason:         "系统补齐第一个章节起点，保证章节可从原文开头追溯。",
		}
	}

	starts := make([]int, 0, len(byStart))
	for start := range byStart {
		starts = append(starts, start)
	}
	sort.Ints(starts)

	result := make([]chapterBoundary, 0, len(starts))
	for _, start := range starts {
		boundary := byStart[start]
		if strings.TrimSpace(boundary.Title) == "" {
			boundary.Title = fmt.Sprintf("第%d章", len(result)+1)
		}
		result = append(result, boundary)
	}
	return result
}

func boundaryRank(boundary chapterBoundary) int {
	rank := len(strings.TrimSpace(boundary.Title))
	switch strings.ToLower(strings.TrimSpace(boundary.Confidence)) {
	case "high":
		rank += 100
	case "medium":
		rank += 50
	}
	return rank
}

func limitParagraphMap(paragraphs []sourceParagraph, maxItems int) []sourceParagraph {
	if maxItems <= 0 || len(paragraphs) <= maxItems {
		return paragraphs
	}
	head := maxItems / 2
	tail := maxItems - head
	result := make([]sourceParagraph, 0, maxItems)
	result = append(result, paragraphs[:head]...)
	result = append(result, paragraphs[len(paragraphs)-tail:]...)
	return result
}

func escapePromptString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
