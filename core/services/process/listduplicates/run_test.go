package listduplicates

import (
	"context"
	"testing"

	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/postgrey"
	"github.com/cyril-fl/tide/core/services/process/createfolder"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/createschema"
	"github.com/cyril-fl/tide/core/services/process/deleteitems"
	"github.com/cyril-fl/tide/core/services/testaccount"
)

func TestListDuplicates(t *testing.T) {
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

	folder := createfolder.Process(ctx, tx, createfolder.Input{Title: "Work"})
	folder.Run()
	if folder.HasErr() {
		t.Fatalf("folder: %v", folder.GetErr())
	}
	parent := folder.GetResult().ID

	var trashed string
	for _, in := range []createnote.Input{
		{Content: "# Pizza\none"},
		{Content: "# Pizza\ntwo"},
		{Content: "# pizza\nthree", ParentID: parent},
		{Content: "# Alone"},
		{Content: "# Gone\na"},
		{Content: "# Gone\nb"},
	} {
		note := createnote.Process(ctx, tx, in)
		note.Run()
		if note.HasErr() {
			t.Fatalf("note %q: %v", in.Content, note.GetErr())
		}
		trashed = note.GetResult().ID
	}
	trash := deleteitems.Process(ctx, tx, domain.NOTE, []string{trashed})
	trash.Run()
	if trash.HasErr() {
		t.Fatalf("trash: %v", trash.GetErr())
	}

	// By title: the 3 Pizza in any folder; Gone has one live note left; Alone is single.
	process := Process(ctx, tx, false)
	process.Run()
	groups := process.GetResult()
	if process.HasErr() || len(groups) != 1 || len(groups[0].Notes) != 3 || groups[0].Notes[0].Length == 0 {
		t.Fatalf("by title = %+v, %v; want one group of 3", groups, process.GetErr())
	}

	// By path: only the 2 in the same folder.
	process = Process(ctx, tx, true)
	process.Run()
	groups = process.GetResult()
	if process.HasErr() || len(groups) != 1 || groups[0].Key != "/Pizza" || len(groups[0].Notes) != 2 {
		t.Errorf("by path = %+v, %v; want the 2 notes of /Pizza", groups, process.GetErr())
	}
}
