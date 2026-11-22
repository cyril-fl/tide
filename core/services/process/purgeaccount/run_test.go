package purgeaccount

import (
	"context"
	"testing"

	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/postgrey"
	"github.com/cyril-fl/tide/core/services/process/createfile"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/createschema"
	"github.com/cyril-fl/tide/core/services/process/createtag"
	"github.com/cyril-fl/tide/core/services/process/deleteitems"
	"github.com/cyril-fl/tide/core/services/process/purgefiles"
	"github.com/cyril-fl/tide/core/services/silo"
	"github.com/cyril-fl/tide/core/services/testaccount"
)

func TestPurgeAccount(t *testing.T) {
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
	// A tx keeps the ctx it began with: the other account's note before the account purged.
	other, tx := testaccount.New(t, ctx, tx)
	kept := createnote.Process(other, tx, createnote.Input{Content: "# Another account's #kept"})
	kept.Run()
	if kept.HasErr() {
		t.Fatalf("other note: %v", kept.GetErr())
	}
	ctx, tx = testaccount.New(t, ctx, tx)
	store := silo.NewMemory()

	// A cited image, one cited by none, a tag alone, a tagged note, a trashed one.
	var images []domain.File
	for _, name := range []string{"cited", "uncited"} {
		file := createfile.Process(ctx, tx, store, createfile.Input{Name: name, Data: []byte("\x89PNG\r\n\x1a\n")})
		file.Run()
		if file.HasErr() {
			t.Fatalf("file: %v", file.GetErr())
		}
		images = append(images, file.GetResult())
	}
	tag := createtag.Process(ctx, tx, []string{"alone"})
	tag.Run()
	if tag.HasErr() {
		t.Fatalf("tag: %v", tag.GetErr())
	}
	var notes []string
	for _, content := range []string{"# Live #shared\n\n![](" + domain.FileURL(images[0].ID) + ")", "# Trashed #shared"} {
		note := createnote.Process(ctx, tx, createnote.Input{Content: content})
		note.Run()
		if note.HasErr() {
			t.Fatalf("note: %v", note.GetErr())
		}
		notes = append(notes, note.GetResult().ID)
	}
	trash := deleteitems.Process(ctx, tx, domain.NOTE, notes[1:])
	trash.Run()
	if trash.HasErr() {
		t.Fatalf("trash: %v", trash.GetErr())
	}

	// Snippets and saved searches of both accounts.
	for _, account := range []context.Context{ctx, other} {
		if _, err := tx.NewUpdate().Table("users").Set(`snippets = '[{"content":"x"}]'`).Set(`saved_searches = '[{"name":"x"}]'`).
			Where("id = ?", domain.UserID(account)).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}

	purge := Process(ctx, tx)
	purge.Run()
	if purge.HasErr() {
		t.Fatalf("purge: %v", purge.GetErr())
	}
	files := purgefiles.Process(ctx, tx, store)
	files.Run()
	if files.HasErr() {
		t.Fatalf("purge files: %v", files.GetErr())
	}

	// The other account keeps its note and its tag, and had no image.
	for _, c := range []struct {
		table   string
		account context.Context
		want    int64
	}{{"items", ctx, 0}, {"tags", ctx, 0}, {"files", ctx, 0}, {"graph_layouts", ctx, 0}, {"items", other, 1}, {"tags", other, 1}} {
		count, err := tx.NewSelect().Table(c.table).Where("user_id = ?", domain.UserID(c.account)).Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if count != c.want {
			t.Errorf("%s of %s = %d, want %d", c.table, domain.UserID(c.account), count, c.want)
		}
	}
	for _, c := range []struct {
		account context.Context
		want    int64
	}{{ctx, 1}, {other, 0}} {
		count, err := tx.NewSelect().Table("users").Where("id = ?", domain.UserID(c.account)).
			Where("snippets = '[]'::jsonb AND saved_searches = '[]'::jsonb").Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if count != c.want {
			t.Errorf("account %s with empty snippets and saved searches = %d, want %d", domain.UserID(c.account), count, c.want)
		}
	}
	for _, image := range images {
		if _, err := store.Get(ctx, image.Key()); err == nil {
			t.Errorf("image %s still in Silo", image.Name)
		}
	}
}
