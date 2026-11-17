package linediff

import (
	"slices"
	"testing"
)

func TestLines(t *testing.T) {
	got := Lines("# A\n\nx #go\nmid\ny #go\nend", "# A\n\nx #rust\nmid\ny #rust\nend")
	want := []string{"- x #go", "+ x #rust", "- y #go", "+ y #rust"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := Lines("same", "same"); got != nil {
		t.Errorf("equal texts = %q", got)
	}
}
