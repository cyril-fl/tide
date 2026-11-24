package cmdreadfolder

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/huh/v2"
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	plumdebug "github.com/cyril-fl/plum/utils/debug"
	cmdeditfolder "github.com/cyril-fl/tide/cli/cmd-edit-folder"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/config"
	"github.com/cyril-fl/tide/core/services/screen"
	"github.com/cyril-fl/tide/core/services/style"
	"github.com/spf13/cobra"
)

// Read shows the folder title and path (the root without an arg), then one line per direct child with its uuid; ctrl+e
// edits it. --json, a page (--limit, --offset) or what notes add (--tags, --length, --preview) print it instead, for
// scripts.
func Read(cobra *cobra.Command, args []string) error {
	var page pageArgs
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"json": &page.asJSON, "tags": &page.tags, "length": &page.length}); err != nil {
		return err
	}
	var errs [3]error
	page.limit, errs[0] = cobra.Flags().GetInt("limit")
	page.offset, errs[1] = cobra.Flags().GetInt("offset")
	page.preview, errs[2] = cobra.Flags().GetInt("preview")
	if err := errors.Join(errs[:]...); err != nil {
		return err
	}
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	id := ""
	if len(args) == 1 {
		if id, err = apiclient.Resolve(cobra.Context(), client, apiclient.FOLDER, args[0]); err != nil {
			return err
		}
	}
	if page.asJSON || page.tags || page.length || page.preview > 0 || page.limit > 0 || page.offset > 0 {
		return printPage(cobra.Context(), client, id, page)
	}

	for {
		folder, err := Fetch(cobra.Context(), client, id)
		if err != nil {
			return err
		}

		edit, err := screen.Show(Output(folder, apiclient.Deref(folder.Children), -1, EmptyFolder))
		if err != nil || !edit || id == "" { // the root has no title to edit
			return err
		}
		// An aborted form goes back to the view unchanged.
		if err := cmdeditfolder.Interactive(cobra.Context(), id); err != nil && !errors.Is(err, huh.ErrUserAborted) {
			return err
		}
	}
}

// pageArgs is what show folder prints without its screen.
type pageArgs struct {
	asJSON, tags, length   bool
	limit, offset, preview int
}

// child is an item of the folder, with what --tags, --length and --preview add to a note.
type child struct {
	apiclient.DomainItem
	Tags    *[]string `json:"tags,omitempty"` // a pointer, so [] says "none"
	Length  *int      `json:"length,omitempty"`
	Preview *string   `json:"preview,omitempty"`
}

// folderPage is the folder, a page of its children (all of them without --limit), and how many it holds in all.
type folderPage struct {
	apiclient.ReaditemResult
	Children []child `json:"children"`
	Total    int     `json:"total"`
}

// printPage prints a page of the folder's children, folders first, as JSON or as a table, the notes described by
// POST /api/notes/read when asked.
func printPage(ctx context.Context, client *apiclient.ClientWithResponses, id string, args pageArgs) error {
	folder, err := header(ctx, client, id)
	if err != nil {
		return err
	}
	params := &apiclient.GetApiItemsParams{Limit: &args.limit, Offset: &args.offset}
	if id != "" {
		params.Parent = &id
	}
	items, total, err := apiclient.Paged[[]apiclient.DomainItem](client.GetApiItems(ctx, params))
	if err != nil {
		return err
	}
	out := folderPage{ReaditemResult: folder, Children: make([]child, len(items)), Total: total}
	var notes []string // the ids of the notes, in their order among the children
	var at []int
	for i, item := range items {
		out.Children[i].DomainItem = item
		if item.Kind == apiclient.NOTE {
			notes, at = append(notes, item.Id), append(at, i)
		}
	}
	if len(notes) > 0 && (args.tags || args.length || args.preview > 0) {
		read, err := apiclient.ReadNotes(ctx, client, notes, max(args.preview, 1)) // 0 would send every content whole
		if err != nil {
			return err
		}
		for j, r := range read {
			c := &out.Children[at[j]]
			if args.tags {
				c.Tags = new(append([]string{}, apiclient.Deref(r.Tags)...))
			}
			if args.length {
				c.Length = &r.Length
			}
			if args.preview > 0 {
				c.Preview = r.Content
			}
		}
	}

	if args.asJSON {
		plumdebug.Json(out)
		return nil
	}
	fmt.Print(style.Header(folder.Item.Title, folder.Path, fmt.Sprintf("%d of %d items", len(items), total)) + "\n")
	if len(items) == 0 {
		fmt.Println(style.Empty().Render(EmptyFolder))
		return nil
	}
	cells := make([][]string, len(out.Children))
	for i, c := range out.Children {
		row := []string{string(c.Kind), cmp.Or(c.Title, "untitled")}
		if args.tags {
			row = append(row, tagsOf(c.Tags))
		}
		if args.length {
			row = append(row, lengthOf(c.Length))
		}
		if args.preview > 0 {
			row = append(row, strings.Join(strings.Fields(apiclient.Deref(c.Preview)), " "))
		}
		cells[i] = append(row, c.Id)
	}
	fmt.Print(style.Table(cells, -1, 1))
	return nil
}

func tagsOf(tags *[]string) string {
	if tags == nil || len(*tags) == 0 {
		return ""
	}
	return "#" + strings.Join(*tags, " #")
}

func lengthOf(length *int) string {
	if length == nil {
		return ""
	}
	return strconv.Itoa(*length)
}

// header reads the folder without its children through GET /api/folders/{id}, or the root as a folder titled after
// the app at path /.
func header(ctx context.Context, client *apiclient.ClientWithResponses, id string) (apiclient.ReaditemResult, error) {
	if id == "" {
		return apiclient.ReaditemResult{Item: apiclient.DomainItem{Kind: apiclient.FOLDER, Title: config.Title()}, Path: "/"}, nil
	}
	response, err := client.GetApiFoldersIdWithResponse(ctx, id)
	if err != nil {
		return apiclient.ReaditemResult{}, err
	}
	if response.JSON200 == nil {
		return apiclient.ReaditemResult{}, apiclient.Fail(response.HTTPResponse, response.Body)
	}
	return *response.JSON200, nil
}

// Fetch reads the folder ("" the root) then its children through GET /api/items.
func Fetch(ctx context.Context, client *apiclient.ClientWithResponses, id string) (apiclient.ReaditemResult, error) {
	folder, err := header(ctx, client, id)
	if err != nil {
		return folder, err
	}
	params := &apiclient.GetApiItemsParams{}
	if id != "" {
		params.Parent = &id
	}
	children, err := client.GetApiItemsWithResponse(ctx, params)
	if err != nil {
		return folder, err
	}
	if children.JSON200 == nil {
		return folder, apiclient.Fail(children.HTTPResponse, children.Body)
	}
	folder.Children = children.JSON200
	return folder, nil
}

// EmptyFolder is the message of a folder without children.
const EmptyFolder = "This folder is empty"

// Output renders a folder like `show folder`: the header from the whole folder, then rows (the finder
// passes its filtered, scrolled window); cursor >= 0 marks that row as selected, empty shows when no row.
func Output(folder apiclient.ReaditemResult, rows []apiclient.DomainItem, cursor int, empty string) string {
	header := style.Header(folder.Item.Title, folder.Path, info(folder)) + "\n"
	if len(rows) == 0 {
		return header + style.Empty().Render(empty)
	}

	cells := make([][]string, len(rows))
	for i, child := range rows {
		title := child.Title
		if title == "" {
			title = "untitled"
		}
		cells[i] = []string{string(child.Kind), title, child.Id}
	}
	return header + style.Table(cells, cursor, 1)
}

// info counts the children, and dates the folder when it is a real one (the root has no row).
func info(folder apiclient.ReaditemResult) string {
	children := apiclient.Deref(folder.Children)
	folders := 0
	for _, child := range children {
		if child.Kind == apiclient.FOLDER {
			folders++
		}
	}
	parts := []string{count(folders, "folder"), count(len(children)-folders, "note")}

	updated := apiclient.Deref(folder.Item.UpdatedAt)
	if updated == "" {
		updated = folder.Item.CreatedAt
	}
	if date, err := time.Parse(time.RFC3339Nano, updated); err == nil {
		parts = append(parts, "updated "+date.Format("2 Jan 2006"))
	}
	return strings.Join(parts, " · ")
}

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
