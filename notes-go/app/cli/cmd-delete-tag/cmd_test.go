package cmddeletetag

import (
	"testing"

	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient/apitest"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/createtag"
	"github.com/spf13/cobra"
)

// The tags are deleted through the API; an unknown one is skipped and left out of the count.
func TestDelete(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	seed := createtag.Process(ctx, tx, []string{"test-go", "test-sql"})
	seed.Run()
	if seed.HasErr() {
		t.Fatal(seed.GetErr())
	}

	command := &cobra.Command{}
	command.SetContext(ctx)
	printed, err := apitest.Stdout(t, func() error { return Delete(command, []string{"test-go", "test-nothing", "test-sql"}) })
	if err != nil || printed != "Successfully deleted 2 tag(s)\n" {
		t.Errorf("printed %q, err = %v; want 2 tags", printed, err)
	}
}
