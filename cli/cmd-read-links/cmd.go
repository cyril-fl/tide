package cmdreadlinks

import (
	"context"
	"fmt"
	"strings"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	plumdebug "github.com/cyril-fl/plum/utils/debug"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/core/services/style"
	"github.com/spf13/cobra"
)

// Read lists the broken links of every note through GET /api/links/broken, as a table or as JSON; the API
// refuses it to a plan without brokenLinks. With a note, it lists that note's links instead.
func Read(cobra *cobra.Command, args []string) error {
	var asJSON bool
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"json": &asJSON}); err != nil {
		return err
	}
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	if len(args) == 1 {
		return printNote(cobra.Context(), client, args[0], asJSON)
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

// noteLinks is what joins a note to the others, the note itself left out: the same as the MCP's note_links.
type noteLinks struct {
	Links     []apiclient.ReadneighborsLinked   `json:"links"`
	Backlinks []apiclient.ReadneighborsLinked   `json:"backlinks"`
	Neighbors []apiclient.ReadneighborsNeighbor `json:"neighbors"`
}

// printNote lists the links of the note of ref, its backlinks and the notes sharing its tags, most shared first,
// through GET /api/notes/{id}/neighbors.
func printNote(ctx context.Context, client *apiclient.ClientWithResponses, ref string, asJSON bool) error {
	id, err := apiclient.Resolve(ctx, client, apiclient.NOTE, ref)
	if err != nil {
		return err
	}
	result, err := apiclient.Decode[apiclient.ReadneighborsResult](client.GetApiNotesIdNeighbors(ctx, id))
	if err != nil {
		return err
	}
	out := noteLinks{
		Links:     append([]apiclient.ReadneighborsLinked{}, result.Links...),
		Backlinks: append([]apiclient.ReadneighborsLinked{}, result.Backlinks...),
		Neighbors: append([]apiclient.ReadneighborsNeighbor{}, result.Neighbors...),
	}
	if asJSON {
		plumdebug.Json(out)
		return nil
	}
	section := func(title string, rows [][]string) {
		fmt.Println(style.Title().Render(title))
		if len(rows) == 0 {
			fmt.Println(style.Empty().Render("None"))
		} else {
			fmt.Print(style.Table(rows, -1, 0))
		}
		fmt.Println()
	}
	linked := func(list []apiclient.ReadneighborsLinked) [][]string {
		rows := make([][]string, len(list))
		for i, l := range list {
			rows[i] = []string{l.Path, l.Id}
		}
		return rows
	}
	section("Links", linked(out.Links))
	section("Backlinks", linked(out.Backlinks))
	rows := make([][]string, len(out.Neighbors))
	for i, n := range out.Neighbors {
		rows[i] = []string{n.Path, "#" + strings.Join(n.Tags, " #"), n.Id}
	}
	section("Sharing its tags", rows)
	return nil
}
