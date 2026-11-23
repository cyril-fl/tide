package cmd

import (
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	cmdreplace "github.com/cyril-fl/tide/cli/cmd-replace"
	"github.com/spf13/cobra"
)

var cmdReplace = &cobra.Command{
	Use:     cmd.Replace.Use,
	Short:   cmd.Replace.Short,
	Long:    cmd.Replace.Long,
	Example: cmd.Replace.Example,
	Args:    cobra.ExactArgs(2),
	RunE:    cmdreplace.Replace,
}

func init() {
	plumcliutilsargs.Makeflags(cmdReplace, cmd.Replace.Flags)

	cmdRoot.AddCommand(cmdReplace)
}
