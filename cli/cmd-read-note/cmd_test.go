package cmdreadnote

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/spf13/cobra"
)

// --json through the API prints the note flat, found by its path, its tags [] when it has none.
func TestReadJSON(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	tagged := createnote.Process(ctx, tx, createnote.Input{Content: "# Ideas\n\nship it #go"})
	tagged.Run()
	bare := createnote.Process(ctx, tx, createnote.Input{Content: "# Bare"})
	bare.Run()
	if tagged.HasErr() || bare.HasErr() {
		t.Fatal(tagged.GetErr(), bare.GetErr())
	}

	for ref, want := range map[string]noteJSON{
		"/Ideas": {ID: tagged.GetResult().ID, Title: "Ideas", Path: "/Ideas", Content: "# Ideas\n\nship it #go", Tags: []string{"go"}},
		"/Bare":  {ID: bare.GetResult().ID, Title: "Bare", Path: "/Bare", Content: "# Bare", Tags: []string{}},
	} {
		command := &cobra.Command{}
		command.Flags().Bool("json", true, "")
		command.SetContext(ctx)
		printed, err := apitest.Stdout(t, func() error { return Read(command, []string{ref}) })
		var got noteJSON
		if err == nil {
			err = json.Unmarshal([]byte(printed), &got)
		}
		if err != nil || got.ID != want.ID || got.Title != want.Title || got.Path != want.Path || got.Content != want.Content ||
			!slices.Equal(got.Tags, want.Tags) || got.Tags == nil || got.Readonly || got.CreatedAt == "" || got.UpdatedAt != nil {
			t.Errorf("%s: printed %s, err = %v; want %+v", ref, printed, err, want)
		}
	}
}

func TestInfo(t *testing.T) {
	created := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	updated := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	tags := []string{"go", "sql"}
	locked := true

	tests := map[string]struct {
		note apiclient.ReaditemResult
		want string
	}{
		"tagged, edited, locked": {apiclient.ReaditemResult{Item: apiclient.DomainItem{CreatedAt: created, UpdatedAt: &updated}, Tags: &tags, Readonly: &locked}, "#go #sql · created 1 Nov 2026 · updated 2 Nov 2026 · locked"},
		"never edited":           {apiclient.ReaditemResult{Item: apiclient.DomainItem{CreatedAt: created}}, "created 1 Nov 2026"},
	}

	for name, test := range tests {
		if got := info(test.note); got != test.want {
			t.Errorf("%s: info() = %q, want %q", name, got, test.want)
		}
	}
}
