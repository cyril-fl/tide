package cmdrestore

import (
	"fmt"

	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient"
	"github.com/spf13/cobra"
)

// Restore takes items out of the trash, by uuid, with everything removed at the same time, through POST /api/restore.
func Restore(cobra *cobra.Command, args []string) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	response, err := client.PostApiRestoreWithResponse(cobra.Context(), apiclient.RestoreBody{Ids: args})
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}

	fmt.Printf("Successfully restored %d item(s)\n", len(*response.JSON200))
	return nil
}
