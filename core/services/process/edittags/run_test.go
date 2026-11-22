package edittags

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/cyril-fl/tide/core/services/postgrey"
	"github.com/cyril-fl/tide/core/services/process/createschema"
	"github.com/cyril-fl/tide/core/services/process/createtag"
	"github.com/cyril-fl/tide/core/services/process/edittag"
	"github.com/cyril-fl/tide/core/services/process/readtag"
	"github.com/cyril-fl/tide/core/services/testaccount"
)

func TestNoValues(t *testing.T) {
	process := Process(context.Background(), nil, nil, "go", false)
	process.Run()
	if !errors.Is(process.GetErr(), ErrNoValue) {
		t.Errorf("err = %v, want %v", process.GetErr(), ErrNoValue)
	}
}

// A tag missing from the list renames none of the ones before it.
func TestAllOrNothing(t *testing.T) {
	ctx := context.Background()
	env := postgrey.NewEnvironment()
	env.ReadEnv()
	db := env.Connect()
	defer db.Close()

	schema := createschema.Process(ctx, db)
	schema.Run()
	if schema.HasErr() {
		t.Fatalf("create schema: %v", schema.GetErr())
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ctx, tx = testaccount.New(t, ctx, tx)

	a := fmt.Sprintf("test-%d-a", time.Now().UnixNano())
	b, to := a+"-b", a+"-to"
	create := createtag.Process(ctx, tx, []string{a, b})
	create.Run()
	if create.HasErr() {
		t.Fatalf("create: %v", create.GetErr())
	}

	// a and b fold into to, in the order given.
	process := Process(ctx, tx, []string{a, b}, to, true)
	process.DryRun().Run()
	if process.HasErr() || len(process.GetResult()) != 2 {
		t.Fatalf("dry run = %v, %v", process.GetResult(), process.GetErr())
	}

	process = Process(ctx, tx, []string{a, a + "-missing"}, to, false)
	process.Run()
	if !errors.Is(process.GetErr(), edittag.ErrNotFound) {
		t.Fatalf("err = %v, want %v", process.GetErr(), edittag.ErrNotFound)
	}
	still := readtag.Process(ctx, tx, a)
	still.Run()
	if still.HasErr() {
		t.Errorf("%s was renamed though the batch failed: %v", a, still.GetErr())
	}

	// A real call renames them all.
	process = Process(ctx, tx, []string{a, b}, to, true)
	process.Run()
	gone := readtag.Process(ctx, tx, a)
	gone.Run()
	if process.HasErr() || !errors.Is(gone.GetErr(), readtag.ErrNotFound) {
		t.Errorf("batch = %v, %v; reading %s = %v, want it gone", process.GetResult(), process.GetErr(), a, gone.GetErr())
	}
}
