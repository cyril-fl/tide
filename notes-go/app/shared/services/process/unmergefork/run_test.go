package unmergefork

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/postgrey"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/createnote"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/createschema"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/forknote"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/mergefork"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/patchnote"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readfork"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readitem"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/testaccount"
)

func TestUnmerge(t *testing.T) {
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
	fork := forknote.Process(ctx, tx, forknote.Input{ID: original, Content: "# Copie\n\n- Pain"})
	fork.Run()
	if fork.HasErr() {
		t.Fatalf("fork: %v", fork.GetErr())
	}
	forkID := fork.GetResult().ID

	read := func(id string) readitem.Result {
		t.Helper()
		process := readitem.Process(ctx, tx, domain.NOTE, id)
		process.Run()
		if process.HasErr() {
			t.Fatalf("read %s: %v", id, process.GetErr())
		}
		return process.GetResult()
	}
	origin := func() readfork.Result {
		t.Helper()
		process := readfork.Process(ctx, tx, forkID)
		process.Run()
		if process.HasErr() {
			t.Fatal(process.GetErr())
		}
		return process.GetResult()
	}
	merge := func(content, base string, trash bool) {
		t.Helper()
		note := read(original)
		at := note.Item.UpdatedAt
		if at.IsZero() {
			at = note.Item.CreatedAt
		}
		process := mergefork.Process(ctx, tx, mergefork.Input{ID: forkID, Content: content, Base: base, UpdatedAt: &at, Trash: trash})
		process.Run()
		if process.HasErr() {
			t.Fatalf("merge: %v", process.GetErr())
		}
	}
	unmerge := func() error {
		process := Process(ctx, tx, forkID)
		process.Run()
		return process.GetErr()
	}

	// Nothing merged yet: nothing to undo.
	if origin().MergedAt != nil {
		t.Errorf("merged_at before a merge = %v", origin().MergedAt)
	}
	if err := unmerge(); !errors.Is(err, ErrNoMerge) {
		t.Errorf("unmerge before a merge: err = %v, want %v", err, ErrNoMerge)
	}

	// Two merges: the undo puts back the original and the base of the last one only.
	merge("# Courses\n\n- Pain\n- Lait", "# Copie\n\n- Pain\n- Lait", false)
	merge("# Courses\n\n- Pain\n- Lait\n- Œufs", "# Copie\n\n- Pain\n- Lait\n- Œufs\n- Sel", false)
	if origin().MergedAt == nil {
		t.Fatal("merged_at after a merge is nil")
	}
	if err := unmerge(); err != nil {
		t.Fatalf("unmerge: %v", err)
	}
	if got := read(original).Content; got != "# Courses\n\n- Pain\n- Lait" {
		t.Errorf("original after the undo = %q", got)
	}
	if got := origin(); got.Base != "# Copie\n\n- Pain\n- Lait" || got.MergedAt != nil {
		t.Errorf("origin after the undo = %+v", got)
	}
	// The text the undo replaced is kept in the original's history, as a restore.
	var versions []domain.NoteVersion
	// One transaction: every version has the same created_at, the one the undo kept is the one with the second merge's text.
	if err := tx.NewSelect().Model(&versions).Where("item_id = ?", original).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if len(versions) != 3 || !slices.ContainsFunc(versions, func(v domain.NoteVersion) bool { return v.Content == "# Courses\n\n- Pain\n- Lait\n- Œufs" }) {
		t.Errorf("versions of the original = %+v", versions)
	}
	// One level only.
	if err := unmerge(); !errors.Is(err, ErrNoMerge) {
		t.Errorf("second unmerge: err = %v, want %v", err, ErrNoMerge)
	}

	// The original written after the merge: refused, neither its text nor the base overwritten.
	merge("# Courses\n\n- Beurre", "# Copie\n\n- Beurre", false)
	later := "# Courses\n\n- Beurre\n- Confiture"
	edit := patchnote.Process(ctx, tx, patchnote.Input{ID: original, Content: &later})
	edit.Run()
	if edit.HasErr() {
		t.Fatal(edit.GetErr())
	}
	// current_timestamp is the transaction's: a later request has a later one.
	if _, err := tx.NewRaw("UPDATE items SET updated_at = updated_at + interval '1 second' WHERE id = ?", original).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if err := unmerge(); !errors.Is(err, patchnote.ErrStale) {
		t.Errorf("unmerge over a later edit: err = %v, want %v", err, patchnote.ErrStale)
	}
	if got := read(original).Content; got != later {
		t.Errorf("original after a refused undo = %q", got)
	}
	if got := origin(); got.Base != "# Copie\n\n- Beurre" {
		t.Errorf("base after a refused undo = %q", got.Base)
	}

	// A merge that trashed the fork: the undo takes it out of the trash.
	merge("# Courses\n\n- Miel", "# Copie\n\n- Miel", true)
	trashed := readitem.Process(ctx, tx, domain.NOTE, forkID)
	trashed.Run()
	if !errors.Is(trashed.GetErr(), readitem.ErrNotFound) {
		t.Fatalf("fork after a merge with trash: err = %v, want it in the trash", trashed.GetErr())
	}
	if err := unmerge(); err != nil {
		t.Fatalf("unmerge of a trashed fork: %v", err)
	}
	if got := read(original).Content; got != later {
		t.Errorf("original after the undo of a trashing merge = %q", got)
	}
	read(forkID)

	// A plain note is no fork.
	plain := Process(ctx, tx, original)
	plain.Run()
	if !errors.Is(plain.GetErr(), readfork.ErrNotFork) {
		t.Errorf("unmerge of a plain note: err = %v, want %v", plain.GetErr(), readfork.ErrNotFork)
	}
}
