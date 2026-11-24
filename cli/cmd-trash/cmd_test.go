package cmdtrash

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/config"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/deleteitems"
	"github.com/cyril-fl/tide/core/services/process/listtrash"
	"github.com/spf13/cobra"
)

func TestPurge(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	tests := map[string]struct {
		deletedAt time.Time
		retention int
		want      string
	}{
		"today":        {now, 30, "purged in 30 days"},
		"last day":     {now.AddDate(0, 0, -29), 30, "purged in 1 day"},
		"overdue":      {now.AddDate(0, 0, -31), 30, "purged at next start"},
		"never":        {now.AddDate(0, 0, -400), 0, "kept"},
		"some hours":   {now.Add(-5 * time.Hour), 2, "purged in 2 days"},
		"server ahead": {now.Add(3 * time.Millisecond), 30, "purged in 30 days"}, // its clock a little ahead of the CLI's
	}

	for name, test := range tests {
		if got := purge(test.deletedAt, test.retention, now); got != test.want {
			t.Errorf("%s: purge() = %q, want %q", name, got, test.want)
		}
	}
}

// The trash and the account's retention come through the API; --json keeps the fields of listtrash.
func TestTrash(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	note := createnote.Process(ctx, tx, createnote.Input{Content: "# Gone"})
	note.Run()
	if note.HasErr() {
		t.Fatal(note.GetErr())
	}
	removed := deleteitems.Process(ctx, tx, domain.NOTE, []string{note.GetResult().ID})
	removed.Run()
	if removed.HasErr() {
		t.Fatal(removed.GetErr())
	}

	command := &cobra.Command{}
	plumcliutilsargs.Makeflags(command, config.Current.Commands.Trash.Flags)
	command.SetContext(ctx)
	printed, err := apitest.Stdout(t, func() error { return Trash(command, nil) })
	if err != nil || !strings.Contains(printed, "Gone") || !strings.Contains(printed, "purged in 30 days") {
		t.Errorf("printed %q, err = %v; want the note and its purge", printed, err)
	}

	if err := command.Flags().Set("json", "true"); err != nil {
		t.Fatal(err)
	}
	printed, err = apitest.Stdout(t, func() error { return Trash(command, nil) })
	var trash []listtrash.Trashed
	if err != nil || json.Unmarshal([]byte(printed), &trash) != nil || len(trash) != 1 || trash[0].ID != note.GetResult().ID {
		t.Errorf("printed %q, err = %v; want the note as JSON", printed, err)
	}
}
