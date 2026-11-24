package cmd

import (
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/spf13/cobra"

	cmdcreatefile "github.com/cyril-fl/tide/cli/cmd-create-file"
	cmdcreatefolder "github.com/cyril-fl/tide/cli/cmd-create-folder"
	cmdcreatenote "github.com/cyril-fl/tide/cli/cmd-create-note"
	cmdcreatesearch "github.com/cyril-fl/tide/cli/cmd-create-search"
	cmdcreatesnippet "github.com/cyril-fl/tide/cli/cmd-create-snippet"
	cmdcreatetag "github.com/cyril-fl/tide/cli/cmd-create-tag"
	cmdcreatetoken "github.com/cyril-fl/tide/cli/cmd-create-token"
	"github.com/cyril-fl/tide/cli/services/apiclient"
)

var cmdCreate = &cobra.Command{
	Use:     cmd.Create.Use,
	Short:   cmd.Create.Short,
	Long:    cmd.Create.Long,
	Example: cmd.Create.Example,
}

var cmdCreateNote = &cobra.Command{
	Use:     cmd.CreateNote.Use,
	Short:   cmd.CreateNote.Short,
	Long:    cmd.CreateNote.Long,
	Example: cmd.CreateNote.Example,
	Args:    cobra.NoArgs,
	RunE:    apiResolved(apiclient.NOTE, cmdcreatenote.Create),
}

var cmdCreateFloder = &cobra.Command{
	Use:     cmd.CreateFolder.Use,
	Short:   cmd.CreateFolder.Short,
	Long:    cmd.CreateFolder.Long,
	Example: cmd.CreateFolder.Example,
	Args:    cobra.NoArgs,
	RunE:    apiResolved(apiclient.FOLDER, cmdcreatefolder.Create),
}

var cmdCreateTag = &cobra.Command{
	Use:     cmd.CreateTag.Use,
	Short:   cmd.CreateTag.Short,
	Long:    cmd.CreateTag.Long,
	Example: cmd.CreateTag.Example,
	Args:    cobra.MinimumNArgs(1),
	RunE:    cmdcreatetag.Create,
}

var cmdCreateToken = &cobra.Command{
	Use:     cmd.CreateToken.Use,
	Short:   cmd.CreateToken.Short,
	Long:    cmd.CreateToken.Long,
	Example: cmd.CreateToken.Example,
	Args:    cobra.ExactArgs(1),
	RunE:    cmdcreatetoken.Create,
}

var cmdCreateFile = &cobra.Command{
	Use:     cmd.CreateFile.Use,
	Short:   cmd.CreateFile.Short,
	Long:    cmd.CreateFile.Long,
	Example: cmd.CreateFile.Example,
	Args:    cobra.MinimumNArgs(1),
	RunE:    cmdcreatefile.Create,
}

var cmdCreateSnippet = &cobra.Command{
	Use:     cmd.CreateSnippet.Use,
	Short:   cmd.CreateSnippet.Short,
	Long:    cmd.CreateSnippet.Long,
	Example: cmd.CreateSnippet.Example,
	Args:    cobra.ExactArgs(1),
	RunE:    withStdin(cmdcreatesnippet.Create),
}

var cmdCreateSearch = &cobra.Command{
	Use:     cmd.CreateSearch.Use,
	Short:   cmd.CreateSearch.Short,
	Long:    cmd.CreateSearch.Long,
	Example: cmd.CreateSearch.Example,
	Args:    cobra.ExactArgs(1),
	RunE:    cmdcreatesearch.Create,
}

func init() {
	plumcliutilsargs.Makeflags(cmdCreateToken, cmd.CreateToken.Flags)

	plumcliutilsargs.Makeflags(cmdCreateNote, cmd.CreateNote.Flags)
	cmdCreateNote.MarkFlagsMutuallyExclusive("tag", "tag-from-list")
	cmdCreateNote.MarkFlagsMutuallyExclusive("parent", "parent-from-list")

	plumcliutilsargs.Makeflags(cmdCreateFloder, cmd.CreateFolder.Flags)
	cmdCreateFloder.MarkFlagsMutuallyExclusive("parent", "parent-from-list")

	plumcliutilsargs.Makeflags(cmdCreateFile, cmd.CreateFile.Flags)
	plumcliutilsargs.Makeflags(cmdCreateSnippet, cmd.CreateSnippet.Flags)
	cobra.CheckErr(cmdCreateSnippet.MarkFlagRequired("content"))
	plumcliutilsargs.Makeflags(cmdCreateSearch, cmd.CreateSearch.Flags)

	cmdCreate.AddCommand(cmdCreateNote, cmdCreateFloder, cmdCreateTag, cmdCreateFile, cmdCreateSnippet, cmdCreateSearch, cmdCreateToken)
	cmdRoot.AddCommand(cmdCreate)
}
