package cmdreadsnippet

import (
	"fmt"
	"slices"
	"strings"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	plumdebug "github.com/cyril-fl/plum/utils/debug"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/core/services/style"
	"github.com/spf13/cobra"
)

// Read lists the snippets with the first line of their markdown, or prints the markdown of the one named.
func Read(cobra *cobra.Command, args []string) error {
	var asJSON bool
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"json": &asJSON}); err != nil {
		return err
	}
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	snippets, err := apiclient.Snippets(cobra.Context(), client)
	if err != nil {
		return err
	}

	if len(args) == 1 {
		i := slices.IndexFunc(snippets, func(s apiclient.DomainSnippet) bool { return s.Name == args[0] })
		switch {
		case i < 0:
			return fmt.Errorf("%w: %q", apiclient.ErrNoSnippet, args[0])
		case asJSON:
			plumdebug.Json(snippets[i])
		default:
			fmt.Println(snippets[i].Content)
		}
		return nil
	}

	switch {
	case asJSON:
		plumdebug.Json(snippets)
	case len(snippets) == 0:
		fmt.Println(style.Empty().Render("No snippet"))
	default:
		cells := [][]string{{"Name", "Starts with"}}
		for _, snippet := range snippets {
			first, _, _ := strings.Cut(strings.TrimSpace(snippet.Content), "\n")
			cells = append(cells, []string{snippet.Name, first})
		}
		fmt.Print(style.Table(cells, -1, 0))
	}
	return nil
}
