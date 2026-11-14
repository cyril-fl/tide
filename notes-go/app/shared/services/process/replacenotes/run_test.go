package replacenotes

import (
	"context"
	"errors"
	"testing"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/postgrey"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/createnote"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/createschema"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/listversions"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/patchnote"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readitem"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/testaccount"
)

// No DB needed: makeInput fails first and replace is guarded, so a nil db is never touched.
func TestBlankValue(t *testing.T) {
	process := Process(context.Background(), nil, Input{Find: "  ", Replace: "x"})
	process.Run()

	if !errors.Is(process.GetErr(), ErrNoValue) {
		t.Fatalf("err = %v, want %v", process.GetErr(), ErrNoValue)
	}
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
	if dry.HasErr() || dry.GetResult() != want {
		t.Fatalf("dry run = %+v, %v; want %+v", dry.GetResult(), dry.GetErr(), want)
	}
	if got := content(two); got != "foo and foo" {
		t.Fatalf("dry run changed the note: %q", got)
	}

	process := Process(ctx, tx, Input{Find: "foo", Replace: "baz"})
	process.Run()
	if process.HasErr() || process.GetResult() != want {
		t.Fatalf("replace = %+v, %v; want %+v", process.GetResult(), process.GetErr(), want)
	}
	for id, text := range map[string]string{two: "baz and baz", one: "a Foo, a baz", none: "bar", locked: "foo locked"} {
		if got := content(id); got != text {
			t.Errorf("content of %s = %q, want %q", id, got, text)
		}
	}

	// The note was just written, within a session: the text replaced is kept all the same.
	versions := listversions.Process(ctx, tx, two)
	versions.Run()
	if versions.HasErr() || len(versions.GetResult()) != 1 {
		t.Fatalf("versions = %+v, %v; want 1", versions.GetResult(), versions.GetErr())
	}
}
