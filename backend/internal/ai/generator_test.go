package ai

import "testing"

func TestExtractNamesNormalizesNarrationPrefixes(t *testing.T) {
	content := `林知夏推开旧书店的门。柜台后的周衡说今晚不营业。
林知夏说如果现在停手，过去十年就都白等了。
许燃看着车票沉默很久，周衡低声问她是不是还想查下去。`

	names := extractNames(content)
	expected := map[string]bool{"林知夏": true, "周衡": true, "许燃": true}
	for _, name := range names {
		delete(expected, name)
	}
	if len(expected) != 0 {
		t.Fatalf("expected extracted names to include 林知夏, 周衡, 许燃, got %#v", names)
	}
}
