package cmdreadlinks

import (
	"fmt"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	plumdebug "github.com/cyril-fl/plum/utils/debug"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/core/services/style"
	"github.com/spf13/cobra"
)

// Read lists the broken links of every note through GET /api/links/broken, as a table or as JSON; the API
// refuses it to a plan without brokenLinks.
func Read(cobra *cobra.Command, args []string) error {
	var asJSON bool
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"json": &asJSON}); err != nil {
		return err
	}
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	response, err := client.GetApiLinksBrokenWithResponse(cobra.Context())
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}
	links := *response.JSON200

	switch {
	case asJSON:
		if links == nil {
			links = []apiclient.ListbrokenlinksLink{}
		}
		plumdebug.Json(links)
	case len(links) == 0:
		fmt.Println(style.Empty().Render("No broken link"))
	default:
		cells := [][]string{{"Note", "Link", "Kind"}}
		for _, link := range links {
			cells = append(cells, []string{link.NotePath, link.Target, string(link.Kind)})
		}
		fmt.Print(style.Table(cells, -1, 0))
	}
	return nil
}
