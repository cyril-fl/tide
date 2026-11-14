package cmdimport

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/spf13/cobra"
)

// Import adds the .md files of the directory args[0] as notes, and its directories as folders, into --parent: the
// directory goes up as a zip to POST /api/import.
func Import(cobra *cobra.Command, args []string) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	var params apiclient.PostApiImportParams
	if parent, _ := cobra.Flags().GetString("parent"); parent != "" {
		// Not cmd's apiResolved(): it would read the directory, a path too, as an item.
		parentID, err := apiclient.Resolve(cobra.Context(), client, apiclient.FOLDER, parent)
		if err != nil {
			return err
		}
		params.Parent = &parentID
	}
	if info, err := os.Stat(args[0]); err != nil || !info.IsDir() {
		return fmt.Errorf("not a directory: %s", args[0])
	}

	// Zipped as it is sent, never whole on disk nor in memory.
	body, writer := io.Pipe()
	go func() { writer.CloseWithError(zipDir(writer, os.DirFS(args[0]))) }()
	defer body.Close()
	response, err := client.PostApiImportWithBodyWithResponse(cobra.Context(), &params, "application/zip", body)
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}
	fmt.Printf("Successfully imported %d note(s) and %d folder(s)\n", response.JSON200.Notes, response.JSON200.Folders)
	return nil
}

// zipDir writes dir as a zip, its empty directories too: the import makes a folder of each. Hidden entries (.git,
// .obsidian...) stay out, as the import skips them, and would only weigh on the upload's limit.
func zipDir(out io.Writer, dir fs.FS) error {
	archive := zip.NewWriter(out)
	err := fs.WalkDir(dir, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || name == "." {
			return err
		}
		if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "__MACOSX" {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			_, err := archive.Create(name + "/")
			return err
		}
		// A link goes up as the file it points to, as the import read it from the disk; to a directory, it is skipped.
		if info, err := fs.Stat(dir, name); err != nil || !info.Mode().IsRegular() {
			return nil
		}
		file, err := dir.Open(name)
		if err != nil {
			return err
		}
		defer file.Close()
		target, err := archive.Create(name)
		if err == nil {
			_, err = io.Copy(target, file)
		}
		return err
	})
	if err != nil {
		return err
	}
	return archive.Close()
}
