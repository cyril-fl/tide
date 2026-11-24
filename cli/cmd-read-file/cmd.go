package cmdreadfile

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	plumdebug "github.com/cyril-fl/plum/utils/debug"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/core/services/style"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	errJSONList = errors.New("--json lists the files: give no uuid")
	errTerminal = errors.New("the bytes of a file would garble the terminal: give --output or redirect stdout")
)

// Read lists the files of the account, largest first, or writes the bytes of the one of uuid the arg to --output or
// stdout.
func Read(cobra *cobra.Command, args []string) error {
	var asJSON bool
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"json": &asJSON}); err != nil {
		return err
	}
	output, err := cobra.Flags().GetString("output")
	if err != nil {
		return err
	}
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		files, err := apiclient.Decode[[]apiclient.DomainFile](client.GetApiFiles(cobra.Context()))
		switch {
		case err != nil:
			return err
		case asJSON:
			plumdebug.Json(append([]apiclient.DomainFile{}, files...))
		case len(files) == 0:
			fmt.Println(style.Empty().Render("No file"))
		default:
			cells := [][]string{{"Name", "Type", "Size", "Id"}}
			for _, file := range files {
				cells = append(cells, []string{file.Name, file.ContentType, strconv.Itoa(file.Size), file.Id})
			}
			fmt.Print(style.Table(cells, -1, 0))
		}
		return nil
	}

	if asJSON {
		return errJSONList
	}
	if output == "" && term.IsTerminal(int(os.Stdout.Fd())) {
		return errTerminal
	}
	ctx, cancel := apiclient.Long(cobra.Context())
	defer cancel()
	response, err := client.GetApiFilesIdWithResponse(ctx, args[0])
	if err != nil {
		return err
	}
	if response.StatusCode() != http.StatusOK {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}
	if output == "" {
		_, err = os.Stdout.Write(response.Body)
		return err
	}
	// O_EXCL: an existing file is never overwritten.
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(response.Body); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
