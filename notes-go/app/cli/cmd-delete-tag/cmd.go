package cmddeletetag

import (
	"fmt"

	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/spf13/cobra"
)

// Delete deletes tags, not the notes carrying them, through DELETE /api/tags/{value}; an unknown one is skipped.
// TODO: one request per tag, so a failure halfway leaves the first ones deleted, add a batch route if it matters.
func Delete(cobra *cobra.Command, args []string) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	count := 0
	for _, value := range args {
		response, err := client.DeleteApiTagsValueWithResponse(cobra.Context(), value)
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

	fmt.Printf("Successfully deleted %d tag(s)\n", count)
	return nil
}
