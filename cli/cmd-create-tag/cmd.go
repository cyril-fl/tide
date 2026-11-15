package cmdcreatetag

import (
	"fmt"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/spf13/cobra"
)

// Create creates the tags through POST /api/tags, which skips the existing ones.
func Create(cobra *cobra.Command, args []string) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	response, err := client.PostApiTagsWithResponse(cobra.Context(), apiclient.CreatetagBody{Values: args})
	if err != nil {
		return err
	}
	if response.JSON201 == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}
	fmt.Printf("Successfully created %d tag(s)\n", len(*response.JSON201))
	return nil
}
