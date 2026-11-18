package replacenotes

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/postgrey"
	"github.com/cyril-fl/tide/core/services/process/createfolder"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/createschema"
	"github.com/cyril-fl/tide/core/services/process/listversions"
	"github.com/cyril-fl/tide/core/services/process/patchnote"
	"github.com/cyril-fl/tide/core/services/process/readitem"
	"github.com/cyril-fl/tide/core/services/testaccount"
)

// No DB needed: makeInput fails first and replace is guarded, so a nil db is never touched.
func TestBlankValue(t *testing.T) {
	process := Process(context.Background(), nil, Input{Find: "  ", Replace: "x"})
	process.Run()

	if !errors.Is(process.GetErr(), ErrNoValue) {
		t.Fatalf("err = %v, want %v", process.GetErr(), ErrNoValue)
	}
}

// Each occurrence gives its line and the text around it, a long start cut.
func TestLines(t *testing.T) {
	content := "# Title\n  a foo b foo\n" + strings.Repeat("é", 70) + "foo"
	got := lines(content, "foo", occurrences(content, "foo"))
	want := []Line{
		{Line: 2, Before: "a ", After: " b foo"},
		{Line: 2, Before: "a foo b ", After: ""},
		{Line: 3, Before: "…" + strings.Repeat("é", 60), After: ""},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("lines = %+v, want %+v", got, want)
	}
}

// The hidden targets of a resolved link and a file are neither counted nor replaced.
func TestHiddenTargets(t *testing.T) {
	const id = "3fa85f64-5717-4562-b3fc-2c963f66afa6"
	content := "a note [X](note:" + id + "#Part) ![](/api/files/" + id + ") files"
	for _, c := range []struct{ find, want string }{
		{"note", "a fiche [X](note:" + id + "#Part) ![](/api/files/" + id + ") files"},
		{"files", "a note [X](note:" + id + "#Part) ![](/api/files/" + id + ") fiche"},
		{"5717", content},
	} {
		if got := replaceAll(content, c.find, "fiche"); got != c.want {
			t.Errorf("replace %q = %q, want %q", c.find, got, c.want)
		}
		if n, want := len(occurrences(content, c.find)), strings.Count(c.want, "fiche"); n != want {
			t.Errorf("count %q = %d, want %d", c.find, n, want)
		}
	}
}

// counts leaves out the matches, a Result with them is not comparable.
func counts(r Result) [3]int {
	return [3]int{r.Notes, r.Occurrences, r.Locked}
}

func TestReplace(t *testing.T) {
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

	note := func(content string) string {
		p := createnote.Process(ctx, tx, createnote.Input{Content: content})
		p.Run()
		if p.HasErr() {
			t.Fatalf("note: %v", p.GetErr())
		}
		return p.GetResult().ID
	}
	content := func(id string) string {
		p := readitem.Process(ctx, tx, domain.NOTE, id)
		p.Run()
		if p.HasErr() {
			t.Fatalf("read: %v", p.GetErr())
		}
		return p.GetResult().Content
	}

	two := note("foo and foo")
	one := note("a Foo, a foo")
	none := note("bar")
	locked := note("foo locked")
	yes := true
	lock := patchnote.Process(ctx, tx, patchnote.Input{ID: locked, Readonly: &yes})
	lock.Run()
	if lock.HasErr() {
		t.Fatalf("lock: %v", lock.GetErr())
	}

	want := Result{Notes: 2, Occurrences: 3, Locked: 1}
	dry := Process(ctx, tx, Input{Find: "foo", Replace: "baz", DryRun: true})
	dry.Run()
	if dry.HasErr() {
		t.Fatalf("dry run: %v", dry.GetErr())
	}
	got := dry.GetResult()
	if matches := got.Matches; len(matches) != 2 || len(matches[0].Occurrences)+len(matches[1].Occurrences) != 3 {
		t.Fatalf("dry run matches = %+v, want 2 notes, 3 occurrences", matches)
	}
	if counts(got) != counts(want) {
		t.Fatalf("dry run = %+v; want %+v", got, want)
	}
	if got := content(two); got != "foo and foo" {
		t.Fatalf("dry run changed the note: %q", got)
	}

	// One occurrence of one note: the second foo of two.
	single := Process(ctx, tx, Input{Find: "foo", Replace: "baz", Notes: []string{two}, Occurrence: 2})
	single.Run()
	if single.HasErr() || single.GetResult().Occurrences != 1 {
		t.Fatalf("single = %+v, %v; want 1 occurrence", single.GetResult(), single.GetErr())
	}
	if got := content(two); got != "foo and baz" {
		t.Fatalf("single replace = %q, want %q", got, "foo and baz")
	}

	// One note: the others left as they are.
	scopedNote := Process(ctx, tx, Input{Find: "foo", Replace: "baz", Notes: []string{one}})
	scopedNote.Run()
	if scopedNote.HasErr() || scopedNote.GetResult().Notes != 1 {
		t.Fatalf("one note = %+v, %v; want 1 note", scopedNote.GetResult(), scopedNote.GetErr())
	}
	if got := content(two); got != "foo and baz" {
		t.Fatalf("one note changed another: %q", got)
	}

	process := Process(ctx, tx, Input{Find: "foo", Replace: "baz"})
	process.Run()
	if want := (Result{Notes: 1, Occurrences: 1, Locked: 1}); process.HasErr() || counts(process.GetResult()) != counts(want) {
		t.Fatalf("replace = %+v, %v; want %+v", process.GetResult(), process.GetErr(), want)
	}
	for id, text := range map[string]string{two: "baz and baz", one: "a Foo, a baz", none: "bar", locked: "foo locked"} {
		if got := content(id); got != text {
			t.Errorf("content of %s = %q, want %q", id, got, text)
		}
	}

	// The note was just written, within a session: the text replaced is kept all the same, at each replace.
	versions := listversions.Process(ctx, tx, two)
	versions.Run()
	if versions.HasErr() || len(versions.GetResult()) != 2 {
		t.Fatalf("versions = %+v, %v; want 2", versions.GetResult(), versions.GetErr())
	}

	// Scoped to a folder: the notes of its subfolders too, none outside.
	folder := func(parent string) string {
		p := createfolder.Process(ctx, tx, createfolder.Input{Title: "f", ParentID: parent})
		p.Run()
		if p.HasErr() {
			t.Fatalf("folder: %v", p.GetErr())
		}
		return p.GetResult().ID
	}
	outer := folder("")
	inner := folder(outer)
	in := createnote.Process(ctx, tx, createnote.Input{Content: "scoped", ParentID: inner})
	in.Run()
	if in.HasErr() {
		t.Fatalf("note in folder: %v", in.GetErr())
	}
	out := note("scoped")
	scoped := Process(ctx, tx, Input{Find: "scoped", Replace: "done", Folder: outer})
	scoped.Run()
	if scoped.HasErr() || scoped.GetResult().Notes != 1 {
		t.Fatalf("scoped = %+v, %v; want 1 note", scoped.GetResult(), scoped.GetErr())
	}
	if got := content(out); got != "scoped" {
		t.Errorf("note outside the folder = %q", got)
	}
	if got := content(in.GetResult().ID); got != "done" {
		t.Errorf("note in the subfolder = %q", got)
	}
}
