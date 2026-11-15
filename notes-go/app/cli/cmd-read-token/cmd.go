package cmdreadtoken

import (
	"fmt"
	"time"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	plumdebug "github.com/cyril-fl/plum/utils/debug"
	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/style"
	"github.com/spf13/cobra"
)

// Read lists the API tokens of the account through GET /api/me/tokens, as a table or as YAML/JSON.
func Read(cobra *cobra.Command, args []string) error {
	var asYAML, asJSON bool
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"yaml": &asYAML, "json": &asJSON}); err != nil {
		return err
	}
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	response, err := client.GetApiMeTokensWithResponse(cobra.Context())
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}
	tokens := *response.JSON200

	switch {
	case asJSON:
		plumdebug.Json(tokens)
	case asYAML:
		plumdebug.Yaml(tokens)
	case len(tokens) == 0:
		fmt.Println(style.Empty().Render("No API token"))
	default:
		cells := [][]string{{"Name", "UUID", "Kind", "Access", "Created", "Last used", "Expires"}}
		for _, token := range tokens {
			kind, access := "API", "full"
			if token.Oauth {
				kind = "OAuth"
			}
			if token.ReadOnly {
				access = "read-only"
			}
			cells = append(cells, []string{token.Name, token.Id, kind, access, day(&token.CreatedAt, ""), day(token.LastUsedAt, "never"), day(token.ExpiresAt, "never")})
		}
		fmt.Print(style.Table(cells, -1, 0))
	}
	return nil
}

// Helpers

// day is the local date of an RFC 3339 time, none when absent.
func day(at *string, none string) string {
	if at == nil {
		return none
	}
	parsed, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		return *at
	}
	return parsed.Local().Format(time.DateOnly)
}
