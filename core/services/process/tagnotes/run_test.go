package tagnotes

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/hashtag"
	"github.com/cyril-fl/tide/core/services/postgrey"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/createschema"
	"github.com/cyril-fl/tide/core/services/process/listversions"
	"github.com/cyril-fl/tide/core/services/process/patchnote"
	"github.com/cyril-fl/tide/core/services/process/readitem"
	"github.com/cyril-fl/tide/core/services/testaccount"
)

// No DB needed: checkInput fails first and tagNotes is guarded, so a nil db is never touched.
func TestInvalidTag(t *testing.T) {
	process := Process(context.Background(), nil, Input{Notes: []string{"x"}, Add: []string{"no space"}})
	process.Run()

	if !errors.Is(process.GetErr(), hashtag.ErrInvalid) {
		t.Fatalf("err = %v, want %v", process.GetErr(), hashtag.ErrInvalid)
	}
}

func TestTag(t *testing.T) {
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

	plan := note("# Plan\n\nship it #go")
	same := note("# Same\n\nalready #old")
	locked := note("# Locked\n\ntext")
	yes := true
	lock := patchnote.Process(ctx, tx, patchnote.Input{ID: locked, Readonly: &yes})
	lock.Run()
	if lock.HasErr() {
		t.Fatalf("lock: %v", lock.GetErr())
	}

	process := Process(ctx, tx, Input{Notes: []string{"/Plan", same, locked, "/Nowhere", "not-a-uuid"}, Add: []string{"old"}, Remove: []string{"GO"}})
	process.Run()
	if process.HasErr() {
		t.Fatalf("tag: %v", process.GetErr())
	}
	got := process.GetResult()
	if len(got) != 5 || got[0].ID != plan || got[0].Path != "/Plan" || !slices.Equal(got[0].Tags, []string{"old"}) || got[1].Error != "" {
		t.Fatalf("result = %+v", got)
	}
	for i, want := range []error{patchnote.ErrLocked, nil, nil} {
		if r := got[i+2]; r.Error == "" || r.ID != "" || want != nil && r.Error != want.Error() {
			t.Errorf("result %d = %+v, want an error %v", i+2, r, want)
		}
	}
	if c := content(plan); c != "# Plan\n\nship it\n\n#old" {
		t.Errorf("plan = %q", c)
	}
	if c := content(locked); c != "# Locked\n\ntext" {
		t.Errorf("locked note changed: %q", c)
	}

	// The note changed keeps its text before, even within a writing session; the one left as it was has none.
	for id, want := range map[string]int{plan: 1, same: 0} {
		versions := listversions.Process(ctx, tx, id)
		versions.Run()
		if versions.HasErr() || len(versions.GetResult()) != want {
			t.Errorf("versions of %s = %+v, %v; want %d", id, versions.GetResult(), versions.GetErr(), want)
		}
	}
}
