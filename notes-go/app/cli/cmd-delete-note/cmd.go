package cmddeletenote

import (
	"fmt"

	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/spf13/cobra"
)

// Delete moves notes to the trash, by uuid, through DELETE /api/notes/{id}; an unknown one is skipped.
// TODO: one request per note, so a failure halfway leaves the first ones in the trash, add a batch route if it matters.
func Delete(cobra *cobra.Command, args []string) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	count := 0
	for _, id := range args {
		response, err := client.DeleteApiNotesIdWithResponse(cobra.Context(), id)
		if err != nil {
			return err
		}
		deleted, err := apiclient.Deleted(response.HTTPResponse, response.Body)
		if err != nil {
			return err
		}
		if deleted {
			count++
		}
	}

	fmt.Printf("Successfully deleted %d note(s)\n", count)
	return nil
}
