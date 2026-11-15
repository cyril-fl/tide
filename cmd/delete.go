package cmd

import (
	"github.com/spf13/cobra"

	cmddeletefolder "github.com/cyril-fl/tide/cli/cmd-delete-folder"
	cmddeletenote "github.com/cyril-fl/tide/cli/cmd-delete-note"
	cmddeletetag "github.com/cyril-fl/tide/cli/cmd-delete-tag"
	cmddeletetoken "github.com/cyril-fl/tide/cli/cmd-delete-token"
	"github.com/cyril-fl/tide/cli/services/apiclient"
)

var cmdDelete = &cobra.Command{
	Use:     cmd.Delete.Use,
	Short:   cmd.Delete.Short,
	Long:    cmd.Delete.Long,
	Example: cmd.Delete.Example,
}

var cmdDeleteNote = &cobra.Command{
	Use:     cmd.DeleteNote.Use,
	Short:   cmd.DeleteNote.Short,
	Long:    cmd.DeleteNote.Long,
	Example: cmd.DeleteNote.Example,
	Args:    cobra.MinimumNArgs(1),
	RunE:    apiResolved(apiclient.NOTE, cmddeletenote.Delete),
}

var cmdDeleteFloder = &cobra.Command{
	Use:     cmd.DeleteFolder.Use,
	Short:   cmd.DeleteFolder.Short,
	Long:    cmd.DeleteFolder.Long,
	Example: cmd.DeleteFolder.Example,
	Args:    cobra.MinimumNArgs(1),
	RunE:    apiResolved(apiclient.FOLDER, cmddeletefolder.Delete),
}

var cmdDeleteTag = &cobra.Command{
	Use:     cmd.DeleteTag.Use,
	Short:   cmd.DeleteTag.Short,
	Long:    cmd.DeleteTag.Long,
	Example: cmd.DeleteTag.Example,
	Args:    cobra.MinimumNArgs(1),
	RunE:    cmddeletetag.Delete,
}

var cmdDeleteToken = &cobra.Command{
	Use:     cmd.DeleteToken.Use,
	Short:   cmd.DeleteToken.Short,
	Long:    cmd.DeleteToken.Long,
	Example: cmd.DeleteToken.Example,
	Args:    cobra.ExactArgs(1),
	RunE:    cmddeletetoken.Delete,
}

func init() {
	cmdDelete.AddCommand(cmdDeleteNote, cmdDeleteFloder, cmdDeleteTag, cmdDeleteToken)
	cmdRoot.AddCommand(cmdDelete)
}
