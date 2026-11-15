package mergefork

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cyril-fl/tide/notes-go/app/shared/domain"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/postgrey"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createnote"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createschema"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/deleteitems"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/forknote"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/patchnote"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/readfork"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/readitem"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/testaccount"
	"github.com/uptrace/bun"
)

func TestForkAndMerge(t *testing.T) {
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

	seed := createnote.Process(ctx, tx, createnote.Input{Content: "# Courses\n\n- Pain"})
	seed.Run()
	if seed.HasErr() {
		t.Fatalf("seed: %v", seed.GetErr())
	}
	original := seed.GetResult().ID
	read := func(id string) readitem.Result {
		t.Helper()
		process := readitem.Process(ctx, tx, domain.NOTE, id)
		process.Run()
		if process.HasErr() {
			t.Fatalf("read %s: %v", id, process.GetErr())
		}
		return process.GetResult()
	}
	origin := func(id string) (readfork.Result, error) {
		process := readfork.Process(ctx, tx, id)
		process.Run()
		return process.GetResult(), process.GetErr()
	}

	// A fork remembers its original, and its own text as the base: no version of the original needed.
	fork := forknote.Process(ctx, tx, forknote.Input{ID: original, Content: "# Courses (copie)\n\n- Pain"})
	fork.Run()
	if fork.HasErr() {
		t.Fatalf("fork: %v", fork.GetErr())
	}
	forkID := fork.GetResult().ID
	if got := read(forkID); got.ForkOf != original {
		t.Errorf("fork_of = %q, want %q", got.ForkOf, original)
	}
	if got, err := origin(forkID); err != nil || got.ForkOf != original || got.Base != "# Courses (copie)\n\n- Pain" {
		t.Errorf("origin = %+v, %v", got, err)
	}
	if _, err := origin(original); !errors.Is(err, readfork.ErrNotFork) {
		t.Errorf("origin of a plain note: err = %v, want %v", err, readfork.ErrNotFork)
	}
	missing := forknote.Process(ctx, tx, forknote.Input{ID: "3f2a4b1c-0d9e-4a8b-9c7d-1e2f3a4b5c6d", Content: "x"})
	missing.Run()
	if !errors.Is(missing.GetErr(), readitem.ErrNotFound) {
		t.Errorf("fork of a missing note: err = %v", missing.GetErr())
	}

	merge := func(content, base string, at *time.Time) *run {
		process := Process(ctx, tx, Input{ID: forkID, Content: content, Base: base, UpdatedAt: at})
		process.Run()
		return process
	}
	version := func(note readitem.Result) *time.Time {
		at := note.Item.UpdatedAt
		if at.IsZero() {
			at = note.Item.CreatedAt
		}
		return &at
	}

	// A merge saves the original, keeps the text it replaces in its history, and moves the fork's base.
	before := read(original)
	if got := merge("# Courses\n\n- Pain\n- Lait", "# Courses (copie)\n\n- Pain\n- Lait", version(before)); got.HasErr() || got.GetResult().Content != "# Courses\n\n- Pain\n- Lait" {
		t.Fatalf("merge: %+v, %v", got.GetResult(), got.GetErr())
	}
	var versions []domain.NoteVersion
	if err := tx.NewSelect().Model(&versions).Where("item_id = ?", original).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].Content != "# Courses\n\n- Pain" {
		t.Errorf("versions of the original = %+v, want the text replaced", versions)
	}
	if got, _ := origin(forkID); got.Base != "# Courses (copie)\n\n- Pain\n- Lait" {
		t.Errorf("base after the merge = %q", got.Base)
	}

	// Made over an older version of the original: refused, the base left as it was.
	older := version(before).Add(-time.Second)
	if got := merge("stale", "stale base", &older); !errors.Is(got.GetErr(), patchnote.ErrStale) {
		t.Errorf("stale merge: err = %v, want %v", got.GetErr(), patchnote.ErrStale)
	}
	if got, _ := origin(forkID); got.Base != "# Courses (copie)\n\n- Pain\n- Lait" {
		t.Errorf("base after a refused merge = %q", got.Base)
	}

	// A locked original refuses it.
	yes, no := true, false
	lock := func(readonly *bool) {
		process := patchnote.Process(ctx, tx, patchnote.Input{ID: original, Readonly: readonly})
		process.Run()
		if process.HasErr() {
			t.Fatal(process.GetErr())
		}
	}
	lock(&yes)
	if got := merge("locked", "locked", nil); !errors.Is(got.GetErr(), patchnote.ErrLocked) {
		t.Errorf("locked original: err = %v, want %v", got.GetErr(), patchnote.ErrLocked)
	}
	lock(&no)

	// An original in the trash refuses it, with its own error.
	trash := deleteitems.Process(ctx, tx, domain.NOTE, []string{original})
	trash.Run()
	if trash.HasErr() {
		t.Fatal(trash.GetErr())
	}
	if got := merge("gone", "gone", nil); !errors.Is(got.GetErr(), ErrOriginGone) {
		t.Errorf("trashed original: err = %v, want %v", got.GetErr(), ErrOriginGone)
	}

	// A plain note is no fork.
	plain := Process(ctx, tx, Input{ID: seedPlain(t, ctx, tx), Content: "x", Base: "x"})
	plain.Run()
	if !errors.Is(plain.GetErr(), readfork.ErrNotFork) {
		t.Errorf("merge of a plain note: err = %v, want %v", plain.GetErr(), readfork.ErrNotFork)
	}
}

func seedPlain(t *testing.T, ctx context.Context, tx bun.Tx) string {
	t.Helper()
	process := createnote.Process(ctx, tx, createnote.Input{Content: "plain"})
	process.Run()
	if process.HasErr() {
		t.Fatal(process.GetErr())
	}
	return process.GetResult().ID
}
