package cmdgraph

import (
	"testing"

	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createnote"
)

// Every note and a note's neighborhood come through the API, the linked notes with their tags.
func TestPages(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	target := createnote.Process(ctx, tx, createnote.Input{Content: "# Target", Tags: []string{"go"}})
	target.Run()
	if target.HasErr() {
		t.Fatal(target.GetErr())
	}
	source := createnote.Process(ctx, tx, createnote.Input{Content: "# Source\n[[Target]]"})
	source.Run()
	if source.HasErr() {
		t.Fatal(source.GetErr())
	}

	all, err := notesPage(ctx)
	if err != nil || all.Info != "2 notes" || len(all.Rows) != 2 || all.Rows[0].Cells[0] != "Source" || all.Rows[0].Cells[2] != "1 link · 0 tags" {
		t.Errorf("notes page = %+v, err = %v; want Source then Target, linked", all, err)
	}

	near, err := notePage(ctx, source.GetResult().ID)
	if err != nil || len(near.Rows) != 1 || near.Rows[0].Target.Key != target.GetResult().ID || len(near.Rows[0].Tags) != 1 || near.Rows[0].Tags[0] != "go" {
		t.Errorf("page of Source = %+v, err = %v; want Target with its tag", near, err)
	}
}
