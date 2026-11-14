package cmdfinder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	cmdcreatefolder "github.com/cyril-fl/notes/notes-go/app/cli/cmd-create-folder"
	cmdcreatenote "github.com/cyril-fl/notes/notes-go/app/cli/cmd-create-note"
	cmdeditfolder "github.com/cyril-fl/notes/notes-go/app/cli/cmd-edit-folder"
	cmdeditnote "github.com/cyril-fl/notes/notes-go/app/cli/cmd-edit-note"
	cmdreadfolder "github.com/cyril-fl/notes/notes-go/app/cli/cmd-read-folder"
	cmdreadnote "github.com/cyril-fl/notes/notes-go/app/cli/cmd-read-note"
	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/huhform"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readitem"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/screen"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Finder browses folders and notes from the given folder (the root by default).
// Forms and the note view run between two finder screens, which then reopens where it was.
func Finder(cobra *cobra.Command, args []string) error {
	ctx, id := cobra.Context(), ""
	if len(args) == 1 {
		id = args[0]
	}
	load := func(id string) (page, error) { return loadFolder(ctx, id) }

	folder, err := load(id)
	if err != nil {
		return err
	}
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		screen.Print(cmdreadfolder.Output(folder.Result, folder.Children, -1, cmdreadfolder.EmptyFolder))
		return nil
	}

	cursor, status, failed := 0, "", false
	var clipboard *domain.Item
	for {
		final, err := tea.NewProgram(newModel(load, folder, cursor, clipboard, status, failed)).Run()
		if err != nil {
			return err
		}
		m := final.(*model)
		if m.err != nil || m.action.kind == actionQuit {
			return m.err
		}

		clipboard, status, failed = m.clipboard, "", false
		if m.action.kind == actionMove {
			// A refused move (into its own subfolder…) is shown in the finder, which stays open.
			if err := move(ctx, m.action.item, m.folder.Item.ID); err != nil {
				status, failed = err.Error(), true
			} else {
				clipboard, status = nil, fmt.Sprintf("Moved %q here", untitled(string(m.action.item.Title)))
			}
		} else if err := do(ctx, m.action, m.folder.Item.ID); err != nil && !errors.Is(err, huh.ErrUserAborted) {
			// An aborted form comes back to the finder unchanged.
			return err
		}
		if folder, err = load(m.folder.Item.ID); err != nil {
			return err
		}
		cursor = m.cursor
	}
}

// move puts the item in the folder through POST /api/move, which refuses a folder going into itself or its
// subfolders.
func move(ctx context.Context, item domain.Item, folderID string) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	response, err := client.PostApiMoveWithResponse(ctx, apiclient.MoveitemsInput{Ids: []string{item.ID}, ParentId: &folderID})
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}
	return nil
}

func do(ctx context.Context, action action, folderID string) error {
	item := action.item
	switch action.kind {
	case actionOpen:
		return cmdreadnote.View(ctx, item.ID)
	case actionEdit:
		if item.Kind == domain.FOLDER {
			return cmdeditfolder.Interactive(ctx, item.ID)
		}
		return cmdeditnote.Interactive(ctx, item.ID)
	case actionCreate:
		return create(ctx, folderID)
	case actionDelete:
		return remove(ctx, item)
	}
	return nil
}

// create asks what to make, then opens its add form inside the current folder.
func create(ctx context.Context, folderID string) error {
	kind := domain.NOTE
	choice := huh.NewSelect[domain.Kind]().Title("New").Options(huh.NewOption("Note", domain.NOTE), huh.NewOption("Folder", domain.FOLDER)).Value(&kind)
	if err := huhform.New(choice).Run(); err != nil {
		return err
	}

	if kind == domain.FOLDER {
		return cmdcreatefolder.Interactive(ctx, folderID)
	}
	return cmdcreatenote.Interactive(ctx, folderID)
}

// remove moves the item to the trash once confirmed; a folder takes its content with it.
func remove(ctx context.Context, item domain.Item) error {
	title := item.Title
	if title == "" {
		title = "untitled"
	}
	question := fmt.Sprintf("Move %q to the trash?", title)
	if item.Kind == domain.FOLDER {
		question = fmt.Sprintf("Move %q and its content to the trash?", title)
	}

	confirmed := false
	if err := huhform.New(huh.NewConfirm().Title(question).Value(&confirmed)).Run(); err != nil || !confirmed {
		return err
	}

	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	var response *http.Response
	var body []byte
	if item.Kind == domain.FOLDER {
		deleted, err := client.DeleteApiFoldersIdWithResponse(ctx, item.ID)
		if err != nil {
			return err
		}
		response, body = deleted.HTTPResponse, deleted.Body
	} else {
		deleted, err := client.DeleteApiNotesIdWithResponse(ctx, item.ID)
		if err != nil {
			return err
		}
		response, body = deleted.HTTPResponse, deleted.Body
	}
	// Already gone (removed elsewhere): nothing left to do, as deleteitems skipped it.
	_, err = apiclient.Deleted(response, body)
	return err
}

// loadFolder reads a folder, or the root as a folder titled Notes at path /, with the tags of its children through
// POST /api/notes/tags.
func loadFolder(ctx context.Context, id string) (page, error) {
	client, err := apiclient.Read().Client()
	if err != nil {
		return page{}, err
	}
	folder, err := readFolder(ctx, client, id)
	if err != nil {
		return page{}, err
	}

	ids := make([]string, len(folder.Children))
	for i, child := range folder.Children {
		ids[i] = child.ID
	}
	tags, err := apiclient.NoteTags(ctx, client, ids)
	return page{Result: folder, tags: tags}, err
}

// readFolder reads a folder with its children, as show folder does; "" is the root, its children read through
// GET /api/items.
func readFolder(ctx context.Context, client *apiclient.ClientWithResponses, id string) (readitem.Result, error) {
	if id != "" {
		return cmdreadfolder.Fetch(ctx, client, id)
	}

	root := readitem.Result{Item: domain.Item{Kind: domain.FOLDER, Title: "Notes"}, Path: "/"}
	response, err := client.GetApiItemsWithResponse(ctx, &apiclient.GetApiItemsParams{})
	if err != nil {
		return root, err
	}
	if response.JSON200 == nil {
		return root, apiclient.Fail(response.HTTPResponse, response.Body)
	}
	return root, json.Unmarshal(response.Body, &root.Children)
}
