package cmdaccount

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"charm.land/huh/v2"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/config"
	"github.com/cyril-fl/tide/core/api"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/huhform"
	"github.com/cyril-fl/tide/core/services/postgrey"
	"github.com/cyril-fl/tide/core/services/process/adduser"
	"github.com/cyril-fl/tide/core/services/process/createschema"
	"github.com/cyril-fl/tide/core/services/process/createsession"
	"github.com/cyril-fl/tide/core/services/process/createtag"
	"github.com/cyril-fl/tide/core/services/process/readsession"
	"github.com/cyril-fl/tide/core/services/silo"
	"github.com/spf13/cobra"
)

func TestSameName(t *testing.T) {
	check := sameName("cyril")
	if err := check("cyril"); err != nil {
		t.Errorf("the name: %v", err)
	}
	for _, typed := range []string{"", "Cyril", "cyril ", "cyri"} {
		if check(typed) == nil {
			t.Errorf("%q confirmed the purge", typed)
		}
	}
}

// server is the URL of the API on the test base, for login and logout, clients of the API.
var server string

// The commands commit on postgrey.GetDb(), not in a rolled-back transaction: each account made here is deleted
// at the end.
func setup(t *testing.T) context.Context {
	t.Helper()
	ctx := context.Background()
	env := postgrey.NewEnvironment()
	env.ReadEnv()
	db := env.Connect()
	schema := createschema.Process(ctx, db)
	schema.Run()
	if schema.HasErr() {
		t.Fatalf("create schema: %v", schema.GetErr())
	}
	previous := postgrey.GetDb()
	postgrey.SetDb(db)
	http := httptest.NewServer(api.New(db, silo.NewMemory(), "", ""))
	server = http.URL
	t.Cleanup(func() {
		http.Close()
		postgrey.SetDb(previous)
		db.Close()
	})

	// The session file of the user, under HOME, stays untouched.
	t.Setenv("HOME", t.TempDir())
	t.Setenv(config.Env("URL"), "")
	t.Setenv(config.Env("SESSION"), "")
	return ctx
}

func account(t *testing.T, ctx context.Context) string {
	t.Helper()
	name := fmt.Sprintf("test-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		if _, err := postgrey.GetDb().NewDelete().Model((*domain.User)(nil)).Where("name = ?", name).Exec(ctx); err != nil {
			t.Errorf("delete %s: %v", name, err)
		}
	})
	return name
}

// keys types each field of the next huh forms, then waits: the form focuses the next field a key late, a key typed
// at once would land in the previous one.
func keys(t *testing.T, fields ...string) {
	t.Helper()
	reader, writer := io.Pipe()
	go func() {
		for _, field := range fields {
			if _, err := writer.Write([]byte(field)); err != nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
	huhform.Input = reader
	t.Cleanup(func() {
		huhform.Input = nil
		writer.Close()
	})
}

// run calls a command as cobra does and returns what it printed. The password comes from stdin, or from the form
// once keys are given.
func run(t *testing.T, ctx context.Context, runE func(*cobra.Command, []string) error, stdin string, args ...string) (string, error) {
	t.Helper()
	command := &cobra.Command{}
	command.Flags().Bool("password-stdin", huhform.Input == nil, "")
	command.Flags().Bool("json", false, "")
	command.SetContext(ctx)

	in, inWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	inWriter.WriteString(stdin)
	inWriter.Close()
	out, outWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdinBefore, stdoutBefore := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = in, outWriter
	defer func() { os.Stdin, os.Stdout = stdinBefore, stdoutBefore }()

	runErr := runE(command, args)
	outWriter.Close()
	printed, _ := io.ReadAll(out)
	return string(printed), runErr
}

// addUser creates the account with the password "a password", as tide-server user add does.
func addUser(ctx context.Context, name string) (domain.User, error) {
	process := adduser.Process(ctx, postgrey.GetDb(), adduser.Input{Name: name, Password: "a password"})
	process.Run()
	return process.GetResult(), process.GetErr()
}

func logIn(t *testing.T, ctx context.Context, name, password string) error {
	t.Helper()
	process := createsession.Process(ctx, postgrey.GetDb(), createsession.Input{Name: name, Password: password})
	process.Run()
	return process.GetErr()
}

func TestLogin(t *testing.T) {
	ctx := setup(t)
	name := account(t, ctx)
	if _, err := addUser(ctx, name); err != nil {
		t.Fatal(err)
	}
	file, err := apiclient.SessionFile()
	if err != nil {
		t.Fatal(err)
	}

	keys(t, name+"\r", "the wrong one\r")
	if _, err := run(t, ctx, Login, "", server); err == nil || err.Error() != createsession.ErrBadCredentials.Error() {
		t.Errorf("wrong password: err = %v, want %v", err, createsession.ErrBadCredentials)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("session file after a wrong password: %v, want none", err)
	}

	keys(t, name+"\r", "a password\r")
	printed, err := run(t, ctx, Login, "", server)
	if err != nil || printed != "Logged in as "+name+"\n" {
		t.Fatalf("printed %q, err = %v; want logged in", printed, err)
	}
	if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("session file: %v, %v; want it readable by the user only", info, err)
	}
	if session := apiclient.Read(); session.URL != server {
		t.Errorf("server kept = %q, want %q", session.URL, server)
	}
	if _, err := withAccount(ctx); err != nil {
		t.Errorf("act for the account: %v; want the session kept", err)
	}
}

// whoami needs a session, then prints its account and server; an expired one gives the API's message.
func TestWhoami(t *testing.T) {
	ctx := setup(t)
	if _, err := run(t, ctx, Whoami, ""); !errors.Is(err, apiclient.ErrNoSession) {
		t.Errorf("no session: err = %v, want %v", err, apiclient.ErrNoSession)
	}

	name := account(t, ctx)
	if _, err := addUser(ctx, name); err != nil {
		t.Fatal(err)
	}
	keys(t, name+"\r", "a password\r")
	if _, err := run(t, ctx, Login, "", server); err != nil {
		t.Fatal(err)
	}
	printed, err := run(t, ctx, Whoami, "")
	if want := name + " on " + server + "\n"; err != nil || printed != want {
		t.Errorf("printed %q, err = %v; want %q", printed, err, want)
	}
	asJSON := func(command *cobra.Command, args []string) error {
		command.Flags().Set("json", "true")
		return Whoami(command, args)
	}
	printed, err = run(t, ctx, asJSON, "")
	if err != nil || !strings.Contains(printed, `"name": "`+name+`"`) || !strings.Contains(printed, `"server": "`+server+`"`) {
		t.Errorf("--json: printed %s, err = %v; want the account and its server", printed, err)
	}

	t.Setenv(config.Env("SESSION"), "expired")
	if _, err := run(t, ctx, Whoami, ""); err == nil || err.Error() != readsession.ErrNoSession.Error() {
		t.Errorf("unknown token: err = %v, want %v", err, readsession.ErrNoSession)
	}
}

// The purge asks the account's name: a wrong one empties nothing, the right one every tag (and note, folder, image).
func TestPurge(t *testing.T) {
	ctx := setup(t)
	name := account(t, ctx)
	if _, err := addUser(ctx, name); err != nil {
		t.Fatal(err)
	}
	keys(t, name+"\r", "a password\r")
	if _, err := run(t, ctx, Login, "", server); err != nil {
		t.Fatal(err)
	}
	ctx, err := withAccount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tag := createtag.Process(ctx, postgrey.GetDb(), []string{"kept"})
	tag.Run()
	if tag.HasErr() {
		t.Fatal(tag.GetErr())
	}
	tags := func() int64 {
		count, err := postgrey.GetDb().NewSelect().Model((*domain.Tag)(nil)).Where("user_id = ?", domain.UserID(ctx)).Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return count
	}

	keys(t, "not "+name+"\r", "\x03")
	if _, err := run(t, ctx, Purge, ""); !errors.Is(err, huh.ErrUserAborted) || tags() != 1 {
		t.Fatalf("wrong name: err = %v, %d tags; want the form kept until aborted and the tag kept", err, tags())
	}
	keys(t, name+"\r")
	printed, err := run(t, ctx, Purge, "")
	if err != nil || printed != "Successfully emptied account "+name+"\n" || tags() != 0 {
		t.Errorf("printed %q, err = %v, %d tags; want the account emptied", printed, err, tags())
	}
}

// Logged out, the commands acting for an account refuse; logged in, they act for its user; logout ends both.
func TestSession(t *testing.T) {
	ctx := setup(t)

	if _, err := withAccount(ctx); !errors.Is(err, readsession.ErrNoSession) {
		t.Errorf("no session: err = %v, want %v", err, readsession.ErrNoSession)
	}
	if _, err := run(t, ctx, Purge, ""); !errors.Is(err, apiclient.ErrNoSession) {
		t.Errorf("purge with no session: err = %v, want %v", err, apiclient.ErrNoSession)
	}

	name := account(t, ctx)
	if _, err := addUser(ctx, name); err != nil {
		t.Fatal(err)
	}
	session := createsession.Process(ctx, postgrey.GetDb(), createsession.Input{Name: name, Password: "a password"})
	session.Run()
	if session.HasErr() {
		t.Fatal(session.GetErr())
	}
	file, err := apiclient.SessionFile()
	if err != nil {
		t.Fatal(err)
	}
	if err := (apiclient.Session{URL: server, Token: session.GetResult().Token}).Save(); err != nil {
		t.Fatal(err)
	}

	withAccount, err := withAccount(ctx)
	if err != nil || domain.UserID(withAccount) != session.GetResult().User.ID {
		t.Fatalf("user = %q, err = %v; want %q", domain.UserID(withAccount), err, session.GetResult().User.ID)
	}

	printed, err := run(t, ctx, Logout, "")
	if err != nil || printed != "Logged out\n" {
		t.Fatalf("printed %q, err = %v; want logged out", printed, err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("session file: %v, want it removed", err)
	}
	check := readsession.Process(ctx, postgrey.GetDb(), session.GetResult().Token)
	check.Run()
	if !errors.Is(check.GetErr(), readsession.ErrNoSession) {
		t.Errorf("session in the base: err = %v, want it deleted", check.GetErr())
	}
}

// --print-token prints a session of its own and keeps no file; TIDE_SESSION then wins over the file, and logout ends
// that session only, the file's staying.
func TestPrintToken(t *testing.T) {
	ctx := setup(t)
	name := account(t, ctx)
	if _, err := addUser(ctx, name); err != nil {
		t.Fatal(err)
	}
	keys(t, name+"\r", "a password\r")
	if _, err := run(t, ctx, Login, "", server); err != nil {
		t.Fatal(err)
	}
	file, err := apiclient.SessionFile()
	if err != nil {
		t.Fatal(err)
	}
	kept, _ := os.ReadFile(file)

	keys(t, name+"\r", "a password\r")
	command := func(cobra *cobra.Command, args []string) error {
		cobra.Flags().Bool("print-token", true, "")
		return Login(cobra, args)
	}
	printed, err := run(t, ctx, command, "", server)
	token := strings.TrimSpace(printed)
	if err != nil || token == "" || strings.Contains(string(kept), token) {
		t.Fatalf("printed %q, err = %v; want a new token", printed, err)
	}
	if now, _ := os.ReadFile(file); string(now) != string(kept) {
		t.Errorf("session file rewritten by --print-token")
	}

	t.Setenv(config.Env("SESSION"), token+"\n")
	if read := apiclient.Read().Token; read != token {
		t.Errorf("token = %q, want TIDE_SESSION's", read)
	}
	if _, err := withAccount(ctx); err != nil {
		t.Errorf("act through TIDE_SESSION: %v", err)
	}
	if _, err := run(t, ctx, Logout, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := withAccount(ctx); !errors.Is(err, readsession.ErrNoSession) {
		t.Errorf("TIDE_SESSION after logout: err = %v, want %v", err, readsession.ErrNoSession)
	}

	t.Setenv(config.Env("SESSION"), "")
	if _, err := withAccount(ctx); err != nil {
		t.Errorf("the file's session after logging TIDE_SESSION out: %v, want it kept", err)
	}
}

// withAccount acts for the account of the CLI's session, as the API does for the token the CLI sends it.
func withAccount(ctx context.Context) (context.Context, error) {
	session := readsession.Process(ctx, postgrey.GetDb(), apiclient.Read().Token)
	session.Run()
	if session.HasErr() {
		return ctx, session.GetErr()
	}
	return domain.WithUser(ctx, session.GetResult().ID), nil
}
