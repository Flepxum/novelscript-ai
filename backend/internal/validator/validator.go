package validator

import (
	"fmt"

	"github.com/Flepxum/novelscript-ai/backend/internal/domain"
)

func ValidateDraft(draft domain.ScriptDraft) []domain.ValidationIssue {
	var issues []domain.ValidationIssue

	if draft.SchemaVersion != "1.0" {
		issues = append(issues, issue("schema_version", "must be 1.0"))
	}
	if draft.Project.Title == "" {
		issues = append(issues, issue("project.title", "is required"))
	}
	if draft.Project.AdaptationTarget == "" {
		issues = append(issues, issue("project.adaptation_target", "is required"))
	}
	if draft.Project.Language == "" {
		issues = append(issues, issue("project.language", "is required"))
	}
	if draft.Source.ChapterCount < 3 {
		issues = append(issues, issue("source.chapter_count", "must be at least 3"))
	}
	if len(draft.Source.ChapterRefs) < 3 {
		issues = append(issues, issue("source.chapter_refs", "must contain at least 3 chapter references"))
	}
	if len(draft.Characters) == 0 {
		issues = append(issues, issue("characters", "must contain at least 1 character"))
	}
	if len(draft.Acts) == 0 {
		issues = append(issues, issue("acts", "must contain at least 1 act"))
	}
	if len(draft.Scenes) == 0 {
		issues = append(issues, issue("scenes", "must contain at least 1 scene"))
	}

	characters := map[string]bool{}
	for i, character := range draft.Characters {
		path := fmt.Sprintf("characters[%d]", i)
		if character.ID == "" {
			issues = append(issues, issue(path+".id", "is required"))
			continue
		}
		if characters[character.ID] {
			issues = append(issues, issue(path+".id", "must be unique"))
		}
		characters[character.ID] = true
		if character.Name == "" {
			issues = append(issues, issue(path+".name", "is required"))
		}
		if character.Role == "" {
			issues = append(issues, issue(path+".role", "is required"))
		}
	}

	acts := map[string]bool{}
	for i, act := range draft.Acts {
		path := fmt.Sprintf("acts[%d]", i)
		if act.ID == "" {
			issues = append(issues, issue(path+".id", "is required"))
			continue
		}
		if acts[act.ID] {
			issues = append(issues, issue(path+".id", "must be unique"))
		}
		acts[act.ID] = true
		if len(act.SceneIDs) == 0 {
			issues = append(issues, issue(path+".scene_ids", "must reference at least 1 scene"))
		}
	}

	scenes := map[string]bool{}
	for i, scene := range draft.Scenes {
		path := fmt.Sprintf("scenes[%d]", i)
		if scene.ID == "" {
			issues = append(issues, issue(path+".id", "is required"))
			continue
		}
		if scenes[scene.ID] {
			issues = append(issues, issue(path+".id", "must be unique"))
		}
		scenes[scene.ID] = true
		if !acts[scene.ActID] {
			issues = append(issues, issue(path+".act_id", "must reference an existing act"))
		}
		if len(scene.ChapterRefs) == 0 {
			issues = append(issues, issue(path+".chapter_refs", "must contain at least 1 chapter"))
		}
		for j, ref := range scene.ChapterRefs {
			if ref < 1 || ref > draft.Source.ChapterCount {
				issues = append(issues, issue(fmt.Sprintf("%s.chapter_refs[%d]", path, j), "must reference an existing source chapter"))
			}
		}
		if len(scene.Characters) == 0 {
			issues = append(issues, issue(path+".characters", "must contain at least 1 character"))
		}
		for j, characterID := range scene.Characters {
			if !characters[characterID] {
				issues = append(issues, issue(fmt.Sprintf("%s.characters[%d]", path, j), "must reference an existing character"))
			}
		}
		if len(scene.Beats) == 0 {
			issues = append(issues, issue(path+".beats", "must contain at least 1 beat"))
		}
	}

	for i, act := range draft.Acts {
		for j, sceneID := range act.SceneIDs {
			if !scenes[sceneID] {
				issues = append(issues, issue(fmt.Sprintf("acts[%d].scene_ids[%d]", i, j), "must reference an existing scene"))
			}
		}
	}

	return issues
}

func issue(path, message string) domain.ValidationIssue {
	return domain.ValidationIssue{Path: path, Message: message}
}
