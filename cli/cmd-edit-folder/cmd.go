package cmdeditfolder

import (
	"context"
	"fmt"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	millflagseditfolder "github.com/cyril-fl/tide/cli/cmd-edit-folder/services/mill"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Edit loads the folder, applies --title (or a pre-filled form), then saves it through PATCH /api/folders/{id}; --pin and
// --unpin make it a favorite or not.
func Edit(cobra *cobra.Command, args []string) error {
	return run(cobra.Context(), args[0], cobra.Flags())
}

// Interactive edits the folder in the pre-filled form, as `edit folder <id>` without flags.
func Interactive(ctx context.Context, id string) error {
	return run(ctx, id, noFlags())
}

// run renames only when --title asks for it, or with the form when no flag says anything: --pin alone keeps the title.
// The API pins and renames in one transaction.
func run(ctx context.Context, id string, flags *pflag.FlagSet) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	var input apiclient.EditfolderBody
	if flags.Changed("pin") || flags.Changed("unpin") {
		pinned := flags.Changed("pin")
		input.Pinned = &pinned
	}
	if input.Pinned == nil || flags.Changed("title") {
		resolver := millflagseditfolder.New(millflagseditfolder.Template{Ctx: ctx, Client: client, Flags: flags, ID: id})
		folder := resolver.Resolve()
		if resolver.HasErr() {
			return resolver.GetErr()
		}
		input.Title = &folder.Title
	}

	response, err := client.PatchApiFoldersIdWithResponse(ctx, id, input)
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}

	if input.Title != nil {
		fmt.Printf("Successfully updated folder %s\n", id)
	}
	if input.Pinned != nil && *input.Pinned {
		fmt.Printf("Successfully pinned folder %s\n", id)
	} else if input.Pinned != nil {
		fmt.Printf("Successfully unpinned folder %s\n", id)
	}
	return nil
}

// noFlags declares the edit flags without setting any: the mill sees no --title and opens the form.
func noFlags() *pflag.FlagSet {
	cmd := &cobra.Command{}
	plumcliutilsargs.Makeflags(cmd, config.Current.Commands.EditFolder.Flags)
	return cmd.Flags()
}
