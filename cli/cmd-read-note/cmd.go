package cmdreadnote

import (
	"context"
	"errors"
	"os"

	"charm.land/glamour/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	cmdeditnote "github.com/cyril-fl/tide/cli/cmd-edit-note"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/core/services/screen"
	"github.com/cyril-fl/tide/core/services/style"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Read shows the note path, then its content rendered as markdown; ctrl+e edits it, then shows it again.
func Read(cobra *cobra.Command, args []string) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	id, err := apiclient.Resolve(cobra.Context(), client, apiclient.NOTE, args[0])
	if err != nil {
		return err
	}
	return View(cobra.Context(), id)
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
	response, err := client.GetApiNotesIdWithResponse(ctx, id)
	if err != nil {
		return "", err
	}
	note := response.JSON200
	if note == nil {
		return "", apiclient.Fail(response.HTTPResponse, response.Body)
	}

	var markdown string
	if note.Content != nil {
		markdown = *note.Content
	}
	content, err := render(markdown)
	if err != nil {
		return "", err
	}

	return style.Muted().Render(note.Path) + "\n\n" + content, nil
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
