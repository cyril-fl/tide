package readnotes

import (
	"context"
	"errors"
	"testing"
)

// No DB needed: checkInput fails first and the steps are guarded. The reads are covered by the API's TestNotes.
func TestInvalid(t *testing.T) {
	for name, process := range map[string]*run{
		"none":      Process(context.Background(), nil, nil, 0),
		"too many":  Process(context.Background(), nil, make([]string, MaxNotes+1), 0),
		"max_chars": Process(context.Background(), nil, []string{"x"}, -1),
	} {
		process.Run()
		if !errors.Is(process.GetErr(), ErrInvalid) {
			t.Errorf("%s: err = %v, want %v", name, process.GetErr(), ErrInvalid)
		}
	}
}
