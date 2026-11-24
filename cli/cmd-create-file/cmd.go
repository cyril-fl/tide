package cmdcreatefile

import (
	"fmt"
	"os"
	"path/filepath"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	plumdebug "github.com/cyril-fl/plum/utils/debug"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/spf13/cobra"
)

// stored is a file stored and the markdown a note cites it with.
type stored struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Markdown string `json:"markdown"`
}

// Create stores the files of the args through POST /api/files, under their base name, and prints the markdown that
// cites each; every path is checked before the first is sent.
func Create(cobra *cobra.Command, args []string) error {
	var asJSON bool
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"json": &asJSON}); err != nil {
		return err
	}
	for _, path := range args {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return fmt.Errorf("%s is a directory", path)
		}
	}
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}

	out := make([]stored, 0, len(args))
	for _, path := range args {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		name := filepath.Base(path)
		saved, err := apiclient.Attach(cobra.Context(), client, name, file)
		file.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, stored{ID: saved.Id, Name: name, Markdown: apiclient.Citation(saved, name)})
		if !asJSON {
			fmt.Println(out[len(out)-1].Markdown)
		}
	}
	if asJSON {
		plumdebug.Json(out)
	}
	return nil
}
