package config

import (
	"fmt"
	"strings"
	"testing"
)

// Outside the repo there is no CONFIG_FILE: the config built into the binary must load.
func TestLoadEmbedded(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")

	loaded, err := load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "notes" || loaded.Commands.Search.Use == "" {
		t.Errorf("loaded = %q, search %q", loaded.Name, loaded.Commands.Search.Use)
	}
}

func TestLoadFile(t *testing.T) {
	t.Setenv("CONFIG_FILE", "config.yaml")

	loaded, err := load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "notes" {
		t.Errorf("name = %q", loaded.Name)
	}
}

// The texts of the commands name the program and its environment variables from the name, none left as a placeholder.
func TestExpand(t *testing.T) {
	if text := fmt.Sprint(Current.Commands); strings.Contains(text, "{name}") || strings.Contains(text, "{NAME}") {
		t.Errorf("a placeholder is left: %s", text)
	}
	if example := Current.Commands.RotateSecret.Example; example != "NOTES_OLD_SECRET=<old> notes-server rotate-secret" {
		t.Errorf("rotate-secret example = %q", example)
	}
}
