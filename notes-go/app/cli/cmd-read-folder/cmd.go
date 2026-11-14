package cmdreadfolder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"charm.land/huh/v2"
	cmdeditfolder "github.com/cyril-fl/notes/notes-go/app/cli/cmd-edit-folder"
	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readitem"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/screen"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/style"
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	plumdebug "github.com/cyril-fl/plum/utils/debug"
	"github.com/spf13/cobra"
)

// Read shows the folder title and path, then one line per direct child with its uuid; ctrl+e edits it; --json prints it for scripts.
func Read(cobra *cobra.Command, args []string) error {
	var asJSON bool
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"json": &asJSON}); err != nil {
		return err
	}
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	id, err := apiclient.Resolve(cobra.Context(), client, apiclient.FOLDER, args[0])
	if err != nil {
		return err
	}
	if asJSON {
		return printJSON(cobra.Context(), client, id)
	}

	for {
		folder, err := fetch(cobra.Context(), client, id)
		if err != nil {
			return err
		}

		edit, err := screen.Show(Output(folder, folder.Children, -1, EmptyFolder))
		if err != nil || !edit {
			return err
		}
		// An aborted form goes back to the view unchanged.
		// TODO: ctrl+e still edits in the base, move it to PATCH /api/folders/{id} with edit folder (#148).
		if err := cmdeditfolder.Interactive(cobra.Context(), id); err != nil && !errors.Is(err, huh.ErrUserAborted) {
			return err
		}
	}
}

// printJSON gives an empty folder "children": [], not null.
func printJSON(ctx context.Context, client *apiclient.ClientWithResponses, id string) error {
	folder, err := fetch(ctx, client, id)
	if err != nil {
		return err
	}

	folder.Children = append([]domain.Item{}, folder.Children...)
	plumdebug.Json(folder)
	return nil
}

// fetch reads the folder through GET /api/folders/{id}, which leaves its children out, then them through
// GET /api/items. Decoded into the domain types, not the generated ones: the output stays that of readitem.
func fetch(ctx context.Context, client *apiclient.ClientWithResponses, id string) (readitem.Result, error) {
	var folder readitem.Result
	response, err := client.GetApiFoldersIdWithResponse(ctx, id)
	if err != nil {
		return folder, err
	}
	if response.JSON200 == nil {
		return folder, apiclient.Fail(response.HTTPResponse, response.Body)
	}
	if err := json.Unmarshal(response.Body, &folder); err != nil {
		return folder, err
	}

	children, err := client.GetApiItemsWithResponse(ctx, &apiclient.GetApiItemsParams{Parent: &id})
	if err != nil {
		return folder, err
	}
	if children.JSON200 == nil {
		return folder, apiclient.Fail(children.HTTPResponse, children.Body)
	}
	return folder, json.Unmarshal(children.Body, &folder.Children)
}

// EmptyFolder is the message of a folder without children.
const EmptyFolder = "This folder is empty"

// Output renders a folder like `show folder`: the header from the whole folder, then rows (the finder
// passes its filtered, scrolled window); cursor >= 0 marks that row as selected, empty shows when no row.
func Output(folder readitem.Result, rows []domain.Item, cursor int, empty string) string {
	header := style.Header(string(folder.Item.Title), folder.Path, info(folder)) + "\n"
	if len(rows) == 0 {
		return header + style.Empty().Render(empty)
	}

	cells := make([][]string, len(rows))
	for i, child := range rows {
		title := child.Title
		if title == "" {
			title = "untitled"
		}
		cells[i] = []string{string(child.Kind), string(title), child.ID}
	}
	return header + style.Table(cells, cursor, 1)
}

// info counts the children, and dates the folder when it is a real one (the root has no row).
func info(folder readitem.Result) string {
	folders := 0
	for _, child := range folder.Children {
		if child.Kind == domain.FOLDER {
			folders++
		}
	}
	parts := []string{count(folders, "folder"), count(len(folder.Children)-folders, "note")}

	updated := folder.Item.UpdatedAt
	if updated.IsZero() {
		updated = folder.Item.CreatedAt
	}
	if !updated.IsZero() {
		parts = append(parts, "updated "+updated.Format("2 Jan 2006"))
	}
	return strings.Join(parts, " · ")
}

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
