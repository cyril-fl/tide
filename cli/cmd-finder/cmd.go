package cmdfinder

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	cmdcreatefolder "github.com/cyril-fl/tide/cli/cmd-create-folder"
	cmdcreatenote "github.com/cyril-fl/tide/cli/cmd-create-note"
	cmdeditfolder "github.com/cyril-fl/tide/cli/cmd-edit-folder"
	cmdeditnote "github.com/cyril-fl/tide/cli/cmd-edit-note"
	cmdreadfolder "github.com/cyril-fl/tide/cli/cmd-read-folder"
	cmdreadnote "github.com/cyril-fl/tide/cli/cmd-read-note"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/core/services/huhform"
	"github.com/cyril-fl/tide/core/services/screen"
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
		screen.Print(cmdreadfolder.Output(folder.ReaditemResult, folder.children, -1, cmdreadfolder.EmptyFolder))
		return nil
	}

	cursor, status, failed := 0, "", false
	var clipboard *apiclient.DomainItem
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
			if err := move(ctx, m.action.item, m.folder.Item.Id); err != nil {
				status, failed = err.Error(), true
			} else {
				clipboard, status = nil, fmt.Sprintf("Moved %q here", untitled(string(m.action.item.Title)))
			}
		} else if err := do(ctx, m.action, m.folder.Item.Id); err != nil && !errors.Is(err, huh.ErrUserAborted) {
			// An aborted form comes back to the finder unchanged.
			return err
		}
		if folder, err = load(m.folder.Item.Id); err != nil {
			return err
		}
		cursor = m.cursor
	}
}

// move puts the item in the folder through POST /api/move, which refuses a folder going into itself or its
// subfolders.
func move(ctx context.Context, item apiclient.DomainItem, folderID string) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	response, err := client.PostApiMoveWithResponse(ctx, apiclient.MoveitemsInput{Ids: []string{item.Id}, ParentId: &folderID})
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
		return cmdreadnote.View(ctx, item.Id)
	case actionEdit:
		if item.Kind == apiclient.FOLDER {
			return cmdeditfolder.Interactive(ctx, item.Id)
		}
		return cmdeditnote.Interactive(ctx, item.Id)
	case actionCreate:
		return create(ctx, folderID)
	case actionDelete:
		return remove(ctx, item)
	}
	return nil
}

// create asks what to make, then opens its add form inside the current folder.
func create(ctx context.Context, folderID string) error {
	kind := apiclient.NOTE
	choice := huh.NewSelect[apiclient.DomainKind]().Title("New").Options(huh.NewOption("Note", apiclient.NOTE), huh.NewOption("Folder", apiclient.FOLDER)).Value(&kind)
	if err := huhform.New(choice).Run(); err != nil {
		return err
	}

	if kind == apiclient.FOLDER {
		return cmdcreatefolder.Interactive(ctx, folderID)
	}
	return cmdcreatenote.Interactive(ctx, folderID)
}

// remove moves the item to the trash once confirmed; a folder takes its content with it.
func remove(ctx context.Context, item apiclient.DomainItem) error {
	title := item.Title
	if title == "" {
		title = "untitled"
	}
	question := fmt.Sprintf("Move %q to the trash?", title)
	if item.Kind == apiclient.FOLDER {
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
	if item.Kind == apiclient.FOLDER {
		deleted, err := client.DeleteApiFoldersIdWithResponse(ctx, item.Id)
		if err != nil {
			return err
		}
		response, body = deleted.HTTPResponse, deleted.Body
	} else {
		deleted, err := client.DeleteApiNotesIdWithResponse(ctx, item.Id)
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
	folder, err := cmdreadfolder.Fetch(ctx, client, id)
	if err != nil {
		return page{}, err
	}

	children := apiclient.Deref(folder.Children)
	ids := make([]string, len(children))
	for i, child := range children {
		ids[i] = child.Id
	}
	tags, err := apiclient.NoteTags(ctx, client, ids)
	return page{ReaditemResult: folder, children: children, tags: tags}, err
}
