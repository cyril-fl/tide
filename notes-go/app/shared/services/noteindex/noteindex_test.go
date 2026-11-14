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
