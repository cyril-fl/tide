package cmd

import (
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	cmdmove "github.com/cyril-fl/tide/notes-go/app/cli/cmd-move"
	"github.com/spf13/cobra"
)

var cmdMove = &cobra.Command{
	Use:     cmd.Move.Use,
	Short:   cmd.Move.Short,
	Long:    cmd.Move.Long,
	Example: cmd.Move.Example,
	Args:    cobra.MinimumNArgs(1),
	RunE:    apiResolved("", cmdmove.Move),
}

func init() {
	plumcliutilsargs.Makeflags(cmdMove, cmd.Move.Flags)
	cmdMove.MarkFlagsMutuallyExclusive("parent", "parent-from-list")

	cmdRoot.AddCommand(cmdMove)
}
