package cmddeletesnippet

import (
	"fmt"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/spf13/cobra"
)

// Delete removes the snippet named by the arg through PATCH /api/me, the others kept.
func Delete(cobra *cobra.Command, args []string) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	if _, err := apiclient.SaveSnippet(cobra.Context(), client, args[0], "", true); err != nil {
		return err
	}
	fmt.Printf("Successfully deleted the snippet %s\n", args[0])
	return nil
}
