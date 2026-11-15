package cmdreadtag

import (
	"context"
	"errors"
	"fmt"

	"charm.land/huh/v2"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	plumdebug "github.com/cyril-fl/plum/utils/debug"
	cmdedittag "github.com/cyril-fl/tide/cli/cmd-edit-tag"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/core/services/screen"
	"github.com/cyril-fl/tide/core/services/style"
	"github.com/spf13/cobra"
)

// Read shows one tag and its notes, or lists tags with --list, one value per line, or as YAML/JSON with --yaml/--json.
func Read(cobra *cobra.Command, args []string) error {
	var asList, asYAML, asJSON bool
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"list": &asList, "yaml": &asYAML, "json": &asJSON}); err != nil {
		return err
	}

	switch {
	case asList:
		return listTags(cobra, asYAML, asJSON)
	case len(args) == 1:
		return showTag(cobra, args[0])
	default:
		return cobra.Help()
	}
}

// showTag shows the tag, then the path and uuid of each note carrying it; ctrl+e renames it.
func showTag(cobra *cobra.Command, value string) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	for {
		output, err := loadTag(cobra.Context(), client, value)
		if err != nil {
			return err
		}

		edit, err := screen.Show(output)
		if err != nil || !edit {
			return err
		}
		// An aborted form goes back to the view unchanged; a rename reloads under the new value.
		if value, err = cmdedittag.Interactive(cobra.Context(), value); err != nil && !errors.Is(err, huh.ErrUserAborted) {
			return err
		}
	}
}

func loadTag(ctx context.Context, client *apiclient.ClientWithResponses, value string) (string, error) {
	response, err := client.GetApiTagsValueWithResponse(ctx, value, nil)
	if err != nil {
		return "", err
	}
	if response.JSON200 == nil {
		return "", apiclient.Fail(response.HTTPResponse, response.Body)
	}
	result := response.JSON200

	header := style.Title().Render(result.Tag.Value) + "\n\n"
	if len(result.Notes) == 0 {
		return header + style.Empty().Render("No note carries this tag"), nil
	}

	cells := make([][]string, len(result.Notes))
	for i, note := range result.Notes {
		cells[i] = []string{note.Path, note.Id}
	}
	return header + style.Table(cells, -1, 0), nil
}

func listTags(cobra *cobra.Command, asYAML bool, asJSON bool) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	response, err := client.GetApiTagsWithResponse(cobra.Context())
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}
	printTags(*response.JSON200, asYAML, asJSON)
	return nil
}

// printTags keeps the plain output to values only: ids are for scripts, through --yaml/--json.
func printTags(tags []apiclient.DomainTag, asYAML bool, asJSON bool) {
	switch {
	case asJSON:
		plumdebug.Json(tags)
	case asYAML:
		plumdebug.Yaml(tags)
	default:
		for _, tag := range tags {
			fmt.Println(tag.Value)
		}
	}
}
