package cmdtrash

import (
	"fmt"
	"math"
	"time"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/config"
	"github.com/cyril-fl/tide/core/services/screen"
	"github.com/cyril-fl/tide/core/services/style"
	"github.com/spf13/cobra"
)

// Trash lists what was removed, each removal once with how many items went with it, and when it is purged, through
// GET /api/trash and the retention of GET /api/me.
func Trash(cobra *cobra.Command, args []string) error {
	var asJSON bool
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"json": &asJSON}); err != nil {
		return err
	}
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	response, err := client.GetApiTrashWithResponse(cobra.Context())
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}
	trash := *response.JSON200

	if asJSON {
		screen.PrintJSON(trash)
		return nil
	}
	me, err := client.GetApiMeWithResponse(cobra.Context())
	if err != nil {
		return err
	}
	if me.JSON200 == nil {
		return apiclient.Fail(me.HTTPResponse, me.Body)
	}
	printed, err := output(trash, me.JSON200.TrashRetentionDays, time.Now())
	if err != nil {
		return err
	}
	screen.Print(printed)
	return nil
}

func output(trash []apiclient.ListtrashTrashed, retention int, now time.Time) (string, error) {
	header := style.Header("Trash", config.Current.Name+" restore <uuid> brings an item back", info(len(trash), retention)) + "\n"
	if len(trash) == 0 {
		return header + style.Empty().Render("The trash is empty"), nil
	}

	cells := make([][]string, len(trash))
	for i, item := range trash {
		deletedAt, err := time.Parse(time.RFC3339Nano, item.DeletedAt)
		if err != nil {
			return "", err
		}
		title := item.Title
		if title == "" {
			title = "untitled"
		}
		if item.Inside > 0 {
			title += fmt.Sprintf(" (+%d inside)", item.Inside)
		}
		cells[i] = []string{string(item.Kind), title, "removed " + deletedAt.Format("2 Jan 2006"), purge(deletedAt, retention, now), item.Id}
	}
	return header + style.Table(cells, -1, 1), nil
}

func info(removals, retention int) string {
	count := fmt.Sprintf("%d removals", removals)
	if removals == 1 {
		count = "1 removal"
	}
	if retention <= 0 {
		return count + " · never purged"
	}
	return fmt.Sprintf("%s · purged after %d days", count, retention)
}

// purge says when purgeitems (run at every start) deletes the item for good.
func purge(deletedAt time.Time, retention int, now time.Time) string {
	if retention <= 0 {
		return "kept"
	}
	// At most retention: the server writing deleted_at with a clock ahead of the CLI's would round it up to one more.
	days := min(int(math.Ceil(deletedAt.AddDate(0, 0, retention).Sub(now).Hours()/24)), retention)
	switch {
	case days <= 0:
		return "purged at next start"
	case days == 1:
		return "purged in 1 day"
	default:
		return fmt.Sprintf("purged in %d days", days)
	}
}
