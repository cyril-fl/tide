package cmdedittag

import (
	"strings"
	"testing"

	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/createtag"
	"github.com/cyril-fl/tide/core/services/process/edittag"
)

// The tag is renamed through the API; --dry-run prints the lines it would change and keeps the old name; a name
// already taken is refused and the old one kept, unless --merge folds it in.
func TestRename(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	seed := createtag.Process(ctx, tx, []string{"test-taken"})
	seed.Run()
	if seed.HasErr() {
		t.Fatal(seed.GetErr())
	}
	note := createnote.Process(ctx, tx, createnote.Input{Content: "# Plan\n#test-old"})
	note.Run()
	if note.HasErr() {
		t.Fatal(note.GetErr())
	}

	printed, err := apitest.Stdout(t, func() error {
		value, err := rename(ctx, "test-old", "test-new", false, true)
		if value != "test-old" {
			t.Errorf("dry run: value = %q, want test-old", value)
		}
		return err
	})
	if err != nil || printed != "/Plan\n    - #test-old\n    + #test-new\nWould rename \"test-old\" to \"test-new\" in 1 note(s) (dry run, nothing written)\n" {
		t.Errorf("dry run: printed %q, err = %v", printed, err)
	}

	printed, err = apitest.Stdout(t, func() error {
		value, err := rename(ctx, "test-old", "test-new", false, false)
		if value != "test-new" {
			t.Errorf("value = %q, want test-new", value)
		}
		return err
	})
	if err != nil || printed != "Successfully renamed \"test-old\" to \"test-new\"\n" {
		t.Errorf("printed %q, err = %v", printed, err)
	}
	if value, err := rename(ctx, "test-new", "test-taken", false, false); value != "test-new" || err == nil || !strings.HasPrefix(err.Error(), edittag.ErrAlreadyExists.Error()) {
		t.Errorf("taken: value = %q, err = %v; want test-new, %v", value, err, edittag.ErrAlreadyExists)
	}
	if _, err := apitest.Stdout(t, func() error {
		value, err := rename(ctx, "test-new", "test-taken", true, false)
		if value != "test-taken" {
			t.Errorf("merge: value = %q, want test-taken", value)
		}
		return err
	}); err != nil {
		t.Errorf("merge: err = %v", err)
	}
}
