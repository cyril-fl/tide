package cmdreadsearch

import (
	"fmt"
	"strings"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	plumdebug "github.com/cyril-fl/plum/utils/debug"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/core/services/style"
	"github.com/spf13/cobra"
)

// run is a saved search run: the search, a page of its notes and their total.
type run struct {
	Search apiclient.SavedSearch           `json:"search"`
	Notes  []apiclient.RunsavedsearchMatch `json:"notes"`
	Total  int                             `json:"total"`
}

// Read lists the saved searches with their criteria, or runs the one of id or name the arg.
func Read(cobra *cobra.Command, args []string) error {
	var asJSON bool
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"json": &asJSON}); err != nil {
		return err
	}
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		searches, err := apiclient.SavedSearches(cobra.Context(), client)
		switch {
		case err != nil:
			return err
		case asJSON:
			plumdebug.Json(searches)
		case len(searches) == 0:
			fmt.Println(style.Empty().Render("No saved search"))
		default:
			cells := [][]string{{"Name", "Criteria", "Id"}}
			for _, search := range searches {
				cells = append(cells, []string{search.Name, criteria(search), search.ID})
			}
			fmt.Print(style.Table(cells, -1, 0))
		}
		return nil
	}

	limit, err := cobra.Flags().GetInt("limit")
	if err != nil {
		return err
	}
	offset, err := cobra.Flags().GetInt("offset")
	if err != nil {
		return err
	}
	search, notes, total, err := apiclient.RunSearch(cobra.Context(), client, args[0], max(limit, 0), max(offset, 0))
	switch {
	case err != nil:
		return err
	case asJSON:
		plumdebug.Json(run{Search: search, Notes: append([]apiclient.RunsavedsearchMatch{}, notes...), Total: total})
	case len(notes) == 0:
		fmt.Println(style.Empty().Render("No note matches"))
	default:
		cells := [][]string{{"Title", "Path", "Id"}}
		for _, note := range notes {
			cells = append(cells, []string{note.Title, note.Path, note.Id})
		}
		fmt.Print(style.Table(cells, -1, 0))
		fmt.Println(style.Subtle().Render(fmt.Sprintf("%d of %d notes", len(notes), total)))
	}
	return nil
}

// criteria sums a saved search up on one line.
func criteria(search apiclient.SavedSearch) string {
	var parts []string
	add := func(label string, values ...string) {
		if joined := strings.Join(values, ", "); joined != "" {
			parts = append(parts, label+" "+joined)
		}
	}
	add("all of", search.TagsAll...)
	add("any of", search.TagsAny...)
	add("none of", search.TagsNone...)
	if search.FolderMode == "exclude" {
		add("not in", search.Folders...)
	} else {
		add("in", search.Folders...)
	}
	add("created from", search.CreatedFrom)
	add("created to", search.CreatedTo)
	add("edited from", search.EditedFrom)
	add("edited to", search.EditedTo)
	add("text", search.Text)
	add("not", search.NotText)
	return strings.Join(parts, " · ")
}
