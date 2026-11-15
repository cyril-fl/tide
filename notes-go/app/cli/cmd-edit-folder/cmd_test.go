package cmdeditfolder

import (
	"testing"

	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/notes-go/app/shared/domain"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createfolder"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/listpinned"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/readitem"
	"github.com/spf13/pflag"
)

// Interactive relies on it: a changed flag would skip the form.
func TestNoFlags(t *testing.T) {
	flags := noFlags()
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Changed {
			t.Errorf("--%s is changed", flag.Name)
		}
	})
	for _, name := range []string{"title", "pin", "unpin"} {
		if flags.Lookup(name) == nil {
			t.Errorf("--%s is not declared", name)
		}
	}
}

// Through the API: --pin alone keeps the title, --title with --unpin renames and unpins.
func TestEdit(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	seed := createfolder.Process(ctx, tx, createfolder.Input{Title: "Old"})
	seed.Run()
	if seed.HasErr() {
		t.Fatal(seed.GetErr())
	}
	id := seed.GetResult().ID
	edit := func(args ...string) (string, error) {
		flags := noFlags()
		if err := flags.Parse(args); err != nil {
			t.Fatal(err)
		}
		return apitest.Stdout(t, func() error { return run(ctx, id, flags) })
	}
	// The title, and whether it is a favorite: readitem reads pinned for notes only.
	read := func() (string, bool) {
		folder := readitem.Process(ctx, tx, domain.FOLDER, id).SkipChildren()
		folder.Run()
		pinned := listpinned.Process(ctx, tx)
		pinned.Run()
		if folder.HasErr() || pinned.HasErr() {
			t.Fatal(folder.GetErr(), pinned.GetErr())
		}
		return string(folder.GetResult().Item.Title), len(pinned.GetResult()) == 1
	}

	printed, err := edit("--pin")
	if title, pinned := read(); err != nil || !pinned || title != "Old" || printed != "Successfully pinned folder "+id+"\n" {
		t.Errorf("--pin: %q pinned %v, printed %q, err = %v; want Old pinned", title, pinned, printed, err)
	}
	printed, err = edit("--title", "New", "--unpin")
	if title, pinned := read(); err != nil || pinned || title != "New" {
		t.Errorf("--title New --unpin: %q pinned %v, printed %q, err = %v; want New unpinned", title, pinned, printed, err)
	}
}
