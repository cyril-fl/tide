package millflagseditfolder

import (
	"strings"
	"testing"

	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient/apitest"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/createfolder"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readitem"
	"github.com/cyril-fl/notes/notes-go/config"
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/spf13/cobra"
)

func TestResolve(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	client, err := apiclient.Read().Client()
	if err != nil {
		t.Fatal(err)
	}

	seed := createfolder.Process(ctx, tx, createfolder.Input{Title: "Old"})
	seed.Run()
	if seed.HasErr() {
		t.Fatalf("seed: %v", seed.GetErr())
	}

	cmd := &cobra.Command{}
	plumcliutilsargs.Makeflags(cmd, config.Current.Commands.EditFolder.Flags)
	if err := cmd.ParseFlags([]string{"--title", "New"}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}

	resolver := New(Template{Ctx: ctx, Client: client, Flags: cmd.Flags(), ID: seed.GetResult().ID})
	if input := resolver.Resolve(); resolver.HasErr() || input.Title != "New" || input.ID != seed.GetResult().ID {
		t.Errorf("input = %+v, err = %v; want New", input, resolver.GetErr())
	}

	// Last: an unknown id fails before any form.
	missing := New(Template{Ctx: ctx, Client: client, Flags: cmd.Flags(), ID: "00000000-0000-0000-0000-000000000000"})
	missing.Resolve()
	if missing.GetErr() == nil || !strings.HasPrefix(missing.GetErr().Error(), readitem.ErrNotFound.Error()) {
		t.Errorf("err = %v, want %v", missing.GetErr(), readitem.ErrNotFound)
	}
}
