package rebasefork

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cyril-fl/tide/notes-go/app/shared/domain"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/postgrey"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createnote"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createschema"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/forknote"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/mergefork"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/patchnote"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/readfork"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/testaccount"
)

func TestRebase(t *testing.T) {
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
	origin := func() readfork.Result {
		t.Helper()
		process := readfork.Process(ctx, tx, forkID)
		process.Run()
		if process.HasErr() {
			t.Fatal(process.GetErr())
		}
		return process.GetResult()
	}
	rebase := func(id, content, base string, at *time.Time) *run {
		process := Process(ctx, tx, Input{ID: id, Content: content, Base: base, UpdatedAt: at})
		process.Run()
		return process
	}

	// A merge first, which unmergefork could undo.
	merged := seed.GetResult().CreatedAt
	merge := mergefork.Process(ctx, tx, mergefork.Input{ID: forkID, Content: "# Courses\n\n- Pain\n- Lait", Base: "# Copie\n\n- Pain", UpdatedAt: &merged})
	merge.Run()
	if merge.HasErr() {
		t.Fatalf("merge: %v", merge.GetErr())
	}

	// A rebase saves the fork, keeps the text it replaces in its history, moves the base to the original's text and
	// drops the merge's undo, which would put back a base from before it.
	created := fork.GetResult().CreatedAt
	if got := rebase(forkID, "# Copie\n\n- Pain\n- Lait", "# Courses\n\n- Pain\n- Lait", &created); got.HasErr() || got.GetResult().Content != "# Copie\n\n- Pain\n- Lait" {
		t.Fatalf("rebase: %+v, %v", got.GetResult(), got.GetErr())
	}
	var versions []domain.NoteVersion
	if err := tx.NewSelect().Model(&versions).Where("item_id = ?", forkID).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].Content != "# Copie\n\n- Pain" {
		t.Errorf("versions of the fork = %+v, want the text replaced", versions)
	}
	if got := origin(); got.Base != "# Courses\n\n- Pain\n- Lait" || got.MergedAt != nil {
		t.Errorf("origin after the rebase = %+v, want the original's text and no merge to undo", got)
	}

	// Made over an older version of the fork: refused, the base left as it was.
	older := created.Add(-time.Second)
	if got := rebase(forkID, "stale", "stale base", &older); !errors.Is(got.GetErr(), patchnote.ErrStale) {
		t.Errorf("stale rebase: err = %v, want %v", got.GetErr(), patchnote.ErrStale)
	}
	if got := origin().Base; got != "# Courses\n\n- Pain\n- Lait" {
		t.Errorf("base after a refused rebase = %q", got)
	}

	// A locked fork refuses it.
	yes := true
	lock := patchnote.Process(ctx, tx, patchnote.Input{ID: forkID, Readonly: &yes})
	lock.Run()
	if lock.HasErr() {
		t.Fatal(lock.GetErr())
	}
	if got := rebase(forkID, "locked", "locked", nil); !errors.Is(got.GetErr(), patchnote.ErrLocked) {
		t.Errorf("locked fork: err = %v, want %v", got.GetErr(), patchnote.ErrLocked)
	}

	// A plain note is no fork.
	if got := rebase(original, "x", "x", nil); !errors.Is(got.GetErr(), readfork.ErrNotFork) {
		t.Errorf("rebase of a plain note: err = %v, want %v", got.GetErr(), readfork.ErrNotFork)
	}
}
