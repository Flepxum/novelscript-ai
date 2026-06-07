package domain

import "time"

type Project struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	AdaptationTarget string    `json:"adaptation_target"`
	Language         string    `json:"language"`
	CreatedAt        time.Time `json:"created_at"`
}

type SourceDocument struct {
	ID         string    `json:"source_id"`
	ProjectID  string    `json:"project_id"`
	NovelTitle string    `json:"novel_title"`
	Author     string    `json:"author"`
	Content    string    `json:"content,omitempty"`
	Chapters   []Chapter `json:"chapters"`
	CreatedAt  time.Time `json:"created_at"`
}

type Chapter struct {
	Index     int    `json:"index" yaml:"index"`
	Title     string `json:"title" yaml:"title"`
	Content   string `json:"content,omitempty" yaml:"content,omitempty"`
	WordCount int    `json:"word_count" yaml:"word_count"`
}

type GenerationConfig struct {
	Style                 string `json:"style"`
	TargetSceneCount      int    `json:"target_scene_count"`
	DialogueDensity       string `json:"dialogue_density"`
	PreserveOriginalNames bool   `json:"preserve_original_names"`
}

type GenerationJob struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"project_id"`
	Status      string     `json:"status"`
	Progress    int        `json:"progress"`
	CurrentStep string     `json:"current_step"`
	Error       *string    `json:"error"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

type ScriptVersion struct {
	ID         string      `json:"version_id"`
	ProjectID  string      `json:"project_id"`
	YAML       string      `json:"yaml"`
	Draft      ScriptDraft `json:"draft"`
	EditorNote string      `json:"editor_note,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
}

type ScriptDraft struct {
	SchemaVersion string        `json:"schema_version" yaml:"schema_version"`
	Project       ScriptProject `json:"project" yaml:"project"`
	Source        ScriptSource  `json:"source" yaml:"source"`
	World         World         `json:"world" yaml:"world"`
	Characters    []Character   `json:"characters" yaml:"characters"`
	Acts          []Act         `json:"acts" yaml:"acts"`
	Scenes        []Scene       `json:"scenes" yaml:"scenes"`
	Continuity    Continuity    `json:"continuity" yaml:"continuity"`
	Revision      Revision      `json:"revision" yaml:"revision"`
}

type ScriptProject struct {
	Title            string   `json:"title" yaml:"title"`
	AdaptationTarget string   `json:"adaptation_target" yaml:"adaptation_target"`
	Language         string   `json:"language" yaml:"language"`
	Genre            []string `json:"genre,omitempty" yaml:"genre,omitempty"`
}

type ScriptSource struct {
	NovelTitle   string `json:"novel_title,omitempty" yaml:"novel_title,omitempty"`
	Author       string `json:"author,omitempty" yaml:"author,omitempty"`
	ChapterCount int    `json:"chapter_count" yaml:"chapter_count"`
	ChapterRefs  []int  `json:"chapter_refs" yaml:"chapter_refs"`
}

type World struct {
	Logline string   `json:"logline" yaml:"logline"`
	Theme   []string `json:"theme" yaml:"theme"`
	Tone    string   `json:"tone" yaml:"tone"`
	Setting string   `json:"setting,omitempty" yaml:"setting,omitempty"`
}

type Character struct {
	ID            string         `json:"id" yaml:"id"`
	Name          string         `json:"name" yaml:"name"`
	Aliases       []string       `json:"aliases,omitempty" yaml:"aliases,omitempty"`
	Role          string         `json:"role" yaml:"role"`
	Traits        []string       `json:"traits,omitempty" yaml:"traits,omitempty"`
	Goal          string         `json:"goal,omitempty" yaml:"goal,omitempty"`
	Conflict      string         `json:"conflict,omitempty" yaml:"conflict,omitempty"`
	Arc           string         `json:"arc,omitempty" yaml:"arc,omitempty"`
	Relationships []Relationship `json:"relationships,omitempty" yaml:"relationships,omitempty"`
}

type Relationship struct {
	Target      string `json:"target" yaml:"target"`
	Type        string `json:"type" yaml:"type"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

type Act struct {
	ID       string   `json:"id" yaml:"id"`
	Title    string   `json:"title" yaml:"title"`
	Purpose  string   `json:"purpose" yaml:"purpose"`
	SceneIDs []string `json:"scene_ids" yaml:"scene_ids"`
}

type Scene struct {
	ID            string     `json:"id" yaml:"id"`
	ActID         string     `json:"act_id" yaml:"act_id"`
	ChapterRefs   []int      `json:"chapter_refs" yaml:"chapter_refs"`
	Location      string     `json:"location" yaml:"location"`
	Time          string     `json:"time" yaml:"time"`
	Purpose       string     `json:"purpose" yaml:"purpose"`
	Conflict      string     `json:"conflict" yaml:"conflict"`
	Summary       string     `json:"summary" yaml:"summary"`
	Characters    []string   `json:"characters" yaml:"characters"`
	Beats         []string   `json:"beats" yaml:"beats"`
	Dialogues     []Dialogue `json:"dialogues,omitempty" yaml:"dialogues,omitempty"`
	Notes         []string   `json:"notes,omitempty" yaml:"notes,omitempty"`
	AIAssumptions []string   `json:"ai_assumptions,omitempty" yaml:"ai_assumptions,omitempty"`
}

type Dialogue struct {
	Speaker       string `json:"speaker" yaml:"speaker"`
	Line          string `json:"line" yaml:"line"`
	Parenthetical string `json:"parenthetical,omitempty" yaml:"parenthetical,omitempty"`
	Action        string `json:"action,omitempty" yaml:"action,omitempty"`
}

type Continuity struct {
	Timeline      []string `json:"timeline,omitempty" yaml:"timeline,omitempty"`
	OpenThreads   []string `json:"open_threads,omitempty" yaml:"open_threads,omitempty"`
	Foreshadowing []string `json:"foreshadowing,omitempty" yaml:"foreshadowing,omitempty"`
	Props         []string `json:"props,omitempty" yaml:"props,omitempty"`
}

type Revision struct {
	GeneratedAt   string   `json:"generated_at,omitempty" yaml:"generated_at,omitempty"`
	Confidence    string   `json:"confidence,omitempty" yaml:"confidence,omitempty"`
	EditorNotes   []string `json:"editor_notes,omitempty" yaml:"editor_notes,omitempty"`
	AIAssumptions []string `json:"ai_assumptions,omitempty" yaml:"ai_assumptions,omitempty"`
}

type ValidationIssue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}
