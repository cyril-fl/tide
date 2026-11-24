package cmdcreatesnippet

import (
	"fmt"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/spf13/cobra"
)

// Create saves the snippet named by the arg with --content, replacing one of that name, through PATCH /api/me.
func Create(cobra *cobra.Command, args []string) error {
	content, err := cobra.Flags().GetString("content")
	if err != nil {
		return err
	}
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	if _, err := apiclient.SaveSnippet(cobra.Context(), client, args[0], content, false); err != nil {
		return err
	}
	fmt.Printf("Successfully saved the snippet %s\n", args[0])
	return nil
}
