package cmd

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// stdinContent is the --content value meaning "read the content from stdin".
const stdinContent = "-"

var errEmptyStdin = errors.New("no content on stdin")

// apiResolved runs a command with paths turned into uuids through GET /api/resolve, in its args (of this kind, any
// when empty) and in --parent, and --content - replaced by stdin. Commands and mills only ever see uuids and values.
func apiResolved(kind apiclient.DomainKind, run func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cobra *cobra.Command, args []string) error {
		client, err := apiclient.Read().Client()
		if err != nil {
			return err
		}
		ids := make([]string, len(args))
		for i, arg := range args {
			if ids[i], err = apiclient.Resolve(cobra.Context(), client, kind, arg); err != nil {
				return err
			}
		}
		if parent := cobra.Flags().Lookup("parent"); parent != nil && parent.Changed {
			id, err := apiclient.Resolve(cobra.Context(), client, apiclient.FOLDER, parent.Value.String())
			if err != nil {
				return err
			}
			if err := cobra.Flags().Set("parent", id); err != nil {
				return err
			}
		}
		if err := readContent(cobra.Flags(), os.Stdin); err != nil {
			return err
		}
		return run(cobra, ids)
	}
}

// readContent fails on a blank stdin: without content, add note would open a form instead.
func readContent(flags *pflag.FlagSet, stdin io.Reader) error {
	content := flags.Lookup("content")
	if content == nil || !content.Changed || content.Value.String() != stdinContent {
		return nil
	}

	data, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(data)) == "" {
		return errEmptyStdin
	}
	return flags.Set("content", string(data))
}
