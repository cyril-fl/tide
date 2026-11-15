package cmdmove

import (
	"fmt"

	millflagsmove "github.com/cyril-fl/tide/notes-go/app/cli/cmd-move/services/mill"
	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient"
	"github.com/spf13/cobra"
)

// Move puts notes and folders into the --parent folder, or at the root without it, through POST /api/move.
func Move(cobra *cobra.Command, args []string) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	resolver := millflagsmove.New(millflagsmove.Template{Ctx: cobra.Context(), Client: client, Flags: cobra.Flags(), IDs: args})
	input := resolver.Resolve()
	if resolver.HasErr() {
		return resolver.GetErr()
	}

	response, err := client.PostApiMoveWithResponse(cobra.Context(), apiclient.MoveitemsInput{Ids: input.IDs, ParentId: &input.ParentID})
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}
	fmt.Printf("Successfully moved %d item(s)\n", len(*response.JSON200))
	return nil
}
