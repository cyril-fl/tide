package cmdreadnote

import (
	"context"
	"errors"
	"fmt"
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

var errOneNote = errors.New("--history and --version take one note")

// Read shows the note path, its tags and dates, then its content rendered as markdown; ctrl+e edits it, then shows it
// again; --history lists its versions, --version shows one; --json prints them for scripts. Several notes, or
// --max-chars, are printed one after the other, without the screen.
func Read(cobra *cobra.Command, args []string) error {
	var asJSON, asHistory bool
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"json": &asJSON, "history": &asHistory}); err != nil {
		return err
	}
	version, err := cobra.Flags().GetString("version")
	if err != nil {
		return err
	}
	maxChars, err := cobra.Flags().GetInt("max-chars")
	if err != nil {
		return err
	}
	if len(args) > 1 && (asHistory || version != "") {
		return errOneNote
	}
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	if len(args) > 1 || maxChars > 0 {
		return printNotes(cobra.Context(), client, args, maxChars, asJSON)
	}
	id, err := apiclient.Resolve(cobra.Context(), client, apiclient.NOTE, args[0])
	if err != nil {
		return err
	}
	if asHistory {
		return printHistory(cobra.Context(), client, id, asJSON)
	}
	if version != "" {
		return printVersion(cobra.Context(), client, id, version, asJSON)
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
	Length    *int     `json:"length,omitempty"`    // the whole content's characters, under --max-chars
	Truncated *bool    `json:"truncated,omitempty"` // a pointer: false is said too, under --max-chars
}

func toJSON(note apiclient.ReaditemResult) noteJSON {
	return noteJSON{
		ID: note.Item.Id, Title: note.Item.Title, Path: note.Path, Content: apiclient.Deref(note.Content),
		Tags: append([]string{}, apiclient.Deref(note.Tags)...), Readonly: apiclient.Deref(note.Readonly),
		CreatedAt: note.Item.CreatedAt, UpdatedAt: note.Item.UpdatedAt,
	}
}

// printNotes reads the notes of refs (uuids or paths) in one request per 50, only the start of each under maxChars,
// and prints them one after the other, or as JSON: an object for one note, an array for several.
func printNotes(ctx context.Context, client *apiclient.ClientWithResponses, refs []string, maxChars int, asJSON bool) error {
	read, err := apiclient.ReadNotes(ctx, client, refs, maxChars)
	if err != nil {
		return err
	}
	if asJSON {
		notes := make([]noteJSON, len(read))
		for i, r := range read {
			notes[i] = toJSON(apiclient.ReaditemResult{Item: r.Item, Path: r.Path, Content: r.Content, Readonly: r.Readonly, Tags: r.Tags})
			if maxChars > 0 {
				notes[i].Length, notes[i].Truncated = &r.Length, &r.Truncated
			}
		}
		if len(notes) == 1 {
			plumdebug.Json(notes[0])
		} else {
			plumdebug.Json(notes)
		}
		return nil
	}
	for i, r := range read {
		note := apiclient.ReaditemResult{Item: r.Item, Path: r.Path, Content: r.Content, Readonly: r.Readonly, Tags: r.Tags}
		content, err := render(apiclient.Deref(r.Content))
		if err != nil {
			return err
		}
		if i > 0 {
			fmt.Println()
		}
		fmt.Print(style.Muted().Render(note.Path) + "\n" + style.Subtle().Render(info(note)) + "\n\n" + content)
		if r.Truncated {
			fmt.Println(style.Subtle().Render(fmt.Sprintf("… %d characters in all", r.Length)))
		}
	}
	return nil
}

// printHistory lists the versions of the note, latest first: date, uuid, and the texts kept from a conflict.
func printHistory(ctx context.Context, client *apiclient.ClientWithResponses, id string, asJSON bool) error {
	versions, err := apiclient.Versions(ctx, client, id)
	if err != nil {
		return err
	}
	if asJSON {
		plumdebug.Json(append([]apiclient.DomainNoteVersion{}, versions...))
		return nil
	}
	if len(versions) == 0 {
		fmt.Println(style.Empty().Render("This note has no version yet"))
		return nil
	}
	fmt.Print(style.Table(historyRows(versions), -1, 1))
	return nil
}

func historyRows(versions []apiclient.DomainNoteVersion) [][]string {
	rows := make([][]string, len(versions))
	for i, version := range versions {
		date := version.CreatedAt
		if parsed, err := time.Parse(time.RFC3339Nano, version.CreatedAt); err == nil {
			date = parsed.Local().Format("2 Jan 2006 15:04")
		}
		rows[i] = []string{date, version.Id}
		if version.Conflict {
			rows[i] = append(rows[i], "conflict")
		}
	}
	return rows
}

// printVersion shows the text of one version, rendered like the note.
func printVersion(ctx context.Context, client *apiclient.ClientWithResponses, id, versionID string, asJSON bool) error {
	version, err := apiclient.Version(ctx, client, id, versionID)
	if err != nil {
		return err
	}
	if asJSON {
		plumdebug.Json(version)
		return nil
	}
	content, err := render(apiclient.Deref(version.Content))
	if err != nil {
		return err
	}
	fmt.Print(content)
	return nil
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
