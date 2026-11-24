package cmd

import (
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/spf13/cobra"

	cmdreadfile "github.com/cyril-fl/tide/cli/cmd-read-file"
	cmdreadfolder "github.com/cyril-fl/tide/cli/cmd-read-folder"
	cmdreadlinks "github.com/cyril-fl/tide/cli/cmd-read-links"
	cmdreadnote "github.com/cyril-fl/tide/cli/cmd-read-note"
	cmdreadsearch "github.com/cyril-fl/tide/cli/cmd-read-search"
	cmdreadsnippet "github.com/cyril-fl/tide/cli/cmd-read-snippet"
	cmdreadtag "github.com/cyril-fl/tide/cli/cmd-read-tag"
	cmdreadtoken "github.com/cyril-fl/tide/cli/cmd-read-token"
)

var cmdRead = &cobra.Command{
	Use:     cmd.Read.Use,
	Short:   cmd.Read.Short,
	Long:    cmd.Read.Long,
	Example: cmd.Read.Example,
}

var cmdReadNote = &cobra.Command{
	Use:     cmd.ReadNote.Use,
	Short:   cmd.ReadNote.Short,
	Long:    cmd.ReadNote.Long,
	Example: cmd.ReadNote.Example,
	Args:    cobra.MinimumNArgs(1),
	RunE:    cmdreadnote.Read,
}

var cmdReadFloder = &cobra.Command{
	Use:     cmd.ReadFolder.Use,
	Short:   cmd.ReadFolder.Short,
	Long:    cmd.ReadFolder.Long,
	Example: cmd.ReadFolder.Example,
	Args:    cobra.MaximumNArgs(1),
	RunE:    cmdreadfolder.Read,
}

var cmdReadTag = &cobra.Command{
	Use:     cmd.ReadTag.Use,
	Short:   cmd.ReadTag.Short,
	Long:    cmd.ReadTag.Long,
	Example: cmd.ReadTag.Example,
	Args:    cobra.MaximumNArgs(1),
	RunE:    cmdreadtag.Read,
}

var cmdReadLinks = &cobra.Command{
	Use:     cmd.ReadLinks.Use,
	Short:   cmd.ReadLinks.Short,
	Long:    cmd.ReadLinks.Long,
	Example: cmd.ReadLinks.Example,
	Args:    cobra.MaximumNArgs(1),
	RunE:    cmdreadlinks.Read,
}

var cmdReadToken = &cobra.Command{
	Use:     cmd.ReadToken.Use,
	Short:   cmd.ReadToken.Short,
	Long:    cmd.ReadToken.Long,
	Example: cmd.ReadToken.Example,
	Args:    cobra.NoArgs,
	RunE:    cmdreadtoken.Read,
}

var cmdReadFile = &cobra.Command{
	Use:     cmd.ReadFile.Use,
	Short:   cmd.ReadFile.Short,
	Long:    cmd.ReadFile.Long,
	Example: cmd.ReadFile.Example,
	Args:    cobra.MaximumNArgs(1),
	RunE:    cmdreadfile.Read,
}

var cmdReadSnippet = &cobra.Command{
	Use:     cmd.ReadSnippet.Use,
	Short:   cmd.ReadSnippet.Short,
	Long:    cmd.ReadSnippet.Long,
	Example: cmd.ReadSnippet.Example,
	Args:    cobra.MaximumNArgs(1),
	RunE:    cmdreadsnippet.Read,
}

var cmdReadSearch = &cobra.Command{
	Use:     cmd.ReadSearch.Use,
	Short:   cmd.ReadSearch.Short,
	Long:    cmd.ReadSearch.Long,
	Example: cmd.ReadSearch.Example,
	Args:    cobra.MaximumNArgs(1),
	RunE:    cmdreadsearch.Read,
}

func init() {
	plumcliutilsargs.Makeflags(cmdReadToken, cmd.ReadToken.Flags)
	cmdReadToken.MarkFlagsMutuallyExclusive("yaml", "json")
	plumcliutilsargs.Makeflags(cmdReadTag, cmd.ReadTag.Flags)
	cmdReadTag.MarkFlagsMutuallyExclusive("yaml", "json")
	cmdReadTag.MarkFlagsMutuallyExclusive("list", "similar")
	plumcliutilsargs.Makeflags(cmdReadFloder, cmd.ReadFolder.Flags)
	plumcliutilsargs.Makeflags(cmdReadLinks, cmd.ReadLinks.Flags)
	plumcliutilsargs.Makeflags(cmdReadNote, cmd.ReadNote.Flags)
	cmdReadNote.MarkFlagsMutuallyExclusive("history", "version")

	plumcliutilsargs.Makeflags(cmdReadFile, cmd.ReadFile.Flags)
	cmdReadFile.MarkFlagsMutuallyExclusive("output", "json")
	plumcliutilsargs.Makeflags(cmdReadSnippet, cmd.ReadSnippet.Flags)
	plumcliutilsargs.Makeflags(cmdReadSearch, cmd.ReadSearch.Flags)

	cmdRead.AddCommand(cmdReadNote, cmdReadFloder, cmdReadTag, cmdReadLinks, cmdReadFile, cmdReadSnippet, cmdReadSearch, cmdReadToken)
	cmdRoot.AddCommand(cmdRead)
}
