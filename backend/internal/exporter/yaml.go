package exporter

import (
	"github.com/Flepxum/novelscript-ai/backend/internal/domain"
	"gopkg.in/yaml.v3"
)

func ToYAML(draft domain.ScriptDraft) (string, error) {
	data, err := yaml.Marshal(draft)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func FromYAML(input string) (domain.ScriptDraft, error) {
	var draft domain.ScriptDraft
	err := yaml.Unmarshal([]byte(input), &draft)
	return draft, err
}
