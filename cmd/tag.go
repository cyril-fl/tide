package cmd

import (
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	cmdtagnotes "github.com/cyril-fl/tide/cli/cmd-tag-notes"
	"github.com/spf13/cobra"
)

var cmdTag = &cobra.Command{
	Use:     cmd.Tag.Use,
	Short:   cmd.Tag.Short,
	Long:    cmd.Tag.Long,
	Example: cmd.Tag.Example,
}

var cmdTagNotes = &cobra.Command{
	Use:     cmd.TagNotes.Use,
	Short:   cmd.TagNotes.Short,
	Long:    cmd.TagNotes.Long,
	Example: cmd.TagNotes.Example,
	RunE:    cmdtagnotes.Tag,
}

func init() {
	plumcliutilsargs.Makeflags(cmdTagNotes, cmdtagnotes.Flags())

	cmdTag.AddCommand(cmdTagNotes)
	cmdRoot.AddCommand(cmdTag)
}
