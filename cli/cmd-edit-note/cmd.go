package cmdeditnote

import (
	"context"
	"errors"
	"fmt"
	"time"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	millflagseditnote "github.com/cyril-fl/tide/cli/cmd-edit-note/services/mill"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// errStale is the text of patchnote.ErrStale the API answers: the note changed since the mill read it.
const errStale = "note changed elsewhere since it was read"

// Edit loads the note, applies the flags (or a pre-filled form when none), then saves it through PATCH /api/notes/{id}.

func Edit(cobra *cobra.Command, args []string) error {
	return run(cobra.Context(), args[0], cobra.Flags())
}

// Interactive edits the note in the pre-filled form, as `edit note <id>` without flags.
func Interactive(ctx context.Context, id string) error {
	return run(ctx, id, noFlags())
}

// run edits the text only when a flag asks for it, or with the form when no flag says anything: --pin or --lock
// alone must not rewrite the note, nor change its last change date. The API pins, locks and edits in one transaction.
func run(ctx context.Context, id string, flags *pflag.FlagSet) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	var input apiclient.EditnoteBody
	if flags.Changed("pin") || flags.Changed("unpin") {
		pinned := flags.Changed("pin")
		input.Pinned = &pinned
	}
	if flags.Changed("lock") || flags.Changed("unlock") {
		readonly := flags.Changed("lock")
		input.Readonly = &readonly
	}
	textChanged := flags.Changed("content") || flags.Changed("tag") || flags.Changed("untag") || flags.Changed("tag-from-list")
	part, err := partInput(flags)
	if err != nil {
		return err
	}
	if part != nil && textChanged {
		return errors.New("--title, --replace and --section cannot be given with --content or tag flags")
	}
	restoreID, _ := flags.GetString("restore-version")
	if restoreID != "" && (part != nil || textChanged) {
		return errors.New("--restore-version cannot be given with --content, tag flags, --title, --replace or --section")
	}
	// A part edit goes after --unlock and before --lock, so a lock given with it does not refuse it.
	unlockFirst := flags.Changed("unlock")
	if part != nil && !unlockFirst {
		if err := editPart(ctx, client, id, *part); err != nil {
			return err
		}
	}
	if part != nil && input.Pinned == nil && input.Readonly == nil {
		return nil
	}
	if restoreID != "" {
		// Saved as a restore, like the web app's: the API keeps the text it replaces in the history.
		version, err := apiclient.Version(ctx, client, id, restoreID)
		if err != nil {
			return err
		}
		content, restore := apiclient.Deref(version.Content), true
		input.Content, input.Restore = &content, &restore
	} else if input.Pinned == nil && input.Readonly == nil || textChanged {
		// The mill applies --tag and --untag over the current note, the API then only saves its result.
		resolver := millflagseditnote.New(millflagseditnote.Template{Ctx: ctx, Client: client, Flags: flags, ID: id})
		note := resolver.Resolve()
		if resolver.HasErr() {
			return resolver.GetErr()
		}
		version := resolver.Version().Format(time.RFC3339Nano)
		input.Content, input.Tags, input.UpdatedAt = &note.Content, &note.Tags, &version
	}

	response, err := client.PatchApiNotesIdWithResponse(ctx, id, input)
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		err := apiclient.Fail(response.HTTPResponse, response.Body)
		if err.Error() == errStale {
			return fmt.Errorf("%w: nothing saved, run the command again to edit the new text", err)
		}
		return err
	}

	if restoreID != "" {
		fmt.Printf("Successfully restored version %s of note %s\n", restoreID, id)
	} else if input.Content != nil {
		fmt.Printf("Successfully updated note %s\n", id)
	}
	if input.Pinned != nil && *input.Pinned {
		fmt.Printf("Successfully pinned note %s\n", id)
	} else if input.Pinned != nil {
		fmt.Printf("Successfully unpinned note %s\n", id)
	}
	if input.Readonly != nil && *input.Readonly {
		fmt.Printf("Successfully locked note %s\n", id)
	} else if input.Readonly != nil {
		fmt.Printf("Successfully unlocked note %s\n", id)
	}
	if part != nil && unlockFirst {
		return editPart(ctx, client, id, *part)
	}
	return nil
}

// partInput reads --title, --replace or --section with --with and --all, nil when none is given. The server checks
// that only one is, the occurrences and the lock.
func partInput(flags *pflag.FlagSet) (*apiclient.EditnotetextInput, error) {
	var input apiclient.EditnotetextInput
	for name, value := range map[string]**string{"title": &input.Title, "replace": &input.Old, "section": &input.Section, "with": &input.New} {
		if flags.Changed(name) {
			text, _ := flags.GetString(name)
			*value = &text
		}
	}
	if flags.Changed("all") {
		all, _ := flags.GetBool("all")
		input.All = &all
	}
	switch {
	case input.Title == nil && input.Old == nil && input.Section == nil:
		if input.New != nil || input.All != nil {
			return nil, errors.New("--with and --all go with --replace or --section")
		}
		return nil, nil
	case input.Title != nil && input.New != nil:
		return nil, errors.New("--with goes with --replace or --section, not --title")
	case input.Title == nil && input.New == nil:
		return nil, errors.New("--replace and --section need --with (empty to delete)")
	case input.All != nil && input.Old == nil:
		return nil, errors.New("--all goes with --replace")
	}
	return &input, nil
}

// editPart changes the title, a text or a section of the note through PATCH /api/notes/{id}/text.
func editPart(ctx context.Context, client *apiclient.ClientWithResponses, id string, input apiclient.EditnotetextInput) error {
	response, err := client.PatchApiNotesIdTextWithResponse(ctx, id, input)
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}
	if input.Title != nil {
		fmt.Printf("Successfully renamed note %s to %q\n", id, response.JSON200.Note.Item.Title)
	} else if input.Section != nil {
		fmt.Printf("Successfully replaced section %q in note %s\n", *input.Section, id)
	} else {
		fmt.Printf("Successfully replaced %d occurrence(s) in note %s\n", response.JSON200.Replaced, id)
	}
	return nil
}

// noFlags declares the edit flags without setting any: the mill reads them all and sees no change.
func noFlags() *pflag.FlagSet {
	cmd := &cobra.Command{}
	plumcliutilsargs.Makeflags(cmd, config.Current.Commands.EditNote.Flags)
	return cmd.Flags()
}
