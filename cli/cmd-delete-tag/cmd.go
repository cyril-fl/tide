package cmddeletetag

import (
	"fmt"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/spf13/cobra"
)

// Delete deletes tags, not the notes carrying them, in one DELETE /api/tags; an unknown one is skipped.
func Delete(cobra *cobra.Command, args []string) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	response, err := client.DeleteApiTagsWithResponse(cobra.Context(), apiclient.DeletetagBody{Values: args})
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}

	fmt.Printf("Successfully deleted %d tag(s)\n", len(*response.JSON200))
	return nil
}
