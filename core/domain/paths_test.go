package domain_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/postgrey"
	"github.com/cyril-fl/tide/core/services/process/createfolder"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/createschema"
	"github.com/cyril-fl/tide/core/services/process/deleteitems"
	"github.com/cyril-fl/tide/core/services/process/editnote"
	"github.com/cyril-fl/tide/core/services/process/moveitems"
	"github.com/cyril-fl/tide/core/services/process/restoreitems"
	"github.com/cyril-fl/tide/core/services/testaccount"
)

// The cache of Paths follows every write that changes a path, even one in plain SQL as another process would make,
// and is kept by a content edit.
func TestPathsCache(t *testing.T) {
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

	folder := createfolder.Process(ctx, tx, createfolder.Input{Title: "Projects"})
	folder.Run()
	if folder.HasErr() {
		t.Fatal(folder.GetErr())
	}
	folderID := folder.GetResult().ID
	note := createnote.Process(ctx, tx, createnote.Input{Content: "# Ideas", ParentID: folderID})
	note.Run()
	if note.HasErr() {
		t.Fatal(note.GetErr())
	}
	noteID := note.GetResult().ID

	paths := func(step string, want string) map[string]string {
		t.Helper()
		got, err := domain.Paths(ctx, tx)
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		if got[noteID] != want {
			t.Fatalf("%s: path = %q, want %q", step, got[noteID], want)
		}
		return got
	}
	same := func(step string, a, b map[string]string) {
		t.Helper()
		if reflect.ValueOf(a).Pointer() != reflect.ValueOf(b).Pointer() {
			t.Fatalf("%s: Paths rebuilt, want the cached map", step)
		}
	}
	run := func(step string, process interface {
		Run()
		HasErr() bool
		GetErr() error
	}) {
		t.Helper()
		process.Run()
		if process.HasErr() {
			t.Fatalf("%s: %v", step, process.GetErr())
		}
	}
	sql := func(step, query string, args ...any) {
		t.Helper()
		if _, err := tx.NewRaw(query, args...).Exec(ctx); err != nil {
			t.Fatalf("%s: %v", step, err)
		}
	}

	first := paths("created", "/Projects/Ideas")
	same("nothing changed", first, paths("again", "/Projects/Ideas"))

	run("edit the content", editnote.Process(ctx, tx, editnote.Input{ID: noteID, Content: "# Ideas\n\nmore"}))
	same("content edited", first, paths("content edited", "/Projects/Ideas"))

	sql("moved by the CLI", "UPDATE items SET parent_id = NULL WHERE id = ?", noteID)
	paths("moved by the CLI", "/Ideas")

	run("move", moveitems.Process(ctx, tx, moveitems.Input{IDs: []string{noteID}, ParentID: folderID}))
	paths("moved", "/Projects/Ideas")

	run("rename", editnote.Process(ctx, tx, editnote.Input{ID: noteID, Content: "# Plans"}))
	paths("renamed", "/Projects/Plans")

	run("trash", deleteitems.Process(ctx, tx, domain.FOLDER, []string{folderID}))
	paths("trashed", "")

	run("restore", restoreitems.Process(ctx, tx, []string{folderID}))
	paths("restored", "/Projects/Plans")

	sql("deleted by the CLI", "DELETE FROM items WHERE id = ?", noteID)
	paths("deleted by the CLI", "")
}

// Live drops the rows missing from the tree and sorts the others by first, then by path; Page cuts them.
func TestLiveAndPage(t *testing.T) {
	tree := domain.Tree{Paths: map[string]string{"a": "/Zoo", "b": "/abricot", "c": "/Été"}}
	type row struct {
		id     string
		pinned bool
	}
	rows := []row{{"a", false}, {"trashed", false}, {"b", false}, {"c", true}}
	id := func(r row) string { return r.id }

	byPath := domain.Live(tree, rows, id, nil)
	if want := []row{{"b", false}, {"c", true}, {"a", false}}; !reflect.DeepEqual(byPath, want) {
		t.Errorf("by path: got %v, want %v", byPath, want)
	}
	pinnedFirst := domain.Live(tree, rows, id, func(a, b row) int {
		if a.pinned == b.pinned {
			return 0
		}
		if a.pinned {
			return -1
		}
		return 1
	})
	if want := []row{{"c", true}, {"b", false}, {"a", false}}; !reflect.DeepEqual(pinnedFirst, want) {
		t.Errorf("pinned first: got %v, want %v", pinnedFirst, want)
	}

	for _, c := range []struct {
		limit, offset int
		want          []row
	}{
		{0, 0, byPath},
		{2, 0, byPath[:2]},
		{2, 2, byPath[2:]},
		{2, 9, []row{}},
	} {
		if got := domain.Page(byPath, c.limit, c.offset); !reflect.DeepEqual(got, c.want) {
			t.Errorf("page %d from %d: got %v, want %v", c.limit, c.offset, got, c.want)
		}
	}
}
