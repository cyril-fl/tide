package cmdreadsnippet

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	plumcliconfig "github.com/cyril-fl/plum/cli/config"
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	cmdcreatesnippet "github.com/cyril-fl/tide/cli/cmd-create-snippet"
	cmddeletesnippet "github.com/cyril-fl/tide/cli/cmd-delete-snippet"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/config"
	"github.com/spf13/cobra"
)

// add snippet saves, then replaces; show snippet lists and prints one; remove snippet deletes it, and a second time
// says it is gone.
func TestSnippets(t *testing.T) {
	ctx, _ := apitest.Serve(t)
	run := func(flags []plumcliconfig.Cmdflags, do func(*cobra.Command, []string) error, args []string, parse ...string) (string, error) {
		t.Helper()
		command := &cobra.Command{}
		plumcliutilsargs.Makeflags(command, flags)
		if err := command.ParseFlags(parse); err != nil {
			t.Fatal(err)
		}
		command.SetContext(ctx)
		return apitest.Stdout(t, func() error { return do(command, args) })
	}
	commands := config.Current.Commands

	if printed, err := run(commands.ReadSnippet.Flags, Read, nil); err != nil || !strings.Contains(printed, "No snippet") {
		t.Errorf("show none = %q, %v", printed, err)
	}
	for _, content := range []string{"# Draft", "# Recette\n\n### Ingrédients"} {
		if _, err := run(commands.CreateSnippet.Flags, cmdcreatesnippet.Create, []string{"Recette"}, "--content", content); err != nil {
			t.Fatal(err)
		}
	}

	printed, err := run(commands.ReadSnippet.Flags, Read, nil, "--json")
	var snippets []apiclient.DomainSnippet
	if err == nil {
		err = json.Unmarshal([]byte(printed), &snippets)
	}
	if err != nil || len(snippets) != 1 || snippets[0].Content != "# Recette\n\n### Ingrédients" {
		t.Errorf("show --json = %s, %v; want the replaced snippet only", printed, err)
	}
	if printed, err := run(commands.ReadSnippet.Flags, Read, []string{"Recette"}); err != nil || printed != "# Recette\n\n### Ingrédients\n" {
		t.Errorf("show Recette = %q, %v", printed, err)
	}

	if _, err := run(commands.DeleteSnippet.Flags, cmddeletesnippet.Delete, []string{"Recette"}); err != nil {
		t.Fatal(err)
	}
	if _, err := run(commands.DeleteSnippet.Flags, cmddeletesnippet.Delete, []string{"Recette"}); !errors.Is(err, apiclient.ErrNoSnippet) {
		t.Errorf("second remove: err = %v, want ErrNoSnippet", err)
	}
}
