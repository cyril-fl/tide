package listnotetags

import (
	"context"
	"slices"
	"testing"

	"github.com/cyril-fl/tide/core/services/postgrey"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/createschema"
	"github.com/cyril-fl/tide/core/services/testaccount"
)

func TestListNoteTags(t *testing.T) {
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

	var ids []string
	for _, input := range []createnote.Input{
		{Content: "# Tagged", Tags: []string{"zeta", "alpha"}},
		{Content: "# Bare"},
	} {
		note := createnote.Process(ctx, tx, input)
		note.Run()
		if note.HasErr() {
			t.Fatalf("note: %v", note.GetErr())
		}
		ids = append(ids, note.GetResult().ID)
	}

	process := Process(ctx, tx, ids)
	process.Run()
	tags := process.GetResult()
	if process.HasErr() || !slices.Equal(tags[ids[0]], []string{"alpha", "zeta"}) || len(tags[ids[1]]) != 0 {
		t.Errorf("tags = %v, %v; want alpha, zeta for the first note only", tags, process.GetErr())
	}
}
