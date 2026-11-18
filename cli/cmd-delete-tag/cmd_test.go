package cmddeletetag

import (
	"testing"

	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/core/services/process/createtag"
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
	command.Flags().Bool("dry-run", true, "")
	command.Flags().Bool("keep-empty", false, "")
	dry, err := apitest.Stdout(t, func() error { return Delete(command, []string{"test-go"}) })
	if err != nil || dry != "Would delete 1 tag(s): 0 note(s) changed, 0 to the trash (dry run, nothing written)\n" {
		t.Errorf("dry run printed %q, err = %v", dry, err)
	}
	if err := command.Flags().Set("dry-run", "false"); err != nil {
		t.Fatal(err)
	}
	printed, err := apitest.Stdout(t, func() error { return Delete(command, []string{"test-go", "test-nothing", "test-sql"}) })
	if err != nil || printed != "Successfully deleted 2 tag(s): 0 note(s) changed, 0 moved to the trash\n" {
		t.Errorf("printed %q, err = %v; want 2 tags", printed, err)
	}
}
