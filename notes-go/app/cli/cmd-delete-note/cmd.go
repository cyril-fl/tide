package cmddeletenote

import (
	"fmt"

	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/spf13/cobra"
)

// Delete moves notes to the trash, by uuid, in one DELETE /api/notes; an unknown one is skipped.
func Delete(cobra *cobra.Command, args []string) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	response, err := client.DeleteApiNotesWithResponse(cobra.Context(), apiclient.DeleteitemsBody{Ids: args})
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}

	fmt.Printf("Successfully deleted %d note(s)\n", len(*response.JSON200))
	return nil
}
