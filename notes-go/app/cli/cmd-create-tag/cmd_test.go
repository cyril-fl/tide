package cmdcreatetag

import (
	"testing"

	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient/apitest"
	"github.com/spf13/cobra"
)

// The tags are created through the API, an existing one skipped and left out of the count.
func TestCreate(t *testing.T) {
	ctx, _ := apitest.Serve(t)
	command := &cobra.Command{}
	command.SetContext(ctx)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"test-go", "test-sql"}, "Successfully created 2 tag(s)\n"},
		{[]string{"test-go", "test-db"}, "Successfully created 1 tag(s)\n"},
	} {
		if printed, err := apitest.Stdout(t, func() error { return Create(command, tc.args) }); err != nil || printed != tc.want {
			t.Errorf("%v: printed %q, err = %v; want %q", tc.args, printed, err, tc.want)
		}
	}
}
