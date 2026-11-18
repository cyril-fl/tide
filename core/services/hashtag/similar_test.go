package hashtag

import (
	"slices"
	"strings"
	"testing"
)

// The pairs of #237 group, each apart from the others, and plurals; short tags a letter apart (chat, chant) and
// words two letters apart under 8 runes (cuisine, cousine) do not.
func TestSimilar(t *testing.T) {
	tags := []string{"DévellopementPersonnel", "Emploie", "Accesoires", "go", "Miniardise", "PommeDeTerres", "Fruitsdemer",
		"js", "DéveloppementPersonnel", "Emploi", "Accessoires", "Mignardise", "PommeDeTerre", "FruitsDeMer",
		"art", "arts", "chat", "chant", "rust", "jeux", "jeu", "cuisine", "cousine"}
	got := []string{}
	for _, group := range Similar(tags, func(tag string) string { return tag }) {
		got = append(got, strings.Join(group, "|"))
	}
	want := []string{"DévellopementPersonnel|DéveloppementPersonnel", "Emploie|Emploi", "Accesoires|Accessoires",
		"Miniardise|Mignardise", "PommeDeTerres|PommeDeTerre", "Fruitsdemer|FruitsDeMer", "art|arts", "jeux|jeu"}
	if !slices.Equal(got, want) {
		t.Errorf("Similar = %q\nwant %q", got, want)
	}
	if groups := Similar([]string{"go"}, func(tag string) string { return tag }); groups == nil || len(groups) != 0 {
		t.Errorf("Similar of one tag = %#v, want an empty list", groups)
	}
}
