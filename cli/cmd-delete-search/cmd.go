package cmddeletesearch

import (
	"fmt"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/spf13/cobra"
)

// Delete removes the saved search of id or name the arg through PATCH /api/me, its notes kept.
func Delete(cobra *cobra.Command, args []string) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	deleted, err := apiclient.SaveSearch(cobra.Context(), client, args[0], nil)
	if err != nil {
		return err
	}
	fmt.Printf("Successfully deleted the search %s\n", deleted.Name)
	return nil
}
