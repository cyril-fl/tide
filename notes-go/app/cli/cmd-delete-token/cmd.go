package cmddeletetoken

import (
	"fmt"
	"net/http"

	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient"
	"github.com/spf13/cobra"
)

// Delete revokes an API token through DELETE /api/me/tokens/{id}.
func Delete(cobra *cobra.Command, args []string) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	response, err := client.DeleteApiMeTokensIdWithResponse(cobra.Context(), args[0])
	if err != nil {
		return err
	}
	if response.StatusCode() != http.StatusNoContent {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}
	fmt.Println("Successfully revoked the token")
	return nil
}
