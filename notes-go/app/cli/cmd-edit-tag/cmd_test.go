package cmdedittag

import (
	"strings"
	"testing"

	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createtag"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/edittag"
)

// The tag is renamed through the API; a name already taken is refused and the old one kept.
func TestRename(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	seed := createtag.Process(ctx, tx, []string{"test-old", "test-taken"})
	seed.Run()
	if seed.HasErr() {
		t.Fatal(seed.GetErr())
	}

	printed, err := apitest.Stdout(t, func() error {
		value, err := rename(ctx, "test-old", "test-new")
		if value != "test-new" {
			t.Errorf("value = %q, want test-new", value)
		}
		return err
	})
	if err != nil || printed != "Successfully renamed \"test-old\" to \"test-new\"\n" {
		t.Errorf("printed %q, err = %v", printed, err)
	}
	if value, err := rename(ctx, "test-new", "test-taken"); value != "test-new" || err == nil || !strings.HasPrefix(err.Error(), edittag.ErrAlreadyExists.Error()) {
		t.Errorf("taken: value = %q, err = %v; want test-new, %v", value, err, edittag.ErrAlreadyExists)
	}
}
