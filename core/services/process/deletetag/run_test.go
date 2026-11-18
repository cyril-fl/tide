package deletetag

import (
	"context"
	"errors"
	"fmt"
	"github.com/cyril-fl/tide/core/services/testaccount"
	"testing"
	"time"

	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/postgrey"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/createschema"
	"github.com/cyril-fl/tide/core/services/process/createtag"
	"github.com/cyril-fl/tide/core/services/process/deleteitems"
	"github.com/cyril-fl/tide/core/services/process/readitem"
)

// No DB needed: makeValues fails first and deleteTags is guarded, so a nil db is never touched.
func TestBlankValues(t *testing.T) {
	process := Process(context.Background(), nil, []string{"", "   "})
	process.Run()

	if !errors.Is(process.GetErr(), ErrNoValue) {
		t.Fatalf("err = %v, want %v", process.GetErr(), ErrNoValue)
	}
}

func TestDeleteIgnoresCaseAndUnknown(t *testing.T) {
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

	// Unique per run so real data in the DB cannot collide.
	value := fmt.Sprintf("test-%d", time.Now().UnixNano())

	create := createtag.Process(ctx, tx, []string{value, value + "-keep"})
	create.Run()
	if create.HasErr() {
		t.Fatalf("create: %v", create.GetErr())
	}

	first := Process(ctx, tx, []string{" TEST" + value[4:] + " ", value + "-unknown"})
	first.Run()
	if first.HasErr() {
		t.Fatalf("first run: %v", first.GetErr())
	}
	got := first.GetResult().Tags
	if len(got) != 1 || string(got[0].Value) != value {
		t.Fatalf("first result = %+v, want [%s]", got, value)
	}

	second := Process(ctx, tx, []string{value})
	second.Run()
	if second.HasErr() {
		t.Fatalf("second run: %v", second.GetErr())
	}
	if len(second.GetResult().Tags) != 0 {
		t.Errorf("second result = %+v, want empty", second.GetResult())
	}

	// The text of its notes loses the tag; a note left empty goes to the trash.
	gone := value + "-gone"
	kept := createnote.Process(ctx, tx, createnote.Input{Content: "du #" + gone + " ici"})
	kept.Run()
	bare := createnote.Process(ctx, tx, createnote.Input{Content: "#" + gone})
	bare.Run()
	if kept.HasErr() || bare.HasErr() {
		t.Fatalf("seed: %v, %v", kept.GetErr(), bare.GetErr())
	}
	// A dry run answers the same and leaves everything as it was.
	dry := Process(ctx, tx, []string{gone}).DryRun()
	dry.Run()
	if r := dry.GetResult(); dry.HasErr() || len(r.Tags) != 1 || r.Changed != 2 || len(r.Trashed) != 1 || r.Trashed[0].ID != bare.GetResult().ID || r.Trashed[0].Path == "" {
		t.Fatalf("dry run = %+v, %v; want 1 tag, 2 changed, the bare note trashed", r, dry.GetErr())
	}
	again := readitem.Process(ctx, tx, domain.NOTE, bare.GetResult().ID)
	again.Run()
	if again.HasErr() || len(again.GetResult().Tags) != 1 {
		t.Fatalf("dry run wrote: %+v, %v", again.GetResult(), again.GetErr())
	}

	inText := Process(ctx, tx, []string{gone})
	inText.Run()
	if r := inText.GetResult(); inText.HasErr() || r.Changed != 2 || len(r.Trashed) != 1 || r.Trashed[0].ID != bare.GetResult().ID {
		t.Fatalf("delete in text = %+v, %v", r, inText.GetErr())
	}
	read := readitem.Process(ctx, tx, domain.NOTE, kept.GetResult().ID)
	read.Run()
	if got := read.GetResult(); got.Content != "du ici" || len(got.Tags) != 0 {
		t.Errorf("after delete: %+v, %v", got, read.GetErr())
	}
	if n, err := tx.NewSelect().Table("note_versions").Where("item_id = ?", kept.GetResult().ID).Count(ctx); n != 1 || err != nil {
		t.Errorf("versions after delete = %d, %v; want 1", n, err)
	}
	trashed := readitem.Process(ctx, tx, domain.NOTE, bare.GetResult().ID)
	trashed.Run()
	if !errors.Is(trashed.GetErr(), readitem.ErrNotFound) {
		t.Errorf("empty note: err = %v, want it in the trash", trashed.GetErr())
	}

	// keep_empty leaves the bare note empty, out of the trash; a note in the trash already stays there, unlisted.
	empty := value + "-empty"
	left := createnote.Process(ctx, tx, createnote.Input{Content: "#" + empty})
	left.Run()
	binned := createnote.Process(ctx, tx, createnote.Input{Content: "#" + empty + "-bin"})
	binned.Run()
	if left.HasErr() || binned.HasErr() {
		t.Fatalf("seed: %v, %v", left.GetErr(), binned.GetErr())
	}
	bin := deleteitems.Process(ctx, tx, domain.NOTE, []string{binned.GetResult().ID})
	bin.Run()
	if bin.HasErr() {
		t.Fatal(bin.GetErr())
	}
	var before time.Time
	if err := tx.NewSelect().Table("items").Column("deleted_at").Where("id = ?", binned.GetResult().ID).Scan(ctx, &before); err != nil {
		t.Fatal(err)
	}
	keepEmpty := Process(ctx, tx, []string{empty}).KeepEmpty()
	keepEmpty.Run()
	if r := keepEmpty.GetResult(); keepEmpty.HasErr() || r.Changed != 1 || len(r.Trashed) != 0 {
		t.Errorf("keep empty = %+v, %v; want 1 changed, none trashed", r, keepEmpty.GetErr())
	}
	emptied := readitem.Process(ctx, tx, domain.NOTE, left.GetResult().ID)
	emptied.Run()
	if emptied.HasErr() || emptied.GetResult().Content != "" {
		t.Errorf("kept empty note: %+v, %v", emptied.GetResult(), emptied.GetErr())
	}
	inBin := Process(ctx, tx, []string{empty + "-bin"})
	inBin.Run()
	if r := inBin.GetResult(); inBin.HasErr() || r.Changed != 1 || len(r.Trashed) != 0 {
		t.Errorf("tag of a trashed note = %+v, %v; want 1 changed, none trashed", r, inBin.GetErr())
	}
	var after time.Time
	if err := tx.NewSelect().Table("items").Column("deleted_at").Where("id = ?", binned.GetResult().ID).Scan(ctx, &after); err != nil || !after.Equal(before) {
		t.Errorf("deleted_at of a trashed note moved: %v -> %v, %v", before, after, err)
	}

	// The other tag must survive.
	keep := Process(ctx, tx, []string{value + "-keep"})
	keep.Run()
	if keep.HasErr() || len(keep.GetResult().Tags) != 1 {
		t.Errorf("keep result = %+v, err = %v, want 1 deleted", keep.GetResult(), keep.GetErr())
	}
}
