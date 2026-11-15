package config

import (
	"fmt"
	"os"
	"path/filepath"
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
	if loaded.Name != "tide" || loaded.Commands.Search.Use == "" {
		t.Errorf("loaded = %q, search %q", loaded.Name, loaded.Commands.Search.Use)
	}
}

func TestLoadFile(t *testing.T) {
	t.Setenv("CONFIG_FILE", "config.yaml")

	loaded, err := load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "tide" {
		t.Errorf("name = %q", loaded.Name)
	}
}

// The texts of the commands name the program and its environment variables from the name, none left as a placeholder.
func TestExpand(t *testing.T) {
	if text := fmt.Sprint(Current.Commands); strings.Contains(text, "{name}") || strings.Contains(text, "{NAME}") {
		t.Errorf("a placeholder is left: %s", text)
	}
	if example := Current.Commands.RotateSecret.Example; example != "TIDE_OLD_SECRET=<old> tide-server rotate-secret" {
		t.Errorf("rotate-secret example = %q", example)
	}
}

// A server whose .env still says NOTES_ keeps working; TIDE_ wins once set, even empty.
func TestEnvFallback(t *testing.T) {
	t.Setenv("NOTES_FALLBACK", "old")
	os.Unsetenv("TIDE_FALLBACK")
	if name := Env("FALLBACK"); name != "NOTES_FALLBACK" {
		t.Errorf("only NOTES_ set: Env = %q", name)
	}
	t.Setenv("TIDE_FALLBACK", "")
	if name := Env("FALLBACK"); name != "TIDE_FALLBACK" {
		t.Errorf("TIDE_ set: Env = %q", name)
	}
	if name := Env("NONE"); name != "TIDE_NONE" {
		t.Errorf("neither set: Env = %q", name)
	}
}

// A session or .env left in ~/.config/notes is read until ~/.config/tide has its own.
func TestFileFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	old, current := filepath.Join(home, ".config", "notes", ".env"), filepath.Join(home, ".config", "tide", ".env")
	if file, err := File(".env"); err != nil || file != current {
		t.Errorf("none: File = %q, %v; want %q", file, err, current)
	}
	for _, file := range []string{old, current} {
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if got, err := File(".env"); err != nil || got != file {
			t.Errorf("File = %q, %v; want %q", got, err, file)
		}
	}
}
