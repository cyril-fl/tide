package cmdexport

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/spf13/cobra"
)

var (
	ErrNotEmpty = errors.New("the directory is not empty")
	ErrBadEntry = errors.New("the export holds a path outside the directory")
)

// Export writes the notes of a folder (args[1], everything without it) as .md files into the directory args[0], from
// the zip of GET /api/export.
func Export(cobra *cobra.Command, args []string) error {
	dir := args[0]
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	var params apiclient.GetApiExportParams
	if len(args) > 1 {
		// Not cmd's apiResolved(): it would read the directory, a path too, as an item.
		folderID, err := apiclient.Resolve(cobra.Context(), client, apiclient.FOLDER, args[1])
		if err != nil {
			return err
		}
		params.Parent = &folderID
	}

	// Never write over files: an export goes into a new or empty directory.
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("%w: %s", ErrNotEmpty, dir)
	}

	// Not the WithResponse variant: it would hold every image of the account in memory.
	ctx, cancel := apiclient.Long(cobra.Context())
	defer cancel()
	response, err := client.GetApiExport(ctx, &params)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return apiclient.Fail(response, body)
	}
	// A zip is read from its end: it lands in a temporary file first, and a download cut halfway writes nothing.
	archive, err := os.CreateTemp("", "notes-export-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(archive.Name())
	defer archive.Close()
	size, err := io.Copy(archive, response.Body)
	if err != nil {
		return err
	}
	files, err := zip.NewReader(archive, size)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	notes := 0
	for _, file := range files.File {
		name := strings.TrimSuffix(file.Name, "/")
		if !filepath.IsLocal(filepath.FromSlash(name)) {
			return fmt.Errorf("%w: %s", ErrBadEntry, file.Name)
		}
		target := filepath.Join(dir, filepath.FromSlash(name))
		if strings.HasSuffix(file.Name, "/") {
			if err := os.Mkdir(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := write(file, target); err != nil {
			return err
		}
		// The images are named by their type's extension, never .md.
		if strings.HasSuffix(name, ".md") {
			notes++
		}
	}

	fmt.Printf("Successfully exported %d note(s) to %s\n", notes, dir)
	return nil
}

// write copies an entry of the zip into target.
func write(file *zip.File, target string) error {
	body, err := file.Open()
	if err != nil {
		return err
	}
	defer body.Close()
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, body); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
