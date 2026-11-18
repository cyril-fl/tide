package readnotes

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cyril-fl/tide/core/services/process/readitem"
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
	// The message names the faulty parameter and the value received.
	for process, want := range map[*run]string{
		Process(context.Background(), nil, make([]string, MaxNotes+1), 0): "notes: 51 items received",
		Process(context.Background(), nil, []string{"x"}, -1):             "max_chars: -1 received",
	} {
		process.Run()
		if !strings.Contains(process.GetErr().Error(), want) {
			t.Errorf("err = %v, want it to contain %q", process.GetErr(), want)
		}
	}
}

func TestPreview(t *testing.T) {
	long := "# Title\nsee https://example.com/a/very/long/path then <a href=\"x\" class=\"y\">link</a> end\n#tag"
	for name, tc := range map[string]struct {
		content string
		n       int
		want    string
	}{
		"line":     {"aaaa bbbb\ncccc dddd eeee", 16, "aaaa bbbb\ncccc"},
		"word":     {"aaaa bbbbbbbb cccc", 12, "aaaa"},
		"url":      {"go to https://example.com/very/long now", 20, "go to"},
		"on break": {"aaaa bbbb cccc", 9, "aaaa bbbb"},
		"hard cut": {strings.Repeat("a", 300), 10, strings.Repeat("a", 10)},
		"tag":      {long, 62, "# Title\nsee https://example.com/a/very/long/path then"},
	} {
		got := preview(readitem.Result{Content: tc.content}, tc.n)
		if got.Content != tc.want || !got.Truncated || got.Length != len([]rune(tc.content)) {
			t.Errorf("%s: %+v, want content %q", name, got, tc.want)
		}
	}
	// Whole content: not truncated, the length still given; the tags stay whole either way.
	got := preview(readitem.Result{Content: "short #a", Tags: []string{"a"}}, 100)
	if got.Truncated || got.Content != "short #a" || got.Length != 8 || len(got.Tags) != 1 {
		t.Errorf("whole = %+v", got)
	}
	cutTagged := preview(readitem.Result{Content: strings.Repeat("word ", 10) + "#a", Tags: []string{"a"}}, 12)
	if !cutTagged.Truncated || strings.Contains(cutTagged.Content, "#a") || len(cutTagged.Tags) != 1 {
		t.Errorf("tags = %+v", cutTagged)
	}
}
