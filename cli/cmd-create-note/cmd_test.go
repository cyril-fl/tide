package cmdcreatenote

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/config"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/process/createfolder"
	"github.com/cyril-fl/tide/core/services/process/readitem"
	"github.com/spf13/cobra"
)

// The flags give everything, so no form opens: the note is created through the API, in its folder, with its tags,
// then pinned and locked.
func TestCreate(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	folder := createfolder.Process(ctx, tx, createfolder.Input{Title: "Projects"})
	folder.Run()
	if folder.HasErr() {
		t.Fatal(folder.GetErr())
	}

	command := &cobra.Command{}
	plumcliutilsargs.Makeflags(command, config.Current.Commands.CreateNote.Flags)
	command.SetContext(ctx)
	if err := command.ParseFlags([]string{"--content", "# Ideas", "--tag", "go", "--parent", folder.GetResult().ID, "--pin", "--lock"}); err != nil {
		t.Fatal(err)
	}
	printed, err := apitest.Stdout(t, func() error { return Create(command, nil) })
	first, _, _ := strings.Cut(printed, "\n")
	id := strings.TrimPrefix(first, "Successfully created note ")
	if err != nil || id == first || !strings.Contains(printed, "Successfully pinned note "+id) || !strings.Contains(printed, "Successfully locked note "+id) {
		t.Fatalf("printed %q, err = %v", printed, err)
	}

	note := readitem.Process(ctx, tx, domain.NOTE, id)
	note.Run()
	if note.HasErr() || note.GetResult().Path != "/Projects/Ideas" || !slices.Equal(note.GetResult().Tags, []string{"go"}) || !note.GetResult().Pinned || !note.GetResult().Readonly {
		t.Errorf("note = %+v, err = %v; want /Projects/Ideas tagged go, pinned and locked", note.GetResult(), note.GetErr())
	}
}

// A PATCH refused after the create says the note exists: a proxy in front of the API fails every PATCH.
func TestCreatePatchRefused(t *testing.T) {
	ctx, _ := apitest.Serve(t)
	api, err := url.Parse(os.Getenv(config.Env("URL")))
	if err != nil {
		t.Fatal(err)
	}
	forward := httputil.NewSingleHostReverseProxy(api)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			http.Error(w, `{"error":"refused"}`, http.StatusInternalServerError)
			return
		}
		forward.ServeHTTP(w, r)
	}))
	t.Cleanup(proxy.Close)
	t.Setenv(config.Env("URL"), proxy.URL)

	command := &cobra.Command{}
	plumcliutilsargs.Makeflags(command, config.Current.Commands.CreateNote.Flags)
	command.SetContext(ctx)
	if err := command.ParseFlags([]string{"--content", "# Ideas", "--pin"}); err != nil {
		t.Fatal(err)
	}
	printed, err := apitest.Stdout(t, func() error { return Create(command, nil) })
	first, _, _ := strings.Cut(printed, "\n")
	id := strings.TrimPrefix(first, "Successfully created note ")
	if want := "note " + id + " was created but not pinned or locked: refused"; id == first || err == nil || err.Error() != want {
		t.Errorf("printed %q, err = %v; want %q", printed, err, want)
	}
}
