package cmdreadtag

import (
	"fmt"
	"strings"
	"testing"

	plumdebug "github.com/cyril-fl/plum/utils/debug"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/listtag"
	"github.com/cyril-fl/tide/core/services/process/readtag"
	"github.com/spf13/cobra"
)

// printKernel is printTags on the kernel's type: the generated one must print the same.
func printKernel(tags []domain.Tag, asYAML bool, asJSON bool) {
	switch {
	case asJSON:
		plumdebug.Json(tags)
	case asYAML:
		plumdebug.Yaml(tags)
	default:
		for _, tag := range tags {
			fmt.Println(tag.Value)
		}
	}
}

// --list through the API prints what listtag gives in the base, in each format, with no tag (an empty list, not
// null) and with some.
func TestListTags(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	for _, tags := range [][]string{nil, {"go", "sql"}} {
		if tags != nil {
			note := createnote.Process(ctx, tx, createnote.Input{Content: "# Ideas", Tags: tags})
			note.Run()
			if note.HasErr() {
				t.Fatal(note.GetErr())
			}
		}
		process := listtag.Process(ctx, tx)
		process.Run()
		if process.HasErr() {
			t.Fatal(process.GetErr())
		}

		listed := process.GetResult()
		if listed == nil {
			listed = []domain.Tag{}
		}
		for _, format := range []struct{ yaml, json bool }{{}, {yaml: true}, {json: true}} {
			want, _ := apitest.Stdout(t, func() error { printKernel(listed, format.yaml, format.json); return nil })
			command := &cobra.Command{}
			command.SetContext(ctx)
			got, err := apitest.Stdout(t, func() error { return listTags(command, format.yaml, format.json) })
			if err != nil || got != want {
				t.Errorf("%v %+v: printed %q, err = %v; want %q", tags, format, got, err, want)
			}
		}
	}
}

func TestLoadTag(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	note := createnote.Process(ctx, tx, createnote.Input{Content: "# Ideas", Tags: []string{"go"}})
	note.Run()
	if note.HasErr() {
		t.Fatal(note.GetErr())
	}
	client, err := apiclient.Read().Client()
	if err != nil {
		t.Fatal(err)
	}

	output, err := loadTag(ctx, client, "go")
	if err != nil || !strings.Contains(output, "/Ideas") || !strings.Contains(output, note.GetResult().ID) {
		t.Errorf("output %q, err = %v; want the note's path and uuid", output, err)
	}
	if _, err := loadTag(ctx, client, "nothing"); err == nil || !strings.HasPrefix(err.Error(), readtag.ErrNotFound.Error()) {
		t.Errorf("unknown tag: err = %v, want %v", err, readtag.ErrNotFound)
	}

	command := &cobra.Command{}
	command.SetContext(ctx)
	printed, err := apitest.Stdout(t, func() error { return printTag(command, "go") })
	if err != nil || !strings.Contains(printed, `"value": "go"`) || !strings.Contains(printed, `"id": "`+note.GetResult().ID+`"`) {
		t.Errorf("--json: printed %s, err = %v; want the tag and its note", printed, err)
	}
}

// --similar prints each group of close tags on a line with the note counts; a tag alone is left out.
func TestListSimilar(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	note := createnote.Process(ctx, tx, createnote.Input{Content: "# Ideas", Tags: []string{"Emploi", "Emploie", "go"}})
	note.Run()
	if note.HasErr() {
		t.Fatal(note.GetErr())
	}
	command := &cobra.Command{}
	command.SetContext(ctx)
	got, err := apitest.Stdout(t, func() error { return listSimilar(command, false, false) })
	if err != nil || got != "Emploi (1), Emploie (1)\n" {
		t.Errorf("printed %q, err = %v", got, err)
	}
	got, err = apitest.Stdout(t, func() error { return listSimilar(command, false, true) })
	if err != nil || !strings.Contains(got, `"value": "Emploie"`) {
		t.Errorf("JSON %q, err = %v", got, err)
	}
}
