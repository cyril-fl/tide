package cmddeletefolder

import (
	"fmt"

	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/spf13/cobra"
)

// Delete moves folders and their whole content to the trash, by uuid, through DELETE /api/folders/{id}; an unknown one
// is skipped.
// TODO: one request per folder, so a failure halfway leaves the first ones in the trash, add a batch route if it
// matters.
func Delete(cobra *cobra.Command, args []string) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	count := 0
	for _, id := range args {
		response, err := client.DeleteApiFoldersIdWithResponse(cobra.Context(), id)
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

	// The API does not count the content: the folders only.
	fmt.Printf("Successfully deleted %d folder(s) and their content\n", count)
	return nil
}
