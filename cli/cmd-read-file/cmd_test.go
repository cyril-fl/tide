package cmdreadfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	plumcliconfig "github.com/cyril-fl/plum/cli/config"
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	cmdcreatefile "github.com/cyril-fl/tide/cli/cmd-create-file"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/config"
	"github.com/spf13/cobra"
)

// png is the smallest PNG the server takes for an image: its signature and an IHDR.
var png = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89")

// add file stores a file and prints the markdown citing it; show file lists it and writes its bytes to --output,
// never over an existing file.
func TestFiles(t *testing.T) {
	ctx, _ := apitest.Serve(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "photo.png")
	if err := os.WriteFile(source, png, 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(flags []plumcliconfig.Cmdflags, do func(*cobra.Command, []string) error, args []string, parse ...string) (string, error) {
		t.Helper()
		command := &cobra.Command{}
		plumcliutilsargs.Makeflags(command, flags)
		if err := command.ParseFlags(parse); err != nil {
			t.Fatal(err)
		}
		command.SetContext(ctx)
		return apitest.Stdout(t, func() error { return do(command, args) })
	}
	commands := config.Current.Commands

	if _, err := run(commands.CreateFile.Flags, cmdcreatefile.Create, []string{source, filepath.Join(dir, "missing.pdf")}); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a missing path: err = %v, want ErrNotExist before any upload", err)
	}
	printed, err := run(commands.CreateFile.Flags, cmdcreatefile.Create, []string{source})
	markdown := strings.TrimSpace(printed)
	if err != nil || !strings.HasPrefix(markdown, "![photo.png](/api/files/") {
		t.Fatalf("add = %q, %v", printed, err)
	}
	id := strings.TrimSuffix(strings.TrimPrefix(markdown, "![photo.png](/api/files/"), ")")

	printed, err = run(commands.ReadFile.Flags, Read, nil, "--json")
	var files []apiclient.DomainFile
	if err == nil {
		err = json.Unmarshal([]byte(printed), &files)
	}
	if err != nil || len(files) != 1 || files[0].Id != id || files[0].Name != "photo.png" {
		t.Errorf("show --json = %s, %v", printed, err)
	}

	output := filepath.Join(dir, "copy.png")
	if _, err := run(commands.ReadFile.Flags, Read, []string{id}, "--output", output); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(output); err != nil || !bytes.Equal(got, png) {
		t.Errorf("--output wrote %q, %v", got, err)
	}
	if _, err := run(commands.ReadFile.Flags, Read, []string{id}, "--output", output); !errors.Is(err, fs.ErrExist) {
		t.Errorf("an existing --output: err = %v, want ErrExist", err)
	}
	if _, err := run(commands.ReadFile.Flags, Read, []string{id}, "--json"); !errors.Is(err, errJSONList) {
		t.Errorf("--json with a uuid: err = %v", err)
	}
}
