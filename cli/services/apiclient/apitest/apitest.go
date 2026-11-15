// Package apitest serves the real API to the tests of the CLI commands that are clients of it.
package apitest

import (
	"context"
	"io"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/cyril-fl/tide/config"
	"github.com/cyril-fl/tide/core/api"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/password"
	"github.com/cyril-fl/tide/core/services/postgrey"
	"github.com/cyril-fl/tide/core/services/process/createschema"
	"github.com/cyril-fl/tide/core/services/silo"
	"github.com/cyril-fl/tide/core/services/testaccount"
	"github.com/uptrace/bun"
)

// Serve starts the API on a transaction of the test base, rolled back at the end, and points TIDE_URL and
// TIDE_SESSION at it, logged in as a new account. The fixtures go into the returned tx, as that account.
func Serve(t *testing.T) (context.Context, bun.Tx) {
	t.Helper()
	ctx := context.Background()
	env := postgrey.NewEnvironment()
	env.ReadEnv()
	db := env.Connect()
	t.Cleanup(func() { db.Close() })
	schema := createschema.Process(ctx, db)
	schema.Run()
	if schema.HasErr() {
		t.Fatalf("create schema: %v", schema.GetErr())
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tx.Rollback() })

	ctx, savepoint := testaccount.New(t, ctx, tx)
	token := password.Token()
	session := domain.Session{UserID: domain.UserID(ctx), TokenHash: password.TokenHash(token), ExpiresAt: time.Now().Add(time.Hour)}
	if _, err := savepoint.NewInsert().Model(&session).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	// Every plan: these tests check the client, the router tests check the plans.
	plan := domain.Plans[len(domain.Plans)-1]
	if _, err := savepoint.NewUpdate().Model((*domain.User)(nil)).Set("plan = ?", plan).Where("id = ?", domain.UserID(ctx)).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.New(savepoint, silo.NewMemory(), "", ""))
	t.Cleanup(server.Close)

	// The session file of the user, under HOME, stays untouched.
	t.Setenv("HOME", t.TempDir())
	t.Setenv(config.Env("URL"), server.URL)
	t.Setenv(config.Env("SESSION"), token)
	return ctx, savepoint
}

// Stdout returns what print writes to stdout, through fmt or lipgloss (screen.Print).
func Stdout(t *testing.T, print func() error) (string, error) {
	t.Helper()
	out, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	before, beforeLipgloss := os.Stdout, lipgloss.Writer.Forward
	os.Stdout, lipgloss.Writer.Forward = writer, writer
	defer func() { os.Stdout, lipgloss.Writer.Forward = before, beforeLipgloss }()

	printErr := print()
	writer.Close()
	printed, _ := io.ReadAll(out)
	return string(printed), printErr
}
