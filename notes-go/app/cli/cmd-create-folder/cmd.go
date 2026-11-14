package cmdcreatefolder

import (
	"context"
	"fmt"

	millflagscreatefolder "github.com/cyril-fl/notes/notes-go/app/cli/cmd-create-folder/services/mill"
	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/cyril-fl/notes/notes-go/config"
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Create resolves the folder from flags (and a form for what is missing), then saves it through POST /api/folders.
func Create(cobra *cobra.Command, args []string) error {
	return run(cobra.Context(), cobra.Flags())
}

// Interactive creates a folder in the form, inside parentID ("" = root).
func Interactive(ctx context.Context, parentID string) error {
	flags, err := parentFlags(parentID)
	if err != nil {
		return err
	}
	return run(ctx, flags)
}

func run(ctx context.Context, flags *pflag.FlagSet) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	resolver := millflagscreatefolder.New(millflagscreatefolder.Template{Ctx: ctx, Client: client, Flags: flags})
	input := resolver.Resolve()
	if resolver.HasErr() {
		return resolver.GetErr()
	}

	response, err := client.PostApiFoldersWithResponse(ctx, apiclient.CreatefolderInput{Title: input.Title, ParentId: &input.ParentID})
	if err != nil {
		return err
	}
	if response.JSON201 == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}
	fmt.Printf("Successfully created folder %s\n", response.JSON201.Id)
	return nil
}

// parentFlags declares the add flags with only --parent set: the mill asks for the rest.
func parentFlags(parentID string) (*pflag.FlagSet, error) {
	cmd := &cobra.Command{}
	plumcliutilsargs.Makeflags(cmd, config.Current.Commands.CreateFolder.Flags)
	if parentID == "" {
		return cmd.Flags(), nil
	}
	return cmd.Flags(), cmd.Flags().Set("parent", parentID)
}
