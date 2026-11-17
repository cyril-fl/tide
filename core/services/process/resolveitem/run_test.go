package resolveitem

import (
	"context"
	"errors"
	"fmt"
	"github.com/cyril-fl/tide/core/services/testaccount"
	"strings"
	"testing"
	"time"

	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/postgrey"
	"github.com/cyril-fl/tide/core/services/process/createfolder"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/createschema"
	"github.com/cyril-fl/tide/core/services/process/deleteitems"
)

// No DB needed: only paths are looked up, so a nil db is never touched.
func TestNotAPath(t *testing.T) {
	process := Process(context.Background(), nil, domain.NOTE, []string{"some-uuid", "other"})
	process.Run()
	if process.HasErr() || strings.Join(process.GetResult(), ",") != "some-uuid,other" {
		t.Fatalf("result = %v, %v", process.GetResult(), process.GetErr())
	}
}

func TestResolvePaths(t *testing.T) {
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

	// Unique per run so real items cannot match.
	root := fmt.Sprintf("test-%d", time.Now().UnixNano())
	folder := createfolder.Process(ctx, tx, createfolder.Input{Title: root})
	folder.Run()
	if folder.HasErr() {
		t.Fatalf("folder: %v", folder.GetErr())
	}
	folderID := folder.GetResult().ID

	note := func(title string) string {
		process := createnote.Process(ctx, tx, createnote.Input{Content: "# " + title, ParentID: folderID})
		process.Run()
		if process.HasErr() {
			t.Fatalf("note %s: %v", title, process.GetErr())
		}
		return process.GetResult().ID
	}
	idea := note("Idea")
	slashed := note("Paris / Rome")
	twin1, twin2 := note("Twin"), note("Twin")
	trashed := note("Trashed")
	trash := deleteitems.Process(ctx, tx, domain.NOTE, []string{trashed})
	trash.Run()
	if trash.HasErr() {
		t.Fatalf("trash: %v", trash.GetErr())
	}
	// A folder with the same path as a note: only the kind tells them apart.
	same := createfolder.Process(ctx, tx, createfolder.Input{Title: "Idea", ParentID: folderID})
	same.Run()
	if same.HasErr() {
		t.Fatalf("same path folder: %v", same.GetErr())
	}

	base := "/" + root
	tests := map[string]struct {
		kind    domain.Kind
		refs    []string
		want    []string
		wantErr error
	}{
		"folder":          {domain.FOLDER, []string{base}, []string{folderID}, nil},
		"nested":          {domain.NOTE, []string{base + "/Idea"}, []string{idea}, nil},
		"/ in the title":  {domain.NOTE, []string{base + "/Paris / Rome"}, []string{slashed}, nil},
		"kept in order":   {domain.NOTE, []string{"a-uuid", base + "/Idea"}, []string{"a-uuid", idea}, nil},
		"kind filters":    {domain.FOLDER, []string{base + "/Idea"}, []string{same.GetResult().ID}, nil},
		"wrong kind":      {domain.NOTE, []string{base}, nil, ErrNotFound},
		"any kind":        {"", []string{base}, []string{folderID}, nil},
		"any kind, twins": {"", []string{base + "/Idea"}, nil, ErrAmbiguous},
		"not found":       {domain.NOTE, []string{base + "/Nope"}, nil, ErrNotFound},
		"trashed":         {domain.NOTE, []string{base + "/Trashed"}, nil, ErrNotFound},
		"ambiguous":       {domain.NOTE, []string{base + "/Twin"}, nil, ErrAmbiguous},
	}

	for name, test := range tests {
		process := Process(ctx, tx, test.kind, test.refs)
		process.Run()
		if test.wantErr != nil {
			if !errors.Is(process.GetErr(), test.wantErr) {
				t.Errorf("%s: err = %v, want %v", name, process.GetErr(), test.wantErr)
			}
			continue
		}
		if process.HasErr() || strings.Join(process.GetResult(), ",") != strings.Join(test.want, ",") {
			t.Errorf("%s: result = %v, %v; want %v", name, process.GetResult(), process.GetErr(), test.want)
		}
	}

	// A path of the other kind says what it is, not just that nothing is there.
	wrong := Process(ctx, tx, domain.NOTE, []string{base})
	wrong.Run()
	if want := fmt.Sprintf("%q is a folder, not a note", base); !wrong.HasErr() || !strings.HasSuffix(wrong.GetErr().Error(), want) {
		t.Errorf("err = %v, want it to end with %s", wrong.GetErr(), want)
	}

	// The error gives both ids with their date, so the user can pick one.
	twins := Process(ctx, tx, domain.NOTE, []string{base + "/Twin"})
	twins.Run()
	if !twins.HasErr() || !strings.Contains(twins.GetErr().Error(), twin1+" (note changed ") || !strings.Contains(twins.GetErr().Error(), twin2) {
		t.Errorf("err = %v, want both %s and %s with their date", twins.GetErr(), twin1, twin2)
	}
}
