package cmdfinder

import (
	"strings"
	"testing"

	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient/apitest"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/huhform"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/createfolder"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/createnote"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/listtrash"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/moveitems"
)

// The root, a folder and the tags of its children come through the API, as do the cut and paste and the delete.
func TestThroughAPI(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	folder := createfolder.Process(ctx, tx, createfolder.Input{Title: "Projects"})
	folder.Run()
	if folder.HasErr() {
		t.Fatal(folder.GetErr())
	}
	note := createnote.Process(ctx, tx, createnote.Input{Content: "# Readme", Tags: []string{"doc"}})
	note.Run()
	if note.HasErr() {
		t.Fatal(note.GetErr())
	}

	root, err := loadFolder(ctx, "")
	if err != nil || root.Path != "/" || len(root.children) != 2 || strings.Join(root.tags[note.GetResult().ID], ",") != "doc" {
		t.Fatalf("root = %+v, err = %v; want Projects and Readme with its tag", root, err)
	}

	if err := move(ctx, apiclient.DomainItem{Id: note.GetResult().ID}, folder.GetResult().ID); err != nil {
		t.Fatal(err)
	}
	projects, err := loadFolder(ctx, folder.GetResult().ID)
	if err != nil || projects.Path != "/Projects" || len(projects.children) != 1 || len(projects.tags[note.GetResult().ID]) != 1 {
		t.Errorf("projects = %+v, err = %v; want Readme inside with its tag", projects, err)
	}
	if err := move(ctx, apiclient.DomainItem{Id: folder.GetResult().ID, Kind: apiclient.FOLDER}, folder.GetResult().ID); err == nil || !strings.HasPrefix(err.Error(), moveitems.ErrCycle.Error()) {
		t.Errorf("into itself: err = %v, want %v", err, moveitems.ErrCycle)
	}
}

// remove asks, then moves the folder and its content to the trash through the API.
func TestRemove(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	folder := createfolder.Process(ctx, tx, createfolder.Input{Title: "Projects"})
	folder.Run()
	if folder.HasErr() {
		t.Fatal(folder.GetErr())
	}
	note := createnote.Process(ctx, tx, createnote.Input{Content: "# Inside", ParentID: folder.GetResult().ID})
	note.Run()
	if note.HasErr() {
		t.Fatal(note.GetErr())
	}

	huhform.Input = strings.NewReader("y\r")
	t.Cleanup(func() { huhform.Input = nil })
	if err := remove(ctx, apiclient.DomainItem{Id: folder.GetResult().ID, Kind: apiclient.FOLDER, Title: "Projects"}); err != nil {
		t.Fatal(err)
	}
	trash := listtrash.Process(ctx, tx)
	trash.Run()
	if trash.HasErr() || len(trash.GetResult()) != 1 || trash.GetResult()[0].ID != folder.GetResult().ID || trash.GetResult()[0].Inside != 1 {
		t.Errorf("trash = %+v, err = %v; want the folder with its note", trash.GetResult(), trash.GetErr())
	}
}
