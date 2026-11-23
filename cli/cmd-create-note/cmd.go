package cmdcreatenote

import (
	"context"
	"fmt"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	millflagscreatenote "github.com/cyril-fl/tide/cli/cmd-create-note/services/mill"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Create resolves the note from flags (and a form for what is missing), then saves it through POST /api/notes.
func Create(cobra *cobra.Command, args []string) error {
	return run(cobra.Context(), cobra.Flags())
}

// Interactive creates a note in the form, inside parentID ("" = root).
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
	resolver := millflagscreatenote.New(millflagscreatenote.Template{Ctx: ctx, Client: client, Flags: flags})
	input := resolver.Resolve()
	if resolver.HasErr() {
		return resolver.GetErr()
	}

	response, err := client.PostApiNotesWithResponse(ctx, apiclient.CreatenoteInput{Content: input.Content, ParentId: &input.ParentID, Tags: &input.Tags})
	if err != nil {
		return err
	}
	if response.JSON201 == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}
	id := response.JSON201.Id
	fmt.Printf("Successfully created note %s\n", id)
	return pinAndLock(ctx, client, id, flags)
}

// pinAndLock applies --pin and --lock through PATCH /api/notes/{id}, as edit note does, once the note exists.
func pinAndLock(ctx context.Context, client *apiclient.ClientWithResponses, id string, flags *pflag.FlagSet) error {
	pin, lock := flags.Changed("pin"), flags.Changed("lock")
	if !pin && !lock {
		return nil
	}
	var input apiclient.EditnoteBody
	if pin {
		input.Pinned = &pin
	}
	if lock {
		input.Readonly = &lock
	}
	response, err := client.PatchApiNotesIdWithResponse(ctx, id, input)
	if err == nil && response.JSON200 == nil {
		err = apiclient.Fail(response.HTTPResponse, response.Body)
	}
	if err != nil {
		return fmt.Errorf("note %s was created but not pinned or locked: %w", id, err)
	}
	if pin {
		fmt.Printf("Successfully pinned note %s\n", id)
	}
	if lock {
		fmt.Printf("Successfully locked note %s\n", id)
	}
	return nil
}

// parentFlags declares the add flags with only --parent set: the mill asks for the rest.
func parentFlags(parentID string) (*pflag.FlagSet, error) {
	cmd := &cobra.Command{}
	plumcliutilsargs.Makeflags(cmd, config.Current.Commands.CreateNote.Flags)
	if parentID == "" {
		return cmd.Flags(), nil
	}
	return cmd.Flags(), cmd.Flags().Set("parent", parentID)
}
