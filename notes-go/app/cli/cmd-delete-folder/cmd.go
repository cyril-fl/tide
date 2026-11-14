package cmddeletefolder

import (
	"fmt"

	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/spf13/cobra"
)

// Delete moves folders and their whole content to the trash, by uuid, in one DELETE /api/folders; an unknown
// one is skipped.
func Delete(cobra *cobra.Command, args []string) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	response, err := client.DeleteApiFoldersWithResponse(cobra.Context(), apiclient.DeleteitemsBody{Ids: args})
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}

	fmt.Printf("Successfully deleted %d item(s): folders and their content\n", len(*response.JSON200))
	return nil
}
