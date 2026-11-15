package cmdcreatetoken

import (
	"fmt"
	"os"
	"time"

	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient"
	"github.com/spf13/cobra"
)

// Create makes an API token through POST /api/me/tokens and prints it alone on stdout, for TOKEN=$(tide add token x):
// the server never shows it again.
func Create(cobra *cobra.Command, args []string) error {
	expires, err := cobra.Flags().GetString("expires")
	if err != nil {
		return err
	}
	readOnly, err := cobra.Flags().GetBool("read-only")
	if err != nil {
		return err
	}
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	lifetime := apiclient.CreateapitokenInputExpiresIn(expires)
	response, err := client.PostApiMeTokensWithResponse(cobra.Context(), apiclient.CreateapitokenInput{Name: args[0], ExpiresIn: &lifetime, ReadOnly: &readOnly})
	if err != nil {
		return err
	}
	if response.JSON201 == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}

	until := "never expires"
	if response.JSON201.ApiToken.ExpiresAt != nil {
		if at, err := time.Parse(time.RFC3339, *response.JSON201.ApiToken.ExpiresAt); err == nil {
			until = "expires on " + at.Local().Format(time.DateOnly)
		}
	}
	fmt.Fprintf(os.Stderr, "Token %q %s. Copy it now, it is not shown again:\n", response.JSON201.ApiToken.Name, until)
	fmt.Println(response.JSON201.Token)
	return nil
}
