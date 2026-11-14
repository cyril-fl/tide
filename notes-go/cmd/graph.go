package cmd

import (
	cmdgraph "github.com/cyril-fl/notes/notes-go/app/cli/cmd-graph"
	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/spf13/cobra"
)

var cmdGraph = &cobra.Command{
	Use:     cmd.ShowGraph.Use,
	Short:   cmd.ShowGraph.Short,
	Long:    cmd.ShowGraph.Long,
	Example: cmd.ShowGraph.Example,
	Args:    cobra.MaximumNArgs(1),
	RunE:    apiResolved(apiclient.NOTE, cmdgraph.Graph),
}

func init() {
	cmdRoot.AddCommand(cmdGraph)
}
