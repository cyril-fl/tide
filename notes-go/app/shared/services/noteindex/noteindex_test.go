package noteindex

import (
	"slices"
	"testing"
)

func TestGrams(t *testing.T) {
	if got, want := Grams("Été!"), []string{"ét", "été", "té", "té!", "é!"}; !slices.Equal(got, want) {
		t.Errorf("Grams() = %q, want %q", got, want)
	}
	if got := Grams("a"); len(got) != 0 {
		t.Errorf("Grams(one letter) = %q, want none", got)
	}
}

func TestLinked(t *testing.T) {
	for content, want := range map[string]bool{
		"# Plain\n\n`[[Code]]`": false,
		"[[Waiting]]":           true,
		"[[#Heading]]":          true,
		"[Note](note:00000000-0000-4000-8000-000000000000)":    true,
		"![](/api/files/00000000-0000-4000-8000-000000000000)": true,
	} {
		if got := Linked(content); got != want {
			t.Errorf("Linked(%q) = %v, want %v", content, got, want)
		}
	}
}
