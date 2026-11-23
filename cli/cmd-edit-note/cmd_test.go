package cmdeditnote

import (
	"slices"
	"strings"
	"testing"

	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/listversions"
	"github.com/cyril-fl/tide/core/services/process/patchnote"
	"github.com/cyril-fl/tide/core/services/process/readitem"
	"github.com/spf13/pflag"
)

// The CLI has no kernel to import ErrStale from: its text must stay the one the API answers.
func TestErrStale(t *testing.T) {
	if errStale != patchnote.ErrStale.Error() {
		t.Errorf("errStale = %q, want %q", errStale, patchnote.ErrStale.Error())
	}
}

// Interactive relies on it: a changed flag would skip the form.
func TestNoFlags(t *testing.T) {
	flags := noFlags()
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Changed {
			t.Errorf("--%s is changed", flag.Name)
		}
	})
	if flags.Lookup("content") == nil {
		t.Error("--content is not declared")
	}
}

// Through the API: --pin and --lock alone leave the text and its last change date, --tag rewrites them, and a locked
// note refuses it.
func TestEdit(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	seed := createnote.Process(ctx, tx, createnote.Input{Content: "# Ideas"})
	seed.Run()
	if seed.HasErr() {
		t.Fatal(seed.GetErr())
	}
	id := seed.GetResult().ID
	edit := func(args ...string) (string, error) {
		flags := noFlags()
		if err := flags.Parse(args); err != nil {
			t.Fatal(err)
		}
		return apitest.Stdout(t, func() error { return run(ctx, id, flags) })
	}
	read := func() readitem.Result {
		note := readitem.Process(ctx, tx, domain.NOTE, id)
		note.Run()
		if note.HasErr() {
			t.Fatal(note.GetErr())
		}
		return note.GetResult()
	}

	printed, err := edit("--pin", "--lock")
	if note := read(); err != nil || !note.Pinned || !note.Readonly || note.Content != "# Ideas" || !note.Item.UpdatedAt.IsZero() {
		t.Errorf("--pin --lock: note = %+v, printed %q, err = %v; want pinned, locked, text untouched", note, printed, err)
	}
	if _, err := edit("--tag", "go"); err == nil {
		t.Error("--tag on a locked note: no error")
	}
	printed, err = edit("--unlock", "--tag", "go")
	if note := read(); err != nil || note.Readonly || !slices.Equal(note.Tags, []string{"go"}) || note.Item.UpdatedAt.IsZero() {
		t.Errorf("--unlock --tag go: note = %+v, printed %q, err = %v; want unlocked and tagged", note, printed, err)
	}
}

// Through the API: --title renames, --replace refuses several occurrences without --all, --section replaces a body,
// the replaced text goes to the history, and a locked note refuses a part edit unless --unlock comes with it.
func TestEditPart(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	seed := createnote.Process(ctx, tx, createnote.Input{Content: "# Ideas\n\nteh cat, teh dog\n\n## Links\n\nold link\n"})
	seed.Run()
	if seed.HasErr() {
		t.Fatal(seed.GetErr())
	}
	id := seed.GetResult().ID
	edit := func(args ...string) error {
		flags := noFlags()
		if err := flags.Parse(args); err != nil {
			t.Fatal(err)
		}
		_, err := apitest.Stdout(t, func() error { return run(ctx, id, flags) })
		return err
	}
	read := func() readitem.Result {
		note := readitem.Process(ctx, tx, domain.NOTE, id)
		note.Run()
		if note.HasErr() {
			t.Fatal(note.GetErr())
		}
		return note.GetResult()
	}

	for _, args := range [][]string{{"--replace", "teh"}, {"--title", "X", "--with", "Y"}, {"--with", "Y"}, {"--section", "Links", "--with", "x", "--all"}, {"--title", "X", "--content", "Y"}} {
		if err := edit(args...); err == nil {
			t.Errorf("%v: no error", args)
		}
	}
	if err := edit("--replace", "teh", "--with", "the"); err == nil || !strings.Contains(err.Error(), "2 times") {
		t.Errorf("--replace teh without --all: err = %v, want found 2 times", err)
	}
	if err := edit("--replace", "teh", "--with", "the", "--all"); err != nil {
		t.Fatal(err)
	}
	if err := edit("--section", "Links", "--with", "new link"); err != nil {
		t.Fatal(err)
	}
	if err := edit("--title", "Go ideas", "--lock"); err != nil {
		t.Fatal(err)
	}
	if note := read(); note.Item.Title != "Go ideas" || !strings.Contains(note.Content, "the cat, the dog") || !strings.Contains(note.Content, "new link") || strings.Contains(note.Content, "old link") || !note.Readonly {
		t.Errorf("note = %q %q, readonly %v; want renamed, replaced and locked", note.Item.Title, note.Content, note.Readonly)
	}
	if err := edit("--title", "Locked"); err == nil {
		t.Error("--title on a locked note: no error")
	}
	if err := edit("--unlock", "--title", "Unlocked"); err != nil || read().Item.Title != "Unlocked" {
		t.Errorf("--unlock --title: title = %q, err = %v", read().Item.Title, err)
	}
	versions := listversions.Process(ctx, tx, id)
	versions.Run()
	if versions.HasErr() || len(versions.GetResult()) == 0 {
		t.Errorf("history: %d version(s), err = %v; want the replaced text kept", len(versions.GetResult()), versions.GetErr())
	}
}

// Through the API: --restore-version brings a version's text back and keeps the replaced one in the history; it
// refuses a text flag beside it.
func TestRestoreVersion(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	seed := createnote.Process(ctx, tx, createnote.Input{Content: "# Ideas\n\nold"})
	seed.Run()
	if seed.HasErr() {
		t.Fatal(seed.GetErr())
	}
	id := seed.GetResult().ID
	edit := func(args ...string) error {
		flags := noFlags()
		if err := flags.Parse(args); err != nil {
			t.Fatal(err)
		}
		_, err := apitest.Stdout(t, func() error { return run(ctx, id, flags) })
		return err
	}
	versions := func() []domain.NoteVersion {
		list := listversions.Process(ctx, tx, id)
		list.Run()
		if list.HasErr() {
			t.Fatal(list.GetErr())
		}
		return list.GetResult()
	}

	if err := edit("--replace", "old", "--with", "new"); err != nil {
		t.Fatal(err)
	}
	before := versions()
	if len(before) == 0 {
		t.Fatal("no version after --replace")
	}
	if err := edit("--restore-version", before[0].ID, "--content", "x"); err == nil {
		t.Error("--restore-version --content: no error")
	}
	if err := edit("--restore-version", before[0].ID); err != nil {
		t.Fatal(err)
	}
	note := readitem.Process(ctx, tx, domain.NOTE, id)
	note.Run()
	if note.HasErr() || note.GetResult().Content != "# Ideas\n\nold" || len(versions()) <= len(before) {
		t.Errorf("restored: content %q, %d version(s), err = %v; want the old text back and the new one kept", note.GetResult().Content, len(versions()), note.GetErr())
	}
}
