package editnotetext

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/postgrey"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/createschema"
	"github.com/cyril-fl/tide/core/services/process/patchnote"
	"github.com/cyril-fl/tide/core/services/process/readitem"
	"github.com/cyril-fl/tide/core/services/testaccount"
)

func TestRetitle(t *testing.T) {
	tests := []struct{ content, want string }{
		{"# Old\ntext", "# New\ntext"},
		{"## Old #tag\ntext", "## New\ntext"},
		{"```\n# comment\n```\n### Old", "```\n# comment\n```\n### New"},
		{"text only", "# New\n\ntext only"},
		{"", "# New"},
	}
	for _, test := range tests {
		if got := Retitle(test.content, "New"); got != test.want {
			t.Errorf("Retitle(%q) = %q, want %q", test.content, got, test.want)
		}
	}
}

func TestReplaceSection(t *testing.T) {
	content := "# Note\n\n## A\nold a\n### A.1\nold a1\n## B\nb"
	got, n, err := replaceSection(content, "A", "new a\n")
	if want := "# Note\n\n## A\n\nnew a\n\n## B\nb"; err != nil || n != 1 || got != want {
		t.Errorf("section A = %q, %d, %v; want %q", got, n, err, want)
	}
	if got, _, _ := replaceSection(content, "B", ""); got != "# Note\n\n## A\nold a\n### A.1\nold a1\n## B" {
		t.Errorf("section B emptied = %q", got)
	}
	if _, _, err := replaceSection(content, "C", "x"); !errors.Is(err, ErrNoMatch) {
		t.Errorf("unknown section: err = %v, want %v", err, ErrNoMatch)
	}
	if _, _, err := replaceSection("## A\n## A", "A", "x"); !errors.Is(err, ErrMany) || !strings.Contains(err.Error(), "2 times") {
		t.Errorf("section twice: err = %v, want %v with the count", err, ErrMany)
	}
}

// No DB needed: makeInput fails first and edit is guarded, so a nil db is never touched.
func TestNoValue(t *testing.T) {
	for _, input := range []Input{{}, {Title: "a", Old: "b"}, {Title: "a\nb"}, {Section: " "}} {
		process := Process(context.Background(), nil, input)
		process.Run()
		if !errors.Is(process.GetErr(), ErrNoValue) {
			t.Errorf("%+v: err = %v, want %v", input, process.GetErr(), ErrNoValue)
		}
	}
}

func TestEditNoteText(t *testing.T) {
	ctx := context.Background()
	env := postgrey.NewEnvironment()
	env.ReadEnv()
	db := env.Connect()
	defer db.Close()

	schema := createschema.Process(ctx, db)
	schema.Run()
	if schema.HasErr() {
		t.Fatalf("create schema: %v", schema.GetErr())
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ctx, tx = testaccount.New(t, ctx, tx)

	seed := createnote.Process(ctx, tx, createnote.Input{Content: "# Old\n\nfoo bar foo #go"})
	seed.Run()
	if seed.HasErr() {
		t.Fatalf("seed: %v", seed.GetErr())
	}
	id := seed.GetResult().ID
	source := createnote.Process(ctx, tx, createnote.Input{Content: "# Source\n\nsee [[Old]]"})
	source.Run()
	if source.HasErr() {
		t.Fatalf("source: %v", source.GetErr())
	}
	edit := func(input Input) *run {
		input.ID = id
		process := Process(ctx, tx, input)
		process.Run()
		return process
	}
	read := func(id string) readitem.Result {
		process := readitem.Process(ctx, tx, domain.NOTE, id)
		process.Run()
		if process.HasErr() {
			t.Fatal(process.GetErr())
		}
		return process.GetResult()
	}

	// A rename rewrites the first heading, the links to the note take the new title, the text before is kept.
	if got := edit(Input{Title: "New"}); got.HasErr() || got.GetResult().Note.Item.Title != "New" || got.GetResult().Note.Content != "# New\n\nfoo bar foo #go" {
		t.Errorf("rename: %+v, %v", got.GetResult(), got.GetErr())
	}
	if content := read(source.GetResult().ID).Content; !strings.Contains(content, "New") || strings.Contains(content, "Old") {
		t.Errorf("link after rename: %q", content)
	}
	var versions []domain.NoteVersion
	if err := tx.NewSelect().Model(&versions).Where("item_id = ?", id).Scan(ctx); err != nil || len(versions) != 1 || versions[0].Content != "# Old\n\nfoo bar foo #go" {
		t.Errorf("versions after rename: %+v, %v", versions, err)
	}

	// A text found once is replaced, the tags following the new text.
	if got := edit(Input{Old: "bar foo #go", New: "baz #rust"}); got.HasErr() || got.GetResult().Replaced != 1 || !slices.Equal(got.GetResult().Note.Tags, []string{"rust"}) {
		t.Errorf("replace once: %+v, %v", got.GetResult(), got.GetErr())
	}

	// None or several refuse, with the count; all replaces them all.
	edit(Input{Old: "baz", New: "foo"})
	if got := edit(Input{Old: "nowhere"}); !errors.Is(got.GetErr(), ErrNoMatch) {
		t.Errorf("no match: err = %v, want %v", got.GetErr(), ErrNoMatch)
	}
	if got := edit(Input{Old: "foo", New: "x"}); !errors.Is(got.GetErr(), ErrMany) || !strings.Contains(got.GetErr().Error(), "2 times") {
		t.Errorf("two matches: err = %v, want %v with the count", got.GetErr(), ErrMany)
	}
	if got := edit(Input{Old: "foo", New: "x", All: true}); got.HasErr() || got.GetResult().Replaced != 2 || got.GetResult().Note.Content != "# New\n\nx x #rust" {
		t.Errorf("all: %+v, %v", got.GetResult(), got.GetErr())
	}

	// A locked note refuses, whether the text is there or not.
	yes := true
	lock := patchnote.Process(ctx, tx, patchnote.Input{ID: id, Readonly: &yes})
	lock.Run()
	if lock.HasErr() {
		t.Fatal(lock.GetErr())
	}
	for _, input := range []Input{{Title: "Locked"}, {Old: "nowhere"}} {
		if got := edit(input); !errors.Is(got.GetErr(), patchnote.ErrLocked) {
			t.Errorf("locked %+v: err = %v, want %v", input, got.GetErr(), patchnote.ErrLocked)
		}
	}
	if got := read(id).Content; got != "# New\n\nx x #rust" {
		t.Errorf("locked note changed: %q", got)
	}
}
