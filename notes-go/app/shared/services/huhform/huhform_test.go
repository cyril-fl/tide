package huhform

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/listfolders"
)

func TestFitFillsHeight(t *testing.T) {
	var title, content string
	tests := map[string][]huh.Field{
		"text grows": {huh.NewInput().Title("Title").Value(&title), huh.NewText().Title("Content").Value(&content)},
		"form pads":  {huh.NewInput().Title("Title").Value(&title)},
	}

	for name, fields := range tests {
		form := fit(fields, 80, 30)
		form.Init()
		if got := lipgloss.Height(form.View()); got != 30 {
			t.Errorf("%s: height = %d, want 30", name, got)
		}
	}
}

func TestFolderOptions(t *testing.T) {
	options := folderOptions([]listfolders.Folder{{ID: "a", Path: "/A"}, {ID: "b", Path: "/A/B"}})
	if len(options) != 3 || options[0].Key != "/" || options[0].Value != "" || options[2].Key != "/A/B" || options[2].Value != "b" {
		t.Errorf("options = %+v, want / then the paths", options)
	}
}

func TestCancel(t *testing.T) {
	filter := key.NewBinding(key.WithKeys("esc"))
	closed := key.NewBinding(key.WithKeys("esc"), key.WithDisabled())

	if escTaken([]key.Binding{closed}) || !escTaken([]key.Binding{closed, filter}) {
		t.Error("escTaken: want esc free while the filter key is off, taken once it is on")
	}
	if keys := cancel(false).Keys(); !slices.Contains(keys, "esc") {
		t.Errorf("cancel keys = %v, want esc", keys)
	}
	if keys := cancel(true).Keys(); slices.Contains(keys, "esc") {
		t.Errorf("cancel keys with esc taken = %v, want ctrl+c only", keys)
	}
}

func TestCancelHelp(t *testing.T) {
	var content string
	form := build([]huh.Field{huh.NewText().Value(&content)})
	form.Init()

	view := ansi.Strip(form.View())
	if line := view[strings.LastIndex(view, "\n")+1:]; !strings.HasPrefix(line, "⎋ Back · ^E Editor · ^↳ Submit · ↳ New line") {
		t.Errorf("help line = %q, want Back, Editor then Submit first", line)
	}
}

func TestEscCancels(t *testing.T) {
	var content string
	Input = strings.NewReader("abc\x1b")
	defer func() { Input = nil }()

	if err := build([]huh.Field{huh.NewText().Value(&content)}).Run(); !errors.Is(err, huh.ErrUserAborted) {
		t.Errorf("esc: err = %v, want the form aborted", err)
	}
}
