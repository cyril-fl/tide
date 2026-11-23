package cmdreadnote

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"charm.land/glamour/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	plumdebug "github.com/cyril-fl/plum/utils/debug"
	cmdeditnote "github.com/cyril-fl/tide/cli/cmd-edit-note"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/core/services/screen"
	"github.com/cyril-fl/tide/core/services/style"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Read shows the note path, its tags and dates, then its content rendered as markdown; ctrl+e edits it, then shows it
// again; --json prints it for scripts.
func Read(cobra *cobra.Command, args []string) error {
	var asJSON bool
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"json": &asJSON}); err != nil {
		return err
	}
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	id, err := apiclient.Resolve(cobra.Context(), client, apiclient.NOTE, args[0])
	if err != nil {
		return err
	}
	if asJSON {
		note, err := fetch(cobra.Context(), client, id)
		if err != nil {
			return err
		}
		plumdebug.Json(toJSON(note))
		return nil
	}
	return View(cobra.Context(), id)
}

// noteJSON is the note for scripts: flat, tags [] rather than null, updated_at null for a note never edited.
type noteJSON struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Path      string   `json:"path"`
	Content   string   `json:"content"`
	Tags      []string `json:"tags"`
	Readonly  bool     `json:"readonly"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt *string  `json:"updated_at"`
}

func toJSON(note apiclient.ReaditemResult) noteJSON {
	return noteJSON{
		ID: note.Item.Id, Title: note.Item.Title, Path: note.Path, Content: apiclient.Deref(note.Content),
		Tags: append([]string{}, apiclient.Deref(note.Tags)...), Readonly: apiclient.Deref(note.Readonly),
		CreatedAt: note.Item.CreatedAt, UpdatedAt: note.Item.UpdatedAt,
	}
}

// View is the show note screen: ctrl+e edits the note, then shows it again, until the user quits, both through the
// API.
func View(ctx context.Context, id string) error {
	for {
		output, err := load(ctx, id)
		if err != nil {
			return err
		}

		edit, err := screen.Show(output)
		if err != nil || !edit {
			return err
		}
		// An aborted form goes back to the view unchanged.
		if err := cmdeditnote.Interactive(ctx, id); err != nil && !errors.Is(err, huh.ErrUserAborted) {
			return err
		}
	}
}

func load(ctx context.Context, id string) (string, error) {
	client, err := apiclient.Read().Client()
	if err != nil {
		return "", err
	}
	note, err := fetch(ctx, client, id)
	if err != nil {
		return "", err
	}
	content, err := render(apiclient.Deref(note.Content))
	if err != nil {
		return "", err
	}

	return style.Muted().Render(note.Path) + "\n" + style.Subtle().Render(info(note)) + "\n\n" + content, nil
}

// fetch reads the note through GET /api/notes/{id}.
func fetch(ctx context.Context, client *apiclient.ClientWithResponses, id string) (apiclient.ReaditemResult, error) {
	response, err := client.GetApiNotesIdWithResponse(ctx, id)
	if err != nil {
		return apiclient.ReaditemResult{}, err
	}
	if response.JSON200 == nil {
		return apiclient.ReaditemResult{}, apiclient.Fail(response.HTTPResponse, response.Body)
	}
	return *response.JSON200, nil
}

// info is the line under the path: the tags, the creation and last change dates, and the lock.
func info(note apiclient.ReaditemResult) string {
	var parts []string
	if tags := apiclient.Deref(note.Tags); len(tags) > 0 {
		parts = append(parts, "#"+strings.Join(tags, " #"))
	}
	if date, err := time.Parse(time.RFC3339Nano, note.Item.CreatedAt); err == nil {
		parts = append(parts, "created "+date.Format("2 Jan 2006"))
	}
	if date, err := time.Parse(time.RFC3339Nano, apiclient.Deref(note.Item.UpdatedAt)); err == nil {
		parts = append(parts, "updated "+date.Format("2 Jan 2006"))
	}
	if apiclient.Deref(note.Readonly) {
		parts = append(parts, "locked")
	}
	return strings.Join(parts, " · ")
}

// render keeps the raw markdown when stdout is not a terminal (pipe, file).
func render(markdown string) (string, error) {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return markdown + "\n", nil
	}

	isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	renderer, err := glamour.NewTermRenderer(glamour.WithStyles(style.Markdown(isDark)), glamour.WithWordWrap(width))
	if err != nil {
		return "", err
	}
	return renderer.Render(markdown)
}
