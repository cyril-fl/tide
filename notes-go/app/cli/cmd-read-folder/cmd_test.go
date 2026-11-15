package cmdreadfolder

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	plumdebug "github.com/cyril-fl/plum/utils/debug"
	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient"
	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/notes-go/app/shared/domain"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createfolder"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createnote"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/readitem"
	"github.com/spf13/cobra"
)

// --json through the API prints what readitem gives in the base, the folder found by its path or its uuid.
func TestReadJSON(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	folder := createfolder.Process(ctx, tx, createfolder.Input{Title: "Projects"})
	folder.Run()
	empty := createfolder.Process(ctx, tx, createfolder.Input{Title: "Empty"})
	empty.Run()
	subfolder := createfolder.Process(ctx, tx, createfolder.Input{Title: "Go", ParentID: folder.GetResult().ID})
	subfolder.Run()
	note := createnote.Process(ctx, tx, createnote.Input{Content: "# Ideas", ParentID: folder.GetResult().ID})
	note.Run()
	if folder.HasErr() || empty.HasErr() || subfolder.HasErr() || note.HasErr() {
		t.Fatal(folder.GetErr(), empty.GetErr(), subfolder.GetErr(), note.GetErr())
	}

	for ref, id := range map[string]string{"/Projects": folder.GetResult().ID, folder.GetResult().ID: folder.GetResult().ID, "/Empty": empty.GetResult().ID} {
		process := readitem.Process(ctx, tx, domain.FOLDER, id)
		process.Run()
		if process.HasErr() {
			t.Fatal(process.GetErr())
		}
		want, _ := apitest.Stdout(t, func() error {
			result := process.GetResult()
			result.Children = append([]domain.Item{}, result.Children...)
			plumdebug.Json(result)
			return nil
		})

		command := &cobra.Command{}
		command.Flags().Bool("json", true, "")
		command.SetContext(ctx)
		printed, err := apitest.Stdout(t, func() error { return Read(command, []string{ref}) })
		// The generated types order and omit fields their own way: printed back in readitem's, it must be the same.
		var decoded readitem.Result
		if err == nil {
			err = json.Unmarshal([]byte(printed), &decoded)
		}
		got, _ := apitest.Stdout(t, func() error { plumdebug.Json(decoded); return nil })
		if err != nil || got != want || !strings.Contains(printed, `"children": [`) {
			t.Errorf("%s: printed %s, err = %v; want %s", ref, printed, err, want)
		}
	}
}

func TestInfo(t *testing.T) {
	created := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	updated := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	children := []apiclient.DomainItem{{Kind: apiclient.FOLDER}, {Kind: apiclient.NOTE}, {Kind: apiclient.NOTE}}
	root := children[:1]

	tests := map[string]struct {
		folder apiclient.ReaditemResult
		want   string
	}{
		"updated":      {apiclient.ReaditemResult{Item: apiclient.DomainItem{CreatedAt: created, UpdatedAt: &updated}, Children: &children}, "1 folder · 2 notes · updated 2 Nov 2026"},
		"never edited": {apiclient.ReaditemResult{Item: apiclient.DomainItem{CreatedAt: created}}, "0 folders · 0 notes · updated 1 Nov 2026"},
		"root":         {apiclient.ReaditemResult{Children: &root}, "1 folder · 0 notes"},
	}

	for name, test := range tests {
		if got := info(test.folder); got != test.want {
			t.Errorf("%s: info() = %q, want %q", name, got, test.want)
		}
	}
}
