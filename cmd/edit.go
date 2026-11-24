package cmd

import (
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/spf13/cobra"

	cmdeditfolder "github.com/cyril-fl/tide/cli/cmd-edit-folder"
	cmdeditnote "github.com/cyril-fl/tide/cli/cmd-edit-note"
	cmdeditsearch "github.com/cyril-fl/tide/cli/cmd-edit-search"
	cmdedittag "github.com/cyril-fl/tide/cli/cmd-edit-tag"
	"github.com/cyril-fl/tide/cli/services/apiclient"
)

var cmdEdit = &cobra.Command{
	Use:     cmd.Edit.Use,
	Short:   cmd.Edit.Short,
	Long:    cmd.Edit.Long,
	Example: cmd.Edit.Example,
}

var cmdEditNote = &cobra.Command{
	Use:     cmd.EditNote.Use,
	Short:   cmd.EditNote.Short,
	Long:    cmd.EditNote.Long,
	Example: cmd.EditNote.Example,
	Args:    cobra.ExactArgs(1),
	RunE:    apiResolved(apiclient.NOTE, cmdeditnote.Edit),
}

var cmdEditFloder = &cobra.Command{
	Use:     cmd.EditFolder.Use,
	Short:   cmd.EditFolder.Short,
	Long:    cmd.EditFolder.Long,
	Example: cmd.EditFolder.Example,
	Args:    cobra.ExactArgs(1),
	RunE:    apiResolved(apiclient.FOLDER, cmdeditfolder.Edit),
}

var cmdEditTag = &cobra.Command{
	Use:     cmd.EditTag.Use,
	Short:   cmd.EditTag.Short,
	Long:    cmd.EditTag.Long,
	Example: cmd.EditTag.Example,
	Args:    cobra.ExactArgs(2),
	RunE:    cmdedittag.Edit,
}

var cmdEditSearch = &cobra.Command{
	Use:     cmd.EditSearch.Use,
	Short:   cmd.EditSearch.Short,
	Long:    cmd.EditSearch.Long,
	Example: cmd.EditSearch.Example,
	Args:    cobra.ExactArgs(1),
	RunE:    cmdeditsearch.Edit,
}

func init() {
	plumcliutilsargs.Makeflags(cmdEditNote, cmd.EditNote.Flags)
	cmdEditNote.MarkFlagsMutuallyExclusive("pin", "unpin")
	cmdEditNote.MarkFlagsMutuallyExclusive("lock", "unlock")
	cmdEditNote.MarkFlagsMutuallyExclusive("title", "replace", "section")
	plumcliutilsargs.Makeflags(cmdEditFloder, cmd.EditFolder.Flags)
	plumcliutilsargs.Makeflags(cmdEditTag, cmd.EditTag.Flags)

	plumcliutilsargs.Makeflags(cmdEditSearch, cmd.EditSearch.Flags)

	cmdEdit.AddCommand(cmdEditNote, cmdEditFloder, cmdEditTag, cmdEditSearch)
	cmdRoot.AddCommand(cmdEdit)
}
