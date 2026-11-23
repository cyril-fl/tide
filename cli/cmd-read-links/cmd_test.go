package cmdreadlinks

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/spf13/cobra"
)

// The broken links come through the API: [] and a message when there are none, then the table and --json
// once a note links to nowhere.
func TestRead(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	read := func(asJSON bool) string {
		t.Helper()
		command := &cobra.Command{}
		command.Flags().Bool("json", asJSON, "")
		command.SetContext(ctx)
		printed, err := apitest.Stdout(t, func() error { return Read(command, nil) })
		if err != nil {
			t.Fatal(err)
		}
		return printed
	}

	if got := read(true); strings.TrimSpace(got) != "[]" {
		t.Errorf("--json without broken links = %q, want []", got)
	}
	if got := read(false); !strings.Contains(got, "No broken link") {
		t.Errorf("without broken links = %q", got)
	}

	note := createnote.Process(ctx, tx, createnote.Input{Content: "# Source\nsee [[Nowhere]]"})
	note.Run()
	if note.HasErr() {
		t.Fatal(note.GetErr())
	}

	var links []apiclient.ListbrokenlinksLink
	if err := json.Unmarshal([]byte(read(true)), &links); err != nil || len(links) != 1 || links[0].NotePath != "/Source" || links[0].Target != "Nowhere" || links[0].Kind != apiclient.Unresolved {
		t.Errorf("--json = %+v, err = %v", links, err)
	}
	if got := read(false); !strings.Contains(got, "/Source") || !strings.Contains(got, "Nowhere") || !strings.Contains(got, "unresolved") {
		t.Errorf("table = %q", got)
	}
}
