package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient"
	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createfolder"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createnote"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func contentFlags(t *testing.T, args ...string) *pflag.FlagSet {
	t.Helper()
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("content", "", "")
	if err := flags.Parse(args); err != nil {
		t.Fatal(err)
	}
	return flags
}

func TestReadContent(t *testing.T) {
	tests := map[string]struct {
		args    []string
		stdin   string
		want    string
		wantErr error
	}{
		"from stdin":  {[]string{"--content", "-"}, "# hello\n", "# hello\n", nil},
		"blank stdin": {[]string{"--content", "-"}, " \n", "", errEmptyStdin},
		"plain value": {[]string{"--content", "text"}, "ignored", "text", nil},
		"no flag":     {nil, "ignored", "", nil},
	}

	for name, test := range tests {
		flags := contentFlags(t, test.args...)
		err := readContent(flags, strings.NewReader(test.stdin))
		if !errors.Is(err, test.wantErr) {
			t.Errorf("%s: err = %v, want %v", name, err, test.wantErr)
			continue
		}
		if test.wantErr != nil {
			continue
		}
		if got, _ := flags.GetString("content"); got != test.want {
			t.Errorf("%s: content = %q, want %q", name, got, test.want)
		}
	}
}

// A command without --content leaves stdin alone.
func TestReadContentNoFlag(t *testing.T) {
	if err := readContent(pflag.NewFlagSet("test", pflag.ContinueOnError), strings.NewReader("")); err != nil {
		t.Fatal(err)
	}
}

// Paths in the args and in --parent become uuids through GET /api/resolve; uuids are kept as they are.
func TestAPIResolved(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	folder := createfolder.Process(ctx, tx, createfolder.Input{Title: "Projects"})
	folder.Run()
	note := createnote.Process(ctx, tx, createnote.Input{Content: "# Ideas", ParentID: folder.GetResult().ID})
	note.Run()
	if folder.HasErr() || note.HasErr() {
		t.Fatal(folder.GetErr(), note.GetErr())
	}

	command := &cobra.Command{}
	command.Flags().String("parent", "", "")
	command.SetContext(ctx)
	if err := command.Flags().Parse([]string{"--parent", "/Projects"}); err != nil {
		t.Fatal(err)
	}
	var got []string
	run := apiResolved(apiclient.NOTE, func(_ *cobra.Command, ids []string) error { got = ids; return nil })
	if err := run(command, []string{"/Projects/Ideas", "a-uuid"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != note.GetResult().ID+",a-uuid" {
		t.Errorf("ids = %v, want the note's uuid then a-uuid", got)
	}
	if parent, _ := command.Flags().GetString("parent"); parent != folder.GetResult().ID {
		t.Errorf("parent = %q, want the folder's uuid", parent)
	}
	if err := run(command, []string{"/Nowhere"}); err == nil {
		t.Error("unknown path: no error")
	}
}
