package cmd

import (
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	cmdsearch "github.com/cyril-fl/tide/cli/cmd-search"
	"github.com/spf13/cobra"
)

var cmdSearch = &cobra.Command{
	Use:     cmd.Search.Use,
	Short:   cmd.Search.Short,
	Long:    cmd.Search.Long,
	Example: cmd.Search.Example,
	RunE:    cmdsearch.Search,
}

func init() {
	plumcliutilsargs.Makeflags(cmdSearch, cmd.Search.Flags)
	cmdSearch.MarkFlagsMutuallyExclusive("duplicates", "same-path")

	cmdRoot.AddCommand(cmdSearch)
}
