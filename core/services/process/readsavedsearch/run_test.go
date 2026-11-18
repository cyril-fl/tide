package readsavedsearch

import (
	"context"
	"errors"
	"testing"

	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/postgrey"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/createschema"
	"github.com/cyril-fl/tide/core/services/process/edituser"
	"github.com/cyril-fl/tide/core/services/testaccount"
)

func TestReadSavedSearch(t *testing.T) {
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

	for content, tags := range map[string][]string{
		"# Match me": nil, "# Draft\n\nTODO": {"idee", "cuisine"}, "# Recipe": {"idee", "boisson"}, "# Other": {"idee", "cuisine", "vieux"},
	} {
		note := createnote.Process(ctx, tx, createnote.Input{Content: content, Tags: tags})
		note.Run()
		if note.HasErr() {
			t.Fatalf("note: %v", note.GetErr())
		}
	}
	var tags []domain.Tag
	if err := tx.NewSelect().Model(&tags).Where("user_id = ?", domain.UserID(ctx)).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	id := map[string]string{}
	for _, tag := range tags {
		id[string(tag.Value)] = tag.ID
	}

	// The new criteria are stored sealed with the rest and replayed: idee AND (cuisine OR boisson), not vieux, no TODO.
	searches, pro := []domain.SavedSearch{{ID: "s1", Name: "Everything", NoTagIDs: []string{id["vieux"]}, NotText: "todo"}, {
		ID: "s2", Name: "Ideas", TagIDs: []string{id["idee"]}, TagMode: "all", AnyTagIDs: []string{id["cuisine"], id["boisson"]},
		NoTagIDs: []string{id["vieux"]}, NotText: "todo",
	}}, "pro"
	save := edituser.Process(ctx, tx, edituser.Input{Plan: &pro, SavedSearches: &searches})
	save.Run()
	if save.HasErr() {
		t.Fatalf("save: %v", save.GetErr())
	}

	found := Process(ctx, tx, "s1")
	found.Run()
	if found.HasErr() || len(found.GetResult()) != 2 || found.GetResult()[0].Title != "Match me" || found.GetResult()[1].Title != "Recipe" {
		t.Fatalf("found = %+v, %v; want Match me and Recipe", found.GetResult(), found.GetErr())
	}
	ideas := Process(ctx, tx, "s2")
	ideas.Run()
	if ideas.HasErr() || len(ideas.GetResult()) != 1 || ideas.GetResult()[0].Title != "Recipe" {
		t.Fatalf("ideas = %+v, %v; want Recipe", ideas.GetResult(), ideas.GetErr())
	}

	missing := Process(ctx, tx, "absent")
	missing.Run()
	if !errors.Is(missing.GetErr(), ErrNotFound) {
		t.Fatalf("missing: err = %v, want %v", missing.GetErr(), ErrNotFound)
	}
}
